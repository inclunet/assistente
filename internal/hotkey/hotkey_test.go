package hotkey

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	nativehotkey "golang.design/x/hotkey"
)

type blockingNativeHotkey struct {
	down              chan nativehotkey.Event
	unregisterStarted chan struct{}
	allowUnregister   chan struct{}
	startOnce         sync.Once
	closeOnce         sync.Once
}

type failingNativeHotkey struct {
	down chan nativehotkey.Event
	err  error
}

func (h *failingNativeHotkey) Register() error                    { return nil }
func (h *failingNativeHotkey) Keydown() <-chan nativehotkey.Event { return h.down }
func (h *failingNativeHotkey) Unregister() error                  { return h.err }

func newBlockingNativeHotkey() *blockingNativeHotkey {
	return &blockingNativeHotkey{
		down:              make(chan nativehotkey.Event),
		unregisterStarted: make(chan struct{}),
		allowUnregister:   make(chan struct{}),
	}
}

func (h *blockingNativeHotkey) Register() error                    { return nil }
func (h *blockingNativeHotkey) Keydown() <-chan nativehotkey.Event { return h.down }
func (h *blockingNativeHotkey) Unregister() error {
	h.startOnce.Do(func() { close(h.unregisterStarted) })
	<-h.allowUnregister
	h.closeOnce.Do(func() { close(h.down) })
	return nil
}

type fakeNativeHotkey struct {
	down         chan nativehotkey.Event
	closeOnce    sync.Once
	unregistered atomic.Bool
}

func newFakeNativeHotkey() *fakeNativeHotkey {
	return &fakeNativeHotkey{
		down: make(chan nativehotkey.Event),
	}
}

func (h *fakeNativeHotkey) Register() error                    { return nil }
func (h *fakeNativeHotkey) Keydown() <-chan nativehotkey.Event { return h.down }
func (h *fakeNativeHotkey) Unregister() error {
	h.unregistered.Store(true)
	h.closeChannels()
	return nil
}
func (h *fakeNativeHotkey) closeChannels() {
	h.closeOnce.Do(func() {
		close(h.down)
	})
}

func newTestManager(h **fakeNativeHotkey) *Manager {
	return NewManager(func([]nativehotkey.Modifier, nativehotkey.Key) NativeHotkey {
		fake := newFakeNativeHotkey()
		*h = fake
		return fake
	})
}

func sendHotkeyEvent(t *testing.T, ch chan nativehotkey.Event) {
	t.Helper()
	select {
	case ch <- nativehotkey.Event{}:
	case <-time.After(time.Second):
		t.Fatal("listener did not receive event")
	}
}

