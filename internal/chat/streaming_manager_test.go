package chat

import (
	"context"
	"testing"
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
