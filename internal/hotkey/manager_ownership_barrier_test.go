package hotkey

import (
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	nativehotkey "golang.design/x/hotkey"
)

type ownershipBarrierTestEvents struct {
	mu     sync.Mutex
	values []string
}

func (e *ownershipBarrierTestEvents) add(value string) {
	e.mu.Lock()
	e.values = append(e.values, value)
	e.mu.Unlock()
}

func (e *ownershipBarrierTestEvents) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.values...)
}

type ownershipBarrierTestNative struct {
	events        *ownershipBarrierTestEvents
	registerErr   error
	unregisterErr error
	down          chan nativehotkey.Event
	closeOnce     sync.Once
}

func (n *ownershipBarrierTestNative) Register() error {
	n.events.add("native-register")
	return n.registerErr
}

func (n *ownershipBarrierTestNative) Unregister() error {
	n.events.add("native-unregister")
	n.closeOnce.Do(func() { close(n.down) })
	return n.unregisterErr
}

func (n *ownershipBarrierTestNative) Keydown() <-chan nativehotkey.Event { return n.down }

type ownershipBarrierTestFactory struct {
	events        *ownershipBarrierTestEvents
	registerErr   error
	unregisterErr error
}

func (f *ownershipBarrierTestFactory) make([]nativehotkey.Modifier, nativehotkey.Key) nativeHotkey {
	return &ownershipBarrierTestNative{
		events:        f.events,
		registerErr:   f.registerErr,
		unregisterErr: f.unregisterErr,
		down:          make(chan nativehotkey.Event),
	}
}

type ownershipBarrierTestHarness struct {
	barrier *OwnershipBarrier
	events  *ownershipBarrierTestEvents
	autoAck bool
	mu      sync.Mutex
	frames  []OwnershipFrame
}

func newOwnershipBarrierTestHarness(events *ownershipBarrierTestEvents, autoAck bool) *ownershipBarrierTestHarness {
	h := &ownershipBarrierTestHarness{events: events, autoAck: autoAck}
	h.barrier = NewOwnershipBarrier(func(frame OwnershipFrame) {
		events.add("publish")
		h.mu.Lock()
		h.frames = append(h.frames, frame)
		h.mu.Unlock()
		if h.autoAck {
			events.add("ack")
			if !h.barrier.Ack(frame.InstanceID, frame.Revision) {
				events.add("ack-rejected")
			}
		}
	})
	return h
}

func (h *ownershipBarrierTestHarness) frameSnapshot() []OwnershipFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]OwnershipFrame(nil), h.frames...)
}

func newOwnershipBarrierTestManager(t *testing.T, harness *ownershipBarrierTestHarness, factory *ownershipBarrierTestFactory) *Manager {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier do Manager é suportado somente no Windows")
	}
	m := newManager(factory.make)
	if err := m.SetOwnershipBarrier(harness.barrier); err != nil {
		t.Fatalf("SetOwnershipBarrier() error = %v", err)
	}
	return m
}

