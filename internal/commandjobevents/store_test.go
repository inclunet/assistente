package commandjobevents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func testUUIDv7(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(Models()...); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func testFact(t *testing.T, now time.Time) Fact {
	t.Helper()
	eventID := testUUIDv7(t)
	return Fact{
		SchemaVersion: SchemaVersion, EventName: SchemaVersion, SourceEventID: eventID,
		UserID: testUUIDv7(t), JobDatabaseID: testUUIDv7(t), JobSlug: "job-a", RunID: "run_opaque_store",
		RunEventID: eventID, Sequence: 1, State: StateQueued, OccurredAt: now,
		RootOriginType: "manual", RootOriginID: testUUIDv7(t), Provenance: map[string]any{"b": 2, "a": 1},
		SourceReplayPolicyGeneration: "jobs.runtime:1", SourceReplayDeadline: now.Add(time.Hour),
	}
}

func insertTestFact(t *testing.T, store *Store, fact Fact) {
	t.Helper()
	if err := store.DB().Transaction(func(tx *gorm.DB) error { return store.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
}

func TestStoreEpochFingerprintAndIdempotency(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if store.Ready(nil) {
		t.Fatal("nil context must not report readiness")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if store.Ready(canceled) {
		t.Fatal("canceled context must not report readiness")
	}
	if store.Ready(ctx) {
		t.Fatal("tables without a producer epoch must not report readiness")
	}

	firstEpoch, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, now.Add(-time.Minute), time.Hour)
	if err != nil {
		t.Fatalf("create epoch: %v", err)
	}
	sameEpoch, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, firstEpoch.EffectiveAt, 2*time.Hour)
	if !errors.Is(err, ErrFingerprintConflict) {
		t.Fatalf("conflicting epoch horizon error = %v, want fingerprint conflict", err)
	}
	sameEpoch, err = store.EnsureReplayPolicyEpoch(ctx, ProducerType, firstEpoch.EffectiveAt, time.Hour)
	if err != nil {
		t.Fatalf("idempotent epoch: %v", err)
	}
	if sameEpoch.ID != firstEpoch.ID || sameEpoch.Generation != firstEpoch.Generation {
		t.Fatalf("epoch changed on replay: first=%+v second=%+v", firstEpoch, sameEpoch)
	}
	secondEpoch, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, now, 2*time.Hour)
	if err != nil {
		t.Fatalf("create second epoch: %v", err)
	}
	if secondEpoch.Generation != firstEpoch.Generation+1 {
		t.Fatalf("epoch generation = %d, want %d", secondEpoch.Generation, firstEpoch.Generation+1)
	}
	if !store.Ready(ctx) {
		t.Fatal("store should be ready after the producer epoch is bootstrapped")
	}

	fact := testFact(t, now)
	insertTestFact(t, store, fact)
	insertTestFact(t, store, fact)
	var count int64
	if err := store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent insert count = %d, want 1", count)
	}

	conflict := fact
	conflict.State = StateStarted
	err = store.DB().Transaction(func(tx *gorm.DB) error { return store.InsertFactTx(tx, conflict) })
	if !errors.Is(err, ErrFingerprintConflict) {
		t.Fatalf("conflicting source event error = %v, want fingerprint conflict", err)
	}

	if err := store.DB().Create(&ActivationOutbox{SourceEventID: "legacy-event-id"}).Error; err == nil {
		t.Fatal("outbox accepted a non-UUIDv7 source_event_id")
	}
	spaced := testFact(t, now.Add(time.Second))
	spaced.SourceEventID = " " + spaced.SourceEventID
	spaced.RunEventID = spaced.SourceEventID
	err = store.DB().Transaction(func(tx *gorm.DB) error { return store.InsertFactTx(tx, spaced) })
	if !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("non-canonical source event error = %v, want invalid fact", err)
	}
	if _, err := store.EnsureReplayPolicyEpoch(ctx, "other.producer", now, time.Hour); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("arbitrary producer error = %v, want invalid fact", err)
	}
}

func TestFingerprintUsesJCSAndDomainWithoutSelfField(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	left := testFact(t, now)
	right := left
	left.Provenance = map[string]any{"z": 1, "a": map[string]any{"b": 2, "a": 1}}
	right.Provenance = map[string]any{"a": map[string]any{"a": 1, "b": 2}, "z": 1}
	first, err := Fingerprint(left)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("JCS fingerprint depends on map order or has wrong size: %q != %q", first, second)
	}
	withSelf := left
	withSelf.EventFingerprint = "prior-value"
	third, err := Fingerprint(withSelf)
	if err != nil {
		t.Fatal(err)
	}
	if third != first {
		t.Fatalf("self fingerprint changed the digest: %q != %q", third, first)
	}
}

