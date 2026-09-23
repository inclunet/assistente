//go:build windows

package hotkey

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	nativehotkey "golang.design/x/hotkey"
)

func TestWindowsManagerStopWithRunningCallbackAndPendingPress(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	manager := newManager(func(mods []nativehotkey.Modifier, key nativehotkey.Key) nativeHotkey {
		return newWindowsHotkey(mods, key, api)
	})
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	_, err := manager.Register([]nativehotkey.Modifier{ModCtrl}, nativehotkey.KeyA, func() {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			close(finished)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer close(release)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback was not entered")
	}
	stopped := make(chan struct{})
	go func() { manager.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop waited for callback or key release")
	}
	select {
	case <-finished:
		t.Fatal("callback unexpectedly finished before release")
	default:
	}
	if calls.Load() != 1 {
		t.Fatalf("delivered %d callbacks", calls.Load())
	}
	_, _, _, unregisters := api.calls()
	if len(unregisters) != 1 {
		t.Fatalf("native unregister calls = %d", len(unregisters))
	}
}

func TestWindowsManagerPropagatesNativeUnregisterError(t *testing.T) {
	api := newFakeWindowsHotkeyAPI()
	api.nextResults <- fakeWindowsNext{id: windowsHotkeyID, ok: true}
	api.unregisterErr = errors.New("native cleanup failed")
	manager := newManager(func(mods []nativehotkey.Modifier, key nativehotkey.Key) nativeHotkey {
		return newWindowsHotkey(mods, key, api)
	})
	// A blocked consumer is unnecessary: cancel immediately before dispatch.
	api.waitResults <- errors.New("pump stopped")
	id, err := manager.Register(nil, nativehotkey.KeyA, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Unregister(id); !errors.Is(err, api.unregisterErr) {
		t.Fatalf("Unregister = %v, expected native cleanup error", err)
	}
}
