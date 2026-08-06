package acpregistry

import (
	"runtime"
	"slices"

	"assistente/internal/acp"
)

// Os tipos de distribuição do formato do registro, como eles aparecem para a
// tela. São vocabulário fechado: a tela traduz cada um, e um valor novo no
// documento não vira rótulo sem alguém decidir o texto.
const (
	DistributionBinary = "binary"
	DistributionNPX    = "npx"
	DistributionUVX    = "uvx"
)

// Integrity diz o que se sabe sobre conferir o artefato binário deste agente
// nesta plataforma (AEP-0086 D4). Ela é vocabulário da tela porque cada valor
// tem uma frase diferente, e "indisponível" não seria nenhuma delas.
type Integrity string

const (
	// IntegrityNotDistributed é o agente que não é distribuído como binário:
	// não há alvo, e portanto não há digest de que falar.
	IntegrityNotDistributed Integrity = "not_distributed"

	// IntegrityNoPlatformTarget é o agente distribuído como binário que não
	// publica alvo para esta plataforma — o caso do Windows ARM, onde só 7 dos
	// 17 agentes com binário têm alvo.
	IntegrityNoPlatformTarget Integrity = "no_platform_target"

	// IntegrityDigest é o alvo desta plataforma que publica `sha256`.
	IntegrityDigest Integrity = "digest"

	// IntegrityNoDigest é o alvo desta plataforma sem `sha256`. É o conjunto que
	// o D4 deixa fora da instalação automática, e o Cursor está nele.
	IntegrityNoDigest Integrity = "no_digest"
)

// detectableKinds é o mapeamento entre o tipo de agente que a detecção escrita à
// mão conhece e o `id` do agente no registro (AEP-0086 D11).
//
// Ele existe escrito num lugar só, e este é o lugar: os dois conjuntos de
// identificadores foram escolhidos em momentos diferentes, e espalhar a
// tradução por `switch` faria cada consumidor ter a própria versão dela.
//
// A lista é curta por decisão, e não por falta de trabalho: nenhum agente novo
// ganha detecção própria a partir do registro (D1). Para os outros agentes do
// catálogo o app não sabe procurar, e é isso que a tela diz — em vez de
// alegar que procurou e não achou.
var detectableKinds = map[string]acp.AgentKind{
	"cursor":     acp.AgentKindCursor,
	"claude-acp": acp.AgentKindClaudeCode,
}

// DetectableKind devolve o tipo de agente que a detecção sabe procurar para
// aquele `id` do registro. O `false` quer dizer que este app não tem detecção
// para o agente — o que é diferente de ter procurado e não encontrado.
func DetectableKind(id string) (acp.AgentKind, bool) {
	kind, ok := detectableKinds[id]
	return kind, ok
}

// DetectableKinds são os tipos que a detecção conhece, sem repetição. Quem monta
// o catálogo procura uma vez por tipo, e não uma vez por linha: a procura vai ao
// sistema de arquivos, e repeti-la por agente custaria 38 varreduras para
// responder sobre 2.
func DetectableKinds() []acp.AgentKind {
	kinds := make([]acp.AgentKind, 0, len(detectableKinds))
	for _, kind := range detectableKinds {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
}

// Platform é o alvo desta máquina no vocabulário do registro
// (`windows-x86_64`, `darwin-aarch64` e companhia).
//
// Vazio quer dizer que o registro não tem nome para esta combinação de sistema e
// arquitetura — o Go compila para mais alvos do que o formato do registro
// nomeia, e chutar um nome parecido escolheria o artefato de outra máquina.
func Platform() string {
	return platformFor(runtime.GOOS, runtime.GOARCH)
}

// platformFor é a Platform com o sistema e a arquitetura injetados, para o teste
// poder perguntar por um Windows ARM sem rodar num Windows ARM.
func platformFor(goos, goarch string) string {
	arch := ""
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	default:
		return ""
	}
	switch goos {
	case "windows", "darwin", "linux":
		return goos + "-" + arch
	default:
		return ""
	}
}

// Fit é o que esta máquina consegue dizer sobre uma linha do catálogo: quais
// formas de distribuição o agente tem, qual runtime elas exigem aqui e o que se
// sabe sobre conferir o artefato binário.
//
// Ele é resolvido no backend, e não na tela, porque cada campo sai de uma regra
// do AEP — o tipo de distribuição decide o runtime (D7), o alvo da plataforma
// decide a integridade (D4) — e uma tela que deduzisse isso do documento cru
// estaria reimplementando o AEP em TypeScript.
type Fit struct {
	// Distributions são os tipos publicados, em ordem estável.
	Distributions []string

	// Runtime é o pré-requisito nomeado desta máquina, ou vazio quando não há
	// nenhum. Ele é o do caminho que o app usaria aqui, e não o de todos os
	// caminhos possíveis: um agente com binário verificável não passa a exigir
	// Node só porque também publica pacote npm.
	Runtime acp.Runtime

	// PlatformTarget é o alvo binário desta plataforma, quando o agente tem um.
	PlatformTarget string

	// Integrity é o que se sabe sobre o digest do alvo desta plataforma.
	Integrity Integrity
}

// FitFor resolve o Fit de um agente para um alvo de plataforma.
//
// A ordem em que o runtime é decidido é a ordem em que os caminhos de instalação
// existem: binário conferível primeiro (D4), pacote npm depois (D6), pacote do
// `uv` em seguida, e por último o binário sem digest — que não é instalável pelo
// app, mas também não pede runtime de ninguém (D4, caminho manual).
func FitFor(agent Agent, platform string) Fit {
	fit := Fit{
		Distributions:  distributionsOf(agent),
		PlatformTarget: platform,
		Integrity:      IntegrityNotDistributed,
	}

	target, hasTarget := binaryTargetFor(agent, platform)
	switch {
	case len(agent.Distribution.Binary) == 0:
		fit.PlatformTarget = ""
	case !hasTarget:
		fit.PlatformTarget = ""
		fit.Integrity = IntegrityNoPlatformTarget
	case target.SHA256 != "":
		fit.Integrity = IntegrityDigest
	default:
		fit.Integrity = IntegrityNoDigest
	}

	switch {
	case fit.Integrity == IntegrityDigest:
		// Binário conferível não depende de runtime nenhum.
	case agent.Distribution.NPX != nil:
		fit.Runtime = acp.RuntimeNode
	case agent.Distribution.UVX != nil:
		fit.Runtime = acp.RuntimeUV
	}
	return fit
}

// binaryTargetFor acha o alvo daquela plataforma. Plataforma vazia — sistema que
// o registro não nomeia — não casa com alvo nenhum, em vez de casar com o
// primeiro do mapa.
func binaryTargetFor(agent Agent, platform string) (BinaryTarget, bool) {
	if platform == "" {
		return BinaryTarget{}, false
	}
	target, ok := agent.Distribution.Binary[platform]
	return target, ok
}

// distributionsOf lista os tipos publicados em ordem fixa. A ordem é do
// documento para a tela, e não a do mapa: iterar mapa em Go dá ordem sorteada, e
// a lista mudaria de posição a cada abertura.
func distributionsOf(agent Agent) []string {
	kinds := make([]string, 0, 3)
	if len(agent.Distribution.Binary) > 0 {
		kinds = append(kinds, DistributionBinary)
	}
	if agent.Distribution.NPX != nil {
		kinds = append(kinds, DistributionNPX)
	}
	if agent.Distribution.UVX != nil {
		kinds = append(kinds, DistributionUVX)
	}
	return kinds
}
