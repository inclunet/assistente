package terminal

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
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
