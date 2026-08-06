package app

import (
	"errors"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
)

// O catálogo da tela é a tradução do índice mais o que esta máquina tem
// (AEP-0086 Fase 2). Os testes descrevem a máquina em vez de perguntar à que
// roda o teste: o estado de cada linha é justamente o que não pode depender de
// quem instalou o quê no CI.

const digestQualquer = "3b1f2e0a7c6d5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2"

func agenteNPM(id, nome string) acpregistry.Agent {
	return acpregistry.Agent{
		ID:      id,
		Name:    nome,
		Version: "1.0.0",
		Distribution: acpregistry.Distribution{
			NPX: &acpregistry.PackageDistribution{Package: id + "@1.0.0"},
		},
	}
}

func agenteBinario(id, nome, alvo, digest string) acpregistry.Agent {
	return acpregistry.Agent{
		ID:      id,
		Name:    nome,
		Version: "2.0.0",
		Distribution: acpregistry.Distribution{
			Binary: map[string]acpregistry.BinaryTarget{
				alvo: {Archive: "https://cdn.exemplo/a.tar.gz", SHA256: digest, Cmd: "./a"},
			},
		},
	}
}

func catalogoDe(agents ...acpregistry.Agent) acpregistry.Catalog {
	return acpregistry.Catalog{Version: "1.0.0", Agents: agents, FetchedAt: time.Unix(1770000000, 0)}
}

func acharPorID(t *testing.T, catalogo ACPCatalog, id string) ACPCatalogAgent {
	t.Helper()
	for _, agent := range catalogo.Agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("o agente %q não está no catálogo", id)
	return ACPCatalogAgent{}
}

func TestOCatalogoVemOrdenadoPorNome(t *testing.T) {
	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("zed-agent", "Zed"), agenteNPM("codex-acp", "codex"), agenteNPM("amp", "Amp")),
		"linux-x86_64",
		nil,
		map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true, Path: "/usr/bin/node"}},
	)

	nomes := make([]string, 0, len(catalogo.Agents))
	for _, agent := range catalogo.Agents {
		nomes = append(nomes, agent.Name)
	}
	// Ordem por nome, e sem levar em conta caixa: "codex" antes de "Zed" é o que
	// alguém procurando na lista espera, e não o que a tabela ASCII manda.
	quer := []string{"Amp", "codex", "Zed"}
	for i, nome := range quer {
		if nomes[i] != nome {
			t.Fatalf("ordem = %v, quer %v", nomes, quer)
		}
	}
}

func TestOAgenteEncontradoPelaDeteccaoDizOndeEstá(t *testing.T) {
	installs := map[acp.AgentKind]acpDetection{
		acp.AgentKindCursor: {install: acp.Install{
			Found:   true,
			Command: `C:\cursor\node.exe`,
			Source:  `C:\cursor\index.js`,
			Version: "2026.01.02-abcdef",
		}},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("cursor", "Cursor", "windows-x86_64", "")),
		"windows-x86_64", installs, nil,
	)

	cursor := acharPorID(t, catalogo, "cursor")
	if cursor.State != ACPCatalogStateInstalled {
		t.Fatalf("estado = %q, quer %q", cursor.State, ACPCatalogStateInstalled)
	}
	if cursor.StateDetail != `C:\cursor\index.js` {
		t.Errorf("detalhe = %q, quer o arquivo que decidiu a detecção", cursor.StateDetail)
	}
	if cursor.DetectedVersion != "2026.01.02-abcdef" {
		t.Errorf("versão detectada = %q, quer a da instalação", cursor.DetectedVersion)
	}
	// A instalação encontrada não apaga o que se sabe do registro: a versão do
	// catálogo é outro dado, e é a diferença entre as duas que a Fase 7 usa.
	if cursor.Version != "2.0.0" {
		t.Errorf("versão do registro = %q, quer 2.0.0", cursor.Version)
	}
	if cursor.Integrity != string(acpregistry.IntegrityNoDigest) {
		t.Errorf("integridade = %q, quer %q", cursor.Integrity, acpregistry.IntegrityNoDigest)
	}
}

