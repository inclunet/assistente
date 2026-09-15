package commandjobevents

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedRequeueFacts(t *testing.T, store *Store, clock time.Time, count int) []Fact {
	t.Helper()
	ctx := context.Background()
	if _, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, clock.Add(-time.Minute), time.Hour); err != nil {
		t.Fatalf("seed replay policy epoch: %v", err)
	}
	facts := make([]Fact, 0, count)
	for i := 0; i < count; i++ {
		fact := testFact(t, clock.Add(time.Duration(i)*time.Second))
		fact.Sequence = i + 1
		insertTestFact(t, store, fact)
		facts = append(facts, fact)
	}
	claimed, err := store.Claim(ctx, "worker", count)
	if err != nil {
		t.Fatalf("claim facts: %v", err)
	}
	if len(claimed) != count {
		t.Fatalf("claimed facts=%d, want %d", len(claimed), count)
	}
	return facts
}

func expireRequeueFacts(t *testing.T, store *Store, ids []string, expiry time.Time) {
	t.Helper()
	if err := store.DB().Model(&ActivationOutbox{}).Where("source_event_id IN ?", ids).Update("lease_expires_at", expiry).Error; err != nil {
		t.Fatalf("expire leases: %v", err)
	}
}

func TestRequeueExpiredLeasesProcessesOneBoundedBatchAndReportsMore(t *testing.T) {
	store := testStore(t)
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 2)
	facts := seedRequeueFacts(t, store, clock, 3)
	ids := []string{facts[0].SourceEventID, facts[1].SourceEventID, facts[2].SourceEventID}
	expireRequeueFacts(t, store, ids, clock.Add(-time.Second))

	processed, more, err := store.RequeueExpiredLeases(context.Background(), 2)
	if err != nil {
		t.Fatalf("first requeue: %v", err)
	}
	if processed != 2 || !more {
		t.Fatalf("first requeue=(%d,%v), want (2,true)", processed, more)
	}
	processed, more, err = store.RequeueExpiredLeases(context.Background(), 2)
	if err != nil {
		t.Fatalf("second requeue: %v", err)
	}
	if processed != 1 || more {
		t.Fatalf("second requeue=(%d,%v), want (1,false)", processed, more)
	}

	var rows []ActivationOutbox
	if err := store.DB().Order("source_event_id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load requeued rows: %v", err)
	}
	if len(rows) != len(ids) {
		t.Fatalf("rows=%d, want %d", len(rows), len(ids))
	}
	for _, row := range rows {
		if row.DeliveryState != DeliveryPending || row.LeaseOwner != nil || row.LeaseExpiresAt != nil {
			t.Fatalf("requeued row still leased: %+v", row)
		}
	}
}

func TestRequeueExpiredLeasesLeavesLiveAndPendingRowsUntouched(t *testing.T) {
	store := testStore(t)
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 2)
	facts := seedRequeueFacts(t, store, clock, 2)
	pending := testFact(t, clock.Add(2*time.Second))
	pending.Sequence = 3
	insertTestFact(t, store, pending)

	expiredID := facts[0].SourceEventID
	liveID := facts[1].SourceEventID
	expireRequeueFacts(t, store, []string{expiredID}, clock.Add(-time.Second))
	liveExpiry := clock.Add(time.Minute)
	if err := store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", liveID).Update("lease_expires_at", liveExpiry).Error; err != nil {
		t.Fatalf("set live lease: %v", err)
	}

	processed, more, err := store.RequeueExpiredLeases(context.Background(), 2)
	if err != nil {
		t.Fatalf("requeue mixed leases: %v", err)
	}
	if processed != 1 || more {
		t.Fatalf("requeue mixed=(%d,%v), want (1,false)", processed, more)
	}

	var expired, live, stillPending ActivationOutbox
	for _, target := range []struct {
		id  string
		out *ActivationOutbox
	}{
		{expiredID, &expired}, {liveID, &live}, {pending.SourceEventID, &stillPending},
	} {
		if err := store.DB().Where("source_event_id = ?", target.id).Take(target.out).Error; err != nil {
			t.Fatalf("load row %s: %v", target.id, err)
		}
	}
	if expired.DeliveryState != DeliveryPending || expired.LeaseOwner != nil || expired.LeaseExpiresAt != nil {
		t.Fatalf("expired row not requeued: %+v", expired)
	}
	if live.DeliveryState != DeliveryProcessing || live.LeaseOwner == nil || live.LeaseExpiresAt == nil || !live.LeaseExpiresAt.Equal(liveExpiry) {
		t.Fatalf("live lease changed: %+v", live)
	}
	if stillPending.DeliveryState != DeliveryPending || stillPending.LeaseOwner != nil || stillPending.LeaseExpiresAt != nil {
		t.Fatalf("pending row changed: %+v", stillPending)
	}
}

func TestRequeueExpiredLeasesValidatesLimitAndCancellation(t *testing.T) {
	store := testStore(t)
	for _, limit := range []int{0, -1, maxRequeueBatch + 1} {
		processed, more, err := store.RequeueExpiredLeases(context.Background(), limit)
		if !errors.Is(err, ErrInvalidFact) || processed != 0 || more {
			t.Fatalf("limit=%d result=(%d,%v,%v), want invalid", limit, processed, more, err)
		}
	}
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 2)
	facts := seedRequeueFacts(t, store, clock, 1)
	expireRequeueFacts(t, store, []string{facts[0].SourceEventID}, clock.Add(-time.Second))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	processed, more, err := store.RequeueExpiredLeases(canceled, 1)
	if !errors.Is(err, context.Canceled) || processed != 0 || more {
		t.Fatalf("canceled result=(%d,%v,%v), want context.Canceled", processed, more, err)
	}
	row, err := store.Get(context.Background(), facts[0].SourceEventID)
	if err != nil {
		t.Fatalf("load canceled row: %v", err)
	}
	if row.DeliveryState != DeliveryProcessing || row.LeaseOwner == nil || row.LeaseExpiresAt == nil {
		t.Fatalf("cancelamento alterou lease: %+v", row)
	}
}
