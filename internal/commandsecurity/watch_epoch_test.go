package commandsecurity

import (
	"context"
	"errors"
	"testing"
)

func TestWatchEpochRejectsInvalidationBetweenCaptureAndSubscription(t *testing.T) {
	s := newEpochServiceForTest(t)
	epoch := executionSnapshot(t, s)
	if err := s.InvalidateSession(context.Background(), epoch.UserID, epoch.SessionID); err != nil {
		t.Fatal(err)
	}
	ctx, release, err := s.WatchEpoch(context.Background(), epoch)
	if !errors.Is(err, ErrStaleEpoch) || ctx != nil || release != nil || executionWatchCount(s) != 0 {
		t.Fatal("epoch obsoleto inscrito", err)
	}
}

func TestWatchEpochIsScopedAndReleaseIsIdempotent(t *testing.T) {
	s := newEpochServiceForTest(t)
	epochA := executionSnapshot(t, s)
	epochB := executionSnapshot(t, s)
	a, releaseA, err := s.WatchEpoch(context.Background(), epochA)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA()
	b, releaseB, err := s.WatchEpoch(context.Background(), epochB)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseB()
	if err := s.InvalidateSession(context.Background(), epochA.UserID, epochA.SessionID); err != nil {
		t.Fatal(err)
	}
	assertExecutionDone(t, a)
	if b.Err() != nil {
		t.Fatal("invalidação atingiu outra sessão", b.Err())
	}
	releaseA()
	releaseA()
	releaseB()
	releaseB()
	if executionWatchCount(s) != 0 {
		t.Fatal("inscrições vazaram")
	}
	assertExecutionDone(t, b)
}

func TestWatchEpochCancelledParentDoesNotSubscribe(t *testing.T) {
	s := newEpochServiceForTest(t)
	epoch := executionSnapshot(t, s)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, release, err := s.WatchEpoch(parent, epoch)
	if !errors.Is(err, context.Canceled) || ctx != nil || release != nil || executionWatchCount(s) != 0 {
		t.Fatal("contexto cancelado inscrito", err)
	}
}
