package commandactivation

import (
	"context"
	"time"

	"assistente/internal/commandmaintenance"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// maintenanceSeal é criado somente pelo construtor da Store. A identidade
// do selo prende a capacidade de retenção ao Store fornecido pelo bootstrap;
// não há identidade, cap ou owner vindo da UI nessa porta.
type maintenanceSeal struct {
	token uuid.UUID
}

// RetentionPolicy controla uma única passagem bounded de retenção. Now deve
// ser o instante autenticado pelo scheduler/coordinator, e não um relógio
// derivado de IDs persistidos.
type RetentionPolicy struct {
	Now         time.Time
	MaxAge      time.Duration
	PerUserKeep int
	BatchSize   int
}

// RetentionResult descreve o trabalho físico desta passagem. More sinaliza
// que outra chamada pode ser agendada; não autoriza um loop interno.
type RetentionResult struct {
	Deleted int64
	More    bool
}

// MaintenanceService expõe a única capacidade interna de retenção do domínio.
// O principal pode adaptá-la ao seu BoundedRetentionPort, sem expor este
// serviço à UI ou aceitar owner/payload para decidir o escopo.
type MaintenanceService struct {
	store *Store
	seal  *maintenanceSeal
	now   func() time.Time
}

// NewMaintenanceService cria uma capacidade vinculada a esta Store.
func (s *Store) NewMaintenanceService() (*MaintenanceService, error) {
	return s.NewMaintenanceServiceAt(time.Now)
}

// NewMaintenanceServiceAt é a variante para o bootstrap fornecer o relógio
// host da manutenção. O instante não é derivado de UUIDs nem de payloads.
func (s *Store) NewMaintenanceServiceAt(now func() time.Time) (*MaintenanceService, error) {
	if s == nil || s.db == nil || s.maintenanceSeal == nil || now == nil {
		return nil, ErrInvalid
	}
	return &MaintenanceService{store: s, seal: s.maintenanceSeal, now: now}, nil
}

func (m *MaintenanceService) valid() bool {
	return m != nil && m.store != nil && m.store.db != nil && m.seal != nil && m.now != nil && m.store.maintenanceSeal == m.seal
}

var terminalClaimStates = []State{
	StateInactive,
	StateDeactivated,
	StateExpired,
	StateStale,
}

// Os estados observados no consumidor são todos terminais no ledger. A lista
// explícita deixa desconhecidos fora da destruição, adotando fail-closed caso
// uma versão futura introduza um estado recuperável.
var terminalLedgerStates = []string{
	"activate",
	"deactivate",
	"terminal",
	"stale",
	"inactive",
	"expired",
	"ignored",
	"conflict",
}

func eligibleLedgerQuery(tx *gorm.DB, now time.Time) *gorm.DB {
	return tx.Table("command_activation_idempotency_keys").
		Where("terminal_state IN ? AND expires_at <= ? AND source_replay_deadline <= ?", terminalLedgerStates, now, now).
		Where(`NOT EXISTS (
    SELECT 1
    FROM command_layer_activation_state AS active_claim
    WHERE active_claim.user_id = command_activation_idempotency_keys.user_id
      AND active_claim.rule_ref_kind = command_activation_idempotency_keys.rule_ref_kind
      AND active_claim.rule_ref = command_activation_idempotency_keys.rule_ref
      AND active_claim.source_event_id = command_activation_idempotency_keys.source_event_id
      AND active_claim.state = ?
      AND ((active_claim.workspace_id IS NULL AND command_activation_idempotency_keys.workspace_id IS NULL) OR active_claim.workspace_id = command_activation_idempotency_keys.workspace_id)
)`, StateActive)
}

// Retain remove auditoria terminal antiga/excedente e ledgers somente depois
// de seus prazos persistidos. Claims ativas permanecem; a barreira de replay
// é o ledger, que só sai após seus deadlines. O total deletado por chamada
// nunca excede BatchSize; More preserva a continuação para a próxima passagem.
func (m *MaintenanceService) Retain(ctx context.Context, policy RetentionPolicy) (RetentionResult, error) {
	if !m.valid() || ctx == nil || policy.Now.IsZero() || policy.MaxAge < 0 || policy.PerUserKeep <= 0 || policy.BatchSize < 0 || policy.BatchSize > 128 {
		return RetentionResult{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return RetentionResult{}, err
	}
	if policy.BatchSize == 0 {
		policy.BatchSize = 128
	}

	now := policy.Now.UTC()
	cutoff := now.Add(-policy.MaxAge)
	result := RetentionResult{}
	err := m.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var claimIDs []string
		if err := tx.Raw(`
SELECT activation_id
FROM (
    SELECT activation_id, user_id, updated_at,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY updated_at DESC, activation_id DESC
           ) AS keep_rank
    FROM command_layer_activation_state
      WHERE state IN ?
)
WHERE keep_rank > ? OR updated_at < ?
ORDER BY activation_id
LIMIT ?`, terminalClaimStates, policy.PerUserKeep, cutoff, policy.BatchSize+1).Scan(&claimIDs).Error; err != nil {
			return err
		}
		if len(claimIDs) > policy.BatchSize {
			result.More = true
			claimIDs = claimIDs[:policy.BatchSize]
		}
		if len(claimIDs) > 0 {
			deleted := tx.Where("activation_id IN ? AND state IN ?", claimIDs, terminalClaimStates).Delete(&Claim{})
			if deleted.Error != nil {
				return deleted.Error
			}
			result.Deleted += deleted.RowsAffected
		}

		remaining := policy.BatchSize - int(result.Deleted)
		if !result.More && remaining == 0 {
			var pendingIDs []string
			err := eligibleLedgerQuery(tx, now).Order("id").Limit(1).Pluck("id", &pendingIDs).Error
			if err != nil {
				return err
			}
			result.More = len(pendingIDs) > 0
		}
		if remaining > 0 && !result.More {
			var ledgerIDs []string
			if err := eligibleLedgerQuery(tx, now).Order("id").Limit(remaining+1).Pluck("id", &ledgerIDs).Error; err != nil {
				return err
			}
			if len(ledgerIDs) > remaining {
				result.More = true
				ledgerIDs = ledgerIDs[:remaining]
			}
			if len(ledgerIDs) > 0 {
				deleted := tx.Exec(`DELETE FROM command_activation_idempotency_keys
WHERE id IN ? AND terminal_state IN ? AND expires_at <= ? AND source_replay_deadline <= ?
  AND NOT EXISTS (
    SELECT 1
    FROM command_layer_activation_state AS active_claim
    WHERE active_claim.user_id = command_activation_idempotency_keys.user_id
      AND active_claim.rule_ref_kind = command_activation_idempotency_keys.rule_ref_kind
      AND active_claim.rule_ref = command_activation_idempotency_keys.rule_ref
      AND active_claim.source_event_id = command_activation_idempotency_keys.source_event_id
      AND active_claim.state = ?
      AND ((active_claim.workspace_id IS NULL AND command_activation_idempotency_keys.workspace_id IS NULL) OR active_claim.workspace_id = command_activation_idempotency_keys.workspace_id)
  )`, ledgerIDs, terminalLedgerStates, now, now, StateActive)
				if deleted.Error != nil {
					return deleted.Error
				}
				result.Deleted += deleted.RowsAffected
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return RetentionResult{}, err
	}
	return result, nil
}

// CoordinatorRetention adapta a capacidade ao contrato bounded do
// coordenador. A política continua sendo traduzida aqui; nenhum owner ou
// limite da UI é aceito como argumento de escopo.
type CoordinatorRetention struct {
	maintenance *MaintenanceService
}

func (m *MaintenanceService) CoordinatorRetention() (*CoordinatorRetention, error) {
	if !m.valid() {
		return nil, ErrInvalid
	}
	return &CoordinatorRetention{maintenance: m}, nil
}

func (a *CoordinatorRetention) Retain(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	result, err := a.RetainBatch(ctx, policy)
	return result.Deleted, err
}

func (a *CoordinatorRetention) RetainBatch(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.RetentionResult, error) {
	if a == nil || !a.maintenance.valid() || ctx == nil {
		return commandmaintenance.RetentionResult{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	if err := policy.Validate(); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	now := a.maintenance.now()
	if now.IsZero() {
		return commandmaintenance.RetentionResult{}, ErrInvalid
	}
	result, err := a.maintenance.Retain(ctx, RetentionPolicy{
		Now: now, MaxAge: policy.ActivationRetention,
		PerUserKeep: policy.ActivationsPerUser, BatchSize: policy.BatchSize,
	})
	if err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	return commandmaintenance.RetentionResult{Deleted: result.Deleted, More: result.More}, nil
}
