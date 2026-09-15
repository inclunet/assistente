package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"assistente/internal/commandruntime"
)

type appLifecyclePort struct {
	generation commandruntime.Generation
}

func (p *appLifecyclePort) Authenticate(context.Context) error { return nil }
func (p *appLifecyclePort) Recover(context.Context, commandruntime.Generation) error {
	return nil
}
func (p *appLifecyclePort) Project(_ context.Context, generation commandruntime.Generation) (commandruntime.Projection, error) {
	return commandruntime.Projection{Generation: generation, Entries: 1, Value: struct{}{}}, nil
}
func (p *appLifecyclePort) Publish(context.Context, commandruntime.Projection) error { return nil }
func (p *appLifecyclePort) Clear(context.Context, commandruntime.Generation) error   { return nil }
func (p *appLifecyclePort) SetEnabled(_ context.Context, generation commandruntime.Generation, enabled bool) error {
	if enabled {
		p.generation = generation
	}
	return nil
}
func (p *appLifecyclePort) ValidateCurrent(context.Context, commandruntime.Generation, commandruntime.Boundary) error {
	return nil
}
func (p *appLifecyclePort) Authorize(context.Context, commandruntime.Generation, commandruntime.Boundary) error {
	return nil
}
func (p *appLifecyclePort) Commit(_ context.Context, _ commandruntime.Generation, commit func() error) error {
	return commit()
}
func (p *appLifecyclePort) Begin(context.Context) (commandruntime.Generation, error) {
	p.generation = commandruntime.Generation{Value: "trusted-generation"}
	return p.generation, nil
}
func (p *appLifecyclePort) Invalidate(context.Context, commandruntime.Generation, string) error {
	return nil
}
func (p *appLifecyclePort) PublishReadiness(context.Context, commandruntime.Snapshot) error {
	return nil
}

type appLifecycleReadinessPort func(context.Context, commandruntime.Snapshot) error

func (f appLifecycleReadinessPort) Publish(ctx context.Context, snapshot commandruntime.Snapshot) error {
	return f(ctx, snapshot)
}

func appLifecycleConfig(probe *appLifecyclePort) commandruntime.Config {
	return commandruntime.Config{
		Authenticator: probe,
		Recovery:      probe,
		Projector:     probe,
		Publisher:     probe,
		Inputs:        probe,
		Core:          probe,
		Generations:   probe,
		Readiness:     appLifecycleReadinessPort(probe.PublishReadiness),
	}
}

func TestAppCommandLifecycleBridgeRequiresMountedRuntimeAndRunsTransitions(t *testing.T) {
	app := &App{}
	if _, err := CommandLifecycleSnapshot(app); err == nil {
		t.Fatal("snapshot criou runtime sem bootstrap confiável")
	}
	probe := &appLifecyclePort{}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	ready, err := CommandLifecycleSnapshot(app)
	if err != nil || ready.State != commandruntime.StateReady || !ready.Published {
		t.Fatalf("bridge não publicou readiness: %+v, %v", ready, err)
	}
	if err := ResetCommandLifecycle(context.Background(), app, "logout"); err != nil {
		t.Fatal(err)
	}
	if snapshot, _ := CommandLifecycleSnapshot(app); snapshot.State != commandruntime.StateCold || snapshot.Published {
		t.Fatalf("bridge reteve estado após reset: %+v", snapshot)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	if _, err := CommandLifecycleSnapshot(app); err == nil {
		t.Fatal("runtime permaneceu montado após shutdown")
	}
}

func TestAppCommandLifecycleConfigureCASAllowsOnlyOneInstance(t *testing.T) {
	app := &App{}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			<-start
			results <- ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{}))
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, errCommandLifecycleAlreadyConfigured) {
			t.Fatalf("erro inesperado no CAS de configuração: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("configurações vencedoras = %d, esperado 1", successes)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleShutdownCASRemovesSameInstance(t *testing.T) {
	app := &App{}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			results <- ShutdownCommandLifecycle(context.Background(), app)
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, commandruntime.ErrInvalidConfiguration) && !errors.Is(err, errCommandLifecycleAlreadyConfigured) {
			t.Fatalf("erro inesperado no CAS de shutdown: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("shutdowns vencedores = %d, esperado 1", successes)
	}
	if _, err := CommandLifecycleSnapshot(app); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("instância permaneceu montada após shutdown: %v", err)
	}
}

func TestAppCommandLifecycleOperationsFailClosedWithoutConfiguration(t *testing.T) {
	app := &App{}
	if err := BootstrapCommandLifecycle(context.Background(), app); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("bootstrap sem montagem foi aceito: %v", err)
	}
	if err := ResetCommandLifecycle(context.Background(), app, "logout"); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("reset sem montagem foi aceito: %v", err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("shutdown sem montagem foi aceito: %v", err)
	}
}
