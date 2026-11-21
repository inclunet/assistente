package terminal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
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
	session := &Session{
		id:         "term-drain",
		state:      StateIdle,
		ptySession: ptySession,
		readerCtx:  context.Background(),
	}
	session.Start()

	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	session.outputMu.Lock()
	output := session.outputBuf.String()
	session.outputMu.Unlock()
	if !strings.Contains(output, "output-final") {
		t.Fatalf("output final não foi drenado: %q", output)
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
			reader := io.MultiReader(strings.NewReader(marker.StartTag()+"\nparcial"), iotest.ErrReader(tc.readerErr))
			session := &Session{
				ptySession: &fakePTYSession{},
				ptyReader:  reader,
				readerCtx:  t.Context(),
			}
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
		ptyReader: io.MultiReader(strings.NewReader(marker.StartTag()+"\nparcial"), iotest.ErrReader(readErr)),
	}
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

func TestReadFailureWhileWritingCommand(t *testing.T) {
	for _, started := range []bool{false, true} {
		name := "antes_do_inicio"
		if started {
			name = "apos_inicio"
		}
		t.Run(name, func(t *testing.T) {
			pty := &failingWritePTY{wrapper: make(chan string, 1), released: make(chan struct{})}
			reader, writer := io.Pipe()
			defer func() { _ = reader.Close() }()
			defer func() { _ = writer.Close() }()
			var emitted strings.Builder
			session := &Session{id: "test", shell: "bash", ptySession: pty, ptyReader: reader, readerCtx: t.Context(),
				onOutput: func(_, _, output string) { emitted.WriteString(output) },
			}
			session.Start()
			manager := NewManager(DefaultManagerConfig(), nil)
			manager.sessions[session.id] = session
			type outcome struct {
				entry *HistoryEntry
				err   error
			}
			finished := make(chan outcome, 1)
			go func() {
				entry, err := manager.RunCommand(t.Context(), session.id, "comando", time.Minute, "test")
				finished <- outcome{entry, err}
			}()

			var wrapper string
			select {
			case wrapper = <-pty.wrapper:
			case <-time.After(time.Second):
				t.Fatal("escrita do comando não iniciou")
			}
			raw, want := wrapper+"\n", ""
			if started {
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
			readErr := errors.New("leitura interrompida")
			if err := writer.CloseWithError(readErr); err != nil {
				t.Fatal(err)
			}
			select {
			case result := <-finished:
				if !errors.Is(result.err, readErr) || !errors.Is(result.err, io.ErrClosedPipe) || result.entry == nil ||
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
			session.outputBuf.WriteString("echo\n" + marker.StartTag() + "\n" + first)
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
					session.outputBuf.WriteString("\n" + marker.EndTag() + "1\nprompt")
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
