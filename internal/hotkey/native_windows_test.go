//go:build windows

package hotkey

import (
	"errors"
	"sync"
	"testing"
	"time"

	nativehotkey "golang.design/x/hotkey"
	"golang.org/x/sys/windows"
)

type fakeWindowsNext struct {
	id  int32
	ok  bool
	err error
}

type fakeWindowsHotkeyAPI struct {
	mu sync.Mutex

	registerErr   error
	unregisterErr error
	heldResults   chan bool
	held          bool
	waitResults   chan error
	nextResults   chan fakeWindowsNext
	stop          <-chan struct{}
	nextCalled    chan struct{}
	waitCalled    chan struct{}
	heldCalled    chan struct{}

	registers   []windowsHotkeyCall
	unregisters []windowsHotkeyCall
	waits       []windowsHotkeyCall
	nexts       []windowsHotkeyCall
	helds       []windowsHotkeyCall
}

type windowsHotkeyCall struct {
	threadID uint32
	id       int32
	mods     uint32
	key      uint32
}

func newFakeWindowsHotkeyAPI() *fakeWindowsHotkeyAPI {
	return &fakeWindowsHotkeyAPI{
		waitResults: make(chan error, 8),
		nextResults: make(chan fakeWindowsNext, 8),
		nextCalled:  make(chan struct{}, 8),
		waitCalled:  make(chan struct{}, 8),
		heldResults: make(chan bool, 8),
		heldCalled:  make(chan struct{}, 8),
	}
}

func (f *fakeWindowsHotkeyAPI) Register(id int32, mods, key uint32) error {
	f.mu.Lock()
	f.registers = append(f.registers, windowsHotkeyCall{
		threadID: windows.GetCurrentThreadId(),
		id:       id,
		mods:     mods,
		key:      key,
	})
	f.mu.Unlock()
	return f.registerErr
}

func (f *fakeWindowsHotkeyAPI) Unregister(id int32) error {
	f.mu.Lock()
	f.unregisters = append(f.unregisters, windowsHotkeyCall{
		threadID: windows.GetCurrentThreadId(),
		id:       id,
	})
	f.mu.Unlock()
	return f.unregisterErr
}

func (f *fakeWindowsHotkeyAPI) Held(key uint32) bool {
	f.mu.Lock()
	f.helds = append(f.helds, windowsHotkeyCall{threadID: windows.GetCurrentThreadId(), key: key})
	held := f.held
	f.mu.Unlock()
	f.heldCalled <- struct{}{}
	select {
	case held = <-f.heldResults:
	default:
	}
	return held
}

func (f *fakeWindowsHotkeyAPI) Next() (int32, bool, error) {
	f.mu.Lock()
	f.nexts = append(f.nexts, windowsHotkeyCall{threadID: windows.GetCurrentThreadId()})
	f.mu.Unlock()
	f.nextCalled <- struct{}{}
	select {
	case result := <-f.nextResults:
		return result.id, result.ok, result.err
	default:
		return 0, false, nil
	}
}

func (f *fakeWindowsHotkeyAPI) Wait() error {
	f.mu.Lock()
	f.waits = append(f.waits, windowsHotkeyCall{threadID: windows.GetCurrentThreadId()})
	f.mu.Unlock()
	f.waitCalled <- struct{}{}
	select {
	case result := <-f.waitResults:
		return result
	case <-f.stop:
		return nil
	}
}

func (f *fakeWindowsHotkeyAPI) calls() (registers, waits, nexts, unregisters []windowsHotkeyCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]windowsHotkeyCall(nil), f.registers...),
		append([]windowsHotkeyCall(nil), f.waits...),
		append([]windowsHotkeyCall(nil), f.nexts...),
		append([]windowsHotkeyCall(nil), f.unregisters...)
}

func (f *fakeWindowsHotkeyAPI) heldCalls() []windowsHotkeyCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]windowsHotkeyCall(nil), f.helds...)
}

func awaitWindowsEvent(t *testing.T, events <-chan nativehotkey.Event) {
	t.Helper()
	select {
	case _, ok := <-events:
		if !ok {
			t.Fatal("native hotkey channel closed before delivering an event")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for native hotkey event")
	}
}

func awaitWindowsClosed(t *testing.T, events <-chan nativehotkey.Event) {
	t.Helper()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("native hotkey channel delivered an unexpected event")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for native hotkey shutdown")
	}
}

