package app

import (
	"context"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/apidto"
	"assistente/internal/wailsapi"
)

// anunciaComandos é o agente contando quais comandos a sessão oferece, que é o
// que ele faz assim que ela abre.
func anunciaComandos(a *agenteFalso, commands ...acp.Command) {
	a.mu.Lock()
	aviso := a.anunciaCmd
	a.comandos = append([]acp.Command(nil), commands...)
	a.mu.Unlock()
	if aviso == nil {
		return
	}
	aviso("sessao-1", commands)
}

func nomesDeComandos(commands []apidto.AgentCommand) []string {
	nomes := make([]string, 0, len(commands))
	for _, command := range commands {
		nomes = append(nomes, command.Name)
	}
	return nomes
}

func commandsAPI(a *App) *wailsapi.ACPCommands {
	api := wailsapi.NewACPCommands()
	wailsapi.AttachACPCommands(api, wailsSession{app: a}, a.acpMgr)
	return api
}

// Quem abre uma conversa que já estava conversando precisa ver os comandos que
// existem agora, e não esperar o próximo anúncio: a lista chega quando a sessão
// abre, muito antes de alguém digitar a barra.
func TestComandosDaConversaSaoConsultaveis(t *testing.T) {
	agente := novoAgenteFalso()
	a, _ := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")
	anunciaComandos(agente, acp.Command{Name: "plan", Description: "Monta um plano", AcceptsInput: true})

	out, err := commandsAPI(a).GetAgentSessionCommands("conversa-1")
	if err != nil {
		t.Fatalf("GetAgentSessionCommands: %v", err)
	}
	if out.ConversationID != "conversa-1" {
		t.Errorf("conversa da resposta = %q", out.ConversationID)
	}
	if len(out.Commands) != 1 {
		t.Fatalf("comandos = %+v", out.Commands)
	}
	if out.Commands[0].Name != "plan" || out.Commands[0].Description != "Monta um plano" {
		t.Errorf("comando devolvido = %+v", out.Commands[0])
	}
	if !out.Commands[0].AcceptsInput {
		t.Error("o comando que aceita argumento chegou à tela sem essa marca")
	}
}

// Conversa que ainda não falou com o agente não tem comando nenhum, e perguntar
// não pode subir um processo: abrir o menu é gesto de digitação.
func TestConversaSemSessaoNaoTemComandosNemSobeAgente(t *testing.T) {
	agente := novoAgenteFalso()
	a, _ := appComAgente(t, agente)

	out, err := commandsAPI(a).GetAgentSessionCommands("conversa-que-nunca-falou")
	if err != nil {
		t.Fatalf("GetAgentSessionCommands: %v", err)
	}
	if len(out.Commands) != 0 {
		t.Fatalf("comandos de conversa sem sessão = %+v", out.Commands)
	}
	if len(agente.sessoes) != 0 {
		t.Fatalf("perguntar os comandos abriu %d sessão(ões)", len(agente.sessoes))
	}
}

// O anúncio do agente vira evento de tela, com a conversa dona junto: sem ela o
// menu de uma conversa mostraria os comandos de outra.
func TestOAnuncioDeComandosViraEventoDaConversa(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	anunciaComandos(agente, acp.Command{Name: "plan"}, acp.Command{Name: "resumir"})

	eventos := emissor.find("chat:agent_commands")
	if len(eventos) != 1 {
		t.Fatalf("eventos de comando = %d, esperado 1", len(eventos))
	}
	evento, ok := eventos[0].data.(AgentSessionCommandsEvent)
	if !ok {
		t.Fatalf("payload inesperado: %T", eventos[0].data)
	}
	if evento.ConversationID != "conversa-1" {
		t.Errorf("o evento não disse de que conversa é: %q", evento.ConversationID)
	}
	if got := nomesDeComandos(evento.Commands); len(got) != 2 || got[0] != "plan" || got[1] != "resumir" {
		t.Errorf("comandos do evento = %v", got)
	}
}

// A lista vazia é emitida: ela é como os comandos de antes saem do menu. Calá-la
// deixaria na tela algo que o agente já não aceita.
func TestListaVaziaDeComandosChegaATela(t *testing.T) {
	agente := novoAgenteFalso()
	a, emissor := appComAgente(t, agente)
	conversaComSessao(t, a, "conversa-1")

	anunciaComandos(agente)

	eventos := emissor.find("chat:agent_commands")
	if len(eventos) != 1 {
		t.Fatalf("eventos de comando = %d, esperado 1", len(eventos))
	}
	evento := eventos[0].data.(AgentSessionCommandsEvent)
	if len(evento.Commands) != 0 {
		t.Fatalf("a lista vazia chegou com comandos: %+v", evento.Commands)
	}
}

func TestComandosExigemSessaoAutenticada(t *testing.T) {
	a := &App{ctx: context.Background()}
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	api := wailsapi.NewACPCommands()
	wailsapi.AttachACPCommands(api, wailsSession{app: a}, mgr)

	if _, err := api.GetAgentSessionCommands("conversa-1"); err == nil {
		t.Fatal("GetAgentSessionCommands sem sessão autenticada deveria falhar")
	}
}
