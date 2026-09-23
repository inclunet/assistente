package commanddeck

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandbridge"
)

type lifecycleController struct{ events atomic.Int32 }

func (c *lifecycleController) Input(context.Context, commandadapter.Event) (commandbridge.InvocationAck, error) {
	c.events.Add(1)
	return commandbridge.InvocationAck{Accepted: true}, nil
}
func (*lifecycleController) Lock(context.Context) error   { return nil }
func (*lifecycleController) Logout(context.Context) error { return nil }

type lifecycleHandle struct {
	mu       sync.Mutex
	writes   []RenderPlan
	writeErr error
	closeErr error
	closes   int
	read     func(context.Context) (PhysicalKeyEvent, error)
	write    func(context.Context, RenderPlan) error
}

func (h *lifecycleHandle) Write(ctx context.Context, plan RenderPlan) error {
	if h.write != nil {
		return h.write(ctx, plan)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writes = append(h.writes, plan)
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.writeErr
}
func (h *lifecycleHandle) Read(ctx context.Context) (PhysicalKeyEvent, error) { return h.read(ctx) }
func (h *lifecycleHandle) Close(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closes++
	return h.closeErr
}
func (h *lifecycleHandle) closedCount() int { h.mu.Lock(); defer h.mu.Unlock(); return h.closes }

type lifecycleDriver struct {
	mu      sync.Mutex
	devices []PhysicalDevice
	handles []*lifecycleHandle
	opens   int
	open    func(context.Context, PhysicalDevice) (Handle, error)
}

func (d *lifecycleDriver) Enumerate(context.Context) ([]PhysicalDevice, error) { return d.devices, nil }
func (d *lifecycleDriver) Open(ctx context.Context, device PhysicalDevice) (Handle, error) {
	d.mu.Lock()
	index := d.opens
	d.opens++
	d.mu.Unlock()
	if d.open != nil {
		return d.open(ctx, device)
	}
	return d.handles[index], nil
}
func (d *lifecycleDriver) count() int { d.mu.Lock(); defer d.mu.Unlock(); return d.opens }
func lifecycleFixture(t *testing.T, handles ...*lifecycleHandle) (*Runtime, *Manager, *lifecycleController, *lifecycleDriver) {
	t.Helper()
	c := &lifecycleController{}
	m := NewManager(NewRenderer(), BackoffPolicy{Initial: time.Second, Max: 4 * time.Second})
	a, err := NewDeviceAdapter(m, c)
	if err != nil {
		t.Fatal(err)
	}
	d := &lifecycleDriver{devices: []PhysicalDevice{{ID: "deck-a", Model: testModel}}, handles: handles}
	r, err := NewRuntime(d, a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})
	return r, m, c, d
}
func mustDiscover(t *testing.T, r *Runtime) {
	t.Helper()
	if errs := r.Discover(context.Background()); len(errs) != 0 {
		t.Fatalf("discover: %v", errs)
	}
}

func TestRuntimeFailedSafeFrameRollsBackAndReconnects(t *testing.T) {
	fault := errors.New("safe frame write failed")
	first, second := &lifecycleHandle{writeErr: fault}, &lifecycleHandle{}
	r, m, _, d := lifecycleFixture(t, first, second)
	now := time.Now()
	m.SetClock(func() time.Time { return now })
	results := r.DiscoverDetailed(context.Background())
	if len(results) != 1 || !errors.Is(results[0].Err, fault) {
		t.Fatalf("results=%+v", results)
	}
	s, err := m.Snapshot("deck-a")
	if err != nil || s.Status != DeviceDisconnected || first.closedCount() != 1 {
		t.Fatalf("rollback: %+v err=%v closes=%d", s, err, first.closedCount())
	}
	results = r.DiscoverDetailed(context.Background())
	if !errors.Is(results[0].Err, ErrReconnectBackoff) || d.count() != 1 {
		t.Fatalf("backoff ignorado: %+v opens=%d", results, d.count())
	}
	now = now.Add(time.Second)
	mustDiscover(t, r)
	if len(second.writes) != 1 || !second.writes[0].FullFrame {
		t.Fatal("reconexão sem frame completo")
	}
}

