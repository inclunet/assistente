package terminal

import (
	"context"
	"io"
	"testing"
	"time"
)

type fakePTYSession struct {
	killCalls  int
	closeCalls int
}

func (f *fakePTYSession) PtyReader() io.Reader  { return nil }
func (f *fakePTYSession) PtyWriter() io.Writer  { return io.Discard }
func (f *fakePTYSession) Resize(int, int) error { return nil }
func (f *fakePTYSession) Wait() error           { return nil }
func (f *fakePTYSession) Kill() error           { f.killCalls++; return nil }
func (f *fakePTYSession) Close() error          { f.closeCalls++; return nil }
func (f *fakePTYSession) Pid() int              { return 1 }
func (f *fakePTYSession) CloseStdin() error     { return nil }

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
	manager.sessions["closing"] = &Session{id: "closing", state: StateClosing}
	manager.sessions["dead"] = &Session{id: "dead", state: StateExited}

	if !manager.Has("live") {
		t.Fatal("sessão live não foi encontrada")
	}
	if manager.Has("dead") {
		t.Fatal("sessão encerrada foi considerada viva")
	}
	if manager.Has("closing") {
		t.Fatal("sessão em encerramento foi considerada viva")
	}
}

func TestListOmitsClosingAndExitedSessions(t *testing.T) {
	manager := NewManager(DefaultManagerConfig(), nil)
	manager.sessions["live"] = &Session{id: "live", name: "Live", state: StateIdle}
	manager.sessions["closing"] = &Session{id: "closing", state: StateClosing}
	manager.sessions["dead"] = &Session{id: "dead", state: StateExited}

	sessions := manager.List()

	if len(sessions) != 1 || sessions[0].ID != "live" {
		t.Fatalf("sessões listadas = %#v", sessions)
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

func TestClosePTYReleasesNaturalExitOnlyOnce(t *testing.T) {
	ptySession := &fakePTYSession{}
	session := &Session{id: "term-natural-exit", ptySession: ptySession}

	session.closePTY(false)
	session.closePTY(false)

	if ptySession.closeCalls != 1 || ptySession.killCalls != 0 {
		t.Fatalf("close=%d kill=%d", ptySession.closeCalls, ptySession.killCalls)
	}
}

func TestFinishCommandPreservesClosingState(t *testing.T) {
	session := &Session{id: "term-closing", state: StateClosing}

	session.finishCommand()

	if got := session.State(); got != StateClosing {
		t.Fatalf("estado = %s, esperado closing", got.String())
	}
}

func TestCompleteCommandEntryMarksFailureWithoutResult(t *testing.T) {
	startedAt := time.Now()
	entry := &HistoryEntry{ID: "cmd-failed", StartedAt: startedAt}

	got := completeCommandEntry(entry, nil, context.DeadlineExceeded)

	if got.ExitCode != -1 {
		t.Fatalf("exitCode = %d, esperado -1", got.ExitCode)
	}
	if got.EndedAt.IsZero() || got.EndedAt.Before(startedAt) {
		t.Fatalf("endedAt inválido: %v", got.EndedAt)
	}
}
