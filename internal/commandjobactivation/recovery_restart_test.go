package commandjobactivation

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandjobevents"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRecoveryPendingLeaseReopensSQLiteRequeuesAndConsumesOnce(t *testing.T) {
	consumer, outbox, fact, _, now := fixture(t)
	ctx := context.Background()
	realNow := time.Now().UTC().Truncate(time.Microsecond)
	*now = realNow
	fact.OccurredAt = realNow
	if _, err := outbox.EnsureReplayPolicyEpoch(ctx, commandjobevents.ProducerType, realNow.Add(-time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := consumer.db.Transaction(func(tx *gorm.DB) error { return outbox.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	claimed, _, err := outbox.ClaimBatch(ctx, "crashed-consumer", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim antes da queda=%d err=%v", len(claimed), err)
	}
	if err := consumer.db.Model(&commandjobevents.ActivationOutbox{}).
		Where("source_event_id = ?", fact.SourceEventID).
		Update("lease_expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}

	reopened, reopenedOutbox := reopenRecoveryConsumer(t, consumer)
	processed, more, err := reopenedOutbox.RequeueExpiredLeases(ctx, 1)
	if err != nil || processed != 1 || more {
		t.Fatalf("requeue após restart=%d more=%v err=%v", processed, more, err)
	}
	reclaimed, _, err := reopenedOutbox.ClaimBatch(ctx, "restarted-consumer", 1)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("reclaim após lease expirada=%d err=%v", len(reclaimed), err)
	}
	outcome, err := reopened.Consume(ctx, fact.SourceEventID, "restarted-consumer")
	if err != nil || outcome.Applied != 1 || outcome.Replayed != 0 {
		t.Fatalf("consume recuperado=%+v err=%v, want aplicação única", outcome, err)
	}
	if got := countActivationClaims(t, reopened); got != 1 || countActivationLedger(t, reopened) != 1 {
		t.Fatalf("recovery duplicou estado claims=%d ledger=%d", got, countActivationLedger(t, reopened))
	}
	row, err := reopenedOutbox.Get(ctx, fact.SourceEventID)
	if err != nil || row.DeliveryState != commandjobevents.DeliveryDelivered {
		t.Fatalf("evento recuperado não foi confirmado: row=%+v err=%v", row, err)
	}
	remaining, _, err := reopenedOutbox.ClaimBatch(ctx, "next-consumer", 1)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("evento recuperado entregue foi reclamado: rows=%d err=%v", len(remaining), err)
	}
}

func TestRecoveryDeliveredCommitReopensSQLiteAndIsNotReclaimed(t *testing.T) {
	consumer, outbox, fact, _, _ := fixture(t)
	initial := deliver(t, consumer, outbox, fact)
	if initial.Applied != 1 || initial.Replayed != 0 {
		t.Fatalf("commit inicial=%+v, want applied=1 replayed=0", initial)
	}

	reopened, reopenedOutbox := reopenRecoveryConsumer(t, consumer)
	claimed, _, err := reopenedOutbox.ClaimBatch(context.Background(), "restarted-consumer", 1)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("outbox entregue foi reclamável após restart: claimed=%d err=%v", len(claimed), err)
	}
	row, err := reopenedOutbox.Get(context.Background(), fact.SourceEventID)
	if err != nil || row.DeliveryState != commandjobevents.DeliveryDelivered {
		t.Fatalf("estado entregue após restart=%+v err=%v", row, err)
	}
	if got := countActivationClaims(t, reopened); got != 1 || countActivationLedger(t, reopened) != 1 {
		t.Fatalf("restart alterou estado committed claims=%d ledger=%d", got, countActivationLedger(t, reopened))
	}
}

func reopenRecoveryConsumer(t *testing.T, previous *Consumer) (*Consumer, *commandjobevents.Store) {
	t.Helper()
	dialector, ok := previous.db.Dialector.(*sqlite.Dialector)
	if !ok || dialector.DSN == "" {
		t.Fatalf("fixture não usa SQLite em arquivo: %#v", previous.db.Dialector)
	}
	oldSQL, err := previous.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := oldSQL.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedDB, err := gorm.Open(sqlite.Open(dialector.DSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	reopenedSQL, err := reopenedDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	reopenedSQL.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = reopenedSQL.Close() })
	reopened, err := New(reopenedDB, previous.gate, previous.ports, previous.lease, previous.retention, previous.now)
	if err != nil {
		t.Fatal(err)
	}
	return reopened, commandjobevents.NewStore(reopenedDB)
}
