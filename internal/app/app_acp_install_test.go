package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/acpinstall"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/wailsapi"
)

func installAPI(a *App) *wailsapi.ACPInstall {
	api := wailsapi.NewACPInstall()
	wailsapi.AttachACPInstall(api, wailsSession{app: a}, wailsapi.ACPInstallHooks{
		Installer: func() *acpinstall.Installer {
			return a.acpCatalogServices().installer
		},
		ProvidersFrom:            a.acpProvidersFrom,
		RefuseUpdateDuringTurn:   a.refuseUpdateDuringTurn,
		RepointProviders:         a.repointACPProviders,
		RemoveSupersededVersions: a.removeSupersededVersions,
	})
	return api
}

func TestProgressoDaInstalacaoVaiParaATelaComOAgenteQueOMotivou(t *testing.T) {
	// Todo marco carrega o identificador do agente: duas instalações podem estar
	// em voo, e um progresso sem dono não diz de quem ele fala (D13).
	emissor := &testEmitter{}
	a := &App{emitter: emissor}

	a.emitACPInstallProgress(acpinstall.Progress{
		AgentID: "codex-acp",
		Agent:   "Codex",
		Stage:   acpinstall.StageFailed,
		Step:    acpinstall.StepInstall,
		Reason:  "EBADENGINE required: { node: '>=22' }",
	})

	eventos := emissor.find(ACPInstallProgressEvent)
	if len(eventos) != 1 {
		t.Fatalf("eventos = %d, queria um", len(eventos))
	}
	progresso, ok := eventos[0].data.(ACPInstallProgress)
	if !ok {
		t.Fatalf("payload = %T, queria ACPInstallProgress", eventos[0].data)
	}
	if progresso.AgentID != "codex-acp" || progresso.Agent != "Codex" {
		t.Errorf("progresso = %+v, queria o agente identificado", progresso)
	}
	if progresso.Stage != "failed" || progresso.Step != "install" {
		t.Errorf("progresso = %+v, queria a etapa que falhou nomeada", progresso)
	}
	// A mensagem do npm chega inteira: quem vai resolver um Node velho demais
	// precisa do texto original.
	if !strings.Contains(progresso.Reason, "EBADENGINE") {
		t.Errorf("motivo = %q, queria a mensagem do npm", progresso.Reason)
	}
}

func TestOInstaladorUsaOMesmoRegistroQueATelaDoCatalogo(t *testing.T) {
	// Com dois serviços, o "atualizar catálogo" da tela traria a versão nova
	// para a lista e deixaria o instalador planejando com o índice velho que ele
	// guarda em memória — dois números para o mesmo agente, na mesma tela.
	a := &App{}
	a.initACP()

	catalogo := a.acpCatalogServices()

	if a.acpRegistry == nil {
		t.Fatal("o initACP não deixou serviço do registro nenhum")
	}
	if catalogo.registry != a.acpRegistry {
		t.Error("o catálogo montou um segundo serviço do registro em vez de usar o do app")
	}
}

func TestSemInitACPOCatalogoMontaOServicoQueFaltava(t *testing.T) {
	// A instalação não depende de a inicialização do ACP ter acontecido: quem
	// chegar primeiro monta o serviço, e quem vier depois acha o mesmo.
	a := &App{}

	catalogo := a.acpCatalogServices()

	if catalogo.registry == nil {
		t.Fatal("não montou serviço do registro nenhum")
	}
	if a.acpRegistry != catalogo.registry {
		t.Error("o serviço montado aqui não ficou no app, e a tela do catálogo montaria outro")
	}
}

