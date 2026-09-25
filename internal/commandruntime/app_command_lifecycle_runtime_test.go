package commandruntime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type lifecycleProbe struct {
	mu               sync.Mutex
	steps            []string
	next             int
	failProject      bool
	empty            bool
	projectBlock     bool
	projectStarted   chan struct{}
	clearError       bool
	clearStarted     chan struct{}
	clearRelease     chan struct{}
	clearOnce        sync.Once
	readinessError   bool
	blockReady       bool
	readyStarted     chan struct{}
	readyRelease     chan struct{}
	generations      []Generation
	invalidated      []Generation
	disabled         []Generation
	cleared          []Generation
	invalidateCalled chan struct{}
	readiness        []Snapshot
}

func (p *lifecycleProbe) step(name string) {
	p.mu.Lock()
	p.steps = append(p.steps, name)
	p.mu.Unlock()
}

func (p *lifecycleProbe) Authenticate(context.Context) error { p.step("authenticate"); return nil }

func (p *lifecycleProbe) Recover(_ context.Context, _ Generation) error {
	p.step("recover")
	return nil
}

func (p *lifecycleProbe) Project(ctx context.Context, generation Generation) (Projection, error) {
	p.step("project")
	if p.projectBlock {
		close(p.projectStarted)
		<-ctx.Done()
	}
	if p.failProject {
		return Projection{}, errors.New("projection failed")
	}
	if p.empty {
		return Projection{Generation: generation, Value: map[string]string{}}, nil
	}
	return Projection{Generation: generation, Entries: 1, Value: map[string]string{"command": "real"}}, nil
}

func (p *lifecycleProbe) Publish(_ context.Context, projection Projection) error {
	p.step("publish")
	if projection.Entries == 0 {
		return errors.New("probe recebeu projeção vazia")
	}
	return nil
}

func (p *lifecycleProbe) Clear(_ context.Context, generation Generation) error {
	p.step("clear")
	p.mu.Lock()
	p.cleared = append(p.cleared, generation)
	p.mu.Unlock()
	if p.clearStarted != nil {
		p.clearOnce.Do(func() { close(p.clearStarted) })
		<-p.clearRelease
	}
	if p.clearError {
		return errors.New("clear failed")
	}
	return nil
}

func (p *lifecycleProbe) SetEnabled(_ context.Context, generation Generation, enabled bool) error {
	if enabled {
		p.step("enable")
	} else {
		p.mu.Lock()
		p.disabled = append(p.disabled, generation)
		p.mu.Unlock()
		p.step("disable")
	}
	return nil
}
func (p *lifecycleProbe) ValidateCurrent(_ context.Context, _ Generation, boundary Boundary) error {
	p.step("validate:" + string(boundary))
	return nil
}
func (p *lifecycleProbe) Authorize(_ context.Context, _ Generation, boundary Boundary) error {
	p.step("authorize:" + string(boundary))
	return nil
}
func (p *lifecycleProbe) Commit(ctx context.Context, _ Generation, commit func() error) error {
	p.step("commit")
	if err := ctx.Err(); err != nil {
		return err
	}
	return commit()
}

func (p *lifecycleProbe) Begin(_ context.Context) (Generation, error) {
	p.mu.Lock()
	p.next++
	generation := Generation{Value: string(rune('0' + p.next))}
	p.generations = append(p.generations, generation)
	p.mu.Unlock()
	p.step("begin")
	return generation, nil
}

func (p *lifecycleProbe) Invalidate(_ context.Context, generation Generation, _ string) error {
	p.mu.Lock()
	p.invalidated = append(p.invalidated, generation)
	if p.invalidateCalled != nil && len(p.invalidated) == 1 {
		close(p.invalidateCalled)
	}
	p.mu.Unlock()
	p.step("invalidate")
	return nil
}

func (p *lifecycleProbe) PublishReadiness(_ context.Context, snapshot Snapshot) error {
	p.mu.Lock()
	p.readiness = append(p.readiness, snapshot)
	p.mu.Unlock()
	if p.readinessError {
		return errors.New("readiness failed")
	}
	if snapshot.State == StateReady && p.blockReady {
		close(p.readyStarted)
		<-p.readyRelease
	}
	return nil
}

func (p *lifecycleProbe) config() Config {
	return Config{Authenticator: p, Recovery: p, Projector: p, Publisher: p, Inputs: p, Core: p, Generations: p, Readiness: readinessFunc(p.PublishReadiness)}
}

type readinessFunc func(context.Context, Snapshot) error

func (f readinessFunc) Publish(ctx context.Context, snapshot Snapshot) error { return f(ctx, snapshot) }

func TestAppCommandLifecycleRejectsMissingPorts(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("New aceitou configuração incompleta: %v", err)
	}
}

