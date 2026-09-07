package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/acpregistry"
	"assistente/internal/apidto"
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

func acharPorID(t *testing.T, catalogo apidto.ACPCatalog, id string) apidto.ACPCatalogAgent {
	t.Helper()
	for _, agent := range catalogo.Agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("o agente %q não está no catálogo", id)
	return apidto.ACPCatalogAgent{}
}

func TestOCatalogoVemOrdenadoPorNome(t *testing.T) {
	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("zed-agent", "Zed"), agenteNPM("codex-acp", "codex"), agenteNPM("amp", "Amp")),
		"linux-x86_64",
		acpMachine{runtimes: map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true, Path: "/usr/bin/node"}}},
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
		"windows-x86_64", acpMachine{detected: installs},
	)

	cursor := acharPorID(t, catalogo, "cursor")
	if cursor.State != apidto.ACPCatalogStateInstalled {
		t.Fatalf("estado = %q, quer %q", cursor.State, apidto.ACPCatalogStateInstalled)
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

func TestOsCaminhosQueVaoAoNomeAcessivelSaoSaneados(t *testing.T) {
	// Caminho é montado a partir de variáveis de ambiente e do PATH, e vira nome
	// acessível do item na tela. Uma marca invisível de direção no meio dele
	// faria o item ser lido diferente do que ele é.
	const marcaDeDirecao = "\u202e"
	installs := map[acp.AgentKind]acpDetection{
		acp.AgentKindCursor: {install: acp.Install{
			Found:  true,
			Source: "/home/ana/" + marcaDeDirecao + "sj.exe\r\n",
		}},
	}
	runtimes := map[acp.Runtime]acp.RuntimeInstall{
		acp.RuntimeNode: {Found: true, Path: "/opt/" + marcaDeDirecao + "node\u0007"},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("cursor", "Cursor", "linux-x86_64", digestQualquer), agenteNPM("amp", "Amp")),
		"linux-x86_64", acpMachine{detected: installs, runtimes: runtimes},
	)

	cursor := acharPorID(t, catalogo, "cursor")
	if strings.ContainsAny(cursor.StateDetail, marcaDeDirecao+"\r\n") {
		t.Errorf("detalhe do estado = %q, quer uma linha só sem marca invisível", cursor.StateDetail)
	}
	amp := acharPorID(t, catalogo, "amp")
	if strings.ContainsAny(amp.RuntimePath, marcaDeDirecao+"\u0007") {
		t.Errorf("caminho do runtime = %q, quer uma linha só sem marca invisível", amp.RuntimePath)
	}
	// Saneado não é apagado: o caminho continua identificando a instalação.
	if !strings.Contains(amp.RuntimePath, "node") {
		t.Errorf("caminho do runtime = %q, quer ainda apontar o executável", amp.RuntimePath)
	}
}

func TestOQueOAppInstalouApareceComoInstaladoAindaQueADeteccaoNaoOConheca(t *testing.T) {
	// O Codex não está entre os dois agentes que a detecção sabe procurar (D1),
	// e antes disto ele aparecia como "o app não sabe procurar este" mesmo
	// depois de o app o ter instalado — falso justamente para o agente sobre o
	// qual ele mais sabe.
	instalado := map[string]acpinstall.Installation{
		"codex-acp": {
			AgentID: "codex-acp",
			Version: "1.0.0",
			Dir:     "/home/ana/.assistente/agents/codex-acp/1.0.0",
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("codex-acp", "Codex")),
		"linux-x86_64",
		acpMachine{
			runtimes:  map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true}},
			installed: instalado,
		},
	)

	codex := acharPorID(t, catalogo, "codex-acp")
	if codex.State != apidto.ACPCatalogStateInstalled {
		t.Fatalf("estado = %q, quer %q", codex.State, apidto.ACPCatalogStateInstalled)
	}
	if !codex.InstalledByApp {
		t.Error("a instalação é do app, e é isso que decide se dá para removê-la daqui")
	}
	if codex.InstalledVersion != "1.0.0" {
		t.Errorf("versão instalada = %q, quer a que o app pôs no disco", codex.InstalledVersion)
	}
	if codex.StateDetail != "/home/ana/.assistente/agents/codex-acp/1.0.0" {
		t.Errorf("detalhe = %q, quer o diretório da instalação", codex.StateDetail)
	}
	// A versão do registro continua ao lado, e não é substituída pela
	// instalada: é a diferença entre as duas que a Fase 7 usa.
	if codex.Version != "1.0.0" {
		t.Errorf("versão do registro = %q, quer a do catálogo", codex.Version)
	}
}