func TestManagerOwnershipBarrierPublishesAndACKsBeforeNativeRegister(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	harness := newOwnershipBarrierTestHarness(events, true)
	defer harness.barrier.Close()
	m := newOwnershipBarrierTestManager(t, harness, &ownershipBarrierTestFactory{events: events})

	if _, err := m.Register([]nativehotkey.Modifier{ModCtrl}, nativehotkey.KeyA, func() {}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got := events.snapshot()
	want := []string{"publish", "ack", "native-register"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	m.UnregisterAll()
}

func TestManagerOwnershipBarrierRollsBackWhenNativeRegisterFails(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	harness := newOwnershipBarrierTestHarness(events, true)
	defer harness.barrier.Close()
	nativeErr := errors.New("native registration failed")
	m := newOwnershipBarrierTestManager(t, harness, &ownershipBarrierTestFactory{
		events:      events,
		registerErr: nativeErr,
	})

	if _, err := m.Register(nil, nativehotkey.KeyB, func() {}); !errors.Is(err, nativeErr) {
		t.Fatalf("Register() error = %v, want native error", err)
	}

	got := events.snapshot()
	want := []string{"publish", "ack", "native-register", "publish", "ack"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want proposal/native/rollback order %v", got, want)
	}
	if snapshot := m.SnapshotGlobalOwnership(); len(snapshot.Combinations) != 0 {
		t.Fatalf("ownership after failed native registration = %#v, want empty", snapshot)
	}
}

func TestManagerOwnershipBarrierFailurePreventsNativeRegister(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	var barrier *OwnershipBarrier
	barrier = NewOwnershipBarrier(func(OwnershipFrame) {
		events.add("publish")
		barrier.Close()
	})
	defer barrier.Close()
	m := newOwnershipBarrierTestManager(t, &ownershipBarrierTestHarness{barrier: barrier, events: events}, &ownershipBarrierTestFactory{events: events})

	if _, err := m.Register(nil, nativehotkey.KeyC, func() {}); err == nil {
		t.Fatal("Register() succeeded after ownership publication failure")
	}

	if got := events.snapshot(); !reflect.DeepEqual(got, []string{"publish"}) {
		t.Fatalf("events = %v, want only failed publication", got)
	}
}

func TestManagerOwnershipBarrierRollsBackFailedProposalBeforeNativeRegister(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	var barrier *OwnershipBarrier
	var publications atomic.Int32
	barrier = NewOwnershipBarrier(func(frame OwnershipFrame) {
		switch publications.Add(1) {
		case 2:
			events.add("publish-panic")
			panic("UI rejected ownership proposal")
		default:
			events.add("publish-ack")
			if !barrier.Ack(frame.InstanceID, frame.Revision) {
				events.add("ack-rejected")
			}
		}
	})
	defer barrier.Close()
	factory := &ownershipBarrierTestFactory{events: events}
	m := newManager(factory.make)
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier do Manager é suportado somente no Windows")
	}
	if err := m.SetOwnershipBarrier(barrier); err != nil {
		t.Fatalf("SetOwnershipBarrier() error = %v", err)
	}
	if _, err := m.Register(nil, nativehotkey.KeyA, func() {}); err != nil {
		t.Fatalf("initial Register() error = %v", err)
	}
	if _, err := m.Register(nil, nativehotkey.KeyB, func() {}); err == nil {
		t.Fatal("Register() succeeded after publisher panic")
	}

	got := events.snapshot()
	want := []string{"publish-ack", "native-register", "publish-panic", "publish-ack"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want proposal/panic/rollback order %v", got, want)
	}
	if snapshot := m.SnapshotGlobalOwnership(); len(snapshot.Combinations) != 1 || snapshot.Combinations[0].Key != nativehotkey.KeyA {
		t.Fatalf("native ownership after rollback = %#v, want only KeyA", snapshot)
	}
	if snapshot := barrier.Snapshot(); len(snapshot.Combinations) != 1 || snapshot.Combinations[0].Key != uint32(nativehotkey.KeyA) {
		t.Fatalf("published ownership after rollback = %#v, want only KeyA", snapshot)
	}
	m.UnregisterAll()
}

func TestManagerOwnershipBarrierRejectsInvalidWindowsCombinationBeforePublish(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	harness := newOwnershipBarrierTestHarness(events, true)
	defer harness.barrier.Close()
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier do Manager é suportado somente no Windows")
	}
	var factoryCalls atomic.Int32
	m := newManager(func([]nativehotkey.Modifier, nativehotkey.Key) nativeHotkey {
		factoryCalls.Add(1)
		return &ownershipBarrierTestNative{events: events, down: make(chan nativehotkey.Event)}
	})
	if err := m.SetOwnershipBarrier(harness.barrier); err != nil {
		t.Fatalf("SetOwnershipBarrier() error = %v", err)
	}
	for _, testCase := range []struct {
		name string
		mods []nativehotkey.Modifier
		key  nativehotkey.Key
	}{
		{name: "zero key", key: 0},
		{name: "key above byte", key: nativehotkey.Key(0xFF)},
		{name: "modifier outside Windows mask", mods: []nativehotkey.Modifier{nativehotkey.Modifier(0x10)}, key: nativehotkey.KeyA},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := m.Register(testCase.mods, testCase.key, func() {}); err == nil {
				t.Fatal("Register() accepted invalid Windows combination")
			}
		})
	}
	if got := factoryCalls.Load(); got != 0 {
		t.Fatalf("native factory calls = %d, want 0", got)
	}
	if got := events.snapshot(); len(got) != 0 {
		t.Fatalf("ownership events = %v, want no publication", got)
	}
}

func TestManagerOwnershipBarrierPublishesRemovalAfterNativeUnregister(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	harness := newOwnershipBarrierTestHarness(events, true)
	defer harness.barrier.Close()
	m := newOwnershipBarrierTestManager(t, harness, &ownershipBarrierTestFactory{events: events})

	id, err := m.Register(nil, nativehotkey.KeyD, func() {})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	events.mu.Lock()
	events.values = nil
	events.mu.Unlock()

	if err := m.Unregister(id); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
	got := events.snapshot()
	want := []string{"native-unregister", "publish", "ack"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want native removal before publication %v", got, want)
	}
}

