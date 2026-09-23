package hotkey

import (
	"errors"
	"runtime"
	"testing"

	"golang.design/x/hotkey"
)

func TestNativeSupportMatchesQualifiedPlatform(t *testing.T) {
	if got := IsSupported(); got != (runtime.GOOS == "windows") {
		t.Fatalf("native support on %s = %v", runtime.GOOS, got)
	}
}

func TestUnsupportedNativeRegistrationDoesNotAcquireOwnership(t *testing.T) {
	m := NewManager(func([]hotkey.Modifier, hotkey.Key) NativeHotkey { return unsupportedNativeHotkey{} })
	for attempt := 0; attempt < 2; attempt++ {
		id, err := m.Register(nil, hotkey.KeyA, func() { t.Error("unsupported adapter dispatched a callback") })
		if id != 0 || !errors.Is(err, ErrNativeUnsupported) {
			t.Fatalf("registration = %d, %v", id, err)
		}
		if snapshot := m.SnapshotGlobalOwnership(); snapshot.Generation != 0 || len(snapshot.Combinations) != 0 {
			t.Fatalf("failed registration acquired ownership: %#v", snapshot)
		}
	}
	if (unsupportedNativeHotkey{}).Keydown() != nil {
		t.Fatal("unsupported adapter exposes an event stream")
	}
}
