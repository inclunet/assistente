package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/KennethanCeyer/ptyx"
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

type drainingPTYSession struct {
	reader      *io.PipeReader
	writer      *io.PipeWriter
	processDone chan struct{}
	killOnce    sync.Once
	mu          sync.Mutex
	events      []string
}

func newDrainingPTYSession() *drainingPTYSession {
	reader, writer := io.Pipe()
	return &drainingPTYSession{
		reader:      reader,
		writer:      writer,
		processDone: make(chan struct{}),
	}
}

func (f *drainingPTYSession) record(event string) {
	f.mu.Lock()
	f.events = append(f.events, event)
	f.mu.Unlock()
}

func (f *drainingPTYSession) PtyReader() io.Reader  { return f.reader }
func (f *drainingPTYSession) PtyWriter() io.Writer  { return io.Discard }
func (f *drainingPTYSession) Resize(int, int) error { return nil }
func (f *drainingPTYSession) Wait() error {
	f.record("wait-start")
	<-f.processDone
	f.record("wait-end")
	return &ptyx.ExitError{ExitCode: 1}
}
func (f *drainingPTYSession) Kill() error {
	f.killOnce.Do(func() {
		f.record("kill")
		_, _ = f.writer.Write([]byte("output-final"))
		_ = f.writer.Close()
		close(f.processDone)
	})
	return nil
}
func (f *drainingPTYSession) Close() error {
	f.record("close")
	return f.reader.Close()
}
func (f *drainingPTYSession) Pid() int          { return 1 }
func (f *drainingPTYSession) CloseStdin() error { return nil }

func (f *drainingPTYSession) eventSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

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

func TestCloseWaitsForProcessAndDrainsReaderBeforeClosingPTY(t *testing.T) {
	ptySession := newDrainingPTYSession()
	var output strings.Builder
	session := &Session{
		id:          "term-drain",
		state:       StateIdle,
		ptySession:  ptySession,
		readerCtx:   context.Background(),
		onRawOutput: func(_, chunk string) { output.WriteString(chunk) },
	}
	session.Start()

	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if output.String() != "output-final" {
		t.Fatalf("output final não foi drenado: %q", output.String())
	}

	events := ptySession.eventSnapshot()
	index := func(want string) int {
		for i, event := range events {
			if event == want {
				return i
			}
		}
		return -1
	}
	if kill, waited, closed := index("kill"), index("wait-end"), index("close"); kill < 0 || waited <= kill || closed <= waited {
		t.Fatalf("ordem de cleanup inválida: %v", events)
	}
}

func TestConcurrentCloseWaitsForSingleCleanup(t *testing.T) {
	ptySession := newDrainingPTYSession()
	session := &Session{
		id:         "term-concurrent-close",
		state:      StateIdle,
		ptySession: ptySession,
		readerCtx:  context.Background(),
	}
	session.Start()

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- session.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Close concorrente: %v", err)
		}
	}

	events := ptySession.eventSnapshot()
	if got := countEvent(events, "kill"); got != 1 {
		t.Fatalf("Kill chamado %d vezes; eventos=%v", got, events)
	}
	if got := countEvent(events, "wait-end"); got != 1 {
		t.Fatalf("Wait concluído %d vezes; eventos=%v", got, events)
	}
	if got := countEvent(events, "close"); got != 1 {
		t.Fatalf("Close do PTY chamado %d vezes; eventos=%v", got, events)
	}
}

func countEvent(events []string, want string) int {
	total := 0
	for _, event := range events {
		if event == want {
			total++
		}
	}
	return total
}

