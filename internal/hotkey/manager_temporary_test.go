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

type temporaryTestNative struct {
	down          chan nativehotkey.Event
	registerErr   error
	unregisterErr error
	unregisters   atomic.Int32
	owner         *temporaryTestFactory
}

func newTemporaryTestNative() *temporaryTestNative {
	return &temporaryTestNative{down: make(chan nativehotkey.Event, 8)}
}

func (n *temporaryTestNative) Register() error {
	if n.owner != nil {
		n.owner.mu.Lock()
		if n.owner.active != 0 {
			n.owner.mu.Unlock()
			return errors.New("native captures overlapped")
		}
		n.owner.active++
		n.owner.maxActive = max(n.owner.maxActive, n.owner.active)
		n.owner.journal = append(n.owner.journal, "register")
		n.owner.mu.Unlock()
	}
	if n.registerErr != nil && n.owner != nil {
		n.owner.mu.Lock()
		n.owner.active--
		n.owner.mu.Unlock()
	}
	return n.registerErr
}

func (n *temporaryTestNative) Unregister() error {
	n.unregisters.Add(1)
	if n.owner != nil {
		n.owner.mu.Lock()
		n.owner.journal = append(n.owner.journal, "unregister")
		if n.unregisterErr == nil {
			n.owner.active--
		}
		n.owner.mu.Unlock()
	}
	return n.unregisterErr
}

func (n *temporaryTestNative) Keydown() <-chan nativehotkey.Event { return n.down }

type temporaryTestFactory struct {
	mu        sync.Mutex
	created   []*temporaryTestNative
	nextErrs  []error
	active    int
	maxActive int
	journal   []string
}

func (f *temporaryTestFactory) make([]nativehotkey.Modifier, nativehotkey.Key) nativeHotkey {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := newTemporaryTestNative()
	n.owner = f
	if len(f.nextErrs) != 0 {
		n.registerErr = f.nextErrs[0]
		f.nextErrs = f.nextErrs[1:]
	}
	f.created = append(f.created, n)
	return n
}

func (f *temporaryTestFactory) natives() []*temporaryTestNative {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*temporaryTestNative(nil), f.created...)
}

func (f *temporaryTestFactory) snapshotJournal() ([]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.journal...), f.maxActive
}

func sendTemporaryEvent(n *temporaryTestNative) {
	n.down <- nativehotkey.Event{}
}

func waitTemporaryCount(t *testing.T, count *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if count.Load() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("callback count = %d, want %d", count.Load(), want)
}

