package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"assistente/internal/acpinstall"
	"assistente/internal/credentials"
	"assistente/internal/llm"
)

// instalacaoDoCodex é o que o app teria escrito ao instalar o adaptador do
// Codex por npm: o par `node` + ponto de entrada, com o ponto de entrada dentro
// do diretório da versão (D5, D8).
func instalacaoDoCodex(root, versao string) acpinstall.Installation {
	dir := filepath.Join(root, "codex-acp", versao)
	return acpinstall.Installation{
		AgentID:      "codex-acp",
		Name:         "Codex",
		Version:      versao,
		Distribution: acpinstall.DistributionNPM,
		Command:      filepath.Join("C:", "nodejs", "node.exe"),
		Args:         []string{filepath.Join(dir, "node_modules", "codex-acp", "dist", "index.js"), "--acp"},
		Dir:          dir,
	}
}

func TestOPlanoTraduzidoLevaOAvisoDeVersaoNova(t *testing.T) {
	// O aviso e a oferta são campos próprios do DTO: deduzi-los na tela
	// comparando duas versões em texto seria reimplementar em TypeScript a
	// regra que decide se dá para atualizar (D10).
	dto := installPlanDTO(acpinstall.Plan{
		AgentID:      "codex-acp",
		Name:         "Codex",
		Version:      "1.2.0",
		Distribution: acpinstall.DistributionNPM,
		Installed:    &acpinstall.Installation{AgentID: "codex-acp", Version: "1.1.9", Command: "node"},
		Update:       true,
		CanUpdate:    false,
		UpdateReason: "o Node.js não foi encontrado nesta máquina",
	}, false)

	if !dto.Update {
		t.Error("perdeu o aviso de que o catálogo publica outra versão")
	}
	if dto.CanUpdate {
		t.Error("ofereceu atualizar contra o que o plano decidiu")
	}
	if dto.UpdateReason == "" {
		t.Error("botão indisponível sem o motivo à vista (D7)")
	}
}

func TestSoOsProvedoresQueSobemAInstalacaoSaoRepontados(t *testing.T) {
	// A pergunta é pelo diretório, e não pelo `acp_agent_id`: um provedor pode
	// apontar para o mesmo agente instalado por fora, à mão, e repontar esse
	// seria reescrever escolha alheia.
	root := t.TempDir()
	instalacao := instalacaoDoCodex(root, "1.1.9")
	registro := llm.NewProviderRegistry()
	daInstalacao := &llm.ProviderConfig{
		ID:         "codex-do-app",
		Name:       "Codex do app",
		APIFormat:  llm.APIFormatACP,
		ACPAgentID: "codex-acp",
		ACPCommand: instalacao.Command,
		ACPArgs:    slices.Clone(instalacao.Args),
	}
	aMao := &llm.ProviderConfig{
		ID:         "codex-a-mao",
		Name:       "Codex de fora",
		APIFormat:  llm.APIFormatACP,
		ACPAgentID: "codex-acp",
		ACPCommand: filepath.Join("C:", "ferramentas", "codex-acp.exe"),
	}
	httpQualquer := &llm.ProviderConfig{
		ID:        "openai",
		Name:      "OpenAI",
		APIFormat: llm.APIFormatOpenAI,
		BaseURL:   "https://api.openai.com/v1",
	}
	for _, provedor := range []*llm.ProviderConfig{daInstalacao, aMao, httpQualquer} {
		if err := registro.Register(provedor); err != nil {
			t.Fatalf("erro ao registrar %s: %v", provedor.ID, err)
		}
	}
	a := &App{llmRegistry: registro}

	encontrados := a.acpProvidersFrom([]acpinstall.Installation{instalacao})

	if len(encontrados) != 1 {
		t.Fatalf("provedores = %d (%+v), queria só o que sobe a instalação", len(encontrados), encontrados)
	}
	if encontrados[0].ID != "codex-do-app" {
		t.Errorf("provedor = %q, queria o que aponta para dentro do diretório instalado", encontrados[0].ID)
	}
}

func TestSemInstalacaoNoDiscoNaoHaProvedorAReapontar(t *testing.T) {
	// A instalação vazia tem diretório vazio, e todo caminho está "dentro" de
	// nada: sem esta guarda, atualizar repontaria a máquina inteira.
	registro := llm.NewProviderRegistry()
	if err := registro.Register(&llm.ProviderConfig{
		ID:         "codex",
		Name:       "Codex",
		APIFormat:  llm.APIFormatACP,
		ACPCommand: filepath.Join("C:", "ferramentas", "codex-acp.exe"),
	}); err != nil {
		t.Fatalf("erro ao registrar o provedor: %v", err)
	}
	a := &App{llmRegistry: registro}

	semNada := a.acpProvidersFrom(nil)
	semDiretorio := a.acpProvidersFrom([]acpinstall.Installation{{AgentID: "codex-acp"}})
	if len(semNada) != 0 || len(semDiretorio) != 0 {
		t.Errorf("provedores = %+v / %+v, queria nenhum", semNada, semDiretorio)
	}
}

