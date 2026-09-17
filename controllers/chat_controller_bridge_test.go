package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/core/usecases"
	"assistente/internal/llm"
	"assistente/internal/messaging"
)

type bridgeConversationRepo struct {
	chat.ConversationRepository
}

func (bridgeConversationRepo) GetConversationInfo(context.Context, string) (*chat.Conversation, error) {
	return &chat.Conversation{Channel: "bridge-test", ContactID: "contact", UserID: "owner"}, nil
}

type bridgeMessenger struct {
	messaging.Messenger
	sent chan messaging.OutgoingMessage
}

func (*bridgeMessenger) SetHandler(messaging.IncomingMessageHandler) {}
func (m *bridgeMessenger) Send(_ context.Context, message messaging.OutgoingMessage) error {
	m.sent <- message
	return nil
}

type bridgeExecutor struct {
	request usecases.SendMessageRequest
	err     error
}

func (e *bridgeExecutor) Execute(req usecases.SendMessageRequest) (string, error) {
	e.request = req
	return req.ConversationID, e.err
}

func newBridgeController(t *testing.T) (*ChatController, *bridgeExecutor, *messaging.ResponseNotifier, <-chan messaging.OutgoingMessage) {
	t.Helper()
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	messenger := &bridgeMessenger{sent: make(chan messaging.OutgoingMessage, 2)}
	gateway := messaging.NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.Register("bridge-test", messenger)
	executor := &bridgeExecutor{}
	controller := &ChatController{
		convRepo: bridgeConversationRepo{}, msgGateway: gateway,
		responseNotifier: notifier, sendMsgUC: executor,
	}
	return controller, executor, notifier, messenger.sent
}

func invokeBridge(t *testing.T, controller *ChatController, retry bool) error {
	t.Helper()
	if retry {
		_, err := controller.RetryMessage(context.Background(), "conversation", "user-message", llm.ChatParams{})
		return err
	}
	_, err := controller.SendMessage(context.Background(), "conversation", "hello", "", llm.ChatParams{})
	return err
}

func TestChatBridgeFinishedRemovesPendingAndAllowsDeletion(t *testing.T) {
	for _, retry := range []bool{false, true} {
		name := "send"
		if retry {
			name = "retry"
		}
		t.Run(name, func(t *testing.T) {
			controller, executor, notifier, _ := newBridgeController(t)
			if err := invokeBridge(t, controller, retry); err != nil {
				t.Fatal(err)
			}
			if notifier.PendingCount() != 1 {
				t.Fatal("bridge deve permanecer até o término assíncrono")
			}
			if executor.request.OnFinished == nil {
				t.Fatal("OnFinished não foi encaminhado")
			}
			// Falha/cancelamento após aceitação: não houve Notify da resposta.
			executor.request.OnFinished()
			if notifier.PendingCount() != 0 {
				t.Fatal("bridge órfão após término")
			}
			release, err := notifier.PrepareConversationDeletion([]string{"conversation"})
			if err != nil {
				t.Fatalf("bridge encerrado bloqueou exclusão: %v", err)
			}
			release(false)
		})
	}
}

func TestChatBridgeLateFinishPreservesNewBridge(t *testing.T) {
	for _, retry := range []bool{false, true} {
		name := "send"
		if retry {
			name = "retry"
		}
		t.Run(name, func(t *testing.T) {
			controller, executor, notifier, sent := newBridgeController(t)
			if err := invokeBridge(t, controller, retry); err != nil {
				t.Fatal(err)
			}
			oldFinish := executor.request.OnFinished
			if oldFinish == nil {
				t.Fatal("OnFinished ausente")
			}
			if err := invokeBridge(t, controller, retry); err != nil {
				t.Fatal(err)
			}
			oldFinish()
			if notifier.PendingCount() != 1 {
				t.Fatal("cleanup antigo removeu o bridge novo")
			}
			notifier.Notify("conversation", "new response", "assistant")
			executor.request.OnFinished()
			select {
			case message := <-sent:
				if message.Text != "new response" || message.ChatID != "contact" {
					t.Fatalf("entrega incorreta: %+v", message)
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup impediu entrega normal do bridge")
			}
			if notifier.PendingCount() != 0 {
				t.Fatal("callback permaneceu após entrega")
			}
		})
	}
}

func TestChatBridgeSynchronousErrorCleansOnlyOwnTrace(t *testing.T) {
	for _, retry := range []bool{false, true} {
		name := "send"
		if retry {
			name = "retry"
		}
		t.Run(name, func(t *testing.T) {
			controller, executor, notifier, _ := newBridgeController(t)
			notifier.Register("conversation", messaging.ResponseCallback{
				Channel: "bridge-test", ChatID: "contact", OwnerUserID: "owner", TraceID: "gateway",
				Callback: func(string, string) { t.Error("callback de gateway não deve ser disparado") },
			})
			executor.err = errors.New("falha antes de Begin")
			if err := invokeBridge(t, controller, retry); !errors.Is(err, executor.err) {
				t.Fatalf("erro não propagado: %v", err)
			}
			if notifier.PendingCount() != 1 {
				t.Fatal("erro síncrono deve remover apenas o bridge próprio")
			}
			// Cleanup repetido continua sem atingir o callback do gateway.
			if executor.request.OnFinished == nil {
				t.Fatal("OnFinished ausente")
			}
			executor.request.OnFinished()
			if notifier.PendingCount() != 1 {
				t.Fatal("cleanup removeu callback de outro trace")
			}
			notifier.CancelTrace("conversation", "gateway")
			if notifier.PendingCount() != 0 {
				t.Fatal("callback preservado não corresponde ao gateway")
			}
		})
	}
}
