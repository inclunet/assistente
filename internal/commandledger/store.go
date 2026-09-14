package commandledger

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db  *gorm.DB
	now func() time.Time
}

func New(db *gorm.DB, now func() time.Time) (*Store, error) {
	if db == nil || now == nil {
		return nil, ErrInvalidRequest
	}
	return &Store{db: db, now: now}, nil
}

func (s *Store) Reserve(ctx context.Context, req LocalReadRequest) (Reservation, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Reservation{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return Reservation{}, err
	}
	if err := validateRequest(req); err != nil {
		return Reservation{}, err
	}
	if err := validateOwner(req.Owner); err != nil {
		return Reservation{}, err
	}
	now := s.now()
	if now.IsZero() {
		return Reservation{}, ErrInvalidRequest
	}
	now = now.UTC()
	if !req.ReceivedAt.IsZero() {
		req.ReceivedAt = req.ReceivedAt.UTC()
	}
	if !req.ExpiresAt.IsZero() {
		req.ExpiresAt = req.ExpiresAt.UTC()
	}
	key := "invocation:" + req.InvocationID
	var result Reservation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now = s.now()
		if now.IsZero() {
			return ErrInvalidRequest
		}
		now = now.UTC()
		owner := req.Owner.UserID
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		row := ledgerRow{ID: id.String(), Key: key, InvocationID: req.InvocationID, UserID: &owner,
			AuthContextType: "local_session", AuthContextID: req.Owner.AuthContextID,
			SourceType: nullable(req.SourceType), RequestFingerprintVersion: req.RequestFingerprintVersion,
			RequestFingerprint: req.RequestFingerprint, Status: Evaluating,
			ReceivedAt: req.ReceivedAt, ExpiresAt: req.ExpiresAt}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if created.Error != nil {
			return created.Error
		}
		// O INSERT pode aguardar o writer de outra conexão.
		now = s.now().UTC()
		if now.IsZero() {
			return ErrInvalidRequest
		}
		if created.RowsAffected == 0 {
			var existing ledgerRow
			if err := tx.Where("invocation_id = ?", req.InvocationID).First(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrConflict
				}
				return err
			}
			if existing.Key != key || existing.UserID == nil || *existing.UserID != req.Owner.UserID ||
				existing.AuthContextType != "local_session" || existing.AuthContextID != req.Owner.AuthContextID ||
				existing.SourceType == nil || *existing.SourceType != req.SourceType || existing.RequestFingerprintVersion != req.RequestFingerprintVersion ||
				existing.RequestFingerprint != req.RequestFingerprint {
				return ErrConflict
			}
			if !existing.ExpiresAt.After(now) {
				return ErrExpired
			}
			result = Reservation{Record: ledgerToRecord(existing), Created: false}
			return nil
		}
		commandID, sessionID := req.CommandID, req.Owner.AuthContextID
		inv := invocationRow{InvocationID: req.InvocationID, SchemaVersion: 1, UserID: &owner,
			AuthContextType: "local_session", AuthContextID: req.Owner.AuthContextID, AuthGeneration: req.AuthGeneration,
			SessionID: &sessionID, SecurityGeneration: req.SecurityGeneration, RegistryVersion: req.RegistryVersion,
			GlobalConfigGeneration: nullable(req.GlobalConfigGeneration), ActiveLayersGeneration: nullable(req.ActiveLayersGeneration),
			CommandID: &commandID, BindingIDs: "[]", ActorType: "user", ActorID: req.Owner.UserID,
			ArgumentsSummary: "{}", ArgumentsFingerprint: req.ArgumentsFingerprint,
			CorrelationID: req.CorrelationID, RequestFingerprintVersion: req.RequestFingerprintVersion,
			RequestFingerprint: req.RequestFingerprint, Risk: "low", PolicyDecision: "pending", SourceType: nullable(req.SourceType),
			Status: Evaluating, ReceivedAt: req.ReceivedAt}
		if !req.ExpiresAt.After(now) {
			return ErrExpired
		}
		if err := tx.Create(&inv).Error; err != nil {
			return err
		}
		result = Reservation{Record: ledgerToRecord(row), Created: true}
		return nil
	})
	if err != nil {
		return Reservation{}, err
	}
	return result, nil
}

func (s *Store) Get(ctx context.Context, owner Owner, invocationID string) (Record, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Record{}, ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if err := validateOwner(owner); err != nil || !validUUID(invocationID) {
		return Record{}, ErrNotFound
	}
	var row ledgerRow
	err := s.db.WithContext(ctx).Where("invocation_id = ? AND user_id = ? AND auth_context_type = ? AND auth_context_id = ?", invocationID, owner.UserID, "local_session", owner.AuthContextID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	return ledgerToRecord(row), nil
}

func (s *Store) CompareAndSwap(ctx context.Context, owner Owner, id string, from, to Status) (bool, error) {
	if s == nil || s.db == nil || ctx == nil {
		return false, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := validateOwner(owner); err != nil || !validUUID(id) || !validTransition(from, to) {
		return false, ErrInvalidTransition
	}
	now := s.now()
	if now.IsZero() {
		return false, ErrInvalidRequest
	}
	now = now.UTC()
	var changed bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now = s.now()
		if now.IsZero() {
			return ErrInvalidRequest
		}
		now = now.UTC()
		ledgerUpdates := map[string]any{"status": to}
		if terminal(to) {
			ledgerUpdates["result_summary"] = "{}"
		}
		updated := tx.Model(&ledgerRow{}).Where("invocation_id = ? AND user_id = ? AND auth_context_type = ? AND auth_context_id = ? AND status = ?", id, owner.UserID, "local_session", owner.AuthContextID, from).Updates(ledgerUpdates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return nil
		}
		changed = true
		updates := map[string]any{"status": to}
		if from == Evaluating && to == Queued {
			updates["policy_decision"] = "allowed"
		}
		if from == Evaluating && to == Denied {
			updates["policy_decision"] = "denied"
		}
		if terminal(to) {
			updates["completed_at"] = now
			updates["result_summary"] = "{}"
			if to != Succeeded {
				updates["error_code"] = errorCode(to)
			}
		}
		audit := tx.Model(&invocationRow{}).Where("invocation_id = ? AND user_id = ? AND auth_context_type = ? AND auth_context_id = ? AND status = ?", id, owner.UserID, "local_session", owner.AuthContextID, from).Updates(updates)
		if audit.Error != nil {
			return audit.Error
		}
		if audit.RowsAffected != 1 {
			return ErrInconsistent
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func errorCode(status Status) string { return "command_" + string(status) }
func ledgerToRecord(row ledgerRow) Record {
	user := ""
	if row.UserID != nil {
		user = *row.UserID
	}
	return Record{ID: row.ID, Key: row.Key, InvocationID: row.InvocationID, Owner: Owner{UserID: user, AuthContextID: row.AuthContextID}, SourceType: valueOrEmpty(row.SourceType), RequestFingerprintVersion: row.RequestFingerprintVersion, RequestFingerprint: row.RequestFingerprint, Status: row.Status, ReceivedAt: row.ReceivedAt, ExpiresAt: row.ExpiresAt}
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