func TestAInstalacaoNaoVerificadaContinuaDizendoIssoNoCatalogo(t *testing.T) {
	// A marca acompanha o agente depois de instalado (D4): quem abre o catálogo
	// semanas depois não viu o diálogo em que ela foi aceita.
	instalado := map[string]acpinstall.Installation{
		"cursor": {
			AgentID:      "cursor",
			Version:      "2026.01.02",
			Dir:          "/home/ana/.assistente/agents/cursor/2026.01.02",
			Distribution: acpinstall.DistributionBinary,
			SHA256Origin: acpinstall.DigestObserved,
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("cursor", "Cursor", "linux-x86_64", "")),
		"linux-x86_64", acpMachine{installed: instalado},
	)

	cursor := acharPorID(t, catalogo, "cursor")
	if !cursor.InstalledUnverified {
		t.Error("a instalação foi feita sem digest publicado e o catálogo não diz")
	}
}

func TestAInstalacaoConferidaNaoEMarcadaComoNaoVerificada(t *testing.T) {
	instalado := map[string]acpinstall.Installation{
		"goose": {
			AgentID:      "goose",
			Version:      "2.0.0",
			Dir:          "/home/ana/.assistente/agents/goose/2.0.0",
			Distribution: acpinstall.DistributionBinary,
			SHA256:       digestQualquer,
			SHA256Origin: acpinstall.DigestVerified,
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("goose", "Goose", "linux-x86_64", digestQualquer)),
		"linux-x86_64", acpMachine{installed: instalado},
	)

	goose := acharPorID(t, catalogo, "goose")
	if goose.InstalledUnverified {
		t.Error("o digest foi conferido e o catálogo diz que não")
	}
	if !goose.InstalledByApp {
		t.Error("a instalação é do app")
	}
}

func TestOBinarioSemOrigemDeDigestConhecidaContaComoNaoVerificado(t *testing.T) {
	// O registro vem do disco: um campo vazio ou com valor que o app não escreveu
	// não é conferência, e tratá-lo como se fosse esconderia a ressalva
	// justamente no caso em que ela mais vale.
	instalado := map[string]acpinstall.Installation{
		"goose": {
			AgentID:      "goose",
			Version:      "2.0.0",
			Dir:          "/home/ana/.assistente/agents/goose/2.0.0",
			Distribution: acpinstall.DistributionBinary,
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("goose", "Goose", "linux-x86_64", digestQualquer)),
		"linux-x86_64", acpMachine{installed: instalado},
	)

	if goose := acharPorID(t, catalogo, "goose"); !goose.InstalledUnverified {
		t.Error("binário sem origem de digest conhecida foi dado como conferido")
	}
}

func TestADistribuicaoQueOAppNaoEscreveuNaoCalaARessalva(t *testing.T) {
	// A distribuição também vem do registro no disco. Se bastasse ela não dizer
	// `binary` para a ressalva sumir, o caminho para contornar o D4 seria editar
	// uma palavra num arquivo de texto.
	instalado := map[string]acpinstall.Installation{
		"goose": {
			AgentID:      "goose",
			Version:      "2.0.0",
			Dir:          "/home/ana/.assistente/agents/goose/2.0.0",
			Distribution: "qualquer-coisa",
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("goose", "Goose", "linux-x86_64", digestQualquer)),
		"linux-x86_64", acpMachine{installed: instalado},
	)

	if goose := acharPorID(t, catalogo, "goose"); !goose.InstalledUnverified {
		t.Error("distribuição desconhecida calou a ressalva de instalação não verificada")
	}
}