func TestAppCommandLifecycleMountedSpecRequiresExplicitDependencies(t *testing.T) {
	probe := &lifecycleProbe{}
	spec := MountSpec{Config: probe.config(), Dependencies: []MountDependency{
		{Role: MountDependencyCatalog, Name: "catalogo-persistido", Instance: struct{}{}},
		{Role: MountDependencyDefaults, Name: "defaults-versionados", Instance: struct{}{}},
		{Role: MountDependencyPolicies, Name: "politicas", Instance: struct{}{}},
		{Role: MountDependencyStores, Name: "stores-sql", Instance: struct{}{}},
		{Role: MountDependencyPresenter, Name: "presenter-app", Instance: struct{}{}},
		{Role: MountDependencyProviders, Name: "providers-contexto", Instance: struct{}{}},
		{Role: MountDependencyDispatcher, Name: "dispatcher-core", Instance: struct{}{}},
		{Role: MountDependencyAdapters, Name: "adapters-fisicos", Instance: struct{}{}},
	}}
	runtime, err := NewMounted(spec)
	if err != nil {
		t.Fatalf("spec completo recusado: %v", err)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	missing := spec
	missing.Dependencies = missing.Dependencies[:len(missing.Dependencies)-1]
	if _, err := NewMounted(missing); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("spec sem adapters foi aceito/erro errado: %v", err)
	}

	duplicate := spec
	duplicate.Dependencies = append(append([]MountDependency(nil), spec.Dependencies...), MountDependency{Role: MountDependencyCatalog, Name: "outro-catalogo", Instance: struct{}{}})
	if _, err := NewMounted(duplicate); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("spec duplicado foi aceito/erro errado: %v", err)
	}

	nilDependency := spec
	nilDependency.Dependencies = append([]MountDependency(nil), spec.Dependencies...)
	nilDependency.Dependencies[0].Instance = (*lifecycleProbe)(nil)
	if _, err := NewMounted(nilDependency); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("spec com ponteiro nil foi aceito/erro errado: %v", err)
	}

	unknown := spec
	unknown.Dependencies = append([]MountDependency(nil), spec.Dependencies...)
	unknown.Dependencies[0].Role = MountDependencyRole("catalogo")
	if _, err := NewMounted(unknown); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("spec com role desconhecido foi aceito/erro errado: %v", err)
	}
}

func TestAppCommandLifecycleDoesNotReadyEmptyProjection(t *testing.T) {
	probe := &lifecycleProbe{empty: true}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Bootstrap(context.Background())
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("erro de projeção vazia = %v", err)
	}
	if got := runtime.Snapshot(); got.State != StateFailed || got.Published || got.PublishedEntries != 0 {
		t.Fatalf("projeção vazia exposta como pronta: %+v", got)
	}
	if err := runtime.WaitReady(context.Background()); !errors.Is(err, ErrNotReady) {
		t.Fatalf("WaitReady não observou falha: %v", err)
	}
	for _, snapshot := range probe.readiness {
		if snapshot.State == StateReady {
			t.Fatal("readiness publicada para projeção vazia")
		}
	}
}

