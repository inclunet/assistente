package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type interruptFakePTYWriter struct {
	mu      sync.Mutex
	writes  [][]byte
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *interruptFakePTYWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes = append(w.writes, append([]byte(nil), p...))
	w.mu.Unlock()
	if w.started != nil {
		w.once.Do(func() { close(w.started) })
	}
	if w.release != nil {
		<-w.release
	}
	return len(p), nil
}

func (w *interruptFakePTYWriter) snapshot() [][]byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([][]byte, len(w.writes))
	for i := range w.writes {
		result[i] = append([]byte(nil), w.writes[i]...)
	}
	return result
}

func newInterruptFakeManager(writer io.Writer) (*Manager, *Session) {
	manager := NewManager(DefaultManagerConfig(), nil)
	session := &Session{id: "interrupt-fake", state: StateIdle, managedCommandID: "command-original", ptyWriter: writer}
	manager.sessions[session.id] = session
	return manager, session
}

func TestInterruptSnapshotCommitsOnlyOnceWithoutTerminalPayload(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, _ := newInterruptFakeManager(writer)

	snapshot, err := manager.CaptureInterrupt("interrupt-fake")
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}
	if encoded, err := json.Marshal(snapshot); err != nil { //nolint:staticcheck // Prova que o snapshot opaco não expõe payload serializável.
		t.Fatalf("snapshot JSON: %v", err)
	} else if !bytes.Equal(encoded, []byte("{}")) {
		t.Fatalf("snapshot expôs payload serializável: %s", encoded)
	}
	if !snapshot.MatchesTarget("interrupt-fake", "command-original") || snapshot.MatchesTarget("other", "command-original") || snapshot.MatchesTarget("interrupt-fake", "cmd-1") {
		t.Fatal("MatchesTarget não respeitou sessão e commandID capturados")
	}

	operation, err := manager.PrepareInterrupt(snapshot)
	if err != nil {
		t.Fatalf("PrepareInterrupt: %v", err)
	}
	if err := operation.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := manager.PrepareInterrupt(snapshot); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("replay = %v, esperado ErrInterruptStale", err)
	}

	writes := writer.snapshot()
	if len(writes) != 1 || !bytes.Equal(writes[0], []byte{0x03}) {
		t.Fatalf("writes = %#v, esperado um Ctrl+C", writes)
	}
}

func TestInterruptSnapshotRejectsNewCommandGeneration(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)

	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}
	if err := session.beginCommandWithID("command-next"); err != nil {
		t.Fatalf("beginCommandWithID: %v", err)
	}
	session.finishCommand()

	if _, err := manager.PrepareInterrupt(snapshot); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("commit após novo comando = %v, esperado ErrInterruptStale", err)
	}
	if writes := writer.snapshot(); len(writes) != 0 {
		t.Fatalf("efeito ocorreu apesar da captura stale: %#v", writes)
	}
}

func TestCaptureInterruptRejectsPendingWrite(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)
	if err := session.beginCommandWithID("cmd-pending"); err != nil {
		t.Fatalf("beginCommandWithID: %v", err)
	}

	if _, err := manager.CaptureInterrupt(session.id); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("captura durante pendingWrite = %v, esperado ErrInterruptStale", err)
	}
	session.finishCommand()
}

func TestCaptureInterruptRejectsSessionWithoutManagedCommandID(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)
	session.mu.Lock()
	session.managedCommandID = ""
	session.mu.Unlock()

	if _, err := manager.CaptureInterrupt(session.id); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("captura sem managedCommandID = %v, esperado ErrInterruptStale", err)
	}
}

func TestSendInputPreservesManagedCommandIDWhenIdle(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)
	entry, err := session.SendInput("raw input", "")
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}
	if !snapshot.MatchesTarget(session.id, entry.ID) || snapshot.MatchesTarget(session.id, "") {
		t.Fatalf("snapshot não preservou último commandID raw: entry=%q", entry.ID)
	}
}

func TestInterruptSnapshotSerializesBeginCommandBehindCommit(t *testing.T) {
	writer := &interruptFakePTYWriter{started: make(chan struct{}), release: make(chan struct{})}
	manager, session := newInterruptFakeManager(writer)
	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}

	operation, err := manager.PrepareInterrupt(snapshot)
	if err != nil {
		t.Fatalf("PrepareInterrupt: %v", err)
	}
	commitDone := make(chan error, 1)
	go func() { commitDone <- operation.Execute(context.Background()) }()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("fake PTY não recebeu Ctrl+C")
	}

	beginDone := make(chan error, 1)
	go func() { beginDone <- session.beginCommandWithID("command-next") }()
	select {
	case err := <-beginDone:
		t.Fatalf("beginCommandWithID atravessou o commit: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(writer.release)
	if err := <-commitDone; err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if err := <-beginDone; err != nil {
		t.Fatalf("beginCommand após commit: %v", err)
	}
	session.finishCommand()
}

func TestPrepareInterruptHandoffReleasesAppGuardsAndReservesSession(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)
	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}

	var authMu, workspaceMu sync.RWMutex
	authMu.RLock()
	workspaceMu.RLock()
	operation, err := manager.PrepareInterrupt(snapshot)
	if err != nil {
		workspaceMu.RUnlock()
		authMu.RUnlock()
		t.Fatalf("PrepareInterrupt: %v", err)
	}
	workspaceMu.RUnlock()
	authMu.RUnlock()

	authReleased := make(chan struct{})
	go func() {
		authMu.Lock()
		close(authReleased)
		authMu.Unlock()
	}()
	select {
	case <-authReleased:
	case <-time.After(time.Second):
		t.Fatal("handoff reteve auth guard após PrepareInterrupt")
	}

	beginDone := make(chan error, 1)
	go func() { beginDone <- session.beginCommandWithID("command-next") }()
	select {
	case err := <-beginDone:
		t.Fatalf("novo comando atravessou a reserva: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	operation.Cancel()
	if err := <-beginDone; err != nil {
		t.Fatalf("novo comando após Cancel: %v", err)
	}
	session.finishCommand()

	snapshot, err = manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt após novo comando: %v", err)
	}
	operation, err = manager.PrepareInterrupt(snapshot)
	if err != nil {
		t.Fatalf("PrepareInterrupt para Close: %v", err)
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- session.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close atravessou a reserva: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	operation.Cancel()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close após Cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close não foi liberado pelo Cancel")
	}
}

