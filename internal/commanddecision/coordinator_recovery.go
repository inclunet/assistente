package commanddecision

import (
	"context"
	"sync"

	"assistente/internal/commandmaintenance"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

// CoordinatorRecovery percorre todos os usuários com cursor privado. Somente
// gerações emitidas e drenadas pelo core local podem ser encerradas; idade,
// ausência de sessão e restart não constituem prova de drenagem.
type CoordinatorRecovery struct {
	store   *Store
	drained commandsecurity.DrainedGenerations
	mu      sync.Mutex
	after   string
}

func NewCoordinatorRecovery(store *Store, drained commandsecurity.DrainedGenerations) (*CoordinatorRecovery, error) {
	if store == nil || store.db == nil || store.now == nil || !drained.Valid() || isTransactionalDB(store.db) {
		return nil, ErrInvalid
	}
	return &CoordinatorRecovery{store: store, drained: drained}, nil
}

var _ commandmaintenance.RecoveryPort = (*CoordinatorRecovery)(nil)

// Recover limita linhas examinadas a limit (máximo 128), com uma linha de
// lookahead. Processed conta commits, inclusive antes de erro/cancelamento.
// More indica continuação da varredura, não prova que outro core está drenado.
// Ao terminar, reinicia o cursor para revisitar o início na próxima passagem.
func (a *CoordinatorRecovery) Recover(ctx context.Context, limit int) (commandmaintenance.BatchResult, error) {
	if a == nil || a.store == nil || ctx == nil || limit < 1 || limit > MaxRecoveryBatch || !a.drained.Valid() {
		return commandmaintenance.BatchResult{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return commandmaintenance.BatchResult{More: true}, err
	}
	if !a.mu.TryLock() {
		return commandmaintenance.BatchResult{More: true}, commandmaintenance.ErrAlreadyRunning
	}
	defer a.mu.Unlock()
	now := a.store.now()
	if now.IsZero() || now.UnixMilli() <= 0 {
		return commandmaintenance.BatchResult{}, ErrInvalid
	}
	var rows []receiptRow
	if err := a.store.db.WithContext(ctx).Where("decision_id > ? AND status IN ? AND auth_context_type = ? AND subject_type IN ?", a.after, []string{Pending, Accepted}, "local_session", []string{"config_mutation", "invocation"}).Order("decision_id").Limit(limit + 1).Find(&rows).Error; err != nil {
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
			err := a.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := closeRecoveryReceiptTx(tx, row, now); err != nil {
					return err
				}
				return ctx.Err()
			})
			if err != nil {
				result.More = true
				return result, err
			}
			result.Processed++
		}
		a.after = row.ID
	}
	if !result.More {
		a.after = ""
	}
	return result, nil
}
