package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
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
	session := &Session{id: "interrupt-fake", state: StateIdle, lastDispatchedID: "command-original", ptyWriter: writer}
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

func TestUndispatchedCommandKeepsManualInterruptTarget(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	var activeID string
	m := NewManager(DefaultManagerConfig(), func(event string, data any) {
		if event == "terminal:command_end" {
			activeID, _ = data.(map[string]any)["activeManualCommandId"].(string)
		}
	})
	s := &Session{id: "sessão", shell: "bash", state: StateIdle, ptyWriter: writer}
	m.sessions[s.id] = s
	if _, err := s.SendInput("manual", "A"); err != nil {
		t.Fatal(err)
	}
	old, err := m.CaptureInterrupt(s.id)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.onCommandStart = func(_, _, _, _ string) {
		if _, captureErr := m.CaptureInterrupt(s.id); !errors.Is(captureErr, ErrInterruptStale) {
			t.Errorf("captura durante o despacho pendente: %v", captureErr)
		}
		cancel()
	}
	entry, err := m.RunCommand(ctx, s.id, "gerenciado", time.Minute, "teste")
	if !errors.Is(err, context.Canceled) || entry == nil || entry.ExitCode != -1 || activeID != "A" {
		t.Fatalf("resultado=%+v erro=%v alvo do evento=%q", entry, err, activeID)
	}
	if _, err = m.PrepareInterrupt(old); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("snapshot anterior reutilizado: %v", err)
	}
	current, err := m.CaptureInterrupt(s.id)
	if err != nil || !current.MatchesTarget(s.id, activeID) {
		t.Fatalf("snapshot diverge do alvo publicado: %v", err)
	}
	op, err := m.PrepareInterrupt(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}
	if writes := writer.snapshot(); len(writes) != 2 || !bytes.Equal(writes[1], []byte{3}) {
		t.Fatalf("input recebido pelo PTY=%q", writes)
	}
}

func TestCommandEndOnlyRestoresAnUndispatchedManualTarget(t *testing.T) {
	for _, tc := range []struct {
		name             string
		manual, dispatch bool
	}{{"sem entrada manual", false, false}, {"despachado depois da entrada manual", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ended := false
			m := NewManager(DefaultManagerConfig(), func(event string, data any) {
				if event == "terminal:command_end" {
					ended = true
					if _, ok := data.(map[string]any)["activeManualCommandId"]; ok {
						t.Error("término anunciou um alvo manual antigo")
					}
				}
			})
			s := &Session{id: "sessão", shell: "bash", state: StateIdle, ptyWriter: io.Discard}
			m.sessions[s.id] = s
			if tc.manual {
				if _, err := s.SendInput("manual", "A"); err != nil {
					t.Fatal(err)
				}
			}
			written := false
			s.ptyWriter = manualTestWriter(func(p []byte) (int, error) {
				written = true
				cancel()
				return len(p), nil
			})
			if !tc.dispatch {
				s.onCommandStart = func(_, _, _, _ string) { cancel() }
			}
			_, err := m.RunCommand(ctx, s.id, "gerenciado", time.Minute, "teste")
			if !errors.Is(err, context.Canceled) || !ended || written != tc.dispatch {
				t.Fatalf("erro=%v término=%t escrito=%t", err, ended, written)
			}
		})
	}
}

