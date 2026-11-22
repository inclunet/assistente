package terminal

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func closeTestManager(session *Session) *Manager {
	m := NewManager(DefaultManagerConfig(), nil)
	m.sessions[session.id] = session
	return m
}

func TestCaptureClosePrepareExecuteUsesExactSessionAndGeneration(t *testing.T) {
	session := &Session{id: "close-session", state: StateIdle}
	m := closeTestManager(session)
	closed := 0
	m.emitEvent = func(event string, data any) {
		payload, ok := data.(map[string]string)
		if event != "terminal:session_closed" || !ok || payload["sessionId"] != session.id || m.Has(session.id) {
			t.Fatalf("evento de fechamento inesperado: %s %v", event, data)
		}
		closed++
	}

	snapshot, err := m.CaptureClose(session.id)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := m.PrepareClose(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if session.State() != StateExited || m.Has(session.id) {
		t.Fatalf("sessão não encerrada: state=%s has=%v", session.State(), m.Has(session.id))
	}
	if _, err := m.PrepareClose(snapshot); !errors.Is(err, ErrCloseStale) {
		t.Fatalf("captura reutilizada aceita: %v", err)
	}
	if err := operation.Execute(t.Context()); !errors.Is(err, ErrCloseStale) || closed != 1 {
		t.Fatalf("reexecução=%v eventos de fechamento=%d", err, closed)
	}
}

func TestPrepareCloseRejectsChangedManagedGenerationWithoutEffect(t *testing.T) {
	session := &Session{id: "close-stale", state: StateIdle}
	m := closeTestManager(session)
	snapshot, err := m.CaptureClose(session.id)
	if err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	session.commandGeneration++
	session.mu.Unlock()
	if _, err := m.PrepareClose(snapshot); !errors.Is(err, ErrCloseStale) {
		t.Fatalf("geração alterada aceita: %v", err)
	}
	if session.State() != StateIdle || !m.Has(session.id) {
		t.Fatalf("stale alterou lifecycle: state=%s has=%v", session.State(), m.Has(session.id))
	}
}

func TestCloseOperationCancelPreservesSession(t *testing.T) {
	session := &Session{id: "close-cancel", state: StateIdle}
	m := closeTestManager(session)
	snapshot, err := m.CaptureClose(session.id)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := m.PrepareClose(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	operation.Cancel()
	if err := operation.Execute(context.Background()); !errors.Is(err, ErrCloseStale) {
		t.Fatalf("operação cancelada executou: %v", err)
	}
	if session.State() != StateIdle || !m.Has(session.id) {
		t.Fatalf("cancel alterou lifecycle: state=%s has=%v", session.State(), m.Has(session.id))
	}
}

func TestCloseOperationExecuteWithDetachedContextContinuesAfterCancellation(t *testing.T) {
	session := &Session{id: "close-detached-context", state: StateIdle}
	m := closeTestManager(session)
	snapshot, err := m.CaptureClose(session.id)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := m.PrepareClose(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// O owner do teardown usa um contexto detached depois do CAS do vínculo;
	// o cancelamento do caller não pode abandonar a sessão viva.
	if err := operation.Execute(context.WithoutCancel(ctx)); err != nil {
		t.Fatalf("teardown abandonado após cancelamento pós-unbind: %v", err)
	}
	if session.State() != StateExited || m.Has(session.id) {
		t.Fatalf("teardown pós-unbind incompleto: state=%s has=%v", session.State(), m.Has(session.id))
	}
}

func TestSessionClosedFollowsCommandEnd(t *testing.T) {
	for _, route := range []string{"capturado", "direto"} {
		t.Run(route, func(t *testing.T) {
			s := &Session{id: "terminal", state: StateIdle}
			m := closeTestManager(s)
			var events []string
			m.emitEvent = func(event string, _ any) { events = append(events, event) }
			s.onCommandStart = func(_, _, _, _ string) {
				if route == "direto" {
					if err := m.Close(s.id); err != nil {
						t.Fatal(err)
					}
				} else {
					snapshot, err := m.CaptureClose(s.id)
					if err != nil {
						t.Fatal(err)
					}
					op, err := m.PrepareClose(snapshot)
					if err != nil {
						t.Fatal(err)
					}
					if err := op.Execute(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				if len(events) != 0 || m.Has(s.id) {
					t.Fatalf("teardown antes do resultado: eventos=%v sessão viva=%t", events, m.Has(s.id))
				}
			}
			entry, err := m.RunCommand(t.Context(), s.id, "comando", time.Second, "test")
			if !errors.Is(err, ErrSessionClosed) || entry == nil || entry.ExitCode != -1 {
				t.Fatalf("entrada=%+v erro=%v", entry, err)
			}
			if !slices.Equal(events, []string{"terminal:command_end", "terminal:session_closed"}) {
				t.Fatalf("sequência recebida: %v", events)
			}
		})
	}
}
