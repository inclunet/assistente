//go:build !windows

package hotkey

import (
	"errors"
	"testing"

	"golang.design/x/hotkey"
)

func TestNativePlatformRefusesUnsupportedRegistration(t *testing.T) {
	m := newManager(nil)
	if _, err := m.Register(nil, hotkey.KeyA, func() {}); !errors.Is(err, ErrNativeUnsupported) {
		t.Fatalf("native default factory accepted unsupported platform: %v", err)
	}
	if got := m.SnapshotGlobalOwnership(); len(got.Combinations) != 0 {
		t.Fatalf("unsupported platform owns combinations: %#v", got)
	}
}