func TestStoreRejectsReplayGenerationOverflow(t *testing.T) {
	store := testStore(t)
	maxInt64 := int64(^uint64(0) >> 1)
	if err := store.DB().Create(&ReplayPolicyEpoch{
		ID: testUUIDv7(t), ProducerType: ProducerType, Generation: maxInt64,
		EffectiveAt: time.Now().UTC().Add(-time.Hour), ReplayHorizonSeconds: 3600,
		CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed max replay generation: %v", err)
	}
	if _, err := store.EnsureReplayPolicyEpoch(context.Background(), ProducerType, time.Now().UTC(), time.Hour); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("generation overflow error = %v, want invalid fact", err)
	}
}

func TestStoreClaimAckRetryDeadLetterAndExpiredLease(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 2)
	if _, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, clock.Add(-time.Minute), time.Hour); err != nil {
		t.Fatalf("seed replay policy epoch: %v", err)
	}

	fact := testFact(t, clock)
	insertTestFact(t, store, fact)
	claimed, err := store.Claim(ctx, "worker-a", 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Attempts != 1 || claimed[0].DeliveryState != DeliveryProcessing {
		t.Fatalf("claim result = %+v", claimed)
	}
	if err := store.Ack(ctx, fact.SourceEventID, "worker-other"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong-owner ack = %v, want lease lost", err)
	}
	if err := store.Retry(ctx, fact.SourceEventID, "worker-a", "temporary_failure"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	claimed, err = store.Claim(ctx, "worker-b", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim after retry = %v, rows=%d", err, len(claimed))
	}
	if err := store.DeadLetter(ctx, fact.SourceEventID, "worker-b", "permanent_failure"); err != nil {
		t.Fatalf("dead letter: %v", err)
	}
	row, err := store.Get(ctx, fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != DeliveryDeadLetter || row.LastErrorCode != "permanent_failure" {
		t.Fatalf("dead-letter row = %+v", row)
	}

	leaseFact := testFact(t, clock.Add(time.Second))
	leaseFact.Sequence = 2
	insertTestFact(t, store, leaseFact)
	if claimed, err := store.Claim(ctx, "worker-c", 1); err != nil || len(claimed) != 1 {
		t.Fatalf("claim lease row = %v, rows=%d", err, len(claimed))
	}
	clock = clock.Add(2 * time.Minute)
	requeued, err := store.RequeueExpiredLeases(ctx)
	if err != nil || requeued != 1 {
		t.Fatalf("requeue expired = %d, err=%v", requeued, err)
	}
	if err := store.Ack(ctx, leaseFact.SourceEventID, "worker-c"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("ack after expiration = %v, want lease lost", err)
	}
	claimed, err = store.Claim(ctx, "worker-c2", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim expired lease = %v, rows=%d", err, len(claimed))
	}
	if err := store.Ack(ctx, leaseFact.SourceEventID, "worker-c2"); err != nil {
		t.Fatalf("ack reclaimed lease = %v", err)
	}

	deadLetterFact := testFact(t, clock.Add(time.Second))
	deadLetterFact.Sequence = 3
	insertTestFact(t, store, deadLetterFact)
	claimed, err = store.Claim(ctx, "worker-d", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("first max-attempt claim = %v, rows=%d", err, len(claimed))
	}
	if err := store.Retry(ctx, deadLetterFact.SourceEventID, "worker-d", "temporary_failure"); err != nil {
		t.Fatalf("first max-attempt retry: %v", err)
	}
	claimed, err = store.Claim(ctx, "worker-d2", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("second max-attempt claim = %v, rows=%d", err, len(claimed))
	}
	if err := store.Retry(ctx, deadLetterFact.SourceEventID, "worker-d2", "attempt_limit"); err != nil {
		t.Fatalf("retry at max attempts: %v", err)
	}
	row, err = store.Get(ctx, deadLetterFact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != DeliveryDeadLetter || row.LastErrorCode != "attempt_limit" {
		t.Fatalf("automatic dead-letter row = %+v", row)
	}
}