func TestORegistroQueSeDizConferidoSemDizerContraOQueNaoConta(t *testing.T) {
	// `verified` com o campo do digest vazio não descreve conferência nenhuma —
	// e a instalação binária que o app faz sempre grava os dois.
	instalado := map[string]acpinstall.Installation{
		"goose": {
			AgentID:      "goose",
			Version:      "2.0.0",
			Dir:          "/home/ana/.assistente/agents/goose/2.0.0",
			Distribution: acpinstall.DistributionBinary,
			SHA256Origin: acpinstall.DigestVerified,
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("goose", "Goose", "linux-x86_64", digestQualquer)),
		"linux-x86_64", acpMachine{installed: instalado},
	)

	if goose := acharPorID(t, catalogo, "goose"); !goose.InstalledUnverified {
		t.Error("registro que se diz conferido sem digest passou por conferido")
	}
}

func TestOPacoteNpmNaoGanhaRessalvaDeDigest(t *testing.T) {
	// Quem confere o pacote é o próprio npm, e ali o campo é naturalmente vazio.
	// Uma ressalva aqui seria alarme nos 21 agentes de pacote — e alarme que
	// sempre toca é alarme que se aprende a ignorar.
	instalado := map[string]acpinstall.Installation{
		"codex-acp": {
			AgentID:      "codex-acp",
			Version:      "1.0.0",
			Dir:          "/home/ana/.assistente/agents/codex-acp/1.0.0",
			Distribution: acpinstall.DistributionNPM,
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("codex-acp", "Codex")),
		"linux-x86_64",
		acpMachine{
			runtimes:  map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true}},
			installed: instalado,
		},
	)

	if codex := acharPorID(t, catalogo, "codex-acp"); codex.InstalledUnverified {
		t.Error("pacote npm marcado como instalação não verificada")
	}
}

func TestAInstalacaoDoAppRespondeAntesDaDeteccao(t *testing.T) {
	// Os dois podem existir: quem já tinha o Cursor instalado por fora pode ter
	// pedido a instalação pelo catálogo depois. Vale a do app, porque é a que
	// ele sabe onde está e sabe remover.
	installs := map[acp.AgentKind]acpDetection{
		acp.AgentKindCursor: {install: acp.Install{
			Found:   true,
			Source:  `C:\fora\cursor-agent.cmd`,
			Version: "2025.12.31",
		}},
	}
	instalado := map[string]acpinstall.Installation{
		"cursor": {AgentID: "cursor", Version: "2026.01.02", Dir: `C:\app\agents\cursor\2026.01.02`},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("cursor", "Cursor", "windows-x86_64", "")),
		"windows-x86_64", acpMachine{detected: installs, installed: instalado},
	)

	cursor := acharPorID(t, catalogo, "cursor")
	if !cursor.InstalledByApp || cursor.InstalledVersion != "2026.01.02" {
		t.Errorf("linha = %+v, quer a instalação do app", cursor)
	}
	if cursor.StateDetail != `C:\app\agents\cursor\2026.01.02` {
		t.Errorf("detalhe = %q, quer o diretório do app", cursor.StateDetail)
	}
}

func TestOQueVemDoRegistroDeInstalacaoTambemESaneado(t *testing.T) {
	// O `installed.json` está no disco de alguém e pode ser editado à mão. O que
	// sai dele vira nome acessível do item como qualquer outro texto.
	const marcaDeDirecao = "\u202e"
	instalado := map[string]acpinstall.Installation{
		"codex-acp": {
			AgentID: "codex-acp",
			Version: "1.0.0" + marcaDeDirecao + "\r\n",
			Dir:     "/home/ana/" + marcaDeDirecao + "agents\u0007",
		},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("codex-acp", "Codex")),
		"linux-x86_64",
		acpMachine{
			runtimes:  map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true}},
			installed: instalado,
		},
	)

	codex := acharPorID(t, catalogo, "codex-acp")
	if strings.ContainsAny(codex.InstalledVersion, marcaDeDirecao+"\r\n") {
		t.Errorf("versão instalada = %q, quer o texto saneado", codex.InstalledVersion)
	}
	if strings.ContainsAny(codex.StateDetail, marcaDeDirecao+"\u0007") {
		t.Errorf("detalhe = %q, quer o texto saneado", codex.StateDetail)
	}
}

