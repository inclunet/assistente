package commandactivation

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// RevokeInvalidRuleTx é a operação de fail-closed usada quando uma
// revalidação transacional encontra grant ausente, revogado ou stale. Ela
// desabilita a regra efetiva e torna claims ativos stale no escopo do owner.
// Não cria grant, não reativa claim e não abre transação.
func (s *Store) RevokeInvalidRuleTx(ctx context.Context, tx *gorm.DB, owner Owner, ref Ref, reason string) (Mutation, error) {
	return s.RevokeInvalidRuleTxAt(ctx, tx, owner, ref, reason, time.Now().UTC())
}

// RevokeInvalidRuleTxAt vincula updated_at ao instante do diff composto. A
// variante normal mantém o contrato anterior para consumidores independentes.
func (s *Store) RevokeInvalidRuleTxAt(ctx context.Context, tx *gorm.DB, owner Owner, ref Ref, reason string, now time.Time) (Mutation, error) {
	if s == nil || tx == nil || ctx == nil || validateOwner(owner) != nil || validateRef(ref) != nil ||
		strings.TrimSpace(reason) != reason || reason == "" || len(reason) > 256 || now.IsZero() || !isTransactionDB(tx) {
		return Mutation{}, ErrInvalid
	}
	var candidates []Rule
	query := tx.WithContext(ctx).Where("user_id = ? AND rule_ref_kind = ? AND rule_ref = ?", owner.UserID, ref.Kind, ref.ID)
	query = scopedQuery(query, owner.WorkspaceID)
	if err := query.Order("CASE WHEN workspace_id IS NULL THEN 0 ELSE 1 END, id").Find(&candidates).Error; err != nil {
		return Mutation{}, err
	}
	var rule *Rule
	for i := range candidates {
		if sameWorkspace(candidates[i].WorkspaceID, owner.WorkspaceID) {
			rule = &candidates[i]
			break
		}
	}
	mutation := Mutation{}
	if rule != nil && rule.Enabled {
		result := tx.WithContext(ctx).Model(&Rule{}).Where("id = ? AND user_id = ?", rule.ID, owner.UserID).
			Where(scopePredicate(rule.WorkspaceID), scopeArg(rule.WorkspaceID)...).
			Updates(map[string]any{"enabled": false, "authorization_decision_id": nil, "automation_grant_id": nil, "automation_grant_generation": nil, "automation_grant_fingerprint": nil})
		if result.Error != nil {
			return Mutation{}, result.Error
		}
		if result.RowsAffected != 1 {
			return Mutation{}, ErrStale
		}
		mutation.Changed = true
	}
	claimState := StateStale
	if rule == nil {
		claimState = StateInactive
	}
	claimQuery := tx.WithContext(ctx).Model(&Claim{}).
		Where("user_id = ? AND rule_ref_kind = ? AND rule_ref = ? AND state = ?", owner.UserID, ref.Kind, ref.ID, StateActive)
	claimQuery = claimQuery.Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...)
	result := claimQuery.Updates(map[string]any{"state": claimState, "terminal_reason": reason, "updated_at": now})
	if result.Error != nil {
		return Mutation{}, result.Error
	}
	if result.RowsAffected > 0 {
		mutation.Changed = true
		mutation.EffectiveClaims = int(result.RowsAffected)
	}
	if !mutation.Changed {
		return Mutation{}, ErrNotFound
	}
	return mutation, ctx.Err()
}
