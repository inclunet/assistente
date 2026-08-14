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

func TestCloseIsIdempotentWithoutPTY(t *testing.T) {
	exitEvents := 0
	session := &Session{
		id:    "term-close",
		state: StateIdle,
		onExit: func(string, error) {
			exitEvents++
		},
	}

	if err := session.Close(); err != nil {
		t.Fatalf("primeiro Close: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("segundo Close: %v", err)
	}
	if got := session.State(); got != StateExited {
		t.Fatalf("estado = %s, esperado exited", got.String())
	}
	if exitEvents != 0 {
		t.Fatalf("fechamento explícito emitiu %d eventos exited", exitEvents)
	}
}

func TestFinishCommandPreservesClosingState(t *testing.T) {
	session := &Session{id: "term-closing", state: StateClosing}

	session.finishCommand()

	if got := session.State(); got != StateClosing {
		t.Fatalf("estado = %s, esperado closing", got.String())
	}
}
