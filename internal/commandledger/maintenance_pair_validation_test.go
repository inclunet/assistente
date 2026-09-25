package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRecoveryProofRejectsDivergentPairAndRollsBackWholeBatch(t *testing.T) {
	for _, column := range []string{"request_fingerprint", "request_fingerprint_version", "source_type"} {
		t.Run(column, func(t *testing.T) {
			now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
			store, db := testStore(t, &now)
			request := validRequest()
			request.ExpiresAt = now.Add(time.Hour)
			var ids []string
			for i := 0; i < 2; i++ {
				request.InvocationID = uuid.Must(uuid.NewV7()).String()
				ids = append(ids, request.InvocationID)
				if _, err := store.Reserve(context.Background(), request); err != nil {
					t.Fatal(err)
				}
			}
			value := "divergent"
			if column == "source_type" {
				value = "cli"
			}
			if err := db.Model(&ledgerRow{}).Where("invocation_id = ?", ids[1]).Update(column, value).Error; err != nil {
				t.Fatal(err)
			}
			user := request.Owner.UserID
			scope := GenerationScope{UserID: &user, AuthContextType: "local_session", AuthContextID: request.Owner.AuthContextID, SecurityGeneration: request.SecurityGeneration}
			marker := uuid.Must(uuid.NewV7()).String()
			if err := db.Create(&closedGenerationRow{ID: marker, UserID: &user, AuthContextType: scope.AuthContextType, AuthContextID: scope.AuthContextID, SecurityGeneration: scope.SecurityGeneration, ClosedAt: now}).Error; err != nil {
				t.Fatal(err)
			}
			batch, err := store.RecoverClosedGenerationWithProof(context.Background(), sealedProof(scope, marker), 128)
			if !errors.Is(err, ErrInconsistent) || batch.Processed != 0 || batch.More {
				t.Fatalf("batch=%+v err=%v", batch, err)
			}
			for _, id := range ids {
				var audit invocationRow
				var ledger ledgerRow
				if err := db.Where("invocation_id = ?", id).First(&audit).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Where("invocation_id = ?", id).First(&ledger).Error; err != nil {
					t.Fatal(err)
				}
				if audit.Status != Evaluating || ledger.Status != Evaluating {
					t.Fatalf("par parcialmente recuperado: %s / %s", audit.Status, ledger.Status)
				}
			}
		})
	}
}

func TestRecoveryProofRejectsZeroHostClockWithoutChangingPair(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	request := validRequest()
	request.ExpiresAt = now.Add(time.Hour)
	if _, err := store.Reserve(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	user := request.Owner.UserID
	scope := GenerationScope{UserID: &user, AuthContextType: "local_session", AuthContextID: request.Owner.AuthContextID, SecurityGeneration: request.SecurityGeneration}
	marker := uuid.Must(uuid.NewV7()).String()
	if err := db.Create(&closedGenerationRow{ID: marker, UserID: &user, AuthContextType: scope.AuthContextType, AuthContextID: scope.AuthContextID, SecurityGeneration: scope.SecurityGeneration, ClosedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	now = time.Time{}
	batch, err := store.RecoverClosedGenerationWithProof(context.Background(), sealedProof(scope, marker), 128)
	if !errors.Is(err, ErrInvalidRequest) || batch.Processed != 0 {
		t.Fatalf("batch=%+v err=%v", batch, err)
	}
	var audit invocationRow
	var ledger ledgerRow
	if err := db.Where("invocation_id = ?", request.InvocationID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("invocation_id = ?", request.InvocationID).First(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Status != Evaluating || ledger.Status != Evaluating {
		t.Fatalf("par alterado sem relógio: %s / %s", audit.Status, ledger.Status)
	}
}
