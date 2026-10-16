package commandconfig

import (
	"context"

	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

// commitConfirmedRequest é a composição comum para uma decisão cujo
// fingerprint foi produzido por uma operação de capacidade específica. A
// receipt é consumida uma única vez; configuração, grant, hook, geração e
// auditoria usam o mesmo TX.
func (s *Store) commitConfirmedRequest(ctx context.Context, p *PreparedMutation, epoch commandsecurity.EpochSnapshot, receipts *commanddecision.Store, request commanddecision.Request, hook MutationTxHook, applyGrant func(context.Context, *gorm.DB) error) error {
	if s == nil || s.db == nil || ctx == nil || p == nil || p.store != s || p.before.stamp == nil || receipts == nil || hook == nil || applyGrant == nil || request.MutationID != p.diff.MutationID {
		return ErrInvalid
	}
	if epoch.UserID != p.before.Scope.UserID || request.UserID != epoch.UserID || request.SubjectType != "config_mutation" || request.Fingerprint == "" {
		return ErrInvalid
	}
	confirmed := &ConfirmedMutation{prepared: p, receipts: receipts, epoch: epoch, request: request}
	confirmed.applyBeforeHook = applyGrant
	return s.CommitConfirmedMutation(ctx, confirmed, epoch, hook)
}
