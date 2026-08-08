package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/acpregistry"
)

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

func TestPlanoQueNaoOfereceNadaAindaTrazAListaDeArgumentos(t *testing.T) {
	// O DTO promete `run_args` sempre presente para a tela não ter de distinguir
	// "sem argumentos" de "campo ausente". O plano que não oferece nada é
	// justamente onde o literal zerado mandaria `null`.
	a := &App{}
	// O Once é consumido aqui para o instalador de mentira não ser trocado pelo
	// de verdade, que consultaria o registro pela rede.
	a.acpCatalogOnce.Do(func() {
		a.acpCatalogSvc = &acpCatalog{installer: acpinstall.New(acpinstall.Config{})}
	})

	plano, err := a.acpInstallPlan(context.Background(), "nao-esta-no-catalogo")
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

	err := a.acpInstallHandshake(context.Background(), "node", []string{"index.js"})

	if err == nil {
		t.Fatal("declarou o agente conferido sem ter como conferi-lo")
	}
}

func TestPlanoTraduzidoNuncaEntregaListaAusente(t *testing.T) {
	// `null` faria a tela distinguir "sem argumentos" de "campo ausente" antes de
	// exibir o que será executado.
	dto := installPlanDTO(acpinstall.Plan{AgentID: "codex-acp", Name: "Codex"}, false)

	if dto.RunArgs == nil {
		t.Error("argumentos de execução vieram nulos")
	}
	if dto.Installed != nil {
		t.Error("disse que havia instalação onde não há")
	}
}

func TestPlanoTraduzidoLevaOEstadoDeJaInstaladoEODaInstalacaoEmVoo(t *testing.T) {
	quando := time.Date(2026, 8, 6, 15, 4, 5, 0, time.UTC)
	dto := installPlanDTO(acpinstall.Plan{
		AgentID:      "codex-acp",
		Name:         "Codex",
		Version:      "1.1.9",
		Distribution: acpinstall.DistributionNPM,
		Origin:       "@agentclientprotocol/codex-acp@1.1.9",
		Runtime:      acpinstall.RuntimeStatus{Name: acpinstall.RuntimeNode, Found: true, Path: "node"},
		Installed: &acpinstall.Installation{
			AgentID:     "codex-acp",
			Version:     "1.1.9",
			Command:     "node",
			InstalledAt: quando,
		},
	}, true)

	if dto.Installed == nil {
		t.Fatal("perdeu o estado de já instalado")
	}
	if dto.Installed.InstalledAt != "2026-08-06T15:04:05Z" {
		t.Errorf("data = %q, queria RFC 3339 para a tela formatar no idioma de quem lê", dto.Installed.InstalledAt)
	}
	if dto.Installed.Args == nil {
		t.Error("argumentos do comando instalado vieram nulos")
	}
	if !dto.Installing {
		t.Error("perdeu a instalação em voo, e então a tela não teria o que cancelar")
	}
}

func TestRuntimeAusenteChegaATelaComOndeSeProcurou(t *testing.T) {
	// Sem Node a instalação não é oferecida, e o motivo precisa ser verificável
	// (D7): "não encontrei" sem dizer onde se olhou não ajuda quem vai instalar.
	dto := runtimeStatusDTO(acp.NodeRuntime{Searched: []string{"C:\\Program Files\\nodejs\\node.exe"}})

	if dto.Found {
		t.Error("disse que achou o Node")
	}
	if dto.Name != acpinstall.RuntimeNode {
		t.Errorf("nome = %q, queria o do pré-requisito", dto.Name)
	}
	if len(dto.Searched) == 0 {
		t.Error("não disse onde procurou")
	}
}

func TestOPlanoDizSeOAppSabeProcurarOAgente(t *testing.T) {
	// A tela precisa saber disso antes de oferecer o botão de detectar: para 36
	// dos 38 agentes a resposta é conhecida de antemão, e um botão que só sabe
	// dizer "não sei procurar" é um convite a um clique inútil (D1).
	if _, ok := acpregistry.DetectableKind("cursor"); !ok {
		t.Error("o app deixou de saber procurar o Cursor")
	}
	if _, ok := acpregistry.DetectableKind("gemini-cli"); ok {
		t.Error("prometeu procurar um agente para o qual não há detecção escrita à mão")
	}
}
