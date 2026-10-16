package commandjobevents

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestStoreNormalizesEpochAndFactTimesToUTC(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	clock := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 2)

	brt := time.FixedZone("BRT", -3*60*60)
	india := time.FixedZone("IST", 5*60*60+30*60)
	effectiveUTC := time.Date(2026, 9, 17, 9, 31, 0, 0, time.UTC)
	effectiveLocal := effectiveUTC.In(india)

	firstEpoch, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, effectiveLocal, time.Hour)
	if err != nil {
		t.Fatalf("create epoch with offset: %v", err)
	}
	secondEpoch, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, effectiveUTC, time.Hour)
	if err != nil {
		t.Fatalf("replay epoch with UTC: %v", err)
	}
	if firstEpoch.ID != secondEpoch.ID || !secondEpoch.EffectiveAt.Equal(effectiveUTC) {
		t.Fatalf("epoch was not normalized/idempotent: first=%+v second=%+v", firstEpoch, secondEpoch)
	}
	futureEpoch, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, clock.Add(time.Hour).In(brt), 2*time.Hour)
	if err != nil {
		t.Fatalf("create future epoch: %v", err)
	}

	factAtUTC := effectiveUTC.Add(30 * time.Second)
	fact := testFact(t, factAtUTC.In(brt))
	insertTestFact(t, store, fact)

	replay := fact
	replay.OccurredAt = factAtUTC.In(india)
	if err := store.DB().Transaction(func(tx *gorm.DB) error { return store.InsertFactTx(tx, replay) }); err != nil {
		t.Fatalf("equivalent offset replay: %v", err)
	}

	row, err := store.Get(ctx, fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.OccurredAt.Equal(factAtUTC) || !row.CreatedAt.Equal(clock) {
		t.Fatalf("persisted timestamps are not UTC-equivalent: occurred=%v created=%v", row.OccurredAt, row.CreatedAt)
	}
	if row.SourceReplayPolicyGeneration != fmt.Sprintf("%s:%d", ProducerType, firstEpoch.Generation) || futureEpoch.Generation == firstEpoch.Generation {
		t.Fatalf("future epoch was selected for earlier fact: row=%q current=%d future=%d", row.SourceReplayPolicyGeneration, firstEpoch.Generation, futureEpoch.Generation)
	}
	var count int64
	if err := store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("equivalent offset replay inserted %d rows, want 1", count)
	}

}

func TestStoreLeaseComparisonsNormalizeClockToUTC(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	clock := base.In(time.FixedZone("BRT", -3*60*60))
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 2)
	if _, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, base.Add(-time.Minute), time.Hour); err != nil {
		t.Fatal(err)
	}

	fact := testFact(t, base.Add(-time.Second).In(time.FixedZone("IST", 5*60*60+30*60)))
	insertTestFact(t, store, fact)
	claimed, _, err := store.ClaimBatch(ctx, "owner-a", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim with offset clock: rows=%d err=%v", len(claimed), err)
	}
	if err := store.Ack(ctx, fact.SourceEventID, "owner-a"); err != nil {
		t.Fatalf("ack with offset clock: %v", err)
	}

	second := testFact(t, base.Add(-2*time.Second).In(time.FixedZone("IST", 5*60*60+30*60)))
	insertTestFact(t, store, second)
	claimed, _, err = store.ClaimBatch(ctx, "owner-b", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("second claim: rows=%d err=%v", len(claimed), err)
	}
	clock = base.Add(2 * time.Minute).In(time.FixedZone("IST", 5*60*60+30*60))
	processed, _, err := store.RequeueExpiredLeases(ctx, 1)
	if err != nil || processed != 1 {
		t.Fatalf("requeue with offset clock: processed=%d err=%v", processed, err)
	}
	row, err := store.Get(ctx, second.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != DeliveryPending {
		t.Fatalf("requeued state = %q, want pending", row.DeliveryState)
	}
}
