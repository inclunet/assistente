package commandjobevents

import (
	"context"
	"testing"
	"time"
)

func TestClaimBatchDeadLettersExpiredLeaseAtAttemptLimit(t *testing.T) {
	store := testStore(t)
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 1)
	if _, err := store.EnsureReplayPolicyEpoch(context.Background(), ProducerType, clock.Add(-time.Minute), time.Hour); err != nil {
		t.Fatal(err)
	}
	fact := testFact(t, clock)
	insertTestFact(t, store, fact)
	if claimed, err := store.Claim(context.Background(), "worker-a", 1); err != nil || len(claimed) != 1 {
		t.Fatalf("claim inicial=%d err=%v", len(claimed), err)
	}
	clock = clock.Add(2 * time.Minute)
	claimed, more, err := store.ClaimBatch(context.Background(), "worker-b", 1)
	if err != nil || len(claimed) != 0 || more {
		t.Fatalf("claim de lease exaurida=(%d,%v,%v)", len(claimed), more, err)
	}
	row, err := store.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != DeliveryDeadLetter || row.LastErrorCode != "attempt_limit" || row.Attempts != 1 {
		t.Fatalf("estado após crashloop=%+v", row)
	}
}

func TestRequeueExpiredLeasesDeadLettersAtAttemptLimit(t *testing.T) {
	store := testStore(t)
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 1)
	if _, err := store.EnsureReplayPolicyEpoch(context.Background(), ProducerType, clock.Add(-time.Minute), time.Hour); err != nil {
		t.Fatal(err)
	}
	fact := testFact(t, clock)
	insertTestFact(t, store, fact)
	if claimed, err := store.Claim(context.Background(), "worker-a", 1); err != nil || len(claimed) != 1 {
		t.Fatalf("claim inicial=%d err=%v", len(claimed), err)
	}
	clock = clock.Add(2 * time.Minute)
	processed, more, err := store.RequeueExpiredLeases(context.Background(), 1)
	if err != nil || processed != 1 || more {
		t.Fatalf("requeue exaurida=(%d,%v,%v)", processed, more, err)
	}
	row, err := store.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != DeliveryDeadLetter || row.LastErrorCode != "attempt_limit" || row.Attempts != 1 {
		t.Fatalf("estado após requeue=%+v", row)
	}
}
