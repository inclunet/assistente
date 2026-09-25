package commandsecurity

import (
	"context"
	"sync"
	"testing"
)

func TestExecutionReleaseRacesWithInvalidationWithoutLeaking(t *testing.T) {
	s := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	for range 100 {
		snapshot, err := s.Capture(context.Background(), user, session)
		if err != nil {
			t.Fatal(err)
		}
		var runCtx context.Context
		release, err := s.AdmitExecution(context.Background(), snapshot, func(context.Context) error { return nil }, func(ctx context.Context) error { runCtx = ctx; return nil })
		if err != nil {
			t.Fatal(err)
		}
		var group sync.WaitGroup
		group.Add(3)
		errors := make(chan error, 1)
		go func() { defer group.Done(); release() }()
		go func() { defer group.Done(); release() }()
		go func() { defer group.Done(); errors <- s.InvalidateSession(context.Background(), user, session) }()
		group.Wait()
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
		assertExecutionDone(t, runCtx)
		if count := executionWatchCount(s); count != 0 {
			t.Fatalf("%d inscrições restantes", count)
		}
	}
}