func TestClosePTYReleasesNaturalExitOnlyOnce(t *testing.T) {
	ptySession := &fakePTYSession{}
	session := &Session{id: "term-natural-exit", ptySession: ptySession}

	session.closePTY()
	session.closePTY()

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

func TestRunCommandMarksFailureBeforeWrite(t *testing.T) {
	startedAt := time.Now()
	session := &Session{state: StateIdle}
	got, err := session.RunCommand(t.Context(), "exemplo", time.Second, "test", "cmd-failed")
	if err == nil || got == nil || got.ID != "cmd-failed" {
		t.Fatalf("resultado da falha anterior à escrita: entrada=%+v erro=%v", got, err)
	}
	if got.ExitCode != -1 {
		t.Fatalf("exitCode = %d, esperado -1", got.ExitCode)
	}
	if got.EndedAt.IsZero() || got.EndedAt.Before(startedAt) {
		t.Fatalf("endedAt inválido: %v", got.EndedAt)
	}
}

func TestWaitForMarkerPreservesReadFailureAndCancellation(t *testing.T) {
	readErr := errors.New("falha de leitura do PTY")
	for _, tc := range []struct {
		name      string
		readerErr error
		cancel    bool
		wantErr   error
		code      int
	}{
		{"eof", io.EOF, false, nil, 0},
		{"leitura", readErr, false, readErr, -1},
		{"descritor fechado sem encerrar a sessão", os.ErrClosed, false, os.ErrClosed, -1},
		{"cancelado_ao_drenar", io.EOF, true, context.Canceled, -1},
		{"prazo_depois_da_falha_de_leitura", readErr, true, context.DeadlineExceeded, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := newCommandMarkerForShell("bash")
			reader := io.MultiReader(strings.NewReader(marker.startTag+"\nparcial"), iotest.ErrReader(tc.readerErr))
			session := &Session{
				ptySession: &fakePTYSession{},
				ptyReader:  reader,
				readerCtx:  t.Context(),
			}
			session.startCommandOutput(marker, &HistoryEntry{})
			session.Start()
			<-session.readerDone
			deadline := time.Now().Add(time.Second)
			if tc.wantErr == context.DeadlineExceeded {
				deadline = time.Time{}
			}
			ctx, cancel := context.WithDeadline(t.Context(), deadline)
			defer cancel()
			if tc.cancel {
				cancel()
			}
			output, code, err := session.waitForMarker(ctx, marker, "test")
			if !errors.Is(err, tc.wantErr) || (tc.readerErr != io.EOF && !errors.Is(err, tc.readerErr)) || code != tc.code || output != "parcial" {
				t.Fatalf("output=%q code=%d err=%v", output, code, err)
			}
		})
	}
}

type blockedWaitPTY struct {
	fakePTYSession
	waitStarted chan struct{}
	releaseWait chan struct{}
}

func (p *blockedWaitPTY) Wait() error {
	close(p.waitStarted)
	<-p.releaseWait
	return nil
}

func TestReadFailureReturnsWhileProcessWaitIsPending(t *testing.T) {
	marker := newCommandMarkerForShell("bash")
	readErr := errors.New("falha de leitura")
	pty := &blockedWaitPTY{waitStarted: make(chan struct{}), releaseWait: make(chan struct{})}
	session := &Session{ptySession: pty, readerCtx: t.Context(),
		ptyReader: io.MultiReader(strings.NewReader(marker.startTag+"\nparcial"), iotest.ErrReader(readErr)),
	}
	session.startCommandOutput(marker, &HistoryEntry{})
	session.Start()
	t.Cleanup(func() {
		close(pty.releaseWait)
		select {
		case <-session.closeDone:
		case <-time.After(time.Second):
			t.Fatal("recursos do processo não foram liberados")
		}
		if pty.killCalls != 1 || pty.closeCalls != 1 {
			t.Fatalf("kill=%d close=%d", pty.killCalls, pty.closeCalls)
		}
	})
	select {
	case <-pty.waitStarted:
	case <-time.After(time.Second):
		t.Fatal("coleta não iniciou")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	output, code, err := session.waitForMarker(ctx, marker, "test")
	if !errors.Is(err, readErr) || code != -1 || output != "parcial" {
		t.Fatalf("output=%q code=%d err=%v", output, code, err)
	}
}

type failingWritePTY struct {
	fakePTYSession
	wrapper  chan string
	released chan struct{}
}

func (p *failingWritePTY) PtyWriter() io.Writer { return p }

func (p *failingWritePTY) Write(data []byte) (int, error) {
	p.wrapper <- string(data)
	<-p.released
	return 0, io.ErrClosedPipe
}

func (p *failingWritePTY) Close() error {
	close(p.released)
	return p.fakePTYSession.Close()
}

func TestCancellationDuringCommandStartSkipsDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var events []string
	s := &Session{id: "canceled", state: StateIdle, shell: "bash", onCommandStart: func(_, _, _, _ string) {
		events = append(events, "start")
		cancel()
	}, ptyWriter: manualTestWriter(func(data []byte) (int, error) {
		t.Errorf("escrita após cancelamento (%d bytes)", len(data))
		return 0, io.ErrClosedPipe
	})}
	m := NewManager(DefaultManagerConfig(), func(event string, _ any) { events = append(events, event) })
	m.sessions[s.id] = s
	entry, err := m.RunCommand(ctx, s.id, "exemplo", time.Minute, "test")
	if !errors.Is(err, context.Canceled) || entry == nil || entry.ExitCode != -1 || entry.EndedAt.Before(entry.StartedAt) {
		t.Fatalf("entrada=%+v erro=%v", entry, err)
	}
	if history := s.GetHistory(); len(history) != 1 || history[0] != *entry || s.State() != StateIdle {
		t.Fatalf("histórico=%+v estado=%v", history, s.State())
	}
	if strings.Join(events, ",") != "start,terminal:command_end" {
		t.Fatalf("eventos=%v", events)
	}
}

