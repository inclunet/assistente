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
	releaseReservation, reserved := manager.ReserveConversation("reserved")
	if !reserved {
		t.Fatal("reserva inicial recusada")
	}
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

	reserved := make(chan bool, 1)
	go func() {
		lateRelease, ok := manager.ReserveConversation("conversation-1")
		if ok {
			lateRelease()
		}
		reserved <- ok
	}()
	select {
	case <-reserved:
		t.Fatal("nova reserva atravessou gate de exclusão")
	case <-time.After(25 * time.Millisecond):
	}

	release(false)
	select {
	case ok := <-reserved:
		if !ok {
			t.Fatal("rollback deixou tombstone na conversa")
		}
	case <-time.After(time.Second):
		t.Fatal("nova reserva não prosseguiu após release")
	}
}

func TestStreamingManager_CommitRejeitaReservaBloqueada(t *testing.T) {
	manager := NewStreamingManager(nil)
	finalize, err := manager.PrepareConversationDeletion([]string{"conversation-1"})
	if err != nil {
		t.Fatalf("PrepareConversationDeletion: %v", err)
	}

	reserved := make(chan bool, 1)
	go func() {
		release, ok := manager.ReserveConversation("conversation-1")
		if ok {
			release()
		}
		reserved <- ok
	}()
	finalize(true)

	select {
	case ok := <-reserved:
		if ok {
			t.Fatal("reserva bloqueada atravessou commit da exclusão")
		}
	case <-time.After(time.Second):
		t.Fatal("reserva bloqueada não foi liberada após commit")
	}
	cancelled := false
	if generation := manager.Register("conversation-1", func() { cancelled = true }); generation != 0 {
		t.Fatalf("Register aceitou conversa excluída com geração %d", generation)
	}
	if !cancelled {
		t.Fatal("Register não cancelou contexto recusado da conversa excluída")
	}
}

func TestStreamingManager_TombstoneExpira(t *testing.T) {
	now := time.Now()
	manager := NewStreamingManager(nil)
	manager.now = func() time.Time { return now }
	finalize, err := manager.PrepareConversationDeletion([]string{" conversation-1 ", "conversation-1"})
	if err != nil {
		t.Fatal(err)
	}
	finalize(true)

	if _, ok := manager.ReserveConversation(" conversation-1 "); ok {
		t.Fatal("tombstone normalizado não bloqueou reserva")
	}
	now = now.Add(deletionTombstoneTTL + time.Second)
	release, ok := manager.ReserveConversation("conversation-1")
	if !ok {
		t.Fatal("tombstone expirado continuou bloqueando reserva")
	}
	release()
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
