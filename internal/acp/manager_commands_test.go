package acp

import (
	"context"
	"testing"
)

// managerComComandos monta um manager que guarda os eventos de comando e devolve
// o gancho que o transporte usa para contá-los. O gancho vem da configuração que
// o próprio manager leva ao transporte: é o que faz o teste passar pelo caminho
// real em vez de chamar o método interno na mão.
func managerComComandos(t *testing.T, client *fakeManagedClient) (*Manager, func() []SessionCommandsEvent, func(string, []Command)) {
	t.Helper()

	var eventos []SessionCommandsEvent
	var doTransporte func(string, []Command)

	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		OnSessionCommands: func(event SessionCommandsEvent) {
			eventos = append(eventos, event)
		},
		Dial: func(cfg Config, _ RequestHandler) (Client, error) {
			doTransporte = cfg.OnCommands
			return client, nil
		},
	})
	t.Cleanup(m.Shutdown)

	avisar := func(sessionID string, commands []Command) {
		if doTransporte == nil {
			t.Fatal("o manager não passou ao transporte quem escuta os comandos da sessão")
		}
		doTransporte(sessionID, commands)
	}
	return m, func() []SessionCommandsEvent { return eventos }, avisar
}

// O transporte só sabe o nome da sessão; quem sabe de que conversa ela é somos
// nós. Sem esse vínculo o menu de comandos de uma conversa receberia a lista de
// outra — ou de nenhuma.
func TestOsComandosDaSessaoChegamComAConversaDona(t *testing.T) {
	client := newFakeManagedClient()
	m, eventos, avisar := managerComComandos(t, client)

	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}

	avisar(conv.Session().ID(), []Command{{Name: "plan", Description: "Monta um plano", AcceptsInput: true}})

	got := eventos()
	if len(got) != 1 {
		t.Fatalf("eventos de comando = %d, quer 1", len(got))
	}
	if got[0].ConversationID != "conv-1" {
		t.Errorf("conversa do evento = %q", got[0].ConversationID)
	}
	if got[0].ProviderID != testSpec().ID {
		t.Errorf("provider do evento = %q", got[0].ProviderID)
	}
	if nomes := nomesDeComandos(got[0].Commands); !iguais(nomes, []string{"plan"}) {
		t.Errorf("comandos do evento = %v", nomes)
	}
}

// Sessão que não é de conversa nenhuma — a de descoberta, ou uma que a conversa
// já soltou — não tem a quem contar. Inventar uma conversa aqui poria comandos
// de um agente no menu de outra.
func TestComandosDeSessaoSemConversaNaoViramEvento(t *testing.T) {
	client := newFakeManagedClient()
	m, eventos, avisar := managerComComandos(t, client)

	// A conversa existe para o processo subir; o aviso é de outra sessão, que é
	// o caso da sessão de descoberta vivendo no mesmo processo.
	if _, err := m.Conversation(context.Background(), testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}
	avisar("sessao-de-descoberta", []Command{{Name: "plan"}})

	if got := eventos(); len(got) != 0 {
		t.Fatalf("evento de comando sem conversa dona: %+v", got)
	}
}

// A lista vazia é notícia, ao contrário do que vale para as opções: ela diz que
// o agente deixou de oferecer comandos, e calá-la deixaria no menu o que já não
// existe.
func TestListaVaziaDeComandosEhContada(t *testing.T) {
	client := newFakeManagedClient()
	m, eventos, avisar := managerComComandos(t, client)

	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}

	avisar(conv.Session().ID(), nil)

	got := eventos()
	if len(got) != 1 {
		t.Fatalf("eventos de comando = %d, quer 1", len(got))
	}
	if len(got[0].Commands) != 0 {
		t.Fatalf("a lista vazia chegou com comandos: %+v", got[0].Commands)
	}
}

// Perguntar os comandos de uma conversa que nunca falou com o agente não pode
// subir processo nenhum: abrir o menu da barra é gesto de digitação, e não
// pedido para pôr um agente de código de pé.
func TestPerguntarComandosNaoSobeOAgente(t *testing.T) {
	client := newFakeManagedClient()
	m, dials := managerWith(newMemoryStore(), client)

	if got := m.ConversationCommands("conversa-que-nunca-falou"); len(got) != 0 {
		t.Fatalf("comandos de conversa sem sessão = %+v", got)
	}
	if *dials != 0 {
		t.Fatalf("perguntar os comandos subiu o agente %d vez(es)", *dials)
	}
}

// A conversa com sessão de pé responde o que a sessão guardou: é isso que a tela
// lê ao abrir, sem depender de ter escutado o anúncio no momento certo.
func TestComandosDaConversaVemDaSessao(t *testing.T) {
	client := newFakeManagedClient()
	m, _ := managerWith(newMemoryStore(), client)

	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sess, ok := conv.Session().(*fakeManagedSession)
	if !ok {
		t.Fatalf("sessão inesperada: %T", conv.Session())
	}
	sess.commands = []Command{{Name: "revisar"}}

	if got := nomesDeComandos(m.ConversationCommands("conv-1")); !iguais(got, []string{"revisar"}) {
		t.Fatalf("comandos da conversa = %v", got)
	}
}