func TestUnavailableCommandWriterKeepsHistory(t *testing.T) {
	for _, writer := range []io.Writer{nil, manualTestWriter(func([]byte) (int, error) { return 0, io.ErrClosedPipe })} {
		s := &Session{id: "missing-writer", shell: "bash", readerCtx: t.Context(), ptyWriter: writer}
		m := NewManager(DefaultManagerConfig(), nil)
		m.sessions[s.id] = s
		entry, err := m.RunCommand(t.Context(), s.id, "example", time.Second, "test")
		if err == nil || entry == nil || entry.ExitCode != -1 || entry.EndedAt.Before(entry.StartedAt) {
			t.Fatalf("entrada=%+v erro=%v", entry, err)
		}
		if writer != nil && !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("causa da escrita perdida: %v", err)
		}
		if got := s.GetHistory(); len(got) != 1 || got[0].ID != entry.ID {
			t.Fatalf("histórico=%+v", got)
		}
	}
}

type pausedKillPTY struct {
	*drainingPTYSession
	killing chan struct{}
	resume  chan struct{}
}

func (p *pausedKillPTY) Kill() error {
	close(p.killing)
	<-p.resume
	return p.drainingPTYSession.Kill()
}

func TestPartialWriteRetiresSessionAndDrainsReader(t *testing.T) {
	for _, tc := range []struct {
		name      string
		writeErr  error
		zeroBytes bool
	}{
		{"curta_sem_erro", nil, false},
		{"curta_com_erro", io.ErrClosedPipe, false},
		{"nenhum_byte_com_erro", io.ErrClosedPipe, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pausedKillPTY{drainingPTYSession: newDrainingPTYSession(), killing: make(chan struct{}), resume: make(chan struct{})}
			resume := sync.OnceFunc(func() { close(p.resume) })
			defer resume()
			var events []string
			var manager *Manager
			manager = NewManager(DefaultManagerConfig(), func(event string, _ any) {
				events = append(events, event)
				if event == "terminal:command_end" {
					// Um consumidor pode pedir Close na entrega de command_end.
					if err := manager.Close("partial"); err != nil {
						t.Errorf("Close: %v", err)
					}
				}
			})
			s := &Session{id: "partial", shell: "bash", readerCtx: t.Context(), ptySession: p,
				onCommandStart: func(_, _, _, _ string) { events = append(events, "start") },
				onOutput:       func(_, _, chunk string) { events = append(events, chunk) },
				onExit: func(_ string, err error) {
					if err == nil {
						t.Error("exited sem causa")
					}
					events = append(events, "exited")
				},
			}
			s.ptyWriter = manualTestWriter(func(data []byte) (int, error) {
				if tc.zeroBytes {
					return 0, tc.writeErr
				}
				_, after, ok := strings.Cut(string(data), "'"+markerStart)
				if !ok {
					return 0, errors.New("moldura sem início")
				}
				tag, _, _ := strings.Cut(after, "'")
				s.recordOutput(markerStart + tag + "\nantes/")
				return len(data) - 1, tc.writeErr
			})
			manager.sessions[s.id] = s
			s.Start()
			done := make(chan error, 1)
			go func() {
				_, err := manager.RunCommand(t.Context(), s.id, "exemplo", time.Minute, "test")
				done <- err
			}()
			select {
			case <-p.killing:
			case <-time.After(time.Second):
				t.Fatal("falha parcial não iniciou cleanup")
			}
			manager.Release(s.id)
			if manager.Has(s.id) || s.State() != StateClosing {
				t.Fatal("sessão voltou a aceitar comandos durante cleanup")
			}
			if _, err := manager.RunCommand(t.Context(), s.id, "outro", time.Minute, "test"); err == nil {
				t.Fatal("segundo comando aceito")
			}
			resume()
			wantErr := tc.writeErr
			if wantErr == nil {
				wantErr = io.ErrShortWrite
			}
			select {
			case err := <-done:
				if !errors.Is(err, wantErr) {
					t.Fatalf("causa perdida: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup bloqueou")
			}
			history := s.GetHistory()
			wantOutput, wantEvents := "antes/output-final", "start,antes/output-final,terminal:command_end,exited"
			if tc.zeroBytes {
				wantOutput, wantEvents = "", "start,terminal:command_end,exited"
			}
			if s.State() != StateExited || len(history) != 1 || history[0].Output != wantOutput || history[0].ExitCode != -1 || strings.Join(events, ",") != wantEvents {
				t.Fatalf("estado=%v histórico=%+v eventos=%v", s.State(), history, events)
			}
			if got := strings.Join(p.eventSnapshot(), ","); got != "kill,wait-start,wait-end,close" {
				t.Fatalf("recursos: %s", got)
			}
		})
	}
}