func TestOAgenteQueADeteccaoNaoConheceNaoEDitoComoNaoEncontrado(t *testing.T) {
	// A detecção sabe procurar dois agentes dos 38 (D1). Dizer "não encontrado"
	// para os outros alegaria uma procura que o app não sabe fazer — e mandaria
	// quem lê concluir que o agente não está na máquina.
	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("codex-acp", "Codex")),
		"linux-x86_64",
		map[acp.AgentKind]acpDetection{},
		map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true, Path: "/usr/bin/node"}},
	)

	codex := acharPorID(t, catalogo, "codex-acp")
	if codex.State != ACPCatalogStateNoDetection {
		t.Errorf("estado = %q, quer %q", codex.State, ACPCatalogStateNoDetection)
	}
}

func TestOAgenteConhecidoQueNaoEstaNaMaquinaEDitoComoNaoEncontrado(t *testing.T) {
	installs := map[acp.AgentKind]acpDetection{
		acp.AgentKindCursor: {install: acp.Install{Found: false}},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("cursor", "Cursor", "linux-x86_64", digestQualquer)),
		"linux-x86_64", installs, nil,
	)

	if cursor := acharPorID(t, catalogo, "cursor"); cursor.State != ACPCatalogStateNotInstalled {
		t.Errorf("estado = %q, quer %q", cursor.State, ACPCatalogStateNotInstalled)
	}
}

func TestAProcuraQueFalhouNaoViraNaoEncontrado(t *testing.T) {
	// Permissão negada é pergunta sem resposta. Chamar isso de "não instalado"
	// mandaria a pessoa instalar de novo o que já está lá.
	installs := map[acp.AgentKind]acpDetection{
		acp.AgentKindCursor: {err: errors.New("permissão negada em /opt:\nlinha dois")},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("cursor", "Cursor", "linux-x86_64", digestQualquer)),
		"linux-x86_64", installs, nil,
	)

	cursor := acharPorID(t, catalogo, "cursor")
	if cursor.State != ACPCatalogStateDetectionFailed {
		t.Fatalf("estado = %q, quer %q", cursor.State, ACPCatalogStateDetectionFailed)
	}
	if cursor.StateDetail == "" {
		t.Error("sem o motivo não há o que dizer a quem vai corrigir")
	}
	if cursor.StateDetail != acp.SanitizeLabel(cursor.StateDetail) {
		t.Errorf("detalhe = %q, quer o texto saneado: ele vai para a tela e para o anúncio", cursor.StateDetail)
	}
}

func TestORuntimeAusenteVenceOsOutrosEstados(t *testing.T) {
	// Sem Node não há caminho para o agente npm, e é isso que precisa ser dito
	// primeiro (D7): "não encontrado" mandaria procurar instalação em vez de
	// resolver o que bloqueia.
	installs := map[acp.AgentKind]acpDetection{
		acp.AgentKindClaudeCode: {install: acp.Install{Found: false}},
	}
	runtimes := map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: false}}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("claude-acp", "Claude Code"), agenteNPM("codex-acp", "Codex")),
		"linux-x86_64", installs, runtimes,
	)

	for _, id := range []string{"claude-acp", "codex-acp"} {
		agent := acharPorID(t, catalogo, id)
		if agent.State != ACPCatalogStateRequirementMissing {
			t.Errorf("%s: estado = %q, quer %q", id, agent.State, ACPCatalogStateRequirementMissing)
		}
		if agent.Runtime != string(acp.RuntimeNode) {
			t.Errorf("%s: runtime = %q, quer %q nomeado em texto", id, agent.Runtime, acp.RuntimeNode)
		}
		if agent.RuntimeFound {
			t.Errorf("%s: RuntimeFound = true numa máquina sem Node", id)
		}
	}
}

func TestARuntimeEncontradoNaoBloqueiaEDizOndeEsta(t *testing.T) {
	runtimes := map[acp.Runtime]acp.RuntimeInstall{
		acp.RuntimeNode: {Found: true, Path: `C:\Program Files\nodejs\node.exe`},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("codex-acp", "Codex")), "windows-x86_64", nil, runtimes,
	)

	codex := acharPorID(t, catalogo, "codex-acp")
	if !codex.RuntimeFound {
		t.Fatal("o Node está na máquina e a linha diz que não")
	}
	if codex.RuntimePath != `C:\Program Files\nodejs\node.exe` {
		t.Errorf("caminho do runtime = %q, quer o que a procura achou", codex.RuntimePath)
	}
	if codex.State == ACPCatalogStateRequirementMissing {
		t.Error("estado de requisito ausente com o requisito presente")
	}
}

