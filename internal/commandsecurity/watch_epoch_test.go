package commandsecurity

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestWatchEpochSeparateAcquisitionAndLifetime(t *testing.T) {
	for _, securityOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "configuration", true: "security"}[securityOnly], func(t *testing.T) {
			s := newEpochServiceForTest(t)
			epoch := executionSnapshot(t, s)
			watch := s.WatchEpochForLifetime
			if securityOnly {
				watch = s.WatchSecurityEpochForLifetime
			}
			sem := s.gate.semaphore()
			if err := sem.Acquire(context.Background(), math.MaxInt64); err != nil {
				t.Fatal(err)
			}
			locked := true
			defer func() {
				if locked {
					sem.Release(math.MaxInt64)
				}
			}()
			read, cancelRead := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancelRead()
			ctx, release, err := watch(read, context.Background(), epoch)
			if !errors.Is(err, context.DeadlineExceeded) || ctx != nil || release != nil || executionWatchCount(s) != 0 {
				t.Fatalf("watch aguardou ou publicou após timeout: %v", err)
			}
			sem.Release(math.MaxInt64)
			locked = false
			read, cancelRead = context.WithCancel(context.Background())
			defer cancelRead()
			life, cancelLife := context.WithCancel(context.Background())
			defer cancelLife()
			ctx, release, err = watch(read, life, epoch)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			cancelRead()
			if ctx.Err() != nil {
				t.Fatal("fim da aquisição cancelou inscrição retida")
			}
			cancelLife()
			assertExecutionDone(t, ctx)
			release()
			ctx, release, err = watch(context.Background(), context.Background(), epoch)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if err := s.InvalidateSession(context.Background(), epoch.UserID, epoch.SessionID); err != nil {
				t.Fatal(err)
			}
			assertExecutionDone(t, ctx)
		})
	}
}

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
