package acp

import (
	"context"
	"testing"
)

func TestOTurnoDizAQuemOAgenteDevePerguntar(t *testing.T) {
	m, _ := managerWith(newMemoryStore(), newFakeManagedClient())
	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessionID := conv.Session().ID()

	fim := conv.BeginTurn(true)

	owner, ok := m.TurnOwnerOf(sessionID)
	if !ok {
		t.Fatal("o pedido do agente não encontrou o turno em voo")
	}
	if owner.ConversationID != "conv-1" || !owner.Interactive {
		t.Errorf("dono do turno = %+v, quer a conversa 1 com alguém esperando", owner)
	}

	fim()
	// Fora do turno não há quem espere: um pedido que chegue agora não pode
	// abrir diálogo nenhum.
	if _, ok := m.TurnOwnerOf(sessionID); ok {
		t.Error("o turno acabou e ainda consta alguém esperando por ele")
	}
}

func TestTurnoSemGenteNaTelaContinuaSendoDaConversa(t *testing.T) {
	m, _ := managerWith(newMemoryStore(), newFakeManagedClient())
	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	defer conv.BeginTurn(false)()

	owner, ok := m.TurnOwnerOf(conv.Session().ID())
	if !ok {
		t.Fatal("o pedido do agente não encontrou o turno em voo")
	}
	if owner.Interactive {
		t.Error("turno sem superfície apareceu como se tivesse quem respondesse")
	}
	if owner.ConversationID != "conv-1" {
		// Mesmo negando, o registro serve para dizer de qual conversa foi.
		t.Errorf("conversa do turno = %q, quer conv-1", owner.ConversationID)
	}
}

func TestTurnoQueSaiNaoApagaODonoDoQueEntrou(t *testing.T) {
	m, _ := managerWith(newMemoryStore(), newFakeManagedClient())
	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	sessionID := conv.Session().ID()

	// Barge-in: a mensagem nova cancela o turno anterior, e por um instante os
	// dois convivem na mesma sessão.
	fimDoAnterior := conv.BeginTurn(false)
	fimDoNovo := conv.BeginTurn(true)
	defer fimDoNovo()

	fimDoAnterior()

	owner, ok := m.TurnOwnerOf(sessionID)
	if !ok {
		t.Fatal("o turno que saiu levou junto o registro do que entrou")
	}
	if !owner.Interactive {
		t.Error("o dono do turno em voo virou o do turno que estava terminando")
	}
}

func TestSessaoSemTurnoNaoTemAQuemPerguntar(t *testing.T) {
	m, _ := managerWith(newMemoryStore(), newFakeManagedClient())
	if _, ok := m.TurnOwnerOf("sessao-que-nunca-teve-turno"); ok {
		t.Error("apareceu alguém esperando por uma sessão sem turno")
	}
}