func TestOAgenteSemAlvoParaEstaPlataformaEDitoAssim(t *testing.T) {
	// Windows ARM: dizer "não encontrado" ou "requisito ausente" mandaria
	// procurar solução para um agente que não tem artefato para esta máquina.
	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("goose", "Goose", "linux-x86_64", digestQualquer)),
		"windows-aarch64", nil, nil,
	)

	goose := acharPorID(t, catalogo, "goose")
	if goose.State != ACPCatalogStateNoPlatformTarget {
		t.Errorf("estado = %q, quer %q", goose.State, ACPCatalogStateNoPlatformTarget)
	}
	if goose.Integrity != string(acpregistry.IntegrityNoPlatformTarget) {
		t.Errorf("integridade = %q, quer %q", goose.Integrity, acpregistry.IntegrityNoPlatformTarget)
	}
}

func TestOCatalogoVazioCarregaOMotivoESemListaNula(t *testing.T) {
	// Primeira execução offline (D2): a tela abre, e o que ela tem para dizer é
	// por quê não há catálogo.
	catalogo := acpCatalogFrom(
		acpregistry.Catalog{
			Reason:       "não foi possível buscar o índice do registro ACP: sem rota",
			ReasonCode:   acpregistry.ReasonUnreachable,
			ReasonDetail: "sem rota para o host",
		},
		"linux-x86_64", nil, nil,
	)

	if catalogo.ReasonCode != string(acpregistry.ReasonUnreachable) {
		t.Errorf("código do motivo = %q, quer %q", catalogo.ReasonCode, acpregistry.ReasonUnreachable)
	}
	if catalogo.ReasonDetail != "sem rota para o host" {
		t.Errorf("detalhe = %q, quer o do transporte", catalogo.ReasonDetail)
	}
	if catalogo.Agents == nil {
		t.Error("lista nula faria a tela distinguir \"sem agentes\" de \"campo ausente\"")
	}
	if catalogo.FetchedAt != "" {
		t.Errorf("carimbo = %q, quer vazio: não há catálogo para datar", catalogo.FetchedAt)
	}
}

func TestOCatalogoVelhoDizQuandoFoiColetado(t *testing.T) {
	catalogo := acpCatalogFrom(
		acpregistry.Catalog{
			Version:   "1.0.0",
			Agents:    []acpregistry.Agent{agenteNPM("codex-acp", "Codex")},
			FetchedAt: time.Unix(1770000000, 0),
			Age:       26 * time.Hour,
			FromCache: true,
			Stale:     true,
		},
		"linux-x86_64", nil,
		map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true}},
	)

	if !catalogo.Stale || !catalogo.FromCache {
		t.Errorf("Stale/FromCache = %v/%v, quer os dois verdadeiros", catalogo.Stale, catalogo.FromCache)
	}
	if catalogo.AgeSeconds != int64((26 * time.Hour).Seconds()) {
		t.Errorf("idade = %ds, quer 26 horas em segundos", catalogo.AgeSeconds)
	}
	// O carimbo sai em RFC 3339 e em UTC: é a tela que o formata no idioma e no
	// fuso de quem lê, e um texto já montado aqui existiria em um idioma só.
	if catalogo.FetchedAt != time.Unix(1770000000, 0).UTC().Format(time.RFC3339) {
		t.Errorf("carimbo = %q, quer RFC 3339 em UTC", catalogo.FetchedAt)
	}
}

func TestSemRuntimeExigidoNaoSeDizNadaSobreRuntime(t *testing.T) {
	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("goose", "Goose", "linux-x86_64", digestQualquer)),
		"linux-x86_64", nil, nil,
	)

	goose := acharPorID(t, catalogo, "goose")
	if goose.Runtime != "" {
		t.Errorf("runtime = %q, quer vazio: binário conferível não exige nenhum", goose.Runtime)
	}
	if goose.RuntimeFound || goose.RuntimePath != "" {
		t.Error("sem requisito não há o que encontrar, e a linha não pode sugerir que há")
	}
	if goose.Distributions == nil {
		t.Error("lista de distribuições nula")
	}
}
