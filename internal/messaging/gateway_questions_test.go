package messaging

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/llm"
	"assistente/internal/questionnaire"
)

// TestGateway_RespostaDePerguntaPendenteNaoViraTurno prova que a pergunta em
// canal usa o caminho de mensagem que já existe nas duas pontas (AEP-0040): ela
// sai pelo mensageiro registrado e a resposta entra por handleIncoming, o único
// ponto de entrada de mensagem de canal. A mensagem que decide não vira turno —
// mandá-la ao modelo cancelaria (barge-in) justamente o turno que espera.
func TestGateway_RespostaDePerguntaPendenteNaoViraTurno(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()
	defer notifier.Stop()

	var mu sync.Mutex
	var turnos []string
	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 4)}
	gateway := NewGateway(notifier, func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		mu.Lock()
		turnos = append(turnos, content)
		mu.Unlock()
		go notifier.NotifyContext(ctx, conversationID, "ok", "asst-1")
		return conversationID, nil
	}, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	// Primeira mensagem: turno comum, que também cria a conversa do contato.
	gateway.handleIncoming(context.Background(), IncomingMessage{
		ID: "msg-1", Channel: "telegram",
		From: Contact{ID: "123", DisplayName: "Fulano", Username: "user"},
		Text: "Oi",
	})
	esperarEnvio(t, fake)

	mu.Lock()
	turnosAntes := len(turnos)
	mu.Unlock()
	if turnosAntes != 1 {
		t.Fatalf("turnos = %d, quer 1", turnosAntes)
	}

	cfg, err := channels.Load("telegram")
	if err != nil || cfg == nil {
		t.Fatalf("erro ao carregar o canal: %v", err)
	}
	conversationID := cfg.GetConversationID("123")
	if conversationID == "" {
		t.Fatal("a conversa do contato não foi criada")
	}

	// O backend pergunta pela superfície de origem da conversa.
	pronto := make(chan questionnaire.Response, 1)
	falhou := make(chan error, 1)
	go func() {
		resp, err := gateway.ChannelQuestions().AskOnChannel(
			context.Background(),
			questionnaire.ChannelSurface(conversationID, "telegram", "123"),
			questionnaire.RequestPayload{
				Title:   questionnaire.Plain("O agente pede permissão"),
				Timeout: 5 * time.Second,
				Questions: []questionnaire.Question{{
					ID:       "decision",
					Type:     "single_choice",
					Prompt:   questionnaire.Plain("O que ele pode fazer?"),
					Options:  questionnaire.PlainTexts([]string{"Permitir uma vez", "Negar"}),
					Required: true,
				}},
			},
		)
		if err != nil {
			falhou <- err
			return
		}
		pronto <- resp
	}()

	pergunta := esperarEnvio(t, fake)
	if pergunta.ChatID != "123" || !strings.Contains(pergunta.Text, "1 - Permitir uma vez") {
		t.Fatalf("a pergunta não saiu pelo mensageiro do canal: %+v", pergunta)
	}

	// A resposta entra pelo mesmo caminho de qualquer mensagem do contato.
	gateway.handleIncoming(context.Background(), IncomingMessage{
		ID: "msg-2", Channel: "telegram",
		From: Contact{ID: "123", DisplayName: "Fulano", Username: "user"},
		Text: "1",
	})

	select {
	case resp := <-pronto:
		if resp.Answers["decision"] != "Permitir uma vez" {
			t.Errorf("resposta = %v, quer a escolha do contato", resp.Answers)
		}
	case err := <-falhou:
		t.Fatalf("a pergunta no canal falhou: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("a decisão do contato não chegou a quem perguntou")
	}

	mu.Lock()
	turnosDepois := len(turnos)
	mu.Unlock()
	if turnosDepois != turnosAntes {
		t.Errorf("turnos = %d, quer %d: a mensagem que decide não é turno novo", turnosDepois, turnosAntes)
	}
}

func esperarEnvio(tb testing.TB, fake *fakeMessenger) OutgoingMessage {
	tb.Helper()
	select {
	case msg := <-fake.sentCh:
		return msg
	case <-time.After(3 * time.Second):
		tb.Fatal("nada foi enviado ao canal")
		return OutgoingMessage{}
	}
}