func TestListenHotkeyPreservesEachKeydown(t *testing.T) {
	var fake *fakeNativeHotkey
	m := newTestManager(&fake)
	callbacks := make(chan struct{}, 2)
	_, err := m.Register(nil, 0, func() { callbacks <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}

	sendHotkeyEvent(t, fake.down)
	select {
	case <-callbacks:
	case <-time.After(time.Second):
		t.Fatal("first callback did not complete")
	}
	sendHotkeyEvent(t, fake.down)
	select {
	case <-callbacks:
	case <-time.After(time.Second):
		t.Fatal("second callback did not complete")
	}
	m.UnregisterAll()
}

func TestListenHotkeyStopsOnClosedChannelsWithoutSpin(t *testing.T) {
	fake := newFakeNativeHotkey()
	fake.closeChannels()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registered := &RegisteredHotkey{
		down: fake.down,
		Callback: func() {
			t.Fatal("callback invoked from closed channel")
		},
	}
	registered.active.Store(true)
	done := make(chan struct{})
	go func() {
		listenHotkey(ctx, registered)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("listener did not terminate after channel close")
	}
}

func TestListenHotkeyCancellationDropsQueuedKeydown(t *testing.T) {
	down := make(chan nativehotkey.Event, 1)
	down <- nativehotkey.Event{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	registered := &RegisteredHotkey{
		down:     down,
		Callback: func() { calls.Add(1) },
	}
	registered.active.Store(true)
	listenHotkey(ctx, registered)
	if got := calls.Load(); got != 0 {
		t.Fatalf("callback count after cancellation = %d, want 0", got)
	}
}

func TestUnregisterRetiresCallbackAndDoesNotHoldManagerMutex(t *testing.T) {
	var fake *fakeNativeHotkey
	m := newTestManager(&fake)
	callbackDone := make(chan struct{})
	var id int
	unregisterErr := make(chan error, 1)
	var err error
	id, err = m.Register(nil, 0, func() {
		unregisterErr <- m.Unregister(id)
		close(callbackDone)
	})
	if err != nil {
		t.Fatal(err)
	}
	sendHotkeyEvent(t, fake.down)
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("callback could not unregister itself")
	}
	if err := <-unregisterErr; err != nil {
		t.Fatalf("self unregister: %v", err)
	}
	if !fake.unregistered.Load() {
		t.Fatal("native hotkey was not unregistered")
	}
}

func TestListenHotkeyRetiredRegistrationDropsBufferedEvents(t *testing.T) {
	down := make(chan nativehotkey.Event, 2)
	down <- nativehotkey.Event{}
	down <- nativehotkey.Event{}
	close(down)
	var calls atomic.Int32
	registered := &RegisteredHotkey{down: down, Callback: func() { calls.Add(1) }}
	registered.active.Store(false)
	listenHotkey(context.Background(), registered)
	if calls.Load() != 0 {
		t.Fatal("retired registration dispatched buffered input")
	}
}

func TestModifierAliasesMatchPlatform(t *testing.T) {
	want := map[string][4]uint32{
		"windows": {0x2, 0x4, 0x1, 0x8},
		"linux":   {0x4, 0x1, 0x8, 0x40},
		"darwin":  {0x1000, 0x200, 0x800, 0x100},
	}
	expected, ok := want[runtime.GOOS]
	if !ok {
		t.Skip("platform without native hotkey mapping")
	}
	got := [4]uint32{uint32(ModCtrl), uint32(ModShift), uint32(ModAlt), uint32(ModWin)}
	if got != expected {
		t.Fatalf("modifier aliases = %#v, want %#v", got, expected)
	}
}

func TestParseCombinationRejectsUnknownAndDuplicateModifiers(t *testing.T) {
	for _, combination := range []string{"Ctrl+Bogus+A", "Ctrl+Ctrl+A", "Ctrl++A", "+A", "Ctrl+"} {
		if _, _, err := ParseCombination(combination); err == nil {
			t.Errorf("ParseCombination(%q) accepted invalid input", combination)
		}
	}
	if modifiers, _, err := ParseCombination("Control+Shift+A"); err != nil || len(modifiers) != 2 {
		t.Fatalf("valid combination rejected: modifiers=%v err=%v", modifiers, err)
	}
}

func TestManagerRejectsCanonicalDuplicateBeforeNativeRegistration(t *testing.T) {
	var factoryCalls atomic.Int32
	var first *fakeNativeHotkey
	m := NewManager(func(modifiers []nativehotkey.Modifier, key nativehotkey.Key) NativeHotkey {
		factoryCalls.Add(1)
		first = newFakeNativeHotkey()
		return first
	})

	mods := []nativehotkey.Modifier{ModShift, ModCtrl}
	if _, err := m.Register(mods, nativehotkey.KeyA, func() {}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Register([]nativehotkey.Modifier{ModCtrl, ModShift, ModCtrl}, nativehotkey.KeyA, func() {}); !errors.Is(err, ErrCombinationAlreadyOwned) {
		t.Fatalf("duplicate registration error = %v, want ErrCombinationAlreadyOwned", err)
	}
	if got := factoryCalls.Load(); got != 1 {
		t.Fatalf("native factory calls = %d, want 1", got)
	}
	snapshot := m.SnapshotGlobalOwnership()
	if snapshot.Generation != 1 || len(snapshot.Combinations) != 1 {
		t.Fatalf("snapshot = %#v, want generation 1 with one combination", snapshot)
	}
	if got := snapshot.Combinations[0].Modifiers; got != ModCtrl|ModShift {
		t.Fatalf("snapshot modifiers = %#x, want %#x", got, ModCtrl|ModShift)
	}
	m.UnregisterAll()
}

func TestManagerDuplicateRegistrationsAreExclusiveConcurrently(t *testing.T) {
	var factoryCalls atomic.Int32
	m := NewManager(func([]nativehotkey.Modifier, nativehotkey.Key) NativeHotkey {
		factoryCalls.Add(1)
		return newFakeNativeHotkey()
	})

	const callers = 32
	results := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Register([]nativehotkey.Modifier{ModShift, ModCtrl}, nativehotkey.KeyA, func() {})
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrCombinationAlreadyOwned) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent registration error: %v", err)
		}
	}
	if successes != 1 || conflicts != callers-1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/%d", successes, conflicts, callers-1)
	}
	if got := factoryCalls.Load(); got != 1 {
		t.Fatalf("native factory calls = %d, want 1", got)
	}
	m.UnregisterAll()
}