func TestHistoryRetainsActiveManualInputUntilAnotherDispatch(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	m, s := newInterruptFakeManager(writer)
	if _, err := s.SendInput("manual", "A"); err != nil {
		t.Fatal(err)
	}
	var last *HistoryEntry
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for i := 0; i < maxHistoryEntries+2; i++ {
		var err error
		last, err = m.RunCommand(ctx, s.id, "cancelado", time.Second, "teste")
		if !errors.Is(err, context.Canceled) || last == nil {
			t.Fatalf("tentativa %d: %v", i, err)
		}
	}
	history, err := m.GetHistory(s.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != maxHistoryEntries || history[0].ID != "A" || history[0].Source != "user-raw" ||
		history[0].Command != "manual" || history[len(history)-1].ID != last.ID {
		t.Fatalf("histórico sem alvo manual ou última tentativa: %d entradas", len(history))
	}
	target, err := m.CaptureInterrupt(s.id)
	if err != nil || !target.MatchesTarget(s.id, history[0].ID) {
		t.Fatalf("alvo restaurado na hidratação: %v", err)
	}
	if _, err := s.SendInput("nova", "B"); err != nil {
		t.Fatal(err)
	}
	history, err = m.GetHistory(s.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != maxHistoryEntries || history[0].ID == "A" || history[len(history)-1].ID != "B" {
		t.Fatal("o próximo despacho não liberou a entrada manual anterior")
	}
}

func TestInterruptSnapshotRejectsNewCommandGeneration(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)

	snapshot, err := manager.CaptureInterrupt(session.id)
	if err != nil {
		t.Fatalf("CaptureInterrupt: %v", err)
	}
	if err := session.reserveCommand(nil); err != nil {
		t.Fatalf("reserveCommand: %v", err)
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
	if err := session.reserveCommand(nil); err != nil {
		t.Fatalf("reserveCommand: %v", err)
	}

	if _, err := manager.CaptureInterrupt(session.id); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("captura durante pendingWrite = %v, esperado ErrInterruptStale", err)
	}
	session.finishCommand()
}

func TestCaptureInterruptRejectsSessionWithoutLastDispatchedID(t *testing.T) {
	writer := &interruptFakePTYWriter{}
	manager, session := newInterruptFakeManager(writer)
	session.mu.Lock()
	session.lastDispatchedID = ""
	session.mu.Unlock()

	if _, err := manager.CaptureInterrupt(session.id); !errors.Is(err, ErrInterruptStale) {
		t.Fatalf("captura sem lastDispatchedID = %v, esperado ErrInterruptStale", err)
	}
}

func TestSendInputPreservesLastDispatchedIDWhenIdle(t *testing.T) {
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
	go func() { beginDone <- session.reserveCommand(nil) }()
	select {
	case err := <-beginDone:
		t.Fatalf("reserveCommand atravessou o commit: %v", err)
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
	go func() { beginDone <- session.reserveCommand(nil) }()
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
	if err := session.reserveCommand(nil); err != nil {
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

func TestInterruptAfterPowerShellProbe(t *testing.T) {
	for _, mode := range []string{"snapshot", "direto", "sem rodapé"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				writer := &interruptFakePTYWriter{}
				manager, s := newInterruptFakeManager(writer)
				s.shell = "powershell"
				s.powerShellKey = "test"
				done := make(chan error, 1)
				go func() {
					_, err := s.runCommandInDirectory(t.Context(), "Write-Output exemplo", "", time.Minute, "test", "cmd", nil)
					done <- err
				}()
				synctest.Wait()
				writes := writer.snapshot()
				if len(writes) != 1 {
					t.Fatalf("escritas antes da resposta: %d", len(writes))
				}
				id := regexp.MustCompile(`([a-f0-9]{8})__'`).FindSubmatch(writes[0])
				if len(id) != 2 {
					t.Fatal("sem identificador de moldura")
				}
				reply := markerStart + string(id[1]) + markerSuffix + "\n"
				if mode != "sem rodapé" {
					reply += markerEnd + string(id[1]) + markerSuffix + "1!"
				}
				s.recordOutput(reply)
				if mode == "snapshot" {
					snapshot, err := manager.CaptureInterrupt(s.id)
					if err != nil {
						t.Fatal(err)
					}
					op, err := manager.PrepareInterrupt(snapshot)
					if err != nil {
						t.Fatal(err)
					}
					if err := op.Execute(t.Context()); err != nil {
						t.Fatal(err)
					}
				} else if err := s.Interrupt(); err != nil {
					t.Fatal(err)
				}
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("comando interrompido: %v", err)
				}
				writes = writer.snapshot()
				if len(writes) != 2 || !bytes.Equal(writes[1], []byte{3}) {
					t.Fatalf("esperado Ctrl+C como última de duas escritas, total=%d", len(writes))
				}
			})
		})
	}
}

func TestPowerShellDispatchesBodyOnce(t *testing.T) {
	for _, reply := range []string{"0", "1"} {
		t.Run(reply, func(t *testing.T) {
			s := &Session{shell: "powershell", powerShellKey: "test", state: StateIdle}
			var writes []string
			const body = "Write-Output exemplo"
			encoded := base64.StdEncoding.EncodeToString([]byte(body))
			s.ptyWriter = manualTestWriter(func(p []byte) (int, error) {
				writes = append(writes, string(p))
				id := regexp.MustCompile(`([a-f0-9]{8})__'`).FindSubmatch(p)
				if len(id) != 2 {
					return 0, errors.New("sem id")
				}
				tag := string(id[1]) + markerSuffix
				code, output := reply, ""
				if len(writes) > 1 {
					code = "0"
					output = "ok\n"
				}
				// Antes do início e depois da execução, a mesma saída residual
				// é compatível com a resposta de ausência usada no protocolo antigo.
				s.recordOutput(markerEnd + tag + "!\n" + markerStart + tag + "\n" + output + markerEnd + tag + code + "!" + markerEnd + tag + "!")
				return len(p), nil
			})
			e, err := s.runCommandInDirectory(t.Context(), body, "", 5*time.Second, "test", "cmd", nil)
			if err != nil || e.ExitCode != 0 || e.Output != "ok" || len(writes) != 2 || strings.Contains(writes[0], encoded) || strings.Count(writes[1], encoded) != 1 {
				t.Fatalf("despacho: resultado=%+v erro=%v escritas=%d", e, err, len(writes))
			}
		})
	}
}

func TestPowerShellProbeRequiresFooterAfterProcessExit(t *testing.T) {
	marker := newCommandMarkerForShell("powershell")
	marker.probeInterrupt = make(chan struct{})
	done := make(chan struct{})
	close(done)
	s := &Session{readerDone: done, processDone: done, onOutput: func(_, _, output string) {
		t.Errorf("saída da consulta entregue como comando: %q", output)
	}}
	s.beginMarkerCapture(marker)
	s.recordOutput(marker.startTag + "\ntexto residual")
	output, code, err := s.waitForMarker(t.Context(), marker, "cmd")
	if err == nil || code != -1 || output != "" {
		t.Fatalf("consulta sem rodapé: output=%q code=%d err=%v", output, code, err)
	}
}