// O backend só prossegue quando o teste o libera. Nenhum timeout interno
// do fake pode fazer uma espera ilimitada do chamador parecer correta.
type stalledCleanupPTY struct {
	fakePTYSession
	phase                   string
	stalled, resume, killed chan struct{}
	reader                  *io.PipeReader
	writer                  *io.PipeWriter
	readErr, waitErr        error
}

func (p *stalledCleanupPTY) pause(phase string) {
	if p.phase == phase {
		close(p.stalled)
		<-p.resume
	}
}
func (p *stalledCleanupPTY) PtyReader() io.Reader { return p }
func (p *stalledCleanupPTY) Read(b []byte) (int, error) {
	p.pause("Read")
	return p.reader.Read(b)
}
func (p *stalledCleanupPTY) Kill() error {
	p.pause("Kill")
	close(p.killed)
	return p.writer.CloseWithError(p.readErr)
}
func (p *stalledCleanupPTY) Wait() error {
	<-p.killed
	p.pause("Wait")
	return p.waitErr
}
func (p *stalledCleanupPTY) Close() error {
	p.pause("Close")
	return p.reader.Close()
}

func TestWriteFailureBoundsCallerWhileCleanupContinues(t *testing.T) {
	for _, phase := range []string{"Kill", "Wait", "Read", "Close"} {
		t.Run(phase, func(t *testing.T) {
			reader, writer := io.Pipe()
			p := &stalledCleanupPTY{phase: phase, reader: reader, writer: writer, stalled: make(chan struct{}), resume: make(chan struct{}), killed: make(chan struct{}), waitErr: errors.New("falha de espera")}
			resume := sync.OnceFunc(func() { close(p.resume) })
			t.Cleanup(resume)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ended := make(chan struct{}, 1)
			exited := make(chan error, 1)
			var raw, output strings.Builder
			s := &Session{id: "blocked", shell: "bash", readerCtx: t.Context(), ptySession: p, onExit: func(_ string, err error) { exited <- err },
				onOutput: func(_, _, chunk string) { output.WriteString(chunk) }, onRawOutput: func(_, chunk string) { raw.WriteString(chunk) }}
			s.ptyWriter = manualTestWriter(func(data []byte) (int, error) {
				_, after, ok := strings.Cut(string(data), "'"+markerStart)
				if !ok {
					return 0, errors.New("moldura sem início")
				}
				id, _, _ := strings.Cut(after, "'")
				s.recordOutput(markerStart + id + "\nparcial")
				return 1, io.ErrShortWrite
			})
			m := NewManager(DefaultManagerConfig(), func(event string, _ any) {
				if event == "terminal:command_end" {
					ended <- struct{}{}
				}
			})
			m.sessions[s.id] = s
			s.Start()
			result := make(chan error, 1)
			go func() { _, err := m.RunCommand(ctx, s.id, "example", time.Hour, "test"); result <- err }()
			select {
			case <-p.stalled:
			case <-time.After(3 * time.Second):
				t.Fatal("backend não bloqueou")
			}
			if phase == "Read" {
				select {
				case <-p.killed:
				case <-time.After(time.Second):
					t.Fatal("cleanup não iniciou")
				}
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, ErrSessionWrite) || !errors.Is(err, io.ErrShortWrite) || !errors.Is(err, context.Canceled) {
					t.Fatalf("causas: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("comando esperou backend bloqueado após cancelamento")
			}
			select {
			case <-ended:
			default:
				t.Fatal("command_end ausente")
			}
			select {
			case err := <-exited:
				t.Fatalf("exited antes do cleanup: %v", err)
			default:
			}
			m.Release(s.id)
			if _, err := m.RunCommand(t.Context(), s.id, "outro", time.Hour, "test"); err == nil {
				t.Fatal("aceitou outro comando")
			}
			if s.State() != StateClosing || len(s.GetHistory()) != 1 || s.GetHistory()[0].Output != "parcial" || output.String() != "parcial" {
				t.Fatalf("resultado parcial inesperado: %v", s.GetHistory())
			}
			s.recordOutput("tardio")
			if raw.Len() != 0 || s.outputBuf.Len() != 0 {
				t.Fatal("saída tardia foi acumulada ou publicada como manual")
			}
			closed := make(chan error, 1)
			go func() { closed <- m.Close(s.id) }()
			resume()
			select {
			case err := <-closed:
				if !errors.Is(err, p.waitErr) {
					t.Fatalf("Close perdeu causa do backend: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Close não concluiu")
			}
			select {
			case err := <-exited:
				if !errors.Is(err, p.waitErr) || !errors.Is(err, ErrSessionWrite) || !errors.Is(err, context.Canceled) {
					t.Fatalf("exited perdeu causas: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("exited não entregue")
			}
		})
	}
}

func TestWriteFailureHonorsCommandTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		p := &stalledCleanupPTY{phase: "Kill", reader: reader, writer: writer, stalled: make(chan struct{}), resume: make(chan struct{}), killed: make(chan struct{})}
		s := &Session{id: "timeout", shell: "bash", readerCtx: t.Context(), ptySession: p, ptyWriter: manualTestWriter(func([]byte) (int, error) { return 0, io.ErrShortWrite })}
		m := NewManager(DefaultManagerConfig(), nil)
		m.sessions[s.id] = s
		s.Start()
		const budget = 3 * time.Second
		start := time.Now()
		_, err := m.RunCommand(t.Context(), s.id, "exemplo", budget, "test")
		close(p.resume)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrSessionWrite) || time.Since(start) != budget {
			t.Fatalf("prazo=%v erro=%v", time.Since(start), err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestWriteCleanupPreservesIndependentReadError(t *testing.T) {
	reader, writer := io.Pipe()
	readErr := errors.New("leitura interrompida por outra causa")
	waitErr := errors.New("erro independente de espera")
	exited := make(chan error, 1)
	p := &stalledCleanupPTY{reader: reader, writer: writer, killed: make(chan struct{}), readErr: readErr, waitErr: waitErr}
	s := &Session{id: "read-during-write-cleanup", shell: "bash", readerCtx: t.Context(), ptySession: p, onExit: func(_ string, err error) { exited <- err }, ptyWriter: manualTestWriter(func([]byte) (int, error) { return 0, io.ErrShortWrite })}
	m := NewManager(DefaultManagerConfig(), nil)
	m.sessions[s.id] = s
	s.Start()
	_, err := m.RunCommand(t.Context(), s.id, "example", time.Minute, "test")
	if !errors.Is(err, ErrSessionRead) || !errors.Is(err, readErr) || !errors.Is(err, ErrSessionWrite) || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("causas: %v", err)
	}
	select {
	case eventErr := <-exited:
		for _, cause := range []error{readErr, waitErr, ErrSessionWrite} {
			if !errors.Is(eventErr, cause) || strings.Count(eventErr.Error(), cause.Error()) != 1 {
				t.Fatalf("causa %q ausente ou repetida em exited: %v", cause, eventErr)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("exited não entregue")
	}
}

func TestPartialWriteDeliversAlreadyBufferedCommandOutput(t *testing.T) {
	const want = "  parcial  \n_"
	for _, reader := range []string{"aberto", "EOF"} {
		for _, footer := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rodape=%t", reader, footer), func(t *testing.T) {
				var emitted []string
				s := &Session{id: "partial-write", shell: "bash", readerCtx: t.Context(), onOutput: func(_, _, chunk string) { emitted = append(emitted, chunk) }}
				s.ensureLifecycleChannels()
				s.ptyWriter = manualTestWriter(func(data []byte) (int, error) {
					_, tag, ok := strings.Cut(string(data), "'"+markerStart)
					if !ok {
						return 0, errors.New("moldura sem início")
					}
					id, _, _ := strings.Cut(tag, "'")
					raw := string(data) + "\n" + markerStart + id + "\n" + want
					if footer {
						raw += "\n" + markerEnd + id + "0!\n"
					}
					s.recordOutput(raw)
					if reader == "EOF" {
						s.signalReaderDone()
					}
					return len(data) - 1, io.ErrClosedPipe
				})
				manager := NewManager(DefaultManagerConfig(), nil)
				manager.sessions[s.id] = s
				entry, err := manager.RunCommand(t.Context(), s.id, "example", time.Minute, "test")
				if !errors.Is(err, io.ErrClosedPipe) || entry == nil || entry.Output != want || entry.ExitCode != -1 || len(emitted) != 1 || emitted[0] != want {
					t.Fatalf("entrada=%+v erro=%v streaming=%q", entry, err, emitted)
				}
				if history := s.GetHistory(); len(history) != 1 || history[0].Output != want || history[0].ExitCode != -1 {
					t.Fatalf("histórico=%v", history)
				}
			})
		}
	}
}

func TestCloseBeforeCommandWriteKeepsHistoryAndCause(t *testing.T) {
	processExit := errors.New("encerramento do processo antes do envio")
	for _, cause := range []error{io.ErrUnexpectedEOF, ErrSessionClosed, processExit} {
		s := &Session{id: "early-close", shell: "bash", readerCtx: t.Context()}
		s.onCommandStart = func(_, _, _, _ string) {
			switch cause {
			case processExit:
				s.ensureLifecycleChannels()
				s.processErr = cause
				close(s.processDone)
				s.markExited(cause)
			case ErrSessionClosed:
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			default:
				s.closeAfterReadFailure(cause)
			}
		}
		manager := NewManager(DefaultManagerConfig(), nil)
		manager.sessions[s.id] = s
		entry, err := manager.RunCommand(t.Context(), s.id, "example", time.Second, "test")
		if entry == nil || entry.ExitCode != -1 || !errors.Is(err, cause) {
			t.Fatalf("causa=%v entrada=%+v erro=%v", cause, entry, err)
		}
		if got := s.GetHistory(); len(got) != 1 || got[0].ID != entry.ID || got[0].ExitCode != -1 {
			t.Fatalf("histórico=%+v", got)
		}
	}
}

func TestReadFailureWhileWritingCommand(t *testing.T) {
	for _, tc := range []struct {
		name              string
		started, canceled bool
	}{
		{"antes_do_inicio", false, false},
		{"apos_inicio", true, false},
		{"cancelado_enquanto_escrevia", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pty := &failingWritePTY{wrapper: make(chan string, 1), released: make(chan struct{})}
			reader, writer := io.Pipe()
			defer func() { _ = reader.Close() }()
			defer func() { _ = writer.Close() }()
			var emitted strings.Builder
			ended := make(chan struct{})
			exited := make(chan bool, 1)
			session := &Session{id: "test", shell: "bash", ptySession: pty, ptyReader: reader, readerCtx: t.Context(),
				onOutput: func(_, _, output string) { emitted.WriteString(output) },
				onExit: func(_ string, err error) {
					select {
					case <-ended:
						exited <- errors.Is(err, ErrSessionWrite) && errors.Is(err, ErrSessionRead)
					default:
						exited <- false
					}
				},
			}
			session.Start()
			manager := NewManager(DefaultManagerConfig(), func(event string, _ any) {
				if event == "terminal:command_end" {
					close(ended)
				}
			})
			manager.sessions[session.id] = session
			type outcome struct {
				entry *HistoryEntry
				err   error
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			finished := make(chan outcome, 1)
			go func() {
				entry, err := manager.RunCommand(ctx, session.id, "comando", time.Minute, "test")
				finished <- outcome{entry, err}
			}()

			var wrapper string
			select {
			case wrapper = <-pty.wrapper:
			case <-time.After(time.Second):
				t.Fatal("escrita do comando não iniciou")
			}
			raw, want := wrapper+"\n", ""
			if tc.started {
				_, tag, ok := strings.Cut(wrapper, "'"+markerStart)
				if !ok {
					t.Fatal("tag de início ausente da moldura bash")
				}
				tag, _, _ = strings.Cut(tag, "'")
				want = "  parte  \n_"
				raw += markerStart + tag + "\n" + want
			}
			if _, err := io.WriteString(writer, raw); err != nil {
				t.Fatal(err)
			}
			if tc.canceled {
				cancel()
			} // O contexto vence enquanto Write ainda está bloqueado.
			readErr := errors.New("leitura interrompida")
			if err := writer.CloseWithError(readErr); err != nil {
				t.Fatal(err)
			}
			select {
			case result := <-finished:
				if errors.Is(result.err, context.Canceled) != tc.canceled || !errors.Is(result.err, readErr) || !errors.Is(result.err, io.ErrClosedPipe) || result.entry == nil ||
					result.entry.ExitCode != -1 || result.entry.Output != want || emitted.String() != want {
					t.Fatalf("entrada=%+v erro=%v streaming=%q", result.entry, result.err, emitted.String())
				}
				if history := session.GetHistory(); len(history) != 1 || history[0].Output != want {
					t.Fatalf("histórico=%v", history)
				}
			case <-time.After(time.Second):
				t.Fatal("falha do leitor não liberou o writer")
			}
			<-session.closeDone
			select {
			case ordered := <-exited:
				if !ordered {
					t.Fatal("exited antes de command_end ou sem as duas causas")
				}
			case <-time.After(time.Second):
				t.Fatal("exited não entregue")
			}
			if pty.killCalls != 1 || pty.closeCalls != 1 {
				t.Fatalf("kill=%d close=%d", pty.killCalls, pty.closeCalls)
			}
		})
	}
}

func TestWaitForMarkerFlushesUnsentPartialOutput(t *testing.T) {
	const first = "  começo "
	const tail = "fim  \n_\n\n"
	for _, mode := range []string{"cancelado", "fechado", "leitura"} {
		t.Run(mode, func(t *testing.T) {
			marker := newCommandMarkerForShell("bash")
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			readErr := errors.New("falha na leitura")
			wantErr := map[string]error{"cancelado": context.Canceled, "fechado": ErrSessionClosed, "leitura": readErr}[mode]
			var input bytes.Buffer
			session := &Session{ptyWriter: &input}
			session.ensureLifecycleChannels()
			session.outputBuf.WriteString("echo\n" + marker.startTag + "\n" + first)
			var emitted strings.Builder
			session.onOutput = func(_, _, chunk string) {
				emitted.WriteString(chunk)
				if emitted.Len() != len(first) {
					return
				}
				// O restante chega depois de a primeira parte alcançar o consumidor.
				session.outputBuf.WriteString(tail)
				switch mode {
				case "leitura":
					session.readerErr = readErr
				case "fechado":
					session.explicitClose = true
				case "cancelado":
					session.outputBuf.WriteString("\n" + marker.endTag + "1!\nprompt")
					cancel()
					return
				}
				session.signalReaderDone()
			}
			output, code, err := session.waitForMarker(ctx, marker, "cmd")
			if mode == "cancelado" && input.String() != "\x03" {
				t.Fatalf("entrada após cancelamento=%q", input.Bytes())
			}
			if !errors.Is(err, wantErr) || code != -1 || output != first+tail || emitted.String() != output {
				t.Fatalf("output=%q streaming=%q code=%d err=%v", output, emitted.String(), code, err)
			}
		})
	}
}

type noEOFPTY struct{ *drainingPTYSession }

func (p *noEOFPTY) Kill() error {
	p.record("kill")
	close(p.processDone)
	// O leitor continua bloqueado no pipe até o fallback fechar o descritor.
	return nil
}

func TestCloseReleasesReaderWhenPTYDoesNotDeliverEOF(t *testing.T) {
	pty := &noEOFPTY{newDrainingPTYSession()}
	defer func() { _ = pty.writer.Close() }()
	s := &Session{ptySession: pty, readerCtx: t.Context()}
	s.Start()
	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < readerDrainTimeout {
		t.Fatalf("descritor fechado antes do prazo de drenagem: %s", elapsed)
	}
	if events := strings.Join(pty.eventSnapshot(), ","); events != "kill,wait-start,wait-end,close" {
		t.Fatalf("ordem de encerramento: %s", events)
	}
}

// A sessão sem PTY força a saída antes da escrita. O caminho de execução real
// é exercitado por TestPersistentExitPublishesResultBeforeExit.
func TestNaturalExitWaitsForCommandResult(t *testing.T) {
	var events []string
	s := &Session{state: StateIdle, shell: "bash", onExit: func(string, error) { events = append(events, "exited") }}
	s.onCommandStart = func(_, _, _, _ string) {
		s.markExited(nil)
		if len(events) != 0 {
			t.Fatalf("saída publicada antes do resultado: %v", events)
		}
	}
	_, _ = s.runCommandInDirectory(context.Background(), "exit 7", "", time.Second, "test", "comando",
		func(*HistoryEntry, error) { events = append(events, "command_end") })
	if got := strings.Join(events, ","); got != "command_end,exited" {
		t.Fatalf("ordem de publicação: %s", got)
	}
}

func TestAcquiredSessionAcceptsCommandAfterManualInputAndRelease(t *testing.T) {
	dir := ptyTestDir(t)
	manager := NewManager(DefaultManagerConfig(), nil)
	t.Cleanup(manager.CloseAll)
	session, err := manager.Acquire(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SendInput(session.ID(), "echo manual"); err != nil {
		t.Fatal(err)
	}
	manager.Release(session.ID())
	if state := session.State(); state != StateIdle {
		t.Fatalf("estado após entrada manual e Release: %v", state)
	}
	entry, err := manager.RunCommand(t.Context(), session.ID(), "echo pronto", 15*time.Second, "test")
	if err != nil || entry == nil || entry.ExitCode != 0 || !strings.Contains(entry.Output, "pronto") {
		t.Fatalf("comando seguinte: entrada=%+v erro=%v", entry, err)
	}
}

func TestAnnouncementPreviewContract(t *testing.T) {
	data, err := os.ReadFile("testdata/announcement-previews.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []map[string]json.RawMessage
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("tabela de prévias vazia")
	}
	for i, row := range cases {
		var name, text, expected string
		var prefixLength int
		var truncated bool
		fields := map[string]any{"name": &name, "text": &text, "want": &expected, "prefixLength": &prefixLength, "truncated": &truncated}
		if len(row) != len(fields) {
			t.Fatalf("caso %d: esperadas exatamente %d chaves", i+1, len(fields))
		}
		// Os nomes exatos importam: o consumidor JavaScript distingue maiúsculas.
		for key, dst := range fields {
			raw := row[key]
			if raw == nil || string(raw) == "null" {
				t.Fatalf("caso %d: campo obrigatório %q ausente ou nulo", i+1, key)
			}
			if err := json.Unmarshal(raw, dst); err != nil {
				t.Fatalf("caso %d, campo %q: %v", i+1, key, err)
			}
		}
		if name == "" || prefixLength < 0 {
			t.Fatalf("caso %d: nome vazio ou prefixo negativo", i+1)
		}
		t.Run(name, func(t *testing.T) {
			prefix := strings.Repeat("a", prefixLength)
			got := terminalAnnouncementPreview(prefix + text)
			want := announcementPreview{Output: prefix + expected, Truncated: truncated}
			if got != want {
				t.Fatalf("prévia = %+v, esperada %+v", got, want)
			}
		})
	}
}

func TestCommandEndPreviewsOriginalOutputBeyondHistoryLimit(t *testing.T) {
	// Limite observável do histórico (AEP-0089), independente da constante de produção.
	const historyBytes = 50 * 1024
	emoji := strings.Repeat("🌍", 300)
	padding := strings.Repeat(" ", 60*1024)
	for _, tc := range []struct {
		name, output string
		want         *announcementPreview
	}{
		{"saída curta", "ok", nil},
		{"limite exato", strings.Repeat("a", historyBytes), nil},
		{"primeiro byte excedente", strings.Repeat("a", historyBytes+1), &announcementPreview{Output: strings.Repeat("a", 300), Truncated: true}},
		{"espaços iniciais", padding + "ok", &announcementPreview{Output: "ok"}},
		{"espaços finais", emoji + padding, &announcementPreview{Output: emoji}},
		{"texto após espaços", emoji + padding + "x", &announcementPreview{Output: emoji, Truncated: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Session{id: "preview", shell: "bash", readerCtx: t.Context()}
			s.ensureLifecycleChannels()
			s.ptyWriter = manualTestWriter(func(data []byte) (int, error) {
				_, tag, ok := strings.Cut(string(data), "'"+markerStart)
				if !ok {
					return 0, errors.New("moldura sem início")
				}
				id, _, _ := strings.Cut(tag, "'")
				s.recordOutput(markerStart + id + "\n" + tc.output + "\n" + markerEnd + id + "0!\n")
				return len(data), nil
			})
			var payload map[string]any
			m := NewManager(DefaultManagerConfig(), func(event string, data any) {
				if event == "terminal:command_end" {
					payload = data.(map[string]any)
				}
			})
			m.sessions[s.id] = s
			entry, err := m.RunCommand(t.Context(), s.id, "example", time.Second, "test")
			if err != nil || entry == nil || entry.Output != tc.output {
				t.Fatalf("saída original incorreta: %v", err)
			}
			if payload["output"] != limitedHistoryEntry(entry).Output {
				t.Fatal("evento perdeu a saída de histórico")
			}
			got, included := payload["announcementPreview"]
			if tc.want == nil {
				if included {
					t.Errorf("evento sem corte incluiu prévia: %+v", got)
				}
			} else if got != *tc.want {
				t.Errorf("prévia = %+v, esperada %+v", got, *tc.want)
			}
		})
	}
}

func TestReleaseKeepsCommandReservedThroughEnd(t *testing.T) {
	s := &Session{id: "terminal", state: StateIdle}
	m := closeTestManager(s)
	m.emitEvent = func(event string, _ any) {
		if event != "terminal:command_end" {
			return
		}
		m.Release(s.id)
		if got := s.State(); got != StateRunning {
			t.Errorf("estado durante entrega do resultado: %v", got)
		}
	}
	if entry, err := m.RunCommand(t.Context(), s.id, "sem writer", time.Second, "test"); err == nil || entry == nil {
		t.Fatalf("entrada=%+v erro=%v", entry, err)
	}
	if got := s.State(); got != StateIdle {
		t.Fatalf("estado após entrega: %v", got)
	}
}
