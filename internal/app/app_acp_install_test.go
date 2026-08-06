package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
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

func TestTipoDeProviderSemCorrespondenteNoCatalogo(t *testing.T) {
	// O mapeamento entre tipo de provider do app e `id` do registro existe
	// escrito num lugar só (D11), e tipo sem correspondente não é erro:
	// configurar comando e argumentos à mão continua sendo caminho válido.
	if id := acpinstall.RegistryIDForKind("claude-code"); id != "claude-acp" {
		t.Errorf("claude-code virou %q, queria claude-acp", id)
	}
	if id := acpinstall.RegistryIDForKind("cursor"); id != "cursor" {
		t.Errorf("cursor virou %q, queria cursor", id)
	}
	if id := acpinstall.RegistryIDForKind("openai"); id != "" {
		t.Errorf("um provedor HTTP virou o agente %q", id)
	}
}