func TestInterruptOperationCanceledContextIsStaleBeforeWrite(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)
	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}
	operation, err := manager.PrepareInterrupt(snapshot)
	if err != nil {
		t.Fatalf("PrepareInterrupt: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = operation.Execute(ctx)
	if err == nil || !errors.Is(err, ErrInterruptStale) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute cancelado antes do write = %v", err)
	}
	if writes := writer.snapshot(); len(writes) != 0 {
		t.Fatalf("write ocorreu após cancelamento prévio: %#v", writes)
	}
	if err := session.beginCommandWithID("command-after-cancel"); err != nil {
		t.Fatalf("sessão permaneceu reservada: %v", err)
	}
	session.finishCommand()
}

func TestInterruptOperationZeroValueAndBusyAdmission(t *testing.T) {
	var zero InterruptOperation
	zero.Cancel()
	if err := zero.Execute(context.Background()); !errors.Is(err, ErrInterruptStale) {
		t.Fatal(err)
	}
	manager, session := newInterruptFakeManager(&interruptFakePTYWriter{})
	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := manager.PrepareInterrupt(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer operation.Cancel()
	// The manager remains available throughout the reserved session handoff.
	if !manager.mu.TryLock() {
		t.Fatal("manager registry remains locked")
	}
	manager.mu.Unlock()
	other, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PrepareInterrupt(other); !errors.Is(err, ErrInterruptStale) {
		t.Fatal("busy session admitted", err)
	}
	operation.Cancel()
	if err := operation.Execute(context.Background()); !errors.Is(err, ErrInterruptStale) {
		t.Fatal("cancelled handoff executed", err)
	}
	if _, err := manager.PrepareInterrupt(snapshot); !errors.Is(err, ErrInterruptStale) {
		t.Fatal("consumed snapshot admitted", err)
	}
}

func TestInterruptSnapshotRejectsClosedSession(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)
	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}

	session.mu.Lock()
	session.state = StateClosing
	session.mu.Unlock()

	if _, err := manager.PrepareInterrupt(snapshot); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("commit de sessão fechada = %v, esperado ErrInterruptStale", err)
	}
	if writes := writer.snapshot(); len(writes) != 0 {
		t.Fatalf("efeito ocorreu em sessão fechada: %#v", writes)
	}
}

func TestInterruptSnapshotRejectsRecreatedSession(t *testing.T) {
	oldWriter := &interruptFakePTYWriter{}
	manager, oldSession := newInterruptFakeManager(oldWriter)
	snapshot, err := manager.CaptureInterrupt(oldSession.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}

	newWriter := &interruptFakePTYWriter{}
	newSession := &Session{id: oldSession.id, state: StateIdle, ptyWriter: newWriter}
	manager.mu.Lock()
	manager.sessions[oldSession.id] = newSession
	manager.mu.Unlock()

	if _, err := manager.PrepareInterrupt(snapshot); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("commit em sessão recriada = %v, esperado ErrInterruptStale", err)
	}
	if writes := oldWriter.snapshot(); len(writes) != 0 {
		t.Fatalf("sessão antiga recebeu efeito: %#v", writes)
	}
	if writes := newWriter.snapshot(); len(writes) != 0 {
		t.Fatalf("sessão recriada recebeu efeito: %#v", writes)
	}
}

type interruptErrorWriter struct {
	n   int
	err error
}

func (w interruptErrorWriter) Write([]byte) (int, error) { return w.n, w.err }

func TestInterruptSnapshotPreservesPTYErrorAsNonStale(t *testing.T) {
	ptyErr := errors.New("fake PTY write failed")
	manager, session := newInterruptFakeManager(interruptErrorWriter{err: ptyErr})
	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}

	operation, err := manager.PrepareInterrupt(snapshot)
	if err != nil {
		t.Fatalf("PrepareInterrupt: %v", err)
	}
	err = operation.Execute(context.Background())
	if err == nil || errors.Is(err, ErrInterruptStale) || !errors.Is(err, ptyErr) {
		t.Fatalf("erro PTY = %v, esperado erro não-stale que preserve a causa", err)
	}
}

func TestInterruptSnapshotRejectsZeroWriteWithoutError(t *testing.T) {
	manager, session := newInterruptFakeManager(interruptErrorWriter{})
	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}

	operation, err := manager.PrepareInterrupt(snapshot)
	if err != nil {
		t.Fatalf("PrepareInterrupt: %v", err)
	}
	err = operation.Execute(context.Background())
	if err == nil || errors.Is(err, ErrInterruptStale) || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write zero sem erro = %v, esperado io.ErrShortWrite não-stale", err)
	}
}
