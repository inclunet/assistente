package terminal

import (
	"context"
	"testing"
	"time"
)

func TestRunCommandDoesNotEmitStartForBusySession(t *testing.T) {
	var events []string
	manager := NewManager(DefaultManagerConfig(), func(event string, _ any) {
		events = append(events, event)
	})
	manager.sessions["term-busy"] = &Session{id: "term-busy", state: StateRunning}

	_, err := manager.RunCommand(context.Background(), "term-busy", "echo x", time.Second, "llm")
	if err == nil {
		t.Fatal("esperava recusa da sessão ocupada")
	}
	if len(events) != 0 {
		t.Fatalf("eventos emitidos para comando recusado: %v", events)
	}
}

func TestHasRejectsExitedSession(t *testing.T) {
	manager := NewManager(DefaultManagerConfig(), nil)
	manager.sessions["live"] = &Session{id: "live", state: StateIdle}
	manager.sessions["dead"] = &Session{id: "dead", state: StateExited}

	if !manager.Has("live") {
		t.Fatal("sessão live não foi encontrada")
	}
	if manager.Has("dead") {
		t.Fatal("sessão encerrada foi considerada viva")
	}
}
