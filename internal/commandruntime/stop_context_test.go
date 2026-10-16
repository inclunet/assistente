package commandruntime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStopSignalsWorkerWhenCallerContextIsAlreadyCancelled(t *testing.T) {
	runtime, err := New((&lifecycleProbe{}).config())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.Stop(ctx); !errors.Is(err, context.Canceled) && !errors.Is(err, ErrStopped) {
		t.Fatalf("Stop cancelado retornou erro inesperado: %v", err)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := runtime.WaitStopped(waitCtx); err != nil {
		t.Fatalf("sinal de Stop não encerrou o worker após contexto cancelado: %v", err)
	}
}

func TestStopTimeoutWhileWorkerCannotReadOperationsStillStopsAfterRelease(t *testing.T) {
	probe := &lifecycleProbe{clearStarted: make(chan struct{}), clearRelease: make(chan struct{})}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	resetResult := make(chan error, 1)
	go func() { resetResult <- runtime.Reset(context.Background(), "test") }()
	select {
	case <-probe.clearStarted:
	case <-time.After(time.Second):
		t.Fatal("reset não entrou no cleanup")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	stopResult := make(chan error, 1)
	go func() { stopResult <- runtime.Stop(ctx) }()
	select {
	case err := <-stopResult:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop=%v", err)
		}
	case <-time.After(time.Second):
		close(probe.clearRelease)
		t.Fatal("Stop bloqueou ao enviar operação ao worker ocupado")
	}
	close(probe.clearRelease)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := runtime.WaitStopped(waitCtx); err != nil {
		t.Fatal(err)
	}
	<-resetResult
}

func TestRebootstrapReadinessFailureStillClearsPreviousGeneration(t *testing.T) {
	probe := &lifecycleProbe{}
	config := probe.config()
	boots := 0
	config.Readiness = readinessFunc(func(_ context.Context, snapshot Snapshot) error {
		if snapshot.State == StateBootstrapping {
			boots++
			if boots == 2 {
				return errors.New("readiness unavailable")
			}
		}
		return nil
	})
	runtime, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Stop(context.Background()) }()
	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	old := runtime.Snapshot().Generation
	if err := runtime.Bootstrap(context.Background()); err == nil {
		t.Fatal("bootstrap aceitou falha de readiness")
	}
	if len(probe.cleared) == 0 || probe.cleared[0] != old || len(probe.disabled) == 0 || probe.disabled[0] != old {
		t.Fatalf("geração anterior não retirada: old=%v cleared=%v disabled=%v", old, probe.cleared, probe.disabled)
	}
}
