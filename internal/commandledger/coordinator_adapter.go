package commandledger

import (
	"context"

	"assistente/internal/commandmaintenance"
)

// CoordinatorRetention mantém a capacidade de manutenção fora das portas de
// usuário. A política vem da passagem corrente; o relógio pertence ao Store.
type CoordinatorRetention struct {
	maintenance *MaintenanceService
}

func (m *MaintenanceService) CoordinatorRetention() (*CoordinatorRetention, error) {
	if !m.valid() {
		return nil, ErrInvalidRequest
	}
	return &CoordinatorRetention{maintenance: m}, nil
}

func (a *CoordinatorRetention) Retain(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	result, err := a.RetainBatch(ctx, policy)
	return result.Deleted, err
}

// RetainBatch preserva More: uma auditoria compactada não significa que a
// passagem terminou, nem autoriza compactação física antes dos lotes restantes.
func (a *CoordinatorRetention) RetainBatch(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.RetentionResult, error) {
	if a == nil || !a.maintenance.valid() || ctx == nil {
		return commandmaintenance.RetentionResult{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return commandmaintenance.RetentionResult{More: true}, err
	}
	if err := policy.Validate(); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	result, err := a.maintenance.Retain(ctx, InvocationRetentionPolicy{
		Now: a.maintenance.store.now(), MaxAge: policy.InvocationRetention,
		PerUserKeep: policy.InvocationsPerUser, SystemKeep: policy.InvocationsSystemKeep,
		BatchSize: policy.BatchSize,
	})
	// A passagem do serviço é transacional: rollback devolve contagem zero.
	// Preserve esse resultado e sinalize retomada em erro; commits de lotes
	// anteriores não são desfeitos nem contados novamente por este adapter.
	adapted := commandmaintenance.RetentionResult{
		Deleted: result.InvocationsDeleted + result.LedgersDeleted,
		More:    result.More,
	}
	if err != nil {
		adapted.More = true
	}
	return adapted, err
}
