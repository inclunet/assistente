package chat

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStreamingManager_LateCleanupDoesNotRemoveNewerTurn(t *testing.T) {
	manager := NewStreamingManager(nil)
	oldCancelled := false
	oldGeneration := manager.Register("conversation-1", func() { oldCancelled = true })
	newCancelled := false
	manager.Register("conversation-1", func() { newCancelled = true })

	if !oldCancelled {
		t.Fatal("registering a newer turn should cancel the previous one")
	}
	if manager.UnregisterIfCurrent("conversation-1", oldGeneration) {
		t.Fatal("late cleanup unexpectedly removed the newer turn")
	}

	manager.Cancel("conversation-1")
	if !newCancelled {
		t.Fatal("newer turn was no longer cancellable after late cleanup")
	}
}

func TestStreamingManager_PrepareDeletionFalhaSemCancelarTrabalhoAtivo(t *testing.T) {
	manager := NewStreamingManager(nil)
	releaseReservation := manager.ReserveConversation("reserved")
	t.Cleanup(releaseReservation)

	if _, err := manager.PrepareConversationDeletion([]string{"reserved"}); !errors.Is(err, ErrConversationActive) {
		t.Fatalf("reserva ativa: erro = %v, want %v", err, ErrConversationActive)
	}

	cancelled := false
	manager.Register("streaming", func() { cancelled = true })
	if _, err := manager.PrepareConversationDeletion([]string{"streaming"}); !errors.Is(err, ErrConversationActive) {
		t.Fatalf("stream ativo: erro = %v, want %v", err, ErrConversationActive)
	}
	if cancelled {
		t.Fatal("preparo da exclusão cancelou stream ativo")
	}
}

func TestStreamingManager_PrepareDeletionBloqueiaNovaReservaAteRelease(t *testing.T) {
	manager := NewStreamingManager(nil)
	release, err := manager.PrepareConversationDeletion([]string{"conversation-1"})
	if err != nil {
		t.Fatalf("PrepareConversationDeletion: %v", err)
	}

	reserved := make(chan func(), 1)
	go func() {
		reserved <- manager.ReserveConversation("conversation-1")
	}()
	select {
	case lateRelease := <-reserved:
		lateRelease()
		t.Fatal("nova reserva atravessou gate de exclusão")
	case <-time.After(25 * time.Millisecond):
	}

	release()
	select {
	case lateRelease := <-reserved:
		lateRelease()
	case <-time.After(time.Second):
		t.Fatal("nova reserva não prosseguiu após release")
	}
}

func TestStreamingManager_CurrentGenerationCanUnregister(t *testing.T) {
	manager := NewStreamingManager(nil)
	generation := manager.Register("conversation-1", func() {})

	if !manager.UnregisterIfCurrent("conversation-1", generation) {
		t.Fatal("current generation was not unregistered")
	}

	found := false
	manager.Mu(func(contexts map[string]context.CancelFunc) {
		_, found = contexts["conversation-1"]
	})
	if found {
		t.Fatal("current context remains registered")
	}
}