func TestOProvedorPresoNumaVersaoAntigaTambemEEncontrado(t *testing.T) {
	// Mais de uma versão no disco é o estado normal depois de uma atualização
	// que não pôde limpar a anterior — ela é adiada quando o agente está em
	// conversa (D10). Um provedor que ficou apontando para uma delas continua
	// sendo daquela instalação, e a atualização seguinte tem de repontá-lo antes
	// de apagar o diretório debaixo dele.
	root := t.TempDir()
	antiga := instalacaoDoCodex(root, "1.0.0")
	corrente := instalacaoDoCodex(root, "1.1.9")
	registro := llm.NewProviderRegistry()
	if err := registro.Register(&llm.ProviderConfig{
		ID:         "codex-preso",
		Name:       "Codex preso",
		APIFormat:  llm.APIFormatACP,
		ACPCommand: antiga.Command,
		ACPArgs:    slices.Clone(antiga.Args),
	}); err != nil {
		t.Fatalf("erro ao registrar o provedor: %v", err)
	}
	a := &App{llmRegistry: registro}

	encontrados := a.acpProvidersFrom([]acpinstall.Installation{antiga, corrente})

	if len(encontrados) != 1 || encontrados[0].ID != "codex-preso" {
		t.Fatalf("provedores = %+v, queria o que ficou na versão antiga", encontrados)
	}
}

func TestRepontarPoeOProvedorNaVersaoNova(t *testing.T) {
	// A versão anterior só sai depois disto (D10): o provedor tem de estar
	// apontando para a nova antes de o diretório da velha ser apagado.
	_ = setupTestDB(t)
	root := t.TempDir()
	anterior := instalacaoDoCodex(root, "1.1.9")
	nova := instalacaoDoCodex(root, "1.2.0")

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	registro := llm.NewProviderRegistry()
	a := newAppForTest(credMgr, registro)
	if err := registro.Register(&llm.ProviderConfig{
		ID:         "codex-do-app",
		Name:       "Codex do app",
		Type:       llm.ProviderACP,
		APIFormat:  llm.APIFormatACP,
		ACPAgentID: "codex-acp",
		ACPCommand: anterior.Command,
		ACPArgs:    slices.Clone(anterior.Args),
	}); err != nil {
		t.Fatalf("erro ao registrar o provedor: %v", err)
	}

	a.repointACPProviders(context.Background(), a.acpProvidersFrom([]acpinstall.Installation{anterior}), nova)

	repontado := registro.Get("codex-do-app")
	if repontado == nil {
		t.Fatal("o provedor sumiu do registro")
	}
	if !slices.Equal(repontado.ACPArgs, nova.Args) {
		t.Errorf("argumentos = %q, queria os da versão nova", repontado.ACPArgs)
	}
	if repontado.ACPCommand != nova.Command {
		t.Errorf("comando = %q, queria o da versão nova", repontado.ACPCommand)
	}
	// E o vínculo com o catálogo continua onde estava: atualizar troca o que
	// sobe, e não de que agente aquele provedor é.
	if repontado.ACPAgentID != "codex-acp" {
		t.Errorf("agente = %q, queria o mesmo de antes", repontado.ACPAgentID)
	}
}

// gravarInstalacao escreve no disco uma instalação como o app a teria deixado:
// o diretório da versão, um ponto de entrada de verdade lá dentro e o
// `installed.json` que o descreve. Sem o arquivo executável o registro é
// descartado na leitura, que é a guarda do D9.
func gravarInstalacao(t *testing.T, root, versao string) {
	t.Helper()
	dir := filepath.Join(root, "codex-acp", versao)
	entrada := filepath.Join(dir, "index.js")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("erro ao montar %s: %v", dir, err)
	}
	if err := os.WriteFile(entrada, []byte("#!/usr/bin/env node\n"), 0o644); err != nil {
		t.Fatalf("erro ao escrever o ponto de entrada: %v", err)
	}
	registro := map[string]any{
		"schema":       1,
		"agent_id":     "codex-acp",
		"name":         "Codex",
		"version":      versao,
		"distribution": acpinstall.DistributionNPM,
		"command":      filepath.Join("C:", "nodejs", "node.exe"),
		"args":         []string{entrada, "--acp"},
		"installed_at": time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(registro)
	if err != nil {
		t.Fatalf("erro ao serializar o registro: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "installed.json"), data, 0o644); err != nil {
		t.Fatalf("erro ao escrever o registro: %v", err)
	}
}

func TestALimpezaVarreAsVersoesQueNinguemMaisSobe(t *testing.T) {
	// Ela apaga tudo o que não é a versão mantida, e não só a anterior: uma
	// atualização passada pode ter deixado a sua para trás quando o agente
	// estava em conversa, e insistir mais tarde é mais barato do que acumular
	// versões que ninguém executa (D10).
	root := t.TempDir()
	for _, versao := range []string{"1.0.0", "1.1.9", "1.2.0"} {
		gravarInstalacao(t, root, versao)
	}
	a := &App{}
	a.acpCatalogOnce.Do(func() {
		a.acpCatalogSvc = &acpCatalog{installer: acpinstall.New(acpinstall.Config{Root: root})}
	})

	a.removeSupersededVersions(context.Background(), "codex-acp", "1.2.0", nil)

	restantes := a.acpCatalogServices().installer.Installations("codex-acp")
	if len(restantes) != 1 || restantes[0].Version != "1.2.0" {
		t.Fatalf("sobraram %+v, queria só a versão mantida", restantes)
	}
}

func TestSemServicoDeAgentesAAtualizacaoNaoERecusadaPorConversa(t *testing.T) {
	// A recusa é sobre um turno que existe. Sem o serviço que sabe dos turnos
	// não há conversa nenhuma de pé, e travar a atualização por precaução
	// deixaria o agente preso na versão instalada sem motivo dizível.
	a := &App{}

	if err := a.refuseUpdateDuringTurn([]*llm.ProviderConfig{{ID: "codex-do-app"}}); err != nil {
		t.Errorf("recusou a atualização sem ter turno nenhum para citar: %v", err)
	}
}
