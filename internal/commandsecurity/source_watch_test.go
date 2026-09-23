package commandsecurity

import (
	"context"
	"errors"
	"testing"
)

func TestSourceWatchSurvivesConfigurationButNotSecurityInvalidation(t *testing.T) {
	for _, operation := range []string{"publish", "mutate"} {
		t.Run(operation, func(t *testing.T) {
			s := newEpochServiceForTest(t)
			epoch := executionSnapshot(t, s)
			source, releaseSource, err := s.WatchSecurityEpoch(context.Background(), epoch)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseSource()
			command, releaseCommand, err := s.WatchEpoch(context.Background(), epoch)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseCommand()
			if operation == "publish" {
				err = s.PublishAuthenticatedConfiguration(context.Background(), epoch, func(context.Context) error { return nil }, func() error { return nil })
			} else {
				err = s.MutateUserConfiguration(context.Background(), epoch.UserID, func() error { return nil })
			}
			if err != nil {
				t.Fatal(err)
			}
			assertExecutionDone(t, command)
			if source.Err() != nil {
				t.Fatal("publicação encerrou fonte ainda viva", source.Err())
			}
			if err := s.InvalidateSession(context.Background(), epoch.UserID, epoch.SessionID); err != nil {
				t.Fatal(err)
			}
			assertExecutionDone(t, source)
			releaseSource()
			releaseSource()
			if executionWatchCount(s) != 0 {
				t.Fatal("inscrições vazaram")
			}
		})
	}
}

func TestSourceWatchRejectsStaleAndCancelledSubscriptions(t *testing.T) {
	for _, stale := range []bool{false, true} {
		s := newEpochServiceForTest(t)
		epoch := executionSnapshot(t, s)
		ctx, cancel := context.WithCancel(context.Background())
		want := context.Canceled
		if stale {
			if err := s.InvalidateSecurity(ctx); err != nil {
				t.Fatal(err)
			}
			want = ErrStaleEpoch
		} else {
			cancel()
		}
		watch, release, err := s.WatchSecurityEpoch(ctx, epoch)
		cancel()
		if !errors.Is(err, want) || watch != nil || release != nil || executionWatchCount(s) != 0 {
			t.Fatalf("inscrição inválida aceita: watch=%v releaseNil=%v err=%v", watch, release == nil, err)
		}
	}
}

func TestSourceWatchEndsOnParentLockAndDrain(t *testing.T) {
	for _, reason := range []string{"parent", "lock", "drain"} {
		t.Run(reason, func(t *testing.T) {
			s := newEpochServiceForTest(t)
			epoch := executionSnapshot(t, s)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			watch, release, err := s.WatchSecurityEpoch(parent, epoch)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			switch reason {
			case "parent":
				cancel()
			case "lock":
				err = s.InvalidateSecurity(context.Background())
			case "drain":
				_, err = s.CloseAndDrain(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			assertExecutionDone(t, watch)
		})
	}
}
