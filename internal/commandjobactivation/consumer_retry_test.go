package commandjobactivation

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

func TestRunPassDeadLettersPermanentAndContinuesBatch(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	for sequence := 1; sequence <= 2; sequence++ {
		current := fact
		current.SourceEventID, _ = freshID()
		current.RunEventID = current.SourceEventID
		current.Sequence = sequence
		if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, current) }); err != nil {
			t.Fatal(err)
		}
	}
	c.ports.Authorize = func(context.Context, *gorm.DB, commandjobevents.Fact, *string) (commandactivation.Owner, error) {
		return commandactivation.Owner{}, commandautomation.ErrStale
	}
	result, err := c.RunPass(context.Background(), "worker-a", 2)
	if err != nil || result.Claimed != 2 || result.Processed != 2 || result.DeadLettered != 2 {
		t.Fatalf("pass permanente=%+v err=%v", result, err)
	}
	var dead int64
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("delivery_state = ?", commandjobevents.DeliveryDeadLetter).Count(&dead).Error; err != nil {
		t.Fatal(err)
	}
	if dead != 2 {
		t.Fatalf("dead-lettered=%d, want 2", dead)
	}
}

func TestRunPassRetriesTransientAndDeadLettersAtMaxAttempts(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, fact) }); err != nil {
		t.Fatal(err)
	}
	c.ports.Authorize = func(context.Context, *gorm.DB, commandjobevents.Fact, *string) (commandactivation.Owner, error) {
		return commandactivation.Owner{}, errors.New("runtime temporarily unavailable")
	}
	for attempt := 1; attempt <= commandjobevents.DefaultMaxAttempts; attempt++ {
		result, err := c.RunPass(context.Background(), "worker-a", 1)
		wantProcessed := 0
		wantDeadLettered := 0
		if attempt == commandjobevents.DefaultMaxAttempts {
			wantProcessed = 1
			wantDeadLettered = 1
		}
		if err != nil || result.Claimed != 1 || result.Processed != wantProcessed || result.DeadLettered != wantDeadLettered {
			t.Fatalf("tentativa %d=%+v err=%v", attempt, result, err)
		}
		row, getErr := out.Get(context.Background(), fact.SourceEventID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if attempt < commandjobevents.DefaultMaxAttempts && row.DeliveryState != commandjobevents.DeliveryPending {
			t.Fatalf("tentativa %d deixou estado=%s", attempt, row.DeliveryState)
		}
	}
	row, err := out.Get(context.Background(), fact.SourceEventID)
	if err != nil {
		t.Fatal(err)
	}
	if row.DeliveryState != commandjobevents.DeliveryDeadLetter || row.LastErrorCode != "transient_consume_failure" || row.Attempts != commandjobevents.DefaultMaxAttempts {
		t.Fatalf("estado final=%+v", row)
	}
}