func TestManagerOwnershipBarrierDoesNotPublishStaleSetDuringConcurrentRemovalAndRegister(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	harness := newOwnershipBarrierTestHarness(events, true)
	defer harness.barrier.Close()

	firstNative := &ownershipBarrierTestNative{
		events: events,
		down:   make(chan nativehotkey.Event),
	}
	removalStarted := make(chan struct{})
	allowRemoval := make(chan struct{})
	var removalOnce sync.Once
	first := &ownershipBarrierBlockingNative{
		ownershipBarrierTestNative: firstNative,
		started:                    removalStarted,
		allow:                      allowRemoval,
		startOnce:                  &removalOnce,
	}
	var factoryCalls atomic.Int32
	factory := func([]nativehotkey.Modifier, nativehotkey.Key) nativeHotkey {
		if factoryCalls.Add(1) == 1 {
			return first
		}
		return &ownershipBarrierTestNative{events: events, down: make(chan nativehotkey.Event)}
	}
	m := newManager(factory)
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier do Manager é suportado somente no Windows")
	}
	if err := m.SetOwnershipBarrier(harness.barrier); err != nil {
		t.Fatalf("SetOwnershipBarrier() error = %v", err)
	}

	firstID, err := m.Register(nil, nativehotkey.KeyG, func() {})
	if err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	before := len(harness.frameSnapshot())
	removalDone := make(chan error, 1)
	go func() { removalDone <- m.Unregister(firstID) }()
	select {
	case <-removalStarted:
	case <-time.After(time.Second):
		t.Fatal("native removal did not start")
	}

	newID := make(chan int, 1)
	newErr := make(chan error, 1)
	go func() {
		id, registerErr := m.Register(nil, nativehotkey.KeyH, func() {})
		newID <- id
		newErr <- registerErr
	}()
	select {
	case err := <-newErr:
		if err != nil {
			t.Fatalf("concurrent Register() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent Register() did not complete while native removal was blocked")
	}
	<-newID

	frames := harness.frameSnapshot()
	if len(frames) != before+1 {
		t.Fatalf("published frames before removal completion = %d, want %d", len(frames), before+1)
	}
	if got := len(frames[len(frames)-1].Combinations); got != 2 {
		t.Fatalf("concurrent registration frame combinations = %d, want old+new", got)
	}

	close(allowRemoval)
	if err := <-removalDone; err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
	finalFrames := harness.frameSnapshot()
	if got := len(finalFrames[len(finalFrames)-1].Combinations); got != 1 {
		t.Fatalf("final removal frame combinations = %d, want only new", got)
	}
	m.UnregisterAll()
}

func TestManagerOwnershipBarrierKeepsReservationWhenNativeUnregisterFails(t *testing.T) {
	events := &ownershipBarrierTestEvents{}
	harness := newOwnershipBarrierTestHarness(events, true)
	defer harness.barrier.Close()
	nativeErr := errors.New("native removal failed")
	m := newOwnershipBarrierTestManager(t, harness, &ownershipBarrierTestFactory{
		events:        events,
		unregisterErr: nativeErr,
	})

	id, err := m.Register(nil, nativehotkey.KeyE, func() {})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	events.mu.Lock()
	events.values = nil
	events.mu.Unlock()

	if err := m.Unregister(id); !errors.Is(err, nativeErr) {
		t.Fatalf("Unregister() error = %v, want native error", err)
	}
	if got := events.snapshot(); !reflect.DeepEqual(got, []string{"native-unregister"}) {
		t.Fatalf("events = %v, want no removal publication after native failure", got)
	}
	if snapshot := m.SnapshotGlobalOwnership(); len(snapshot.Combinations) != 1 {
		t.Fatalf("ownership after failed native removal = %#v, want reservation retained", snapshot)
	}
}

func TestManagerOwnershipBarrierCanOnlyBeConfiguredOnceBeforeRegistration(t *testing.T) {
	if runtime.GOOS != "windows" {
		m := newManager(nil)
		if err := m.SetOwnershipBarrier(NewOwnershipBarrier(func(OwnershipFrame) {})); !errors.Is(err, ErrOwnershipBarrierUnsupported) {
			t.Fatalf("SetOwnershipBarrier() on %s error = %v, want unsupported", runtime.GOOS, err)
		}
		return
	}

	first := newOwnershipBarrierTestHarness(&ownershipBarrierTestEvents{}, true)
	defer first.barrier.Close()
	second := newOwnershipBarrierTestHarness(&ownershipBarrierTestEvents{}, true)
	defer second.barrier.Close()
	m := newManager(nil)
	if err := m.SetOwnershipBarrier(first.barrier); err != nil {
		t.Fatalf("first SetOwnershipBarrier() error = %v", err)
	}
	if err := m.SetOwnershipBarrier(second.barrier); !errors.Is(err, ErrOwnershipBarrierConfigured) {
		t.Fatalf("second SetOwnershipBarrier() error = %v, want configured", err)
	}
	if _, err := m.Register(nil, nativehotkey.KeyF, func() {}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := m.SetOwnershipBarrier(second.barrier); !errors.Is(err, ErrOwnershipBarrierConfigured) {
		t.Fatalf("SetOwnershipBarrier() after Register error = %v, want configured", err)
	}
	m.UnregisterAll()
}

type ownershipBarrierBlockingNative struct {
	*ownershipBarrierTestNative
	started   chan struct{}
	allow     chan struct{}
	startOnce *sync.Once
}

func (n *ownershipBarrierBlockingNative) Unregister() error {
	n.startOnce.Do(func() { close(n.started) })
	<-n.allow
	return n.ownershipBarrierTestNative.Unregister()
}
