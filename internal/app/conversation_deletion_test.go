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

	reserved := make(chan func(), 1)
	go func() {
		reserved <- streamMgr.ReserveConversation("conversation-1")
	}()
	select {
	case release := <-reserved:
		release()
	case <-time.After(time.Second):
		t.Fatal("gate de streaming permaneceu preso após falha do notifier")
	}
}
