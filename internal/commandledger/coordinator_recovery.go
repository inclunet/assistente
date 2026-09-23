package commandledger

import (
	"context"
	"sync"

	"assistente/internal/commandmaintenance"
	"assistente/internal/commandsecurity"
)

// CoordinatorRecovery percorre pendências de toda a instância, mas somente
// recupera gerações comprovadamente fechadas (drain local ou protocolo de
// exclusão interprocesso vinculado a este banco). O cursor limita
// linhas examinadas, inclusive as pertencentes a outro core ainda ativo. Não é
// uma prova por inferência de restart e não recebe lista de usuários da UI.
type CoordinatorRecovery struct {
	store   *Store
	drained commandsecurity.DrainedGenerations
	mu      sync.Mutex
	after   string
}

func NewCoordinatorRecovery(store *Store, drained commandsecurity.DrainedGenerations) (*CoordinatorRecovery, error) {
	if store == nil || store.db == nil || store.now == nil || !drained.Valid() || !drained.AllowsDatabase(store.db) {
		return nil, ErrInvalidRequest
	}
	return &CoordinatorRecovery{store: store, drained: drained}, nil
}

var _ commandmaintenance.RecoveryPort = (*CoordinatorRecovery)(nil)

// Recover conserva progresso já confirmado quando um escopo posterior falha.
// A escrita é exclusivamente RecoverClosedGenerationWithProof; não há CAS ou
// classificação de resultado paralelos neste adapter.
func (a *CoordinatorRecovery) Recover(ctx context.Context, limit int) (commandmaintenance.BatchResult, error) {
	if a == nil || a.store == nil || ctx == nil || limit < 1 || limit > 128 || !a.drained.Valid() || !a.drained.AllowsDatabase(a.store.db) {
		return commandmaintenance.BatchResult{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	if !a.mu.TryLock() {
		return commandmaintenance.BatchResult{}, commandmaintenance.ErrAlreadyRunning
	}
	defer a.mu.Unlock()
	var rows []invocationRow
	err := a.store.db.WithContext(ctx).Select("invocation_id", "user_id", "auth_context_type", "auth_context_id", "security_generation").
		Where("invocation_id > ? AND status IN ?", a.after, []Status{Evaluating, Queued, Running}).
		Order("invocation_id").Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return commandmaintenance.BatchResult{More: true}, err
	}
	result := commandmaintenance.BatchResult{More: len(rows) > limit}
	if result.More {
		rows = rows[:limit]
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			result.More = true
			return result, err
		}
		if a.drained.Includes(row.SecurityGeneration) {
			scope := GenerationScope{UserID: row.UserID, AuthContextType: row.AuthContextType, AuthContextID: row.AuthContextID, SecurityGeneration: row.SecurityGeneration}
			proof, err := a.store.SealDrainedGeneration(ctx, a.drained, scope)
			if err != nil {
				result.More = true
				return result, err
			}
			batch, err := a.store.RecoverClosedGenerationWithProof(ctx, proof, 1)
			result.Processed += batch.Processed
			if err != nil {
				result.More = true
				return result, err
			}
		}
		a.after = row.InvocationID
	}
	if !result.More {
		a.after = ""
	}
	return result, nil
}
