package terminal

import (
	"context"
	"testing"
)

func TestRunEphemeralNaoCriaSecaoPersistente(t *testing.T) {
	events := []string{}
	mgr := NewManager(DefaultManagerConfig(), func(event string, _ any) {
		events = append(events, event)
	})

	entry, err := mgr.RunEphemeral(context.Background(), "", "echo hello", 0, "test")
	if err != nil {
		t.Fatalf("RunEphemeral erro: %v", err)
	}
	if entry == nil || entry.Output == "" {
		t.Fatalf("entry vazia: %#v", entry)
	}
	if len(mgr.List()) != 0 {
		t.Fatalf("RunEphemeral não deve deixar sessão no pool, got %d", len(mgr.List()))
	}
	for _, ev := range events {
		if ev == "terminal:session_created" || ev == "terminal:session_closed" {
			t.Errorf("RunEphemeral não deve emitir %q, eventos: %v", ev, events)
		}
	}
	if mgr.Stats().TotalSessions != 0 {
		t.Errorf("Stats deve ser 0 após ephemeral, got %d", mgr.Stats().TotalSessions)
	}
}
