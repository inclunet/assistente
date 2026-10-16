//go:build !windows

package commandforeground

import (
	"context"
	"errors"
	"testing"
)

func TestNewNativeIsExplicitlyUnavailableOutsideWindows(t *testing.T) {
	snapshot, err := NewNative().Capture(context.Background())
	if !errors.Is(err, ErrUnavailable) || !snapshot.Identity.IsZero() {
		t.Fatalf("snapshot=%#v err=%v, want zero snapshot and ErrUnavailable", snapshot, err)
	}
}
