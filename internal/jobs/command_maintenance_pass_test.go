package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
)

func TestRetentionLoopHasSingleOwnerAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Manager{}
	m.startRetentionLoop(ctx)
	first := m.retentionDone
	m.startRetentionLoop(ctx)
	if m.retentionDone != first {
		t.Fatal("segundo loop criado")
	}
	cancel()
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("loop não terminou com contexto")
	}
}

func TestRetentionLoopCancellationBeforeTickDoesNotResumeMaintenance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var passes atomic.Int32
	heartbeat := maintenanceHeartbeatFunc(func(context.Context, commandmaintenance.Policy) (commandmaintenance.BatchResult, error) {
		passes.Add(1)
		return commandmaintenance.BatchResult{}, nil
	})
	empty := commandMaintenanceNoopPort{}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Heartbeat: heartbeat,
		Outbox:    empty, Decisions: empty, Invocations: empty, Claims: empty,
		Jobs: empty, Tools: empty, InvocationDB: empty, Activations: empty, Compaction: &recordingMaintenance{},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: ManagerConfig{MaintenanceCoordinator: coordinator}}
	m.startRetentionLoop(ctx)
	done := m.retentionDone
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop não terminou após cancelamento")
	}
	if got := passes.Load(); got != 0 {
		t.Fatalf("passagens após cancelamento=%d, want 0", got)
	}
}

func TestManagerStartCoordinatorMaintenanceRunsAfterStartLocks(t *testing.T) {
	repo, user, _ := setupJobsRepositoryTest(t)
	ctx, cancel := context.WithCancel(user)
	defer cancel()
	entered := make(chan bool, 1)
	var passes atomic.Int32
	m := &Manager{}
	heartbeat := maintenanceHeartbeatFunc(func(ctx context.Context, _ commandmaintenance.Policy) (commandmaintenance.BatchResult, error) {
		m.mu.Lock()
		m.runtimeMu.Lock()
		entered <- m.started
		passes.Add(1)
		m.runtimeMu.Unlock()
		m.mu.Unlock()
		<-ctx.Done()
		return commandmaintenance.BatchResult{}, ctx.Err()
	})
	empty := commandMaintenanceNoopPort{}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Heartbeat: heartbeat,
		Outbox:    empty, Decisions: empty, Invocations: empty, Claims: empty,
		Jobs: empty, Tools: empty, InvocationDB: empty, Activations: empty, Compaction: &recordingMaintenance{},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Build through NewManager so Start exercises the production lifecycle,
	// while the heartbeat remains an instrumented real coordinator port.
	m = mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return ctx }, MaintenanceCoordinator: coordinator})
	startDone := make(chan error, 1)
	go func() { startDone <- m.Start() }()
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("Start=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start não terminou; possível deadlock antes da cadência")
	}
	select {
	case started := <-entered:
		if !started {
			t.Fatal("manutenção iniciou antes da publicação do estado started")
		}
	case <-time.After(time.Second):
		t.Fatal("cadência não iniciou após Start")
	}

	stopDone := make(chan struct{})
	go func() { m.Stop(); close(stopDone) }()
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("Stop não cancelou e drenou a cadência")
	}
	if got := passes.Load(); got != 1 {
		t.Fatalf("passagens=%d, want 1 (um único loop)", got)
	}
}

func TestCommandMaintenanceDelayUsesLeaseAndEveryContinuation(t *testing.T) {
	p := commandMaintenanceTestPolicy()
	p.LeaseDuration = 30 * time.Second
	if got := commandMaintenanceDelay(p, commandmaintenance.Report{}, nil); got != 10*time.Second {
		t.Fatalf("delay=%v", got)
	}
	for _, r := range []commandmaintenance.Report{{MoreHeartbeat: true}, {MoreOutbox: true}, {MoreRecovery: true}, {MoreRetention: true}} {
		if got := commandMaintenanceDelay(p, r, nil); got != time.Second {
			t.Fatalf("continuação=%+v delay=%v", r, got)
		}
	}
	if got := commandMaintenanceDelay(p, commandmaintenance.Report{}, errors.New("erro")); got != time.Second {
		t.Fatalf("retry=%v", got)
	}
	p.LeaseDuration = 300 * time.Millisecond
	if got := commandMaintenanceDelay(p, commandmaintenance.Report{MoreHeartbeat: true}, nil); got != 100*time.Millisecond {
		t.Fatalf("TTL curto=%v", got)
	}
	p.LeaseDuration = time.Hour
	if got := commandMaintenanceDelay(p, commandmaintenance.Report{}, nil); got != time.Minute {
		t.Fatalf("releitura settings=%v", got)
	}
}

