//go:build !windows

package ossession

import (
	"context"
	"errors"
	"testing"
)

func TestWatchUnsupportedPublishesUnknownAtStartAndEnd(t *testing.T) {
	var got []State
	err := Watch(context.Background(), func(state State) error {
		got = append(got, state)
		return nil
	})
	if !errors.Is(err, errUnsupported) {
		t.Fatalf("Watch error = %v, want unsupported", err)
	}
	if len(got) != 2 || got[0] != unknownState() || got[1] != unknownState() {
		t.Fatalf("observed states = %+v, want unknown at start and end", got)
	}
}

func TestProbeUnsupportedReturnsUnknown(t *testing.T) {
	state, err := Probe(context.Background())
	if state != unknownState() || !errors.Is(err, errUnsupported) {
		t.Fatalf("Probe = (%+v, %v), want unknown/unsupported", state, err)
	}
}

func TestWatchCancelledBeforeNativeStartStillPublishesUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var got []State
	err := Watch(ctx, func(state State) error {
		got = append(got, state)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch error = %v, want cancellation", err)
	}
	if len(got) != 2 || got[0] != unknownState() || got[1] != unknownState() {
		t.Fatalf("observed states = %+v, want unknown at start and end", got)
	}
}