func awaitWindowsHeldCalls(t *testing.T, api *fakeWindowsHotkeyAPI, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-api.heldCalled:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for Held call %d", i+1)
		}
	}
}

func TestWindowsHotkeyRegisterUsesNoRepeatAndNativeCtrlShift(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	hk := newWindowsHotkey([]nativehotkey.Modifier{ModCtrl, ModShift}, nativehotkey.KeyA, api)
	api.stop = hk.stop

	if err := hk.Register(); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := hk.Unregister(); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
	awaitWindowsClosed(t, hk.Keydown())

	registers, _, _, _ := api.calls()
	if len(registers) != 1 {
		t.Fatalf("Register calls = %d, want 1", len(registers))
	}
	if registers[0].id != windowsHotkeyID {
		t.Fatalf("registered id = %d, want %d", registers[0].id, windowsHotkeyID)
	}
	wantMods := uint32(ModCtrl) | uint32(ModShift) | windowsNoRepeat
	if registers[0].mods != wantMods {
		t.Fatalf("registered modifiers = %#x, want %#x", registers[0].mods, wantMods)
	}
	if registers[0].key != uint32(nativehotkey.KeyA) {
		t.Fatalf("registered key = %#x, want %#x", registers[0].key, nativehotkey.KeyA)
	}
}

func TestWindowsHotkeyHeldAtRegistrationDrainsUntilReleaseBeforeArming(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.heldResults <- true  // admission: key was already held
	api.heldResults <- false // queue empty and release observed
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop

	if err := hk.Register(); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	// Register is ready after the initial Held check. The stale queued native
	// message must be drained before the release check can arm the pump.
	awaitWindowsHeldCalls(t, api, 2)
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	api.waitResults <- nil
	awaitWindowsEvent(t, hk.Keydown())
	if err := hk.Unregister(); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
}

func TestWindowsHotkeyDeliversTwoQueuedFreshPresses(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.heldResults <- false
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop
	if err := hk.Register(); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	api.waitResults <- nil
	awaitWindowsEvent(t, hk.Keydown())
	awaitWindowsEvent(t, hk.Keydown())
	if err := hk.Unregister(); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
}

func TestWindowsHotkeyCancelWhileHeldIsPrompt(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.held = true // Remains held throughout teardown, not just the first sample.
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop
	registered := make(chan error, 1)
	go func() { registered <- hk.Register() }()
	select {
	case err := <-registered:
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Register() waited for held key release")
	}
	closed := make(chan error, 1)
	go func() { closed <- hk.Unregister() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Unregister() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Unregister() waited for held key release")
	}
}

func TestWindowsHotkeyRegistrationFailureClosesDownWithoutUnregister(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.registerErr = errors.New("registration failed")
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)

	if err := hk.Register(); !errors.Is(err, api.registerErr) {
		t.Fatalf("Register() error = %v, want %v", err, api.registerErr)
	}
	select {
	case _, ok := <-hk.Keydown():
		if ok {
			t.Fatal("Keydown() remained open after registration failure")
		}
	case <-time.After(time.Second):
		t.Fatal("Keydown() did not close after registration failure")
	}
	_, _, _, unregisters := api.calls()
	if len(unregisters) != 0 {
		t.Fatalf("Unregister calls = %d, want 0", len(unregisters))
	}
}

func TestWindowsHotkeyCancelDoesNotWaitForPendingPress(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.waitResults <- nil
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop
	if err := hk.Register(); err != nil {
		t.Fatal(err)
	}

	// Do not consume Keydown: this is the pending press that cancellation must
	// discard without requiring a matching release.
	if err := hk.Unregister(); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
	_, _, _, unregisters := api.calls()
	if len(unregisters) != 1 {
		t.Fatalf("Unregister calls = %d, want 1", len(unregisters))
	}
}

