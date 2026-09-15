package commandconfig

import (
	"context"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandjson"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConfirmedMutation struct {
	prepared *PreparedMutation
	receipts *commanddecision.Store
	epoch    commandsecurity.EpochSnapshot
	request  commanddecision.Request
}

// ConfirmMutation recebe somente portas do host e espera fora do gate. A
// confirmação vincula diff, escopo, operação, contadores, sessão e epochs.
func (s *Store) ConfirmMutation(ctx context.Context, p *PreparedMutation, epoch commandsecurity.EpochSnapshot, receipts *commanddecision.Store, version string, keys commandledger.FingerprintKeyProvider, expires time.Time, render func(MutationDiff) (string, error)) (*ConfirmedMutation, error) {
	if s == nil || ctx == nil || p == nil || p.store != s || receipts == nil || keys == nil || render == nil || epoch.UserID != p.before.Scope.UserID || !validID(epoch.SessionID) || !expires.After(time.Now()) {
		return nil, ErrInvalid
	}
	before, after, err := mutationDocuments(p)
	if err != nil {
		return nil, err
	}
	raw, err := commandjson.Marshal(map[string]any{"version": 2, "mutation_id": p.diff.MutationID, "operation": p.diff.Operation, "scope": p.before.Scope, "generations": p.before.Generations, "epoch": epoch, "before": before, "after": after})
	if err != nil {
		return nil, ErrInvalid
	}
	// O provider gerencia versões. Não criar chave nem usar JWT/pepper.
	if version == "" || len(version) > 64 {
		return nil, ErrInvalid
	}
	key, err := keys(ctx, "command-request-hmac:"+version)
	if err != nil || len(key) < 32 {
		return nil, ErrInvalid
	}
	copyKey := append([]byte{}, key...)
	defer clear(copyKey)
	fingerprint, err := commandjson.HMAC(copyKey, "assistente.command.config-mutation.v2:"+version, raw)
	if err != nil {
		return nil, err
	}
	body, err := render(p.Diff())
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	r := commanddecision.Request{SubjectType: "config_mutation", DecisionID: id.String(), MutationID: p.diff.MutationID, UserID: epoch.UserID, SessionID: epoch.SessionID, Fingerprint: fingerprint, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration, ExpiresAt: time.UnixMilli(expires.UnixMilli()).UTC(), Body: body}
	status, err := receipts.Decide(ctx, r)
	if err != nil {
		return nil, err
	}
	if status != commanddecision.Accepted {
		return nil, commanddecision.ErrStale
	}
	r.Body = ""
	return &ConfirmedMutation{prepared: p, receipts: receipts, epoch: epoch, request: r}, nil
}

// MutationTxHook integra revogação de grants e recomposição de claims na mesma
// transação. É instalado pelo bootstrap, nunca fornecido no payload da UI.
type MutationTxHook func(context.Context, *gorm.DB, MutationDiff) error

// CommitConfirmedMutation exige gate EXCLUSIVO e reautenticação pelo host.
// A API runtime é MutationService.Apply; esta é apenas a composição repository.
func (s *Store) CommitConfirmedMutation(ctx context.Context, c *ConfirmedMutation, epoch commandsecurity.EpochSnapshot, hook MutationTxHook) error {
	if s == nil || ctx == nil || c == nil || c.prepared == nil || c.prepared.store != s || c.receipts == nil || c.epoch != epoch || hook == nil {
		return ErrInvalid
	}
	return c.receipts.ConsumeForDatabase(ctx, s.db, c.request, func(tx *gorm.DB) error {
		g, err := s.applyMutationTx(ctx, tx, c.prepared)
		if err != nil {
			return err
		}
		if err := hook(ctx, tx, c.prepared.Diff()); err != nil {
			return err
		}
		return recordCompleteMutation(tx, c, g)
	})
}
