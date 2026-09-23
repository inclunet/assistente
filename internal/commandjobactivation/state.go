package commandjobactivation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

type eventLedger struct {
	ID, Key, UserID                                      string
	WorkspaceID                                          *string
	RuleRefKind                                          commandactivation.RefKind
	RuleRef, SourceType, SourceInstanceID, SourceEventID string
	SourceCorrelationID                                  *string
	Sequence                                             int64
	EventFingerprint                                     string
	SourceJobDatabaseID, SourceJobSlug                   *string
	SourceReplayPolicyGeneration                         string
	SourceReplayDeadline                                 time.Time
	TerminalState                                        string
	CreatedAt, ExpiresAt                                 time.Time
}

func (eventLedger) TableName() string { return "command_activation_idempotency_keys" }

// Lease é independente da retenção terminal. Renovação nunca muda o deadline
// da ocorrência nem ressuscita claim terminal. IDs são gerados pelo backend.
type Lease struct {
	ID                string    `gorm:"type:text;primaryKey"`
	ActivationID      string    `gorm:"type:text;not null;uniqueIndex"`
	UserID            string    `gorm:"type:text;not null;index"`
	RunID             string    `gorm:"type:text;not null;index"`
	RuntimeGeneration string    `gorm:"type:text;not null"`
	ExpiresAt         time.Time `gorm:"not null;index"`
	UpdatedAt         time.Time `gorm:"not null"`
}

func (Lease) TableName() string { return "command_job_activation_leases" }

