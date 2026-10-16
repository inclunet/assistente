package commandjobevents

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestVerifiedRuntimeFactSeparatesReplayWindowAndKeepsEpochIntegrity(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := store.EnsureReplayPolicyEpoch(ctx, ProducerType, now.Add(-2*time.Hour), time.Hour); err != nil {
		t.Fatal(err)
	}
	fact := testFact(t, now.Add(-90*time.Minute))
	fact.Provenance = map[string]any{"_source": "job", "_source_job_id": fact.JobSlug}
	insertTestFact(t, store, fact)
	if err := store.DB().Transaction(func(tx *gorm.DB) error {
		_, err := store.VerifiedFactTx(ctx, tx, fact.SourceEventID, now)
		return err
	}); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("fact fora do replay aceito pelo caminho normal: %v", err)
	}
	if err := store.DB().Transaction(func(tx *gorm.DB) error {
		got, err := store.VerifiedRuntimeFactTx(ctx, tx, fact.SourceEventID, now)
		if err != nil {
			return err
		}
		if got.SourceReplayPolicyGeneration != fact.SourceReplayPolicyGeneration || !got.SourceReplayDeadline.Equal(fact.SourceReplayDeadline) {
			t.Fatalf("runtime perdeu metadados imutáveis: %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("runtime válido após horizonte rejeitado: %v", err)
	}
	if err := store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Update("source_replay_deadline", fact.SourceReplayDeadline.Add(time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Transaction(func(tx *gorm.DB) error {
		_, err := store.VerifiedRuntimeFactTx(ctx, tx, fact.SourceEventID, now)
		return err
	}); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("deadline adulterado passou pela validação do epoch: %v", err)
	}
}
