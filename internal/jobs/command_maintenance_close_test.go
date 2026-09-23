package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
)

func TestCloseCommandMaintenanceCancelsAndJoinsRetention(t *testing.T) {
	entered := make(chan struct{})
	released := make(chan struct{})
	coordinator := closeTestCoordinator(func(ctx context.Context) (commandmaintenance.BatchResult, error) {
		close(entered)
		<-released
		return commandmaintenance.BatchResult{}, ctx.Err()
	})
	m := mustNewManager(t, ManagerConfig{MaintenanceCoordinator: coordinator})
	m.startRetentionLoop(context.Background())
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("retenção não iniciou")
	}

	timeout, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := m.CloseCommandMaintenance(timeout); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fechamento com timeout=%v", err)
	}
	if !m.commandMaintenanceClosed {
		t.Fatal("timeout reabriu manutenção")
	}

	joined := make(chan error, 1)
	go func() { joined <- m.CloseCommandMaintenance(context.Background()) }()
	select {
	case err := <-joined:
		t.Fatalf("segunda chamada liberou antes do join: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(released)
	select {
	case err := <-joined:
		if err != nil {
			t.Fatalf("retentativa após drenagem=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("segunda chamada não fez join após liberação")
	}
	if err := m.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatalf("terceira chamada idempotente=%v", err)
	}
}

func TestCloseCommandMaintenanceIsIdempotentAndStopDoesNotDeadlock(t *testing.T) {
	entered := make(chan struct{})
	released := make(chan struct{})
	coordinator := closeTestCoordinator(func(ctx context.Context) (commandmaintenance.BatchResult, error) {
		close(entered)
		<-released
		return commandmaintenance.BatchResult{}, ctx.Err()
	})
	m := mustNewManager(t, ManagerConfig{MaintenanceCoordinator: coordinator})
	m.started = true
	m.startRetentionLoop(context.Background())
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("retenção não iniciou")
	}

	closed := make(chan error, 1)
	go func() { closed <- m.CloseCommandMaintenance(context.Background()) }()
	select {
	case <-closed:
		t.Fatal("fechamento retornou antes da retenção terminar")
	case <-time.After(20 * time.Millisecond):
	}

	stopped := make(chan struct{})
	go func() { m.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop retornou antes da retenção terminar")
	case <-time.After(20 * time.Millisecond):
	}
	close(released)

	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("fechamento=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fechamento não drenou retenção")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop não terminou após drenagem")
	}
	if err := m.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatalf("idempotência=%v", err)
	}
}

func TestCloseCommandMaintenanceRejectsNilInputs(t *testing.T) {
	var nilManager *Manager
	if err := nilManager.CloseCommandMaintenance(context.Background()); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("manager nil=%v", err)
	}
	m := &Manager{}
	if err := m.CloseCommandMaintenance(nil); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("contexto nil=%v", err)
	}
}

func TestStartRejectsManagerAfterCommandMaintenanceClose(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	m := mustNewManager(t, ManagerConfig{Repository: repo})
	m.commandMaintenanceClosed = true
	if err := m.Start(); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("Start após fechamento=%v", err)
	}
}

type closeTestHeartbeat func(context.Context) (commandmaintenance.BatchResult, error)

func (f closeTestHeartbeat) Heartbeat(ctx context.Context, _ commandmaintenance.Policy) (commandmaintenance.BatchResult, error) {
	return f(ctx)
}

func closeTestCoordinator(heartbeat closeTestHeartbeat) *commandmaintenance.Coordinator {
	port := commandMaintenanceNoopPort{}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Heartbeat: heartbeat,
		Outbox:    port, Decisions: port, Invocations: port, Claims: port,
		Jobs: port, Tools: port, InvocationDB: port, Activations: port,
		Compaction: &recordingMaintenance{},
	})
	if err != nil {
		panic(err)
	}
	return coordinator
}