func TestManagerKeepsOwnershipDuringNativeUnregister(t *testing.T) {
	var created *blockingNativeHotkey
	m := NewManager(func([]nativehotkey.Modifier, nativehotkey.Key) NativeHotkey {
		created = newBlockingNativeHotkey()
		return created
	})
	id, err := m.Register([]nativehotkey.Modifier{ModCtrl, ModShift}, nativehotkey.KeyA, func() {})
	if err != nil {
		t.Fatal(err)
	}

	unregistered := make(chan error, 1)
	go func() { unregistered <- m.Unregister(id) }()
	select {
	case <-created.unregisterStarted:
	case <-time.After(time.Second):
		t.Fatal("native unregister did not start")
	}

	if _, err := m.Register([]nativehotkey.Modifier{ModShift, ModCtrl}, nativehotkey.KeyA, func() {}); !errors.Is(err, ErrCombinationAlreadyOwned) {
		t.Fatalf("re-register during native teardown error = %v, want ErrCombinationAlreadyOwned", err)
	}
	inProgress := m.SnapshotGlobalOwnership()
	if inProgress.Generation != 1 || len(inProgress.Combinations) != 1 {
		t.Fatalf("snapshot during teardown = %#v, want unchanged ownership", inProgress)
	}

	close(created.allowUnregister)
	select {
	case err := <-unregistered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("manager unregister did not finish")
	}

	cleared := m.SnapshotGlobalOwnership()
	if cleared.Generation != 2 || len(cleared.Combinations) != 0 {
		t.Fatalf("snapshot after teardown = %#v, want generation 2 and no combinations", cleared)
	}
	if _, err := m.Register([]nativehotkey.Modifier{ModCtrl, ModShift}, nativehotkey.KeyA, func() {}); err != nil {
		t.Fatalf("re-register after native teardown: %v", err)
	}
	close(created.allowUnregister)
	m.UnregisterAll()
}

func TestManagerFailsClosedWhenNativeUnregisterFails(t *testing.T) {
	nativeErr := errors.New("native ownership release was not confirmed")
	native := &failingNativeHotkey{down: make(chan nativehotkey.Event), err: nativeErr}
	m := NewManager(func([]nativehotkey.Modifier, nativehotkey.Key) NativeHotkey { return native })
	id, err := m.Register([]nativehotkey.Modifier{ModCtrl}, nativehotkey.KeyA, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Unregister(id); !errors.Is(err, nativeErr) {
		t.Fatalf("first unregister error = %v, want %v", err, nativeErr)
	}
	if _, err := m.Register([]nativehotkey.Modifier{ModCtrl}, nativehotkey.KeyA, func() {}); !errors.Is(err, ErrCombinationAlreadyOwned) {
		t.Fatalf("register after failed native release = %v, want ownership conflict", err)
	}
	if err := m.Unregister(id); !errors.Is(err, nativeErr) {
		t.Fatalf("second unregister error = %v, want original native error", err)
	}
	snapshot := m.SnapshotGlobalOwnership()
	if snapshot.Generation != 1 || len(snapshot.Combinations) != 1 {
		t.Fatalf("snapshot after failed release = %#v, want reservation retained", snapshot)
	}
	m.UnregisterAll()
}

func TestUnregisterAllJoinsTeardownAlreadyInProgress(t *testing.T) {
	native := newBlockingNativeHotkey()
	m := NewManager(func([]nativehotkey.Modifier, nativehotkey.Key) NativeHotkey { return native })
	id, err := m.Register(nil, nativehotkey.KeyA, func() {})
	if err != nil {
		t.Fatal(err)
	}
	individualDone := make(chan error, 1)
	go func() { individualDone <- m.Unregister(id) }()
	select {
	case <-native.unregisterStarted:
	case <-time.After(time.Second):
		t.Fatal("native unregister did not start")
	}

	allDone := make(chan struct{})
	go func() {
		m.UnregisterAll()
		close(allDone)
	}()
	select {
	case <-allDone:
		t.Fatal("UnregisterAll returned before pending teardown completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(native.allowUnregister)
	select {
	case err := <-individualDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("individual unregister did not finish")
	}
	select {
	case <-allDone:
	case <-time.After(time.Second):
		t.Fatal("UnregisterAll did not join pending teardown")
	}
}

func TestManagerSnapshotIsIndependentCopyAndGenerationTracksSet(t *testing.T) {
	m := NewManager(func([]nativehotkey.Modifier, nativehotkey.Key) NativeHotkey { return newFakeNativeHotkey() })
	firstID, err := m.Register([]nativehotkey.Modifier{ModShift, ModCtrl}, nativehotkey.KeyB, func() {})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := m.Register([]nativehotkey.Modifier{ModAlt}, nativehotkey.KeyA, func() {})
	if err != nil {
		t.Fatal(err)
	}

	first := m.SnapshotGlobalOwnership()
	if first.Generation != 2 || len(first.Combinations) != 2 {
		t.Fatalf("first snapshot = %#v, want generation 2 with two combinations", first)
	}
	first.Combinations[0] = NativeCombination{}
	second := m.SnapshotGlobalOwnership()
	if second.Generation != first.Generation || len(second.Combinations) != 2 {
		t.Fatalf("snapshot after caller mutation = %#v, want unchanged copy", second)
	}
	if second.Combinations[0] == (NativeCombination{}) {
		t.Fatal("snapshot shares mutable combination storage")
	}
	if got := m.SnapshotGlobalOwnership().Generation; got != 2 {
		t.Fatalf("generation changed without ownership change: %d", got)
	}
	if err := m.Unregister(firstID); err != nil {
		t.Fatal(err)
	}
	if got := m.SnapshotGlobalOwnership().Generation; got != 3 {
		t.Fatalf("generation after removal = %d, want 3", got)
	}
	if err := m.Unregister(secondID); err != nil {
		t.Fatal(err)
	}
}