func TestAppCommandLifecycleSerializesBootstrapAndInvalidatesOnReset(t *testing.T) {
	probe := &lifecycleProbe{}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() { results <- runtime.Bootstrap(context.Background()) }()
	go func() { results <- runtime.Bootstrap(context.Background()) }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if got := runtime.Snapshot(); got.State != StateReady || got.PublishedEntries != 1 {
		t.Fatalf("runtime não ficou pronto: %+v", got)
	}
	if len(probe.generations) != 2 || len(probe.invalidated) != 1 {
		t.Fatalf("transições/g gerações inesperadas: gerações=%v invalidadas=%v", probe.generations, probe.invalidated)
	}
	if err := runtime.Reset(context.Background(), "logout"); err != nil {
		t.Fatal(err)
	}
	if got := runtime.Snapshot(); got.State != StateCold || got.Published || got.Generation.Value != "" {
		t.Fatalf("reset reteve prontidão: %+v", got)
	}
	if len(probe.invalidated) != 2 {
		t.Fatalf("reset não invalidou geração: %v", probe.invalidated)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := runtime.Snapshot(); got.State != StateStopped {
		t.Fatalf("stop não encerrou: %+v", got)
	}
}

func TestAppCommandLifecycleClearsPublishedGenerationBeforeRebootstrap(t *testing.T) {
	probe := &lifecycleProbe{}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	old := probe.generations[0]
	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(probe.disabled) == 0 || probe.disabled[0] != old {
		t.Fatalf("geração antiga não foi desabilitada antes da nova: old=%v disabled=%v", old, probe.disabled)
	}
	if len(probe.cleared) == 0 || probe.cleared[0] != old {
		t.Fatalf("publicação antiga não foi limpa antes da nova: old=%v cleared=%v", old, probe.cleared)
	}
	probe.mu.Lock()
	steps := append([]string(nil), probe.steps...)
	probe.mu.Unlock()
	lastBegin := -1
	for i, step := range steps {
		if step == "begin" {
			lastBegin = i
		}
	}
	if lastBegin < 3 || steps[lastBegin-1] != "clear" || steps[lastBegin-2] != "disable" {
		t.Fatalf("limpeza da geração antiga não precedeu Begin: %v", steps)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleResetCancelsActiveProjectionBeforeQueue(t *testing.T) {
	probe := &lifecycleProbe{projectBlock: true, projectStarted: make(chan struct{}), invalidateCalled: make(chan struct{})}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	bootstrapDone := make(chan error, 1)
	go func() { bootstrapDone <- runtime.Bootstrap(context.Background()) }()
	select {
	case <-probe.projectStarted:
	case <-time.After(time.Second):
		t.Fatal("projetor não iniciou")
	}
	resetDone := make(chan error, 1)
	go func() { resetDone <- runtime.Reset(context.Background(), "logout") }()
	select {
	case <-probe.invalidateCalled:
	case <-time.After(time.Second):
		t.Fatal("Reset não invalidou a geração antes de aguardar a fila")
	}
	select {
	case err := <-resetDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Reset ficou preso no projetor ativo")
	}
	if err := <-bootstrapDone; err == nil {
		t.Fatal("bootstrap cancelado pelo reset foi aceito")
	}
	if got := runtime.Snapshot(); got.State != StateCold || got.Published {
		t.Fatalf("reset publicou estado antigo: %+v", got)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleStopTerminatesWorkerAfterCleanupFailure(t *testing.T) {
	probe := &lifecycleProbe{clearError: true}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Stop(context.Background()); err == nil {
		t.Fatal("Stop mascarou falha de limpeza")
	}
	if got := runtime.Snapshot(); got.State != StateStopped {
		t.Fatalf("worker não terminou após falha de limpeza: %+v", got)
	}
	if err := runtime.Bootstrap(context.Background()); !errors.Is(err, ErrStopped) {
		t.Fatalf("worker aceitou nova operação após Stop: %v", err)
	}
}

func TestAppCommandLifecycleWaitStoppedRequiresWorkerTermination(t *testing.T) {
	probe := &lifecycleProbe{clearStarted: make(chan struct{}), clearRelease: make(chan struct{})}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := runtime.Stop(stopCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop sem worker encerrado = %v", err)
	}
	select {
	case <-probe.clearStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup não iniciou")
	}
	if err := runtime.WaitStopped(stopCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitStopped confirmou worker bloqueado: %v", err)
	}
	close(probe.clearRelease)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := runtime.WaitStopped(waitCtx); err != nil {
		t.Fatalf("WaitStopped após release = %v", err)
	}
}

func TestAppCommandLifecycleRevalidatesEveryPublishBoundary(t *testing.T) {
	probe := &lifecycleProbe{}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"begin", "authenticate", "recover", "validate:after-recovery", "project", "validate:after-projection", "authorize:before-publish", "publish", "validate:after-publish", "authorize:before-enable", "enable", "validate:before-ready", "commit"}
	probe.mu.Lock()
	steps := append([]string(nil), probe.steps...)
	probe.mu.Unlock()
	for i := 0; i < len(steps); i++ {
		if steps[i] != want[0] {
			continue
		}
		matched := true
		for j := range want {
			if i+j >= len(steps) || steps[i+j] != want[j] {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("fronteiras não revalidadas antes de publicar/habilitar: got %v", steps)
}

func TestAppCommandLifecycleReadinessFailureNeverLeavesReadyState(t *testing.T) {
	probe := &lifecycleProbe{readinessError: true}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Bootstrap(context.Background()); err == nil {
		t.Fatal("falha de readiness foi mascarada")
	}
	if got := runtime.Snapshot(); got.State == StateReady || got.Published {
		t.Fatalf("snapshot pronto antes de readiness: %+v", got)
	}
}

func TestAppCommandLifecycleResetDuringReadyReadinessCannotStoreOldGeneration(t *testing.T) {
	probe := &lifecycleProbe{blockReady: true, readyStarted: make(chan struct{}), readyRelease: make(chan struct{}), invalidateCalled: make(chan struct{})}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	bootstrapDone := make(chan error, 1)
	go func() { bootstrapDone <- runtime.Bootstrap(context.Background()) }()
	select {
	case <-probe.readyStarted:
	case <-time.After(time.Second):
		t.Fatal("readiness de Ready não bloqueou")
	}
	resetDone := make(chan error, 1)
	go func() { resetDone <- runtime.Reset(context.Background(), "logout") }()
	select {
	case <-probe.invalidateCalled:
	case <-time.After(time.Second):
		t.Fatal("Reset não invalidou a geração durante readiness")
	}
	close(probe.readyRelease)
	if err := <-bootstrapDone; err == nil {
		t.Fatal("bootstrap publicou geração cancelada")
	}
	if err := <-resetDone; err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := runtime.WaitReady(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitReady observou readiness antiga: %v, snapshot=%+v", err, runtime.Snapshot())
	}
	if got := runtime.Snapshot(); got.State == StateReady || got.Published {
		t.Fatalf("geração cancelada ficou pronta: %+v", got)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleCancellationCleansWithoutOrphanReadiness(t *testing.T) {
	probe := &lifecycleProbe{}
	runtime, err := New(probe.config())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.Bootstrap(ctx); err == nil {
		t.Fatal("bootstrap cancelado foi aceito")
	}
	if got := runtime.Snapshot(); got.State == StateReady {
		t.Fatalf("cancelamento publicou pronto: %+v", got)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := runtime.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}
