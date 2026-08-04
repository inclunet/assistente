package agent

import (
	"context"
	"errors"
	"testing"
)

type pedidoDeRenome struct {
	conversationID string
	turnMessageID  string
	title          string
}

// handlerComRenome monta o handler pelo mesmo construtor que a produção usa, para
// que os identificadores do turno cheguem ao renomeador do jeito que chegam lá.
func handlerComRenome(t *testing.T, erro error, pedidos *[]pedidoDeRenome) *SimpleStreamHandler {
	t.Helper()
	svc := NewService(ServiceConfig{
		Emitter: &mockEmitter{},
		MsgRepo: &mockMsgRepo{},
		RenameFromAgent: func(_ context.Context, conversationID, turnMessageID, title string) error {
			*pedidos = append(*pedidos, pedidoDeRenome{conversationID, turnMessageID, title})
			return erro
		},
	})
	handler, err := svc.NewSimpleStreamHandler(context.Background(), "conversa-1", "turno-1", "perfil", nil)
	if err != nil {
		t.Fatalf("NewSimpleStreamHandler: %v", err)
	}
	return handler
}

// O título do agente chega ao renomeador com a conversa e a mensagem do turno:
// é por essa mensagem que se reconhece o rótulo provisório que o app escreveu, e
// sem ela o título do agente nunca substituiria o recorte da primeira frase.
func TestTituloDoAgenteChegaComAConversaEOTurno(t *testing.T) {
	var pedidos []pedidoDeRenome
	handler := handlerComRenome(t, nil, &pedidos)

	handler.OnAgentTitle("Corrigir o teste de anexos")

	if len(pedidos) != 1 {
		t.Fatalf("pedidos de renome = %d, quer 1", len(pedidos))
	}
	pedido := pedidos[0]
	if pedido.conversationID != "conversa-1" {
		t.Errorf("conversa = %q", pedido.conversationID)
	}
	if pedido.turnMessageID != "turno-1" {
		t.Errorf("mensagem do turno = %q", pedido.turnMessageID)
	}
	if pedido.title != "Corrigir o teste de anexos" {
		t.Errorf("título = %q", pedido.title)
	}
}

// Não renomear é perder uma conveniência, não o turno: a resposta do agente já
// está salva, e derrubar o handler por causa do rótulo custaria a conversa.
func TestFalhaAoRenomearNaoDerrubaOTurno(t *testing.T) {
	var pedidos []pedidoDeRenome
	handler := handlerComRenome(t, errors.New("banco indisponível"), &pedidos)

	handler.OnAgentTitle("Corrigir o teste")

	if len(pedidos) != 1 {
		t.Fatalf("pedidos de renome = %d, quer 1", len(pedidos))
	}
}

// Título vazio não vira pedido nenhum: renomear para nada deixaria a aba sem
// rótulo para o leitor de telas anunciar.
func TestTituloVazioNaoPedeRenome(t *testing.T) {
	var pedidos []pedidoDeRenome
	handler := handlerComRenome(t, nil, &pedidos)

	handler.OnAgentTitle("   ")

	if len(pedidos) != 0 {
		t.Fatalf("pediu renome com título vazio: %+v", pedidos)
	}
}

// Sem renomeador configurado o handler simplesmente ignora o título, em vez de
// quebrar: o app roda com provedores que nunca mandam nome de sessão.
func TestSemRenomeadorOTituloEhIgnorado(t *testing.T) {
	svc := NewService(ServiceConfig{Emitter: &mockEmitter{}, MsgRepo: &mockMsgRepo{}})
	handler, err := svc.NewSimpleStreamHandler(context.Background(), "conversa-1", "turno-1", "perfil", nil)
	if err != nil {
		t.Fatalf("NewSimpleStreamHandler: %v", err)
	}

	handler.OnAgentTitle("Corrigir o teste")
}
