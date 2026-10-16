package commandjobactivation

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

func TestRunPassClaimsAndConsumesBoundedBatch(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	for sequence, state := range []string{commandjobevents.StateQueued, commandjobevents.StateStarted} {
		current := fact
		current.SourceEventID, _ = freshID()
		current.RunEventID = current.SourceEventID
		current.Sequence = sequence + 1
		current.State = state
		if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, current) }); err != nil {
			t.Fatal(err)
		}
	}

	first, err := c.RunPass(context.Background(), "worker-a", 1)
	if err != nil || first.Claimed != 1 || first.Processed != 1 || first.Applied != 1 || !first.More {
		t.Fatalf("first pass=%+v err=%v", first, err)
	}
	second, err := c.RunPass(context.Background(), "worker-a", 1)
	if err != nil || second.Claimed != 1 || second.Processed != 1 || second.Applied != 1 || second.More {
		t.Fatalf("second pass=%+v err=%v", second, err)
	}
	var delivered int64
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("delivery_state = ?", commandjobevents.DeliveryDelivered).Count(&delivered).Error; err != nil {
		t.Fatal(err)
	}
	if delivered != 2 {
		t.Fatalf("delivered=%d, want 2", delivered)
	}
}

func TestRunPassCancellationDoesNotClaimWork(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := c.RunPass(canceled, "worker-a", 1)
	if !errors.Is(err, context.Canceled) || result != (PassResult{}) {
		t.Fatalf("canceled pass=%+v err=%v", result, err)
	}
	row, err := out.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != commandjobevents.DeliveryPending || row.LeaseOwner != nil || row.LeaseExpiresAt != nil {
		t.Fatalf("cancelamento reivindicou ocorrência: %+v", row)
	}
}

func TestRunPassKeepsDeliveryOwnerAsLeaseAuthority(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	claimed, _, err := out.ClaimBatch(context.Background(), "worker-a", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%d err=%v", len(claimed), err)
	}
	if _, err := c.Consume(context.Background(), fact.SourceEventID, "worker-b"); !errors.Is(err, commandjobevents.ErrLeaseLost) {
		t.Fatalf("wrong owner consume=%v", err)
	}
	row, err := out.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != commandjobevents.DeliveryProcessing || row.LeaseOwner == nil || *row.LeaseOwner != "worker-a" {
		t.Fatalf("lease alterada por owner divergente: %+v", row)
	}
}
