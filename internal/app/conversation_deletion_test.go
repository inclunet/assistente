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

func TestPrepareConversationRestorationLiberaTombstonesAposCommit(t *testing.T) {
	ctx := context.Background()
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	streamMgr := chat.NewStreamingManager(notifier)
	a := &App{streamMgr: streamMgr, responseNotifier: notifier}

	finalizeDelete, err := a.prepareConversationDeletion(ctx, []string{"restored"})
	if err != nil {
		t.Fatal(err)
	}
	finalizeDelete(true)

	finalizeRestore, err := a.prepareConversationRestoration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reserved := make(chan bool, 1)
	go func() {
		release, ok := streamMgr.ReserveConversation("restored")
		if ok {
			release()
		}
		reserved <- ok
	}()
	select {
	case <-reserved:
		t.Fatal("reserva atravessou gate de restauração antes do commit")
	case <-time.After(25 * time.Millisecond):
	}
	finalizeRestore([]string{" restored "})
	select {
	case ok := <-reserved:
		if !ok {
			t.Fatal("tombstone permaneceu após restauração")
		}
	case <-time.After(time.Second):
		t.Fatal("gate de restauração permaneceu preso")
	}
}