func TestManagerReserveTemporaryPrioritizesAndRestoresOneNativeSlot(t *testing.T) {
	factory := &temporaryTestFactory{}
	m := newManager(factory.make)
	var lowerCalls, temporaryCalls atomic.Int32
	lowerID, err := m.Register(nil, nativehotkey.KeyA, func() { lowerCalls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	natives := factory.natives()
	if len(natives) != 1 {
		t.Fatalf("native registrations after base = %d, want 1", len(natives))
	}
	sendTemporaryEvent(natives[0])
	waitTemporaryCount(t, &lowerCalls, 1)

	release, err := m.ReserveTemporary(nil, nativehotkey.KeyA, func() { temporaryCalls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReserveTemporary(nil, nativehotkey.KeyA, func() {}); !errors.Is(err, ErrCombinationAlreadyOwned) {
		t.Fatalf("second temporary reservation = %v, want conflict", err)
	}
	natives = factory.natives()
	if len(natives) != 2 {
		t.Fatalf("native registrations while temporary = %d, want 2", len(natives))
	}
	sendTemporaryEvent(natives[0])
	time.Sleep(20 * time.Millisecond)
	if got := lowerCalls.Load(); got != 1 {
		t.Fatalf("lower callback while temporary = %d, want 1", got)
	}
	sendTemporaryEvent(natives[1])
	waitTemporaryCount(t, &temporaryCalls, 1)

	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	natives = factory.natives()
	if len(natives) != 3 {
		t.Fatalf("native registrations after release = %d, want 3", len(natives))
	}
	sendTemporaryEvent(natives[1])
	time.Sleep(20 * time.Millisecond)
	if got := temporaryCalls.Load(); got != 1 {
		t.Fatalf("old temporary callback after release = %d, want 1", got)
	}
	sendTemporaryEvent(natives[2])
	waitTemporaryCount(t, &lowerCalls, 2)

	if err := m.Unregister(lowerID); err != nil {
		t.Fatal(err)
	}
	journal, maxActive := factory.snapshotJournal()
	if maxActive != 1 {
		t.Fatalf("maximum simultaneous native captures = %d, want 1", maxActive)
	}
	if want := []string{"register", "unregister", "register", "unregister", "register", "unregister"}; !reflect.DeepEqual(journal, want) {
		t.Fatalf("native journal = %v, want %v", journal, want)
	}
}

func TestManagerTemporaryAllowsRemovalAndReplacementOfInferiorRegistration(t *testing.T) {
	factory := &temporaryTestFactory{}
	m := newManager(factory.make)
	var oldCalls, newCalls, temporaryCalls atomic.Int32
	oldID, err := m.Register(nil, nativehotkey.KeyB, func() { oldCalls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	release, err := m.ReserveTemporary(nil, nativehotkey.KeyB, func() { temporaryCalls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Unregister(oldID); err != nil {
		t.Fatal(err)
	}
	newID, err := m.Register(nil, nativehotkey.KeyB, func() { newCalls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	natives := factory.natives()
	if len(natives) != 2 {
		t.Fatalf("native registrations before replacement = %d, want 2", len(natives))
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	natives = factory.natives()
	if len(natives) != 3 {
		t.Fatalf("native registrations after replacement = %d, want 3", len(natives))
	}
	sendTemporaryEvent(natives[1])
	time.Sleep(20 * time.Millisecond)
	if temporaryCalls.Load() != 0 {
		t.Fatal("temporary callback received a stale event after release")
	}
	sendTemporaryEvent(natives[2])
	waitTemporaryCount(t, &newCalls, 1)
	if oldCalls.Load() != 0 {
		t.Fatalf("removed inferior callback count = %d, want 0", oldCalls.Load())
	}
	if err := m.Unregister(newID); err != nil {
		t.Fatal(err)
	}
	m.Stop()
}

func TestManagerTemporaryReleaseKeepsReservationAfterNativeFailure(t *testing.T) {
	factory := &temporaryTestFactory{}
	m := newManager(factory.make)
	var lowerCalls atomic.Int32
	if _, err := m.Register(nil, nativehotkey.KeyC, func() { lowerCalls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	release, err := m.ReserveTemporary(nil, nativehotkey.KeyC, func() {})
	if err != nil {
		t.Fatal(err)
	}
	natives := factory.natives()
	natives[1].unregisterErr = errors.New("temporary removal failed")
	firstErr := release()
	if firstErr == nil {
		t.Fatal("release succeeded after native removal failure")
	}
	if secondErr := release(); !errors.Is(secondErr, firstErr) {
		t.Fatalf("idempotent release error = %v, first = %v", secondErr, firstErr)
	}
	sendTemporaryEvent(natives[0])
	time.Sleep(20 * time.Millisecond)
	if lowerCalls.Load() != 0 {
		t.Fatal("inferior callback was delivered after failed temporary release")
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 1 {
		t.Fatalf("ownership after failed release = %#v, want one combination", got)
	}
	m.Stop()
}

func TestManagerTemporaryFailedRestoreDoesNotReacquireClosedTemporary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier is Windows-only")
	}
	factory := &temporaryTestFactory{}
	var barrier *OwnershipBarrier
	barrier = NewOwnershipBarrier(func(frame OwnershipFrame) { barrier.Ack(frame.InstanceID, frame.Revision) })
	m := newManager(factory.make)
	if err := m.SetOwnershipBarrier(barrier); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Register(nil, nativehotkey.KeyG, func() {}); err != nil {
		t.Fatal(err)
	}
	release, err := m.ReserveTemporary(nil, nativehotkey.KeyG, func() {})
	if err != nil {
		t.Fatal(err)
	}
	restoreErr := errors.New("inferior restore failed")
	factory.mu.Lock()
	factory.nextErrs = append(factory.nextErrs, restoreErr)
	factory.mu.Unlock()
	if err := release(); !errors.Is(err, restoreErr) {
		t.Fatalf("release error = %v, want restore error %v", err, restoreErr)
	}
	natives := factory.natives()
	if len(natives) != 3 {
		t.Fatalf("native registrations after failed restore = %d, want 3", len(natives))
	}
	if natives[1].unregisters.Load() != 1 {
		t.Fatalf("temporary unregister calls = %d, want 1", natives[1].unregisters.Load())
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 1 {
		t.Fatalf("ownership after failed restore = %#v, want conservative one-slot ownership", got)
	}
	m.Stop()
}

func TestManagerTemporaryWithoutInferiorCanAddAndRemoveOne(t *testing.T) {
	factory := &temporaryTestFactory{}
	m := newManager(factory.make)
	before := m.SnapshotGlobalOwnership()
	release, err := m.ReserveTemporary(nil, nativehotkey.KeyH, func() {})
	if err != nil {
		t.Fatal(err)
	}
	reserved := m.SnapshotGlobalOwnership()
	if reserved.Generation != before.Generation+1 || len(reserved.Combinations) != 1 {
		t.Fatalf("ownership after new temporary slot = %#v, want generation %d and one combination", reserved, before.Generation+1)
	}
	baseID, err := m.Register(nil, nativehotkey.KeyH, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Unregister(baseID); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 0 {
		t.Fatalf("ownership after empty temporary lifecycle = %#v, want empty", got)
	}
	m.Stop()
}

func TestManagerTemporaryAcquireFailureLeavesInferiorDisabledUntilRetry(t *testing.T) {
	factory := &temporaryTestFactory{}
	m := newManager(factory.make)
	var lowerCalls atomic.Int32
	lowerID, err := m.Register(nil, nativehotkey.KeyI, func() { lowerCalls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	temporaryErr := errors.New("temporary registration failed")
	factory.mu.Lock()
	factory.nextErrs = append(factory.nextErrs, temporaryErr)
	factory.mu.Unlock()
	if _, err := m.ReserveTemporary(nil, nativehotkey.KeyI, func() {}); !errors.Is(err, temporaryErr) {
		t.Fatalf("temporary reservation error = %v, want %v", err, temporaryErr)
	}
	natives := factory.natives()
	if len(natives) != 2 {
		t.Fatalf("native registrations after failed reservation = %d, want 2", len(natives))
	}
	sendTemporaryEvent(natives[0])
	time.Sleep(20 * time.Millisecond)
	if lowerCalls.Load() != 0 {
		t.Fatal("inferior callback was delivered after failed priority acquisition")
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 1 {
		t.Fatalf("ownership after failed reservation = %#v, want one conservative slot", got)
	}

	release, err := m.ReserveTemporary(nil, nativehotkey.KeyI, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	natives = factory.natives()
	if len(natives) != 4 {
		t.Fatalf("native registrations after retry/release = %d, want 4", len(natives))
	}
	sendTemporaryEvent(natives[3])
	waitTemporaryCount(t, &lowerCalls, 1)
	if err := m.Unregister(lowerID); err != nil {
		t.Fatal(err)
	}
	m.Stop()
}

type concurrentStopNative struct {
	down              chan nativehotkey.Event
	unregisterStarted chan struct{}
	allowUnregister   chan struct{}
	startOnce         sync.Once
	closeOnce         sync.Once
	attempts          atomic.Int32
	unregisters       atomic.Int32
}

func (n *concurrentStopNative) Register() error { return nil }

func (n *concurrentStopNative) Unregister() error {
	n.attempts.Add(1)
	n.startOnce.Do(func() { close(n.unregisterStarted) })
	<-n.allowUnregister
	n.unregisters.Add(1)
	n.closeOnce.Do(func() { close(n.down) })
	return nil
}

func (n *concurrentStopNative) Keydown() <-chan nativehotkey.Event { return n.down }

func TestManagerStopConcurrentWithTemporaryOnlyDoesNotDoubleUnregister(t *testing.T) {
	native := &concurrentStopNative{
		down:              make(chan nativehotkey.Event),
		unregisterStarted: make(chan struct{}),
		allowUnregister:   make(chan struct{}),
	}
	m := newManager(func([]nativehotkey.Modifier, nativehotkey.Key) nativeHotkey { return native })
	release, err := m.ReserveTemporary(nil, nativehotkey.KeyJ, func() {})
	if err != nil {
		t.Fatal(err)
	}
	stopDone := make(chan struct{})
	go func() {
		m.Stop()
		close(stopDone)
	}()
	select {
	case <-native.unregisterStarted:
	case <-time.After(time.Second):
		t.Fatal("Stop did not start native temporary teardown")
	}
	releaseDone := make(chan error, 1)
	go func() { releaseDone <- release() }()
	close(native.allowUnregister)
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish")
	}
	select {
	case err := <-releaseDone:
		if err != nil {
			t.Fatalf("concurrent release = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent release did not finish")
	}
	if got := native.unregisters.Load(); got != 1 {
		t.Fatalf("native unregister calls = %d, want 1", got)
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 0 {
		t.Fatalf("ownership after concurrent Stop = %#v, want empty", got)
	}
}

func TestManagerTwoConcurrentStopsTemporaryOnlyDoNotDoubleUnregister(t *testing.T) {
	native := &concurrentStopNative{
		down:              make(chan nativehotkey.Event),
		unregisterStarted: make(chan struct{}),
		allowUnregister:   make(chan struct{}),
	}
	m := newManager(func([]nativehotkey.Modifier, nativehotkey.Key) nativeHotkey { return native })
	if _, err := m.ReserveTemporary(nil, nativehotkey.KeyK, func() {}); err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan struct{})
	go func() {
		m.Stop()
		close(firstDone)
	}()
	select {
	case <-native.unregisterStarted:
	case <-time.After(time.Second):
		t.Fatal("first Stop did not start native teardown")
	}
	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		close(secondStarted)
		m.Stop()
		close(secondDone)
	}()
	<-secondStarted
	time.Sleep(20 * time.Millisecond)
	if got := native.attempts.Load(); got != 1 {
		t.Fatalf("native unregister attempts while first Stop blocked = %d, want 1", got)
	}
	close(native.allowUnregister)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first Stop did not finish")
	}
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second Stop did not finish")
	}
	if got := native.attempts.Load(); got != 1 {
		t.Fatalf("native unregister attempts = %d, want 1", got)
	}
	if got := native.unregisters.Load(); got != 1 {
		t.Fatalf("native unregister calls = %d, want 1", got)
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 0 {
		t.Fatalf("ownership after concurrent Stops = %#v, want empty", got)
	}
}

func TestManagerReserveTemporaryACKsBeforeNewNativeCapture(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier is Windows-only")
	}
	factory := &temporaryTestFactory{}
	var barrier *OwnershipBarrier
	var mu sync.Mutex
	var events []string
	barrier = NewOwnershipBarrier(func(frame OwnershipFrame) {
		mu.Lock()
		events = append(events, "publish")
		mu.Unlock()
		if !barrier.Ack(frame.InstanceID, frame.Revision) {
			t.Error("matching ownership ACK was rejected")
		}
	})
	defer barrier.Close()
	m := newManager(func(mods []nativehotkey.Modifier, key nativehotkey.Key) nativeHotkey {
		mu.Lock()
		events = append(events, "native-register")
		mu.Unlock()
		return factory.make(mods, key)
	})
	if err := m.SetOwnershipBarrier(barrier); err != nil {
		t.Fatal(err)
	}
	release, err := m.ReserveTemporary(nil, nativehotkey.KeyD, func() {})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"publish", "native-register"}) {
		t.Fatalf("events = %v, want publish before native registration", got)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerStopWithClosedBarrierRemovesTemporaryCaptureWithoutResurrection(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier is Windows-only")
	}
	factory := &temporaryTestFactory{}
	var barrier *OwnershipBarrier
	barrier = NewOwnershipBarrier(func(frame OwnershipFrame) { barrier.Ack(frame.InstanceID, frame.Revision) })
	m := newManager(factory.make)
	if err := m.SetOwnershipBarrier(barrier); err != nil {
		t.Fatal(err)
	}
	var lowerCalls, temporaryCalls atomic.Int32
	if _, err := m.Register(nil, nativehotkey.KeyE, func() { lowerCalls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReserveTemporary(nil, nativehotkey.KeyE, func() { temporaryCalls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	natives := factory.natives()
	barrier.Close()
	m.Stop()
	if natives[0].unregisters.Load() != 1 || natives[1].unregisters.Load() != 1 {
		t.Fatalf("unregister calls = %d/%d, want 1/1", natives[0].unregisters.Load(), natives[1].unregisters.Load())
	}
	sendTemporaryEvent(natives[0])
	sendTemporaryEvent(natives[1])
	time.Sleep(20 * time.Millisecond)
	if lowerCalls.Load() != 0 || temporaryCalls.Load() != 0 {
		t.Fatalf("callbacks after Stop = lower:%d temporary:%d", lowerCalls.Load(), temporaryCalls.Load())
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 0 {
		t.Fatalf("ownership after Stop = %#v, want empty", got)
	}
}

func TestManagerTemporaryReleaseClosedBarrierRemovesTemporaryBeforeFailedRestore(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ownership barrier is Windows-only")
	}
	factory := &temporaryTestFactory{}
	var barrier *OwnershipBarrier
	barrier = NewOwnershipBarrier(func(frame OwnershipFrame) { barrier.Ack(frame.InstanceID, frame.Revision) })
	m := newManager(factory.make)
	if err := m.SetOwnershipBarrier(barrier); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Register(nil, nativehotkey.KeyF, func() {}); err != nil {
		t.Fatal(err)
	}
	release, err := m.ReserveTemporary(nil, nativehotkey.KeyF, func() {})
	if err != nil {
		t.Fatal(err)
	}
	barrier.Close()

	if err := release(); !errors.Is(err, ErrOwnershipBarrierClosed) {
		t.Fatalf("release error = %v, want closed barrier", err)
	}
	natives := factory.natives()
	if len(natives) != 2 {
		t.Fatalf("native registrations after failed restore = %d, want 2", len(natives))
	}
	if natives[1].unregisters.Load() != 1 {
		t.Fatalf("temporary unregister calls = %d, want 1", natives[1].unregisters.Load())
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 1 {
		t.Fatalf("ownership after failed restore = %#v, want conservative one-slot ownership", got)
	}
}