func TestCommandMaintenanceDelayKeepsContinuationWithinLeaseTTL(t *testing.T) {
	continuations := []struct {
		name   string
		report commandmaintenance.Report
		runErr error
	}{
		{name: "heartbeat", report: commandmaintenance.Report{MoreHeartbeat: true}},
		{name: "outbox", report: commandmaintenance.Report{MoreOutbox: true}},
		{name: "recovery", report: commandmaintenance.Report{MoreRecovery: true}},
		{name: "retention", report: commandmaintenance.Report{MoreRetention: true}},
		{name: "error", runErr: errors.New("transient")},
	}
	for _, tc := range continuations {
		t.Run(tc.name, func(t *testing.T) {
			p := commandMaintenanceTestPolicy()
			p.LeaseDuration = 1500 * time.Millisecond
			if got, want := commandMaintenanceDelay(p, tc.report, tc.runErr), 500*time.Millisecond; got != want {
				t.Fatalf("continuação delay=%v, want %v", got, want)
			}

			p.LeaseDuration = 30 * time.Second
			if got, want := commandMaintenanceDelay(p, tc.report, tc.runErr), time.Second; got != want {
				t.Fatalf("continuação longa delay=%v, want %v", got, want)
			}
		})
	}
}

type maintenanceHeartbeatFunc func(context.Context, commandmaintenance.Policy) (commandmaintenance.BatchResult, error)

func (f maintenanceHeartbeatFunc) Heartbeat(ctx context.Context, p commandmaintenance.Policy) (commandmaintenance.BatchResult, error) {
	return f(ctx, p)
}

func TestManagerStopJoinsMaintenanceWithoutHoldingManagerLocks(t *testing.T) {
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(release)
	empty := commandMaintenanceNoopPort{}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Heartbeat: maintenanceHeartbeatFunc(func(ctx context.Context, _ commandmaintenance.Policy) (commandmaintenance.BatchResult, error) {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			<-release
			return commandmaintenance.BatchResult{}, ctx.Err()
		}),
		Outbox: empty, Decisions: empty, Invocations: empty, Claims: empty,
		Jobs: empty, Tools: empty, InvocationDB: empty, Activations: empty, Compaction: &recordingMaintenance{},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := mustNewManager(t, ManagerConfig{MaintenanceCoordinator: coordinator})
	m.started = true
	m.startRetentionLoop(context.Background())
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("manutenção não iniciou")
	}
	stopped := make(chan struct{})
	go func() { m.Stop(); close(stopped) }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("Stop não cancelou passagem")
	}
	select {
	case <-stopped:
		t.Fatal("Stop retornou antes do adapter terminar")
	default:
	}
	if !m.mu.TryLock() {
		t.Fatal("Stop espera manutenção mantendo m.mu")
	}
	m.mu.Unlock()
	if !m.runtimeMu.TryLock() {
		t.Fatal("Stop espera manutenção mantendo runtimeMu")
	}
	m.runtimeMu.Unlock()
	if m.eventBus.closed {
		t.Fatal("dependência destruída antes da drenagem")
	}
	if err := m.Start(); !errors.Is(err, ErrCommandMaintenanceBusy) {
		t.Fatalf("Start durante Stop=%v", err)
	}
	// Libera o adapter sem fechar duas vezes no defer e aguarda ambos os Stops.
	second := make(chan struct{})
	go func() { m.Stop(); close(second) }()
	release <- struct{}{}
	for _, done := range []chan struct{}{stopped, second} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Stop não terminou após drenagem")
		}
	}
	if !m.eventBus.closed {
		t.Fatal("dependência não encerrada após drenagem")
	}
}
