package commandjobactivation

import (
	"context"
	"time"

	"assistente/internal/commandactivation"
	"gorm.io/gorm"
)

// ReconcileBatch não cria uma cadência. O coordenador único da instância
// chama lotes antes da retenção de jobs. Cursor é PK, não usuário ativo.
func (c *Consumer) ReconcileBatch(ctx context.Context, after string, limit int) (string, bool, error) {
	if c == nil || ctx == nil || limit <= 0 || limit > 100 {
		return after, false, ErrUnavailable
	}
	cursor, done := after, false
	err := c.gate.WithMutation(ctx, func() error {
		return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var leases []Lease
			if err := tx.Where("activation_id > ?", after).Order("activation_id").Limit(limit).Find(&leases).Error; err != nil {
				return err
			}
			done = len(leases) < limit
			for _, lease := range leases {
				if err := ctx.Err(); err != nil {
					return err
				}
				cursor = lease.ActivationID
				claim, _, err := c.liveClaim(ctx, tx, lease, c.now().UTC())
				if err == nil {
					continue
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// Revogar disponibilidade é seguro mesmo se a prova estiver ausente.
				// Não apagar a claim/ledger, não renovar lease nem executar efeitos.
				update := tx.Model(&commandactivation.Claim{}).Where("activation_id = ? AND user_id = ? AND source_type = ? AND state = ?", lease.ActivationID, lease.UserID, "job", commandactivation.StateActive).Updates(map[string]any{"state": commandactivation.StateInactive, "terminal_reason": "source_unavailable", "updated_at": c.now().UTC()})
				if update.Error != nil {
					return update.Error
				}
				if update.RowsAffected > 0 {
					if claim.ActivationID == "" {
						if err := tx.Where("activation_id = ? AND user_id = ?", lease.ActivationID, lease.UserID).Take(&claim).Error; err != nil {
							return err
						}
					}
					store, err := commandactivation.NewStore(tx)
					if err != nil {
						return err
					}
					o := commandactivation.Owner{Scope: commandactivation.Scope{UserID: claim.UserID, WorkspaceID: clone(claim.WorkspaceID)}, AuthContextType: claim.AuthContextType, AuthContextID: claim.AuthContextID, AuthGeneration: claim.AuthGeneration, SecurityGeneration: claim.SecurityGeneration}
					if _, err := store.BumpActiveLayersTx(ctx, tx, o); err != nil {
						return err
					}
				}
				if err := tx.Where("id = ?", lease.ID).Delete(&Lease{}).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return after, false, err
	}
	return cursor, done, nil
}

// RenewRuntime é chamado pelo heartbeat confiável antes da metade do TTL.
// A origem é relida, a geração deve ser exatamente a mesma e lease vencida
// nunca é renovada. O método não instala heartbeat nem aceita proof do payload.
func (c *Consumer) RenewRuntime(ctx context.Context, activationID string) error {
	if c == nil || ctx == nil {
		return ErrUnavailable
	}
	return c.gate.WithMutation(ctx, func() error {
		return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var lease Lease
			if err := tx.Where("activation_id = ?", activationID).Take(&lease).Error; err != nil {
				return err
			}
			now := c.now().UTC()
			claim, _, err := c.liveClaim(ctx, tx, lease, now)
			if err != nil {
				return err
			}
			update := tx.Model(&Lease{}).Where("id = ? AND runtime_generation = ? AND expires_at = ? AND expires_at > ?", lease.ID, lease.RuntimeGeneration, lease.ExpiresAt, now).Updates(map[string]any{"expires_at": now.Add(c.lease), "updated_at": now})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return ErrUnavailable
			}
			// Ativo não expira por idade de auditoria enquanto tem fonte viva.
			deadline := now.Add(c.retention)
			if claim.ExpiresAt != nil && claim.ExpiresAt.After(deadline) {
				deadline = *claim.ExpiresAt
			}
			return tx.Model(&commandactivation.Claim{}).Where("activation_id = ? AND state = ?", claim.ActivationID, commandactivation.StateActive).Update("expires_at", deadline).Error
		})
	})
}

func (c *Consumer) liveClaim(ctx context.Context, tx *gorm.DB, lease Lease, now time.Time) (commandactivation.Claim, commandactivation.Owner, error) {
	var claim commandactivation.Claim
	if err := tx.Where("activation_id = ? AND user_id = ? AND source_type = ?", lease.ActivationID, lease.UserID, "job").Take(&claim).Error; err != nil {
		return claim, commandactivation.Owner{}, err
	}
	if claim.State != commandactivation.StateActive || (claim.ExpiresAt != nil && !claim.ExpiresAt.After(now)) || !lease.ExpiresAt.After(now) || claim.SourceEventID == nil || claim.SourceCorrelationID == nil || *claim.SourceCorrelationID != lease.RunID {
		return claim, commandactivation.Owner{}, ErrUnavailable
	}
	// Heartbeat prova a continuidade de um runtime já admitido. Ele não é uma
	// nova admissão do evento e, portanto, não pode exigir que o deadline de
	// replay ainda esteja no futuro.
	f, err := c.outbox.VerifiedRuntimeFactTx(ctx, tx, *claim.SourceEventID, now)
	if err != nil {
		return claim, commandactivation.Owner{}, err
	}
	if err := verifyJob(tx, f); err != nil {
		return claim, commandactivation.Owner{}, err
	}
	owner, err := c.ports.Authorize(ctx, tx, claim.UserID, clone(claim.WorkspaceID))
	if err != nil {
		return claim, owner, err
	}
	if owner.UserID != claim.UserID || !sameScope(owner.WorkspaceID, claim.WorkspaceID) || owner.AuthContextType != claim.AuthContextType || owner.AuthContextID != claim.AuthContextID || owner.AuthGeneration != claim.AuthGeneration || owner.SecurityGeneration != claim.SecurityGeneration {
		return claim, owner, ErrUnavailable
	}
	var rules []commandactivation.Rule
	if err := scope(tx.Where("user_id = ? AND rule_ref_kind = ? AND rule_ref = ? AND enabled = ?", owner.UserID, claim.RuleRefKind, claim.RuleRef, true), owner.WorkspaceID).Find(&rules).Error; err != nil {
		return claim, owner, err
	}
	if len(rules) != 1 {
		return claim, owner, ErrUnavailable
	}
	rule := rules[0]
	if rule.LayerRefKind != claim.LayerRefKind || rule.LayerRef != claim.LayerRef {
		return claim, owner, ErrUnavailable
	}
	if err := c.validateGrant(ctx, tx, rule); err != nil {
		return claim, owner, err
	}
	enabled, err := c.ports.Layer(ctx, tx, detachedOwner(owner), detachedRule(rule))
	if err != nil || !enabled {
		return claim, owner, ErrUnavailable
	}
	active, err := c.ports.Condition(ctx, tx, detachedOwner(owner), detachedRule(rule), detachedFact(f))
	if err != nil || !active {
		return claim, owner, ErrUnavailable
	}
	runtime, err := c.ports.Runtime(ctx, tx, detachedFact(f))
	if err != nil || !runtime.matches(owner) || runtime.Generation != lease.RuntimeGeneration {
		return claim, owner, ErrUnavailable
	}
	return claim, owner, nil
}
