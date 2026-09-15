package jobs

import (
	"context"
	"errors"
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