func TestWindowsHotkeyFiltersForeignID(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	waitErr := errors.New("end after foreign ID")
	api.waitResults <- waitErr
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID + 1, ok: true}
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	if err := hk.Register(); err != nil {
		t.Fatal(err)
	}
	// Observe the channel before cancellation: a foreign delivery must fail,
	// rather than being hidden by teardown winning the send/stop race.
	awaitWindowsClosed(t, hk.Keydown())
	if err := hk.Unregister(); !errors.Is(err, waitErr) {
		t.Fatal(err)
	}
}

func TestWindowsHotkeyWaitFailureCleansUp(t *testing.T) {
	waitErr := errors.New("message wait failed")
	api := newFakeWindowsHotkeyAPI()
	api.waitResults <- waitErr
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	if err := hk.Register(); err != nil {
		t.Fatal(err)
	}
	awaitWindowsClosed(t, hk.Keydown())
	if err := hk.Unregister(); !errors.Is(err, waitErr) {
		t.Fatalf("Unregister() error = %v, want %v", err, waitErr)
	}
	_, _, _, unregisters := api.calls()
	if len(unregisters) != 1 {
		t.Fatalf("Unregister calls = %d, want 1", len(unregisters))
	}
}

func TestWindowsHotkeyUnregisterErrorIsPropagated(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	unregisterErr := errors.New("unregister failed")
	api.unregisterErr = unregisterErr
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop
	if err := hk.Register(); err != nil {
		t.Fatal(err)
	}
	if err := hk.Unregister(); !errors.Is(err, unregisterErr) {
		t.Fatalf("Unregister() error = %v, want %v", err, unregisterErr)
	}
	awaitWindowsClosed(t, hk.Keydown())
}

func TestWindowsHotkeyUnregisterIsConcurrentAndIdempotent(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop
	if err := hk.Register(); err != nil {
		t.Fatal(err)
	}
	const callers = 8
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- hk.Unregister()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Unregister() error = %v", err)
		}
	}
	awaitWindowsClosed(t, hk.Keydown())
	_, _, _, unregisters := api.calls()
	if len(unregisters) != 1 {
		t.Fatalf("Unregister calls = %d, want 1", len(unregisters))
	}
}

func TestWindowsHotkeyRejectsRegisterTwiceAndAfterStop(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop
	if err := hk.Register(); err != nil {
		t.Fatal(err)
	}
	if err := hk.Register(); err == nil {
		t.Fatal("Register() succeeded twice on the same native object")
	}
	if err := hk.Unregister(); err != nil {
		t.Fatal(err)
	}
	awaitWindowsClosed(t, hk.Keydown())
	if err := hk.Register(); err == nil {
		t.Fatal("Register() succeeded after stop on the same native object")
	}
}

func TestWindowsHotkeyUsesOneOSThreadForWin32Calls(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.waitResults <- nil
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	hk := newWindowsHotkey(nil, nativehotkey.KeyA, api)
	api.stop = hk.stop
	if err := hk.Register(); err != nil {
		t.Fatal(err)
	}
	awaitWindowsEvent(t, hk.Keydown())
	for i := 0; i < 2; i++ {
		select {
		case <-api.nextCalled:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for native Next call")
		}
	}
	select {
	case <-api.waitCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for native Wait call")
	}
	if err := hk.Unregister(); err != nil {
		t.Fatal(err)
	}
	awaitWindowsClosed(t, hk.Keydown())

	registers, waits, nexts, unregisters := api.calls()
	if len(registers) != 1 || len(waits) == 0 || len(nexts) < 2 || len(unregisters) != 1 {
		t.Fatalf("unexpected call counts: Register=%d Wait=%d Next=%d Unregister=%d", len(registers), len(waits), len(nexts), len(unregisters))
	}
	threadID := registers[0].threadID
	helds := api.heldCalls()
	if len(helds) == 0 {
		t.Fatal("Held was not called during admission")
	}
	for _, call := range append(append(append(waits, nexts...), unregisters...), registers...) {
		if call.threadID != threadID {
			t.Fatalf("Win32 call ran on thread %d, want %d", call.threadID, threadID)
		}
	}
	for _, call := range helds {
		if call.threadID != threadID {
			t.Fatalf("Held ran on thread %d, want %d", call.threadID, threadID)
		}
	}
}
