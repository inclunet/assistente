package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"assistente/internal/commandsecurity"
)

func TestExecutorDrainActionRollsBackRegistrationOnMountFailure(t *testing.T) {
	core := commandSecurityCoreForTest(t)
	var drained int
	want := errors.New("montagem falhou")
	err := core.RegisterExecutorDrainAction(context.Background(), func(context.Context) error {
		drained++
		return nil
	}, func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("erro de montagem=%v, want=%v", err, want)
	}
	if _, err := core.CloseAndDrain(context.Background()); err != nil {
		t.Fatalf("drain após rollback=%v", err)
	}
	if drained != 0 {
		t.Fatalf("drain de montagem abortada foi executado %d vezes", drained)
	}
}

func TestExecutorLifecyclePublicationIsAtomicWithCloseAndDrain(t *testing.T) {
	core := commandSecurityCoreForTest(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var published, drained atomic.Bool
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- core.RegisterExecutorDrainAction(context.Background(), func(context.Context) error {
			if !published.Load() {
				return errors.New("drain observado antes da publicação")
			}
			drained.Store(true)
			return nil
		}, func() error {
			once.Do(func() { close(started) })
			<-release
			published.Store(true)
			return nil
		})
	}()
	<-started
	drainDone := make(chan error, 1)
	go func() {
		_, err := core.CloseAndDrain(context.Background())
		drainDone <- err
	}()
	select {
	case err := <-drainDone:
		t.Fatalf("CloseAndDrain atravessou ação de montagem: %v", err)
	default:
	}
	close(release)
	if err := <-commitDone; err != nil {
		t.Fatalf("commit=%v", err)
	}
	if err := <-drainDone; err != nil {
		t.Fatalf("drain=%v", err)
	}
	if !published.Load() || !drained.Load() {
		t.Fatalf("ordem publicada/drain=%v/%v", published.Load(), drained.Load())
	}

	var actionRan atomic.Bool
	if err := core.WithExecutorLifecycle(context.Background(), func() error {
		actionRan.Store(true)
		return nil
	}); !errors.Is(err, commandsecurity.ErrStaleEpoch) || actionRan.Load() {
		t.Fatalf("lifecycle pós-drain executou ação: err=%v ran=%v", err, actionRan.Load())
	}
	if err := core.RegisterExecutorDrainAction(context.Background(), func(context.Context) error {
		actionRan.Store(true)
		return nil
	}, func() error {
		actionRan.Store(true)
		return nil
	}); !errors.Is(err, commandsecurity.ErrStaleEpoch) || actionRan.Load() {
		t.Fatalf("registro pós-drain executou ação: err=%v ran=%v", err, actionRan.Load())
	}
}

func commandSecurityCoreForTest(t *testing.T) *commandsecurity.EpochService {
	t.Helper()
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func TestCommandMaintenanceRealMountFailureCanRetryAndShutdown(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	ctx := context.Background()
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	if err := a.configureCommandMaintenance(ctx); err == nil {
		t.Fatal("montagem enquanto jobs estava iniciado deveria falhar")
	}
	a.jobMgr.Stop()
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatalf("retry após falha de montagem: %v", err)
	}
	if a.commandMaintenance.Load() == nil {
		t.Fatal("retry não publicou manutenção")
	}
	a.Shutdown()
	core, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if err := core.WithExecutorLifecycle(ctx, func() error {
		called = true
		return nil
	}); !errors.Is(err, commandsecurity.ErrStaleEpoch) || called {
		t.Fatalf("shutdown permitiu lifecycle: err=%v called=%v", err, called)
	}
}