func TestRuntimeFailedRenderDisconnectsInsteadOfCachingUnwrittenFrame(t *testing.T) {
	first, second := &lifecycleHandle{}, &lifecycleHandle{}
	r, m, _, _ := lifecycleFixture(t, first, second)
	now := time.Now()
	m.SetClock(func() time.Time { return now })
	mustDiscover(t, r)
	fault := errors.New("partial frame")
	first.mu.Lock()
	first.writeErr = fault
	first.mu.Unlock()
	frame := Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{0: {Title: "changed"}}}
	if err := r.Render(context.Background(), frame); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	if err := r.Render(context.Background(), frame); !errors.Is(err, ErrInvalidDevice) {
		t.Fatalf("render após falha=%v", err)
	}
	if first.closedCount() != 1 {
		t.Fatal("handle com falha não liberado")
	}
	now = now.Add(time.Second)
	mustDiscover(t, r)
	if err := r.Render(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if len(second.writes) != 2 || !second.writes[0].FullFrame || len(second.writes[1].Updates) != 1 {
		t.Fatalf("frame não reenviado: %+v", second.writes)
	}
}

func TestRuntimeCancelledPollDoesNotDisconnectHealthyHandle(t *testing.T) {
	started := make(chan struct{})
	h := &lifecycleHandle{read: func(ctx context.Context) (PhysicalKeyEvent, error) {
		close(started)
		<-ctx.Done()
		return PhysicalKeyEvent{}, ctx.Err()
	}}
	r, m, _, _ := lifecycleFixture(t, h)
	mustDiscover(t, r)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.PollOne(ctx, "deck-a") }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s, err := m.Snapshot("deck-a")
	if err != nil || s.Status != DeviceConnected || h.closedCount() != 0 {
		t.Fatalf("cancel desligou dispositivo: %+v %v", s, err)
	}
}

func TestRuntimeShutdownCancelsReadAndRejectsLateEvent(t *testing.T) {
	started := make(chan struct{})
	h := &lifecycleHandle{read: func(ctx context.Context) (PhysicalKeyEvent, error) {
		close(started)
		<-ctx.Done()
		return PhysicalKeyEvent{Index: 0, Down: true}, nil
	}}
	r, m, c, _ := lifecycleFixture(t, h)
	mustDiscover(t, r)
	done := make(chan error, 1)
	go func() { done <- r.PollOne(context.Background(), "deck-a") }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("evento tardio aceito")
	}
	if c.events.Load() != 0 || h.closedCount() != 1 {
		t.Fatalf("events=%d closes=%d", c.events.Load(), h.closedCount())
	}
	if err := r.Shutdown(ctx); err != nil || h.closedCount() != 1 {
		t.Fatalf("shutdown repetido: %v", err)
	}
	s, err := m.Snapshot("deck-a")
	if err != nil || s.Status != DeviceDisconnected {
		t.Fatalf("shutdown manager=%+v %v", s, err)
	}
}

func TestRuntimeShutdownDuringOpenClosesUnpublishedHandle(t *testing.T) {
	h := &lifecycleHandle{}
	r, m, _, d := lifecycleFixture(t, h)
	started := make(chan struct{})
	d.open = func(ctx context.Context, _ PhysicalDevice) (Handle, error) {
		close(started)
		<-ctx.Done()
		return h, nil
	}
	discovered := make(chan []DiscoverResult, 1)
	go func() { discovered <- r.DiscoverDetailed(context.Background()) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	results := <-discovered
	if len(results) != 1 || results[0].Opened || results[0].Err == nil {
		t.Fatalf("publicou abertura cancelada: %+v", results)
	}
	if h.closedCount() != 1 {
		t.Fatal("handle não publicado vazou")
	}
	if _, err := m.Snapshot("deck-a"); !errors.Is(err, ErrInvalidDevice) {
		t.Fatalf("abertura cancelada publicou manager: %v", err)
	}
}

func TestRuntimeConcurrentDiscoverOpensOnlyOnce(t *testing.T) {
	h := &lifecycleHandle{}
	r, _, _, d := lifecycleFixture(t, h)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.DiscoverDetailed(context.Background()) }()
	}
	wg.Wait()
	if d.count() != 1 {
		t.Fatalf("opens=%d", d.count())
	}
}

func TestRuntimeBlockedReadDoesNotBlockOtherDeviceRender(t *testing.T) {
	started := make(chan struct{})
	first := &lifecycleHandle{read: func(ctx context.Context) (PhysicalKeyEvent, error) {
		close(started)
		<-ctx.Done()
		return PhysicalKeyEvent{}, ctx.Err()
	}}
	second := &lifecycleHandle{}
	r, _, _, d := lifecycleFixture(t, first, second)
	d.devices = append(d.devices, PhysicalDevice{ID: "deck-b", Model: testModel})
	mustDiscover(t, r)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poll := make(chan error, 1)
	go func() { poll <- r.PollOne(ctx, "deck-a") }()
	<-started
	complete := make(chan error, 1)
	go func() {
		complete <- r.Render(context.Background(), Frame{Device: "deck-b", Model: testModel, Keys: map[int]KeyView{0: {Title: "B"}}})
	}()
	select {
	case err := <-complete:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("leitura de A bloqueou render de B")
	}
	cancel()
	<-poll
}

