package commanddecision

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func acceptedBatchFixture(t *testing.T, count int) (*Store, *gorm.DB, *time.Time, []Request) {
	t.Helper()
	store, db, clock, first := acceptedFixture(t)
	requests := []Request{first}
	for i := 1; i < count; i++ {
		request := defaultRequest(t, *clock)
		request.UserID = first.UserID
		request.SessionID = first.SessionID
		request.AuthGeneration = first.AuthGeneration
		request.SecurityGeneration = first.SecurityGeneration
		state, err := store.Decide(context.Background(), request)
		if err != nil || state != Accepted {
			t.Fatalf("fixture batch accepted: state=%q err=%v", state, err)
		}
		requests = append(requests, request)
	}
	return store, db, clock, requests
}

func TestConsumeBatchForDatabaseConsumesAllAndAppliesOnce(t *testing.T) {
	store, db, _, requests := acceptedBatchFixture(t, 2)
	applyCalls := 0
	err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(tx *gorm.DB) error {
		applyCalls++
		for _, request := range requests {
			if err := tx.Create(&decisionEffectRow{ID: request.MutationID, Value: "batch-applied"}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if applyCalls != 1 || countEffects(t, db) != 2 {
		t.Fatalf("apply/effects: calls=%d effects=%d", applyCalls, countEffects(t, db))
	}
	for _, request := range requests {
		if row := loadReceipt(t, db, request.DecisionID); row.State != Consumed || row.ConsumedAt == nil {
			t.Fatalf("receipt não consumida: %+v", row)
		}
		if countEvents(t, db, request.DecisionID) != 3 {
			t.Fatalf("eventos da receipt: %d", countEvents(t, db, request.DecisionID))
		}
	}
}

func TestConsumeBatchForDatabaseRollsBackWhenApplyFailsOnLastReceipt(t *testing.T) {
	store, db, _, requests := acceptedBatchFixture(t, 2)
	wantErr := errors.New("falha no último efeito")
	applyCalls := 0
	err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(tx *gorm.DB) error {
		applyCalls++
		if err := tx.Create(&decisionEffectRow{ID: requests[0].MutationID, Value: "must-rollback"}).Error; err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) || applyCalls != 1 {
		t.Fatalf("erro/apply: err=%v calls=%d", err, applyCalls)
	}
	for _, request := range requests {
		if row := loadReceipt(t, db, request.DecisionID); row.State != Accepted || row.ConsumedAt != nil {
			t.Fatalf("rollback não restaurou receipt: %+v", row)
		}
	}
	if countEffects(t, db) != 0 {
		t.Fatal("efeito parcial sobreviveu ao rollback")
	}
}

func TestConsumeBatchForDatabaseRejectsReplayDuplicateAndMixedIdentity(t *testing.T) {
	store, db, _, requests := acceptedBatchFixture(t, 2)
	if err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(*gorm.DB) error { return nil }); err != nil {
		t.Fatal(err)
	}
	applyCalls := 0
	if err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(*gorm.DB) error { applyCalls++; return nil }); !errors.Is(err, ErrStale) {
		t.Fatalf("replay: %v", err)
	}
	if applyCalls != 0 {
		t.Fatal("replay chamou apply")
	}

	store, db, _, requests = acceptedBatchFixture(t, 2)
	duplicate := []Request{requests[0], requests[0]}
	if err := store.ConsumeBatchForDatabase(context.Background(), db, duplicate, func(*gorm.DB) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("IDs duplicados: %v", err)
	}
	if loadReceipt(t, db, requests[0].DecisionID).State != Accepted {
		t.Fatal("ID duplicado alterou receipt")
	}

	store, db, _, requests = acceptedBatchFixture(t, 2)
	requests[1].SessionID = testUUIDv7(t)
	if err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(*gorm.DB) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("sessões misturadas: %v", err)
	}
}

func TestConsumeBatchForDatabaseRejectsExpirationBeforeAndAfterApply(t *testing.T) {
	store, db, clock, requests := acceptedBatchFixture(t, 2)
	*clock = requests[0].ExpiresAt.Add(time.Millisecond)
	if err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(*gorm.DB) error { return nil }); !errors.Is(err, ErrStale) {
		t.Fatalf("expiração antes: %v", err)
	}
	for _, request := range requests {
		if loadReceipt(t, db, request.DecisionID).State != Accepted {
			t.Fatal("expiração antes consumiu receipt")
		}
	}

	store, db, clock, requests = acceptedBatchFixture(t, 2)
	if err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(tx *gorm.DB) error {
		*clock = requests[0].ExpiresAt.Add(time.Millisecond)
		return tx.Create(&decisionEffectRow{ID: requests[0].MutationID, Value: "must-rollback"}).Error
	}); !errors.Is(err, ErrStale) {
		t.Fatalf("expiração depois: %v", err)
	}
	if countEffects(t, db) != 0 {
		t.Fatal("efeito sobreviveu à expiração após apply")
	}
	for _, request := range requests {
		if loadReceipt(t, db, request.DecisionID).State != Accepted {
			t.Fatal("expiração depois não reverteu receipt")
		}
	}
}

func TestConsumeBatchFreezesExpectedBeforeCallbackMutation(t *testing.T) {
	store, db, clock, requests := acceptedBatchFixture(t, 2)
	originalExpiry := requests[0].ExpiresAt
	err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(*gorm.DB) error {
		requests[0].ExpiresAt = originalExpiry.Add(time.Hour)
		*clock = originalExpiry.Add(time.Millisecond)
		return nil
	})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("mutação do expected relaxou expiração: %v", err)
	}
	if loadReceipt(t, db, requests[0].DecisionID).State != Accepted || loadReceipt(t, db, requests[0].DecisionID).ExpiresMS != originalExpiry.UnixMilli() {
		t.Fatal("callback alterou ou renovou a receipt")
	}
	if countEffects(t, db) != 0 {
		t.Fatal("efeito sobreviveu ao rollback da expiração")
	}
}

func TestConsumeBatchInvalidLastReceiptRollsBackEarlierCAS(t *testing.T) {
	store, db, _, requests := acceptedBatchFixture(t, 2)
	requests[1].Fingerprint = "different-fingerprint"
	calls := 0
	err := store.ConsumeBatchForDatabase(context.Background(), db, requests, func(*gorm.DB) error {
		calls++
		return nil
	})
	if !errors.Is(err, ErrStale) || calls != 0 {
		t.Fatalf("última receipt inválida: err=%v calls=%d", err, calls)
	}
	for _, request := range requests {
		row := loadReceipt(t, db, request.DecisionID)
		if row.State != Accepted || row.ConsumedAt != nil || countEvents(t, db, request.DecisionID) != 2 {
			t.Fatalf("CAS ou auditoria parcial persistiu: %+v", row)
		}
	}
}
