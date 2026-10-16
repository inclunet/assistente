package terminal

import (
	"context"
	"errors"
	"testing"
)

func closeTestManager(session *Session) *Manager {
	m := NewManager(DefaultManagerConfig(), nil)
	m.sessions[session.id] = session
	return m
}

func TestCaptureClosePrepareExecuteUsesExactSessionAndGeneration(t *testing.T) {
	session := &Session{id: "close-session", state: StateIdle}
	m := closeTestManager(session)

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
