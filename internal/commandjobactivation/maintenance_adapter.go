package commandjobactivation

import (
	"context"
	"strings"
	"sync"

	"assistente/internal/commandmaintenance"
)

const maintenanceAdapterBatchLimit = 100

// MaintenanceOutboxAdapter adapta o consumidor real à sequência única de
// manutenção. deliveryOwner é uma capacidade opaca gerada pelo host; não é
// owner de autenticação e nunca vem de payload de evento.
type MaintenanceOutboxAdapter struct {
	consumer      *Consumer
	deliveryOwner string
}

var _ commandmaintenance.OutboxPort = (*MaintenanceOutboxAdapter)(nil)

func NewMaintenanceOutboxAdapter(consumer *Consumer, deliveryOwner string) (*MaintenanceOutboxAdapter, error) {
	if consumer == nil || consumer.db == nil || consumer.gate == nil || consumer.outbox == nil || strings.TrimSpace(deliveryOwner) == "" {
		return nil, ErrUnavailable
	}
	return &MaintenanceOutboxAdapter{consumer: consumer, deliveryOwner: deliveryOwner}, nil
}

// RequeueExpiredLeases delega a transação da Store. O coordinator aceita até
// 128, mas o adapter usa a fronteira de 100 do Consumer e propaga More para a
// próxima passagem, sem processar um lote ilimitado.
func (a *MaintenanceOutboxAdapter) RequeueExpiredLeases(ctx context.Context, limit int) (int, bool, error) {
	if a == nil || a.consumer == nil || ctx == nil {
		return 0, false, ErrUnavailable
	}
	bounded, err := maintenanceAdapterLimit(limit)
	if err != nil {
		return 0, false, err
	}
	return a.consumer.outbox.RequeueExpiredLeases(ctx, bounded)
}

// Drain usa RunPass e, portanto, o mesmo Consumer comum de eventos. Em caso
// de erro, Processed conta apenas consumos confirmados; a ocorrência que
// falhou não é escondida e More permanece verdadeiro para continuação segura.
func (a *MaintenanceOutboxAdapter) Drain(ctx context.Context, limit int) (commandmaintenance.BatchResult, error) {
	if a == nil || a.consumer == nil || ctx == nil {
		return commandmaintenance.BatchResult{}, ErrUnavailable
	}
	bounded, err := maintenanceAdapterLimit(limit)
	if err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	result, err := a.consumer.RunPass(ctx, a.deliveryOwner, bounded)
	more := result.More
	if err != nil && result.Processed < result.Claimed {
		more = true
	}
	return commandmaintenance.BatchResult{Processed: result.Processed, More: more}, err
}

// MaintenanceRecoveryAdapter adapta a reconciliação autenticada de claims à
// porta sem cursor do coordinator. O cursor é estado do adapter, não do
// payload; o mutex evita duas chamadas concorrentes saltarem a continuação.
type MaintenanceRecoveryAdapter struct {
	consumer *Consumer
	mu       sync.Mutex
	cursor   string
}

var _ commandmaintenance.RecoveryPort = (*MaintenanceRecoveryAdapter)(nil)

func NewMaintenanceRecoveryAdapter(consumer *Consumer) (*MaintenanceRecoveryAdapter, error) {
	if consumer == nil || consumer.db == nil || consumer.gate == nil || consumer.outbox == nil {
		return nil, ErrUnavailable
	}
	return &MaintenanceRecoveryAdapter{consumer: consumer}, nil
}

// MaintenanceHeartbeatAdapter adapta o Consumer à porta opcional do
// coordinator. Ele não conhece nem executa a ordem da manutenção: apenas
// percorre uma página de leases e mantém a continuação do heartbeat.
type MaintenanceHeartbeatAdapter struct {
	consumer *Consumer
	mu       sync.Mutex
	cursor   string
}

var _ commandmaintenance.HeartbeatPort = (*MaintenanceHeartbeatAdapter)(nil)

func NewMaintenanceHeartbeatAdapter(consumer *Consumer) (*MaintenanceHeartbeatAdapter, error) {
	if consumer == nil || consumer.db == nil || consumer.gate == nil || consumer.outbox == nil {
		return nil, ErrUnavailable
	}
	return &MaintenanceHeartbeatAdapter{consumer: consumer}, nil
}

// Heartbeat usa exatamente a policy recebida pelo coordinator. BatchSize 128
// é limitado a 100, More sinaliza continuação sem impedir outbox/recovery, e
// o cursor reseta ao concluir para revisitar leases no próximo ciclo.
func (a *MaintenanceHeartbeatAdapter) Heartbeat(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.BatchResult, error) {
	if a == nil || a.consumer == nil || ctx == nil {
		return commandmaintenance.BatchResult{}, ErrUnavailable
	}
	if err := policy.Validate(); err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	limit := policy.BatchSize
	if limit == 0 {
		limit = commandmaintenance.DefaultBatchSize
	}
	bounded, err := maintenanceAdapterLimit(limit)
	if err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	if !a.mu.TryLock() {
		return commandmaintenance.BatchResult{}, ErrUnavailable
	}
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	cursor, heartbeat, err := a.consumer.HeartbeatPass(ctx, a.cursor, bounded, policy.LeaseDuration)
	processed := heartbeat.Renewed + heartbeat.Rejected
	if err != nil {
		// O item que falhou não foi confirmado; o cursor permanece no prefixo
		// anterior para que a próxima chamada possa revalidá-lo.
		return commandmaintenance.BatchResult{Processed: processed, More: true}, err
	}
	if heartbeat.More {
		a.cursor = cursor
	} else {
		a.cursor = ""
	}
	return commandmaintenance.BatchResult{Processed: processed, More: heartbeat.More}, nil
}

// Recover executa uma única fatia de ReconcileBatch. Uma falha de transação
// não avança cursor nem inventa Processed; a próxima chamada repete a fatia.
func (a *MaintenanceRecoveryAdapter) Recover(ctx context.Context, limit int) (commandmaintenance.BatchResult, error) {
	if a == nil || a.consumer == nil || ctx == nil {
		return commandmaintenance.BatchResult{}, ErrUnavailable
	}
	bounded, err := maintenanceAdapterLimit(limit)
	if err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	if !a.mu.TryLock() {
		return commandmaintenance.BatchResult{}, ErrUnavailable
	}
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	next, done, processed, err := a.consumer.reconcileBatch(ctx, a.cursor, bounded)
	if err != nil {
		return commandmaintenance.BatchResult{}, err
	}
	if done {
		// A completed page closes only this scan. The next maintenance cycle
		// must revisit every lease, since validity can change after this pass.
		a.cursor = ""
	} else {
		a.cursor = next
	}
	return commandmaintenance.BatchResult{Processed: processed, More: !done}, nil
}

func maintenanceAdapterLimit(limit int) (int, error) {
	if limit <= 0 {
		return 0, ErrUnavailable
	}
	if limit > maintenanceAdapterBatchLimit {
		return maintenanceAdapterBatchLimit, nil
	}
	return limit, nil
}