func TestRuntimeShutdownRetainsCleanupErrors(t *testing.T) {
	fault := errors.New("close failed")
	h := &lifecycleHandle{closeErr: fault}
	r, _, _, _ := lifecycleFixture(t, h)
	mustDiscover(t, r)
	for i := 0; i < 2; i++ {
		if err := r.Shutdown(context.Background()); !errors.Is(err, fault) {
			t.Fatalf("shutdown %d: %v", i, err)
		}
	}
	if h.closedCount() != 1 {
		t.Fatalf("closes=%d", h.closedCount())
	}
}

func TestRuntimeLockDuringSafeFrameDoesNotActivateNewGeneration(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	h := &lifecycleHandle{write: func(context.Context, RenderPlan) error { close(started); <-release; return nil }}
	r, m, _, _ := lifecycleFixture(t, h)
	done := make(chan []DiscoverResult, 1)
	go func() { done <- r.DiscoverDetailed(context.Background()) }()
	<-started
	m.LockOrLogout()
	close(release)
	results := <-done
	if len(results) != 1 || results[0].Opened || !errors.Is(results[0].Err, ErrInvalidDevice) {
		t.Fatalf("ativou geração não capturada: %+v", results)
	}
	state, err := m.Snapshot("deck-a")
	if err != nil || state.Status != DeviceDisconnected || h.closedCount() != 1 {
		t.Fatalf("rollback pós lock: %+v %v closes=%d", state, err, h.closedCount())
	}
}

func TestRuntimeRollbackReportsWriteAndCloseFailures(t *testing.T) {
	writeFault, closeFault := errors.New("write failed"), errors.New("close failed")
	h := &lifecycleHandle{writeErr: writeFault, closeErr: closeFault}
	r, _, _, _ := lifecycleFixture(t, h)
	results := r.DiscoverDetailed(context.Background())
	if len(results) != 1 || !errors.Is(results[0].Err, writeFault) || !errors.Is(results[0].Err, closeFault) {
		t.Fatalf("perdeu erro de cleanup: %+v", results)
	}
}

func TestRuntimeCancelledShutdownStillReleasesResources(t *testing.T) {
	h := &lifecycleHandle{}
	r, _, _, _ := lifecycleFixture(t, h)
	mustDiscover(t, r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = r.Shutdown(ctx)
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.closedCount() != 1 {
		t.Fatalf("cancelamento abandonou cleanup: %d", h.closedCount())
	}
}

func TestRuntimeReadCannotRetargetEventAfterGenerationChange(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	h := &lifecycleHandle{read: func(context.Context) (PhysicalKeyEvent, error) {
		close(started)
		<-release
		return PhysicalKeyEvent{Index: 0, Down: true}, nil
	}}
	r, m, c, _ := lifecycleFixture(t, h)
	mustDiscover(t, r)
	done := make(chan error, 1)
	go func() { done <- r.PollOne(context.Background(), "deck-a") }()
	<-started
	m.LockOrLogout()
	state, err := m.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Activate("deck-a", state.Generation); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrDeviceSafe) {
		t.Fatalf("evento antigo reaproveitado: %v", err)
	}
	if c.events.Load() != 0 {
		t.Fatal("callback antigo chegou ao controller")
	}
}

func TestRuntimeOldReadCannotDisconnectReopenedSerial(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	first := &lifecycleHandle{read: func(context.Context) (PhysicalKeyEvent, error) {
		close(started)
		<-release
		return PhysicalKeyEvent{}, errors.New("old read failed")
	}}
	second := &lifecycleHandle{}
	r, m, c, _ := lifecycleFixture(t, first, second)
	now := time.Now()
	m.SetClock(func() time.Time { return now })
	mustDiscover(t, r)
	done := make(chan error, 1)
	go func() { done <- r.PollOne(context.Background(), "deck-a") }()
	<-started
	first.mu.Lock()
	first.writeErr = errors.New("write failed")
	first.mu.Unlock()
	if err := r.Render(context.Background(), Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{0: {Title: "A"}}}); err == nil {
		t.Fatal("escrita deveria falhar")
	}
	now = now.Add(time.Second)
	mustDiscover(t, r)
	close(release)
	if err := <-done; err == nil {
		t.Fatal("leitura antiga aceita")
	}
	state, err := m.Snapshot("deck-a")
	if err != nil || state.Status != DeviceConnected || second.closedCount() != 0 || c.events.Load() != 0 {
		t.Fatalf("leitura antiga afetou nova abertura: %+v %v", state, err)
	}
}
