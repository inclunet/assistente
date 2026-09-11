package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/messaging"
)

func TestPrepareConversationDeletionLiberaGatesAnterioresQuandoNotifierRecusa(t *testing.T) {
	notifier := messaging.NewResponseNotifier()
	notifier.Register("conversation-1", messaging.ResponseCallback{Callback: func(string, string) {}})
	streamMgr := chat.NewStreamingManager(notifier)
	a := &App{
		streamMgr:        streamMgr,
		responseNotifier: notifier,
	}

	if _, err := a.prepareConversationDeletion(context.Background(), []string{"conversation-1"}); !errors.Is(err, messaging.ErrConversationCallbackActive) {
		t.Fatalf("erro = %v, want %v", err, messaging.ErrConversationCallbackActive)
	}

	reserved := make(chan bool, 1)
	go func() {
		release, ok := streamMgr.ReserveConversation("conversation-1")
		if ok {
			release()
		}
		reserved <- ok
	}()
	select {
	case ok := <-reserved:
		if !ok {
			t.Fatal("falha do notifier marcou conversa como excluída")
		}
	case <-time.After(time.Second):
		t.Fatal("gate de streaming permaneceu preso após falha do notifier")
	}
}
