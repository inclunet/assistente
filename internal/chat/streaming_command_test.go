package chat

import "testing"

func TestStreamingCommandCancelComparesGenerationAndAbsence(t *testing.T) {
	m := NewStreamingManager(nil)
	if generation, ok := m.CurrentGeneration("c"); ok || generation != 0 {
		t.Fatal("unexpected active generation")
	}
	if !m.CancelIfCurrent("c", 0) {
		t.Fatal("captured absence must succeed")
	}
	oldCancelled, newCancelled := false, false
	old := m.Register("c", func() { oldCancelled = true })
	if m.CancelIfCurrent("c", 0) || oldCancelled {
		t.Fatal("absence cancelled new stream")
	}
	current := m.Register("c", func() { newCancelled = true })
	if !oldCancelled || current == old {
		t.Fatal("registration generation")
	}
	if m.CancelIfCurrent("c", old) || newCancelled {
		t.Fatal("stale cancel affected current stream")
	}
	if !m.CancelIfCurrent("c", current) || !newCancelled {
		t.Fatal("current cancel failed")
	}
	if m.CancelIfCurrent("c", current) {
		t.Fatal("replayed generation accepted")
	}
}