func TestOAgenteQueADeteccaoNaoConheceNaoEDitoComoNaoEncontrado(t *testing.T) {
	// A detecção sabe procurar dois agentes dos 38 (D1). Dizer "não encontrado"
	// para os outros alegaria uma procura que o app não sabe fazer — e mandaria
	// quem lê concluir que o agente não está na máquina.
	catalogo := acpCatalogFrom(
		catalogoDe(agenteNPM("codex-acp", "Codex")),
		"linux-x86_64",
		acpMachine{
			detected: map[acp.AgentKind]acpDetection{},
			runtimes: map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true, Path: "/usr/bin/node"}},
		},
	)

	codex := acharPorID(t, catalogo, "codex-acp")
	if codex.State != apidto.ACPCatalogStateNoDetection {
		t.Errorf("estado = %q, quer %q", codex.State, apidto.ACPCatalogStateNoDetection)
	}
}

func TestOAgenteConhecidoQueNaoEstaNaMaquinaEDitoComoNaoEncontrado(t *testing.T) {
	installs := map[acp.AgentKind]acpDetection{
		acp.AgentKindCursor: {install: acp.Install{Found: false}},
	}

	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("cursor", "Cursor", "linux-x86_64", digestQualquer)),
		"linux-x86_64", acpMachine{detected: installs},
	)

	if cursor := acharPorID(t, catalogo, "cursor"); cursor.State != apidto.ACPCatalogStateNotInstalled {
		t.Errorf("estado = %q, quer %q", cursor.State, apidto.ACPCatalogStateNotInstalled)
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
		"linux-x86_64", acpMachine{detected: installs},
	)

	cursor := acharPorID(t, catalogo, "cursor")
	if cursor.State != apidto.ACPCatalogStateDetectionFailed {
		t.Fatalf("estado = %q, quer %q", cursor.State, apidto.ACPCatalogStateDetectionFailed)
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
		"linux-x86_64", acpMachine{detected: installs, runtimes: runtimes},
	)

	for _, id := range []string{"claude-acp", "codex-acp"} {
		agent := acharPorID(t, catalogo, id)
		if agent.State != apidto.ACPCatalogStateRequirementMissing {
			t.Errorf("%s: estado = %q, quer %q", id, agent.State, apidto.ACPCatalogStateRequirementMissing)
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
		catalogoDe(agenteNPM("codex-acp", "Codex")), "windows-x86_64", acpMachine{runtimes: runtimes},
	)

	codex := acharPorID(t, catalogo, "codex-acp")
	if !codex.RuntimeFound {
		t.Fatal("o Node está na máquina e a linha diz que não")
	}
	if codex.RuntimePath != `C:\Program Files\nodejs\node.exe` {
		t.Errorf("caminho do runtime = %q, quer o que a procura achou", codex.RuntimePath)
	}
	if codex.State == apidto.ACPCatalogStateRequirementMissing {
		t.Error("estado de requisito ausente com o requisito presente")
	}
}

func TestOAgenteSemAlvoParaEstaPlataformaEDitoAssim(t *testing.T) {
	// Windows ARM: dizer "não encontrado" ou "requisito ausente" mandaria
	// procurar solução para um agente que não tem artefato para esta máquina.
	catalogo := acpCatalogFrom(
		catalogoDe(agenteBinario("goose", "Goose", "linux-x86_64", digestQualquer)),
		"windows-aarch64", acpMachine{},
	)

	goose := acharPorID(t, catalogo, "goose")
	if goose.State != apidto.ACPCatalogStateNoPlatformTarget {
		t.Errorf("estado = %q, quer %q", goose.State, apidto.ACPCatalogStateNoPlatformTarget)
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
		"linux-x86_64", acpMachine{},
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
		"linux-x86_64",
		acpMachine{runtimes: map[acp.Runtime]acp.RuntimeInstall{acp.RuntimeNode: {Found: true}}},
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
		"linux-x86_64", acpMachine{},
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
