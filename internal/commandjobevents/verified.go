package commandjobevents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"assistente/internal/commandjson"
	"gorm.io/gorm"
)

// VerifiedFactTx relê a ocorrência durável, sem confiar no DTO recebido pelo
// consumidor. O chamador mantém a transação e o DispatchGate exclusivos.
// O epoch original é revalidado: uma retenção nova nunca reabre a ocorrência.
func (s *Store) VerifiedFactTx(ctx context.Context, tx *gorm.DB, eventID string, now time.Time) (Fact, error) {
	return s.verifiedFactTx(ctx, tx, eventID, now, true)
}

// VerifiedRuntimeFactTx relê a mesma ocorrência e valida sua integridade e o
// epoch/deadline originais, mas não aplica o limite temporal de replay. É
// exclusivo para provar a continuidade de um runtime que já possui claim e
// lease; não cria ledger, não aceita DTO e não transforma o evento em novo.
func (s *Store) VerifiedRuntimeFactTx(ctx context.Context, tx *gorm.DB, eventID string, now time.Time) (Fact, error) {
	return s.verifiedFactTx(ctx, tx, eventID, now, false)
}

func (s *Store) verifiedFactTx(ctx context.Context, tx *gorm.DB, eventID string, now time.Time, requireReplayWindow bool) (Fact, error) {
	if ctx == nil || tx == nil || s == nil || s.db == nil || !isCanonicalUUID7(eventID) || now.IsZero() {
		return Fact{}, ErrInvalidFact
	}
	pool := tx.ConnPool
	if tx.Statement != nil && tx.Statement.ConnPool != nil {
		pool = tx.Statement.ConnPool
	}
	if _, ok := pool.(gorm.TxCommitter); !ok {
		return Fact{}, ErrInvalidFact
	}
	left, le := s.db.DB()
	right, re := tx.DB()
	if le != nil || re != nil || left != right {
		return Fact{}, ErrInvalidFact
	}
	var row ActivationOutbox
	if err := tx.WithContext(ctx).Where("source_event_id = ?", eventID).Take(&row).Error; err != nil {
		return Fact{}, err
	}
	f, err := factFromOutbox(row)
	if err != nil {
		return Fact{}, err
	}
	if f.OccurredAt.After(now) {
		return Fact{}, ErrInvalidFact
	}
	if requireReplayWindow && !f.SourceReplayDeadline.After(now) {
		return Fact{}, ErrInvalidFact
	}
	epoch, err := s.currentEpochTx(tx.WithContext(ctx), ProducerType, f.OccurredAt)
	if err != nil {
		return Fact{}, err
	}
	if epoch.ReplayHorizonSeconds <= 0 || epoch.ReplayHorizonSeconds > int64((1<<63-1)/int64(time.Second)) || f.SourceReplayPolicyGeneration != fmt.Sprintf("%s:%d", ProducerType, epoch.Generation) || !f.SourceReplayDeadline.Equal(f.OccurredAt.Add(time.Duration(epoch.ReplayHorizonSeconds)*time.Second)) {
		return Fact{}, ErrInvalidFact
	}
	fp, err := Fingerprint(f)
	if err != nil || fp != f.EventFingerprint {
		return Fact{}, ErrFingerprintConflict
	}
	if f.Provenance["_source"] != "job" || f.Provenance["_source_job_id"] != f.JobSlug {
		return Fact{}, ErrInvalidFact
	}
	return f, nil
}

func factFromOutbox(row ActivationOutbox) (Fact, error) {
	var provenance map[string]any
	if row.Provenance != "" {
		canonical, err := commandjson.Canonicalize([]byte(row.Provenance))
		if err != nil || json.Unmarshal(canonical, &provenance) != nil {
			return Fact{}, ErrInvalidFact
		}
	}
	fact := Fact{
		SchemaVersion: row.SchemaVersion, EventName: row.EventName, SourceEventID: row.SourceEventID,
		UserID: row.UserID, JobDatabaseID: row.JobDatabaseID, JobSlug: row.JobSlug, RunID: row.RunID,
		RunEventID: row.SourceEventID, Sequence: row.Sequence, State: row.State, OccurredAt: row.OccurredAt,
		RootOriginType: row.RootOriginType, RootOriginID: row.RootOriginID, Provenance: provenance,
		SourceReplayPolicyGeneration: row.SourceReplayPolicyGeneration, SourceReplayDeadline: row.SourceReplayDeadline,
		EventFingerprint: row.EventFingerprint,
	}
	if err := validateFact(fact); err != nil {
		return Fact{}, err
	}
	return fact, nil
}