func (c *Consumer) apply(ctx context.Context, tx *gorm.DB, owner commandactivation.Owner, rule commandactivation.Rule, f commandjobevents.Fact, now time.Time) (string, error) {
	key := eventKey(owner.UserID, owner.WorkspaceID, rule.RuleRefKind, rule.RuleRef, f.SourceEventID)
	var prior eventLedger
	err := tx.Where("key = ?", key).Take(&prior).Error
	if err == nil {
		if prior.UserID != owner.UserID || !sameScope(prior.WorkspaceID, owner.WorkspaceID) || prior.RuleRefKind != rule.RuleRefKind || prior.RuleRef != rule.RuleRef || prior.SourceEventID != f.SourceEventID || prior.EventFingerprint != f.EventFingerprint {
			return "", commandjobevents.ErrFingerprintConflict
		}
		return "replay", nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	var claims []commandactivation.Claim
	q := scope(tx.Where("user_id = ? AND rule_ref_kind = ? AND rule_ref = ? AND source_type = ? AND source_correlation_id = ?", owner.UserID, rule.RuleRefKind, rule.RuleRef, "job", f.RunID), owner.WorkspaceID)
	if err := q.Find(&claims).Error; err != nil {
		return "", err
	}
	if len(claims) > 1 {
		return "", ErrUnavailable
	}
	status := "applied"
	terminal := f.State == commandjobevents.StateCompleted || f.State == commandjobevents.StateFailed || f.State == commandjobevents.StateSkipped
	var claim commandactivation.Claim
	if len(claims) == 1 {
		claim = claims[0]
		if claim.Sequence == nil || claim.EventFingerprint == nil || claim.LayerRefKind != rule.LayerRefKind || claim.LayerRef != rule.LayerRef {
			return "", ErrUnavailable
		}
		switch {
		case int64(f.Sequence) == *claim.Sequence && f.EventFingerprint != *claim.EventFingerprint:
			status = "conflict"
		case int64(f.Sequence) <= *claim.Sequence:
			status = "ignored"
		case claim.State == commandactivation.StateDeactivated || claim.State == commandactivation.StateExpired || claim.State == commandactivation.StateStale:
			status = "terminal"
		case claim.AuthContextType != owner.AuthContextType || claim.AuthContextID != owner.AuthContextID || claim.AuthGeneration != owner.AuthGeneration || claim.SecurityGeneration != owner.SecurityGeneration:
			status = "stale"
		}
	} else {
		// O cap pode remover auditoria terminal, nunca a barreira mínima.
		// Uma ocorrência nova da mesma correlação não pode criar outro ciclo
		// quando o cursor persistido já prova encerramento ou perda de estado.
		var cursor eventLedger
		err := scope(tx.Where("user_id = ? AND rule_ref_kind = ? AND rule_ref = ? AND source_type = ? AND source_correlation_id = ? AND terminal_state IN ?", owner.UserID, rule.RuleRefKind, rule.RuleRef, "job", f.RunID, []string{"activate", "deactivate", "terminal", "stale", "inactive", "expired"}), owner.WorkspaceID).Order("sequence DESC, created_at DESC").Take(&cursor).Error
		if err == nil {
			switch {
			case int64(f.Sequence) == cursor.Sequence && f.EventFingerprint != cursor.EventFingerprint:
				status = "conflict"
			case int64(f.Sequence) <= cursor.Sequence:
				status = "ignored"
			case cursor.TerminalState != "activate":
				status = "terminal"
			default:
				status = "stale"
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}
	leaseDuration, retention := c.durations(ctx)
	deadline := now.Add(retention)
	if f.SourceReplayDeadline.After(deadline) {
		deadline = f.SourceReplayDeadline
	}
	if claim.ExpiresAt != nil && claim.ExpiresAt.After(deadline) {
		deadline = *claim.ExpiresAt
	}
	ledgerID, err := freshID()
	if err != nil {
		return "", err
	}
	entry := eventLedger{ID: ledgerID, Key: key, UserID: owner.UserID, WorkspaceID: clone(owner.WorkspaceID), RuleRefKind: rule.RuleRefKind, RuleRef: rule.RuleRef, SourceType: "job", SourceInstanceID: owner.AuthContextID, SourceEventID: f.SourceEventID, SourceCorrelationID: &f.RunID, Sequence: int64(f.Sequence), EventFingerprint: f.EventFingerprint, SourceJobDatabaseID: &f.JobDatabaseID, SourceJobSlug: &f.JobSlug, SourceReplayPolicyGeneration: f.SourceReplayPolicyGeneration, SourceReplayDeadline: f.SourceReplayDeadline, TerminalState: status, CreatedAt: now, ExpiresAt: deadline}
	if status != "applied" {
		return status, tx.Create(&entry).Error
	}
	entry.TerminalState = "activate"
	if terminal {
		entry.TerminalState = "deactivate"
	}
	runtimeGeneration := ""
	if !terminal {
		runtime, err := c.ports.Runtime(ctx, tx, detachedFact(f))
		if err != nil {
			return "", err
		}
		if !runtime.matches(owner) {
			return "", ErrUnavailable
		}
		runtimeGeneration = runtime.Generation
		// A claim existente sem lease válida fica inativa; replay não a renova.
		if len(claims) == 1 {
			var lease Lease
			if err := tx.Where("activation_id = ? AND user_id = ? AND run_id = ? AND runtime_generation = ? AND expires_at > ?", claim.ActivationID, owner.UserID, f.RunID, runtimeGeneration, now).Take(&lease).Error; err != nil {
				return "", ErrUnavailable
			}
		}
	}
	sequence := int64(f.Sequence)
	before, err := effectiveLayer(tx, owner, rule, now)
	if err != nil {
		return "", err
	}
	provenance, err := json.Marshal(f.Provenance)
	if err != nil {
		return "", err
	}
	provenanceText := string(provenance)
	state := commandactivation.StateActive
	if terminal {
		state = commandactivation.StateDeactivated
	}
	if len(claims) == 0 {
		id, err := freshID()
		if err != nil {
			return "", err
		}
		claim = commandactivation.Claim{ActivationID: id, LayerRefKind: rule.LayerRefKind, LayerRef: rule.LayerRef, RuleRefKind: rule.RuleRefKind, RuleRef: rule.RuleRef, UserID: owner.UserID, WorkspaceID: clone(owner.WorkspaceID), AuthContextType: owner.AuthContextType, AuthContextID: owner.AuthContextID, AuthGeneration: owner.AuthGeneration, SecurityGeneration: owner.SecurityGeneration, SourceType: "job", SourceInstanceID: &owner.AuthContextID, SourceEventID: &f.SourceEventID, SourceCorrelationID: &f.RunID, Sequence: &sequence, SourceJobDatabaseID: &f.JobDatabaseID, SourceJobSlug: &f.JobSlug, EventFingerprint: &f.EventFingerprint, SourceReplayPolicyGeneration: &f.SourceReplayPolicyGeneration, SourceReplayDeadline: &f.SourceReplayDeadline, State: state, Provenance: &provenanceText, ActivatedAt: now, ExpiresAt: &deadline, UpdatedAt: now}
		if terminal {
			reason := "source_terminal"
			claim.TerminalReason = &reason
		}
		if err := tx.Create(&claim).Error; err != nil {
			return "", err
		}
	} else {
		values := map[string]any{"sequence": sequence, "event_fingerprint": f.EventFingerprint, "source_event_id": f.SourceEventID, "source_replay_policy_generation": f.SourceReplayPolicyGeneration, "source_replay_deadline": f.SourceReplayDeadline, "state": state, "provenance": provenanceText, "expires_at": deadline, "updated_at": now}
		if terminal {
			values["terminal_reason"] = "source_terminal"
		}
		update := tx.Model(&commandactivation.Claim{}).Where("activation_id = ? AND sequence = ? AND state = ?", claim.ActivationID, *claim.Sequence, claim.State).Updates(values)
		if update.Error != nil {
			return "", update.Error
		}
		if update.RowsAffected != 1 {
			return "", ErrUnavailable
		}
	}
	if !terminal {
		if len(claims) == 0 {
			id, e := freshID()
			if e != nil {
				return "", e
			}
			if e = tx.Create(&Lease{ID: id, ActivationID: claim.ActivationID, UserID: owner.UserID, RunID: f.RunID, RuntimeGeneration: runtimeGeneration, ExpiresAt: now.Add(leaseDuration), UpdatedAt: now}).Error; e != nil {
				return "", e
			}
		}
	} else if err := tx.Where("activation_id = ?", claim.ActivationID).Delete(&Lease{}).Error; err != nil {
		return "", err
	}
	if err := tx.Create(&entry).Error; err != nil {
		return "", err
	}
	after, err := effectiveLayer(tx, owner, rule, now)
	if err != nil {
		return "", err
	}
	if before != after {
		store, err := commandactivation.NewStore(tx)
		if err != nil {
			return "", err
		}
		_, err = store.BumpActiveLayersTx(ctx, tx, owner)
		return status, err
	}
	return status, nil
}

func effectiveLayer(tx *gorm.DB, owner commandactivation.Owner, rule commandactivation.Rule, now time.Time) (bool, error) {
	var count int64
	q := scope(tx.Model(&commandactivation.Claim{}).Where("user_id = ? AND layer_ref_kind = ? AND layer_ref = ? AND state = ? AND auth_context_type = ? AND auth_context_id = ? AND auth_generation = ? AND security_generation = ? AND (expires_at IS NULL OR expires_at > ?)", owner.UserID, rule.LayerRefKind, rule.LayerRef, commandactivation.StateActive, owner.AuthContextType, owner.AuthContextID, owner.AuthGeneration, owner.SecurityGeneration, now), owner.WorkspaceID)
	err := q.Where("source_type <> 'job' OR EXISTS (SELECT 1 FROM command_job_activation_leases l WHERE l.activation_id = command_layer_activation_state.activation_id AND l.user_id = command_layer_activation_state.user_id AND l.expires_at > ?)", now).Count(&count).Error
	return count > 0, err
}
