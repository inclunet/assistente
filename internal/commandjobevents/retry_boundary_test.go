package commandjobevents

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryBoundaryDeadLettersExactlyAtMaxAttemptsAndNeverRequeues(t *testing.T) {
	store := testStore(t)
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 3)
	ctx := context.Background()
	if _, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, clock.Add(-time.Minute), time.Hour); err != nil {
		t.Fatal(err)
	}
	fact := testFact(t, clock)
	insertTestFact(t, store, fact)
	for attempt, owner := range []string{"worker-1", "worker-2", "worker-3"} {
		claimed, err := store.Claim(ctx, owner, 1)
		if err != nil || len(claimed) != 1 || claimed[0].Attempts != attempt+1 {
			t.Fatalf("claim attempt %d=(%d,%v), row=%+v", attempt+1, len(claimed), err, claimed)
		}
		retryErr := store.Retry(ctx, fact.SourceEventID, owner, "temporary_failure")
		if retryErr != nil {
			t.Fatalf("retry attempt %d: %v", attempt+1, retryErr)
		}
	}
	row, err := store.Get(ctx, fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Attempts != 3 || row.DeliveryState != DeliveryDeadLetter || row.LastErrorCode != "temporary_failure" || row.LeaseOwner != nil {
		t.Fatalf("boundary state=%+v", row)
	}
	if _, err := store.Claim(ctx, "worker-after-limit", 1); err != nil {
		t.Fatal(err)
	}
	row, err = store.Get(ctx, fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Attempts != 3 || row.DeliveryState != DeliveryDeadLetter {
		t.Fatalf("dead letter requeued or incremented: %+v", row)
	}
	if err := store.Retry(ctx, fact.SourceEventID, "worker-after-limit", "again"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("retry after dead letter=%v, want ErrLeaseLost", err)
	}
}