func TestAgenteSoPodeSerDesinstaladoDepoisDoUltimoProvider(t *testing.T) {
	_ = setupTestDB(t)
	root := t.TempDir()
	gravarInstalacao(t, root, "1.2.0")

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	registry := llm.NewProviderRegistry()
	a := newAppForTest(credMgr, registry)
	a.acpCatalogOnce.Do(func() {
		a.acpCatalogSvc = &acpCatalog{
			installer: acpinstall.New(acpinstall.Config{Root: root}),
		}
	})
	api := installAPI(a)

	if _, err := a.createLLMProvider(CreateLLMProviderRequest{
		ID:         "codex-1",
		Name:       "Codex",
		Type:       "acp",
		APIFormat:  "acp",
		ACPCommand: "node",
		ACPArgs:    []string{"codex-acp"},
		ACPAgentID: "codex-acp",
	}); err != nil {
		t.Fatalf("criar provider: %v", err)
	}

	canRemove, err := api.CanRemoveACPAgent("codex-acp")
	if err != nil {
		t.Fatalf("consultar uso: %v", err)
	}
	if canRemove {
		t.Fatal("ofereceu desinstalar um agente ainda usado")
	}
	if err := api.RemoveACPAgent("codex-acp"); err == nil {
		t.Fatal("desinstalou um agente ainda usado")
	}

	if err := a.deleteLLMProvider("codex-1"); err != nil {
		t.Fatalf("remover provider: %v", err)
	}
	canRemove, err = api.CanRemoveACPAgent("codex-acp")
	if err != nil {
		t.Fatalf("consultar órfão: %v", err)
	}
	if !canRemove {
		t.Fatal("não ofereceu desinstalar o agente órfão")
	}
	if err := api.RemoveACPAgent("codex-acp"); err != nil {
		t.Fatalf("desinstalar agente órfão: %v", err)
	}
	if installations := a.acpCatalogServices().installer.List(); len(installations) != 0 {
		t.Fatalf("instalações restantes = %+v", installations)
	}
}

func TestPlanoQueNaoOfereceNadaAindaTrazAListaDeArgumentos(t *testing.T) {
	// O DTO promete `run_args` sempre presente para a tela não ter de distinguir
	// "sem argumentos" de "campo ausente". O plano que não oferece nada é
	// justamente onde o literal zerado mandaria `null`.
	a := newAppForTest(
		credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!")),
		llm.NewProviderRegistry(),
	)
	a.acpCatalogOnce.Do(func() {
		a.acpCatalogSvc = &acpCatalog{installer: acpinstall.New(acpinstall.Config{})}
	})

	plano, err := installAPI(a).ACPAgentInstallPlan("nao-esta-no-catalogo")
	if err != nil {
		t.Fatalf("o plano falhou em vez de explicar: %v", err)
	}
	if plano.RunArgs == nil {
		t.Error("run_args veio nulo, e a tela teria de distinguir null de lista vazia")
	}
	if plano.Reason == "" {
		t.Error("plano indisponível sem motivo em texto (D7)")
	}
}

func TestProgressoSemEmissorNaoQuebra(t *testing.T) {
	// A instalação pode acontecer antes de a janela existir; falhar aqui faria a
	// instalação inteira falhar por causa de um anúncio.
	a := &App{}
	a.emitACPInstallProgress(acpinstall.Progress{AgentID: "codex-acp", Stage: acpinstall.StageDone})
}

func TestHandshakeSemServicoDeAgentesRecusaEmVezDeAceitarSemProva(t *testing.T) {
	// A instalação só é declarada concluída depois do `initialize` (D8). Sem o
	// serviço que sonda, o desfecho é recusar: um provider salvo que nunca sobe é
	// pior do que uma instalação que falhou.
	a := &App{}

	err := a.acpInstallHandshake(context.Background(), "node", []string{"index.js"}, nil)

	if err == nil {
		t.Fatal("declarou o agente conferido sem ter como conferi-lo")
	}
}

func TestACPInstallFailClosedSemSessao(t *testing.T) {
	a := &App{}
	a.acpCatalogOnce.Do(func() {
		a.acpCatalogSvc = &acpCatalog{installer: acpinstall.New(acpinstall.Config{})}
	})
	_, err := installAPI(a).ListInstalledACPAgents()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("want ErrUserScopeRequired, got %v", err)
	}
}
