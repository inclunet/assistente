package commandledger

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// GenerationScope é produzido pelo lifecycle autenticado. UserID nulo só é
// válido para auth_context_type=system; nesse caso AuthContextID identifica o
// ciclo da instância e não é usado como identidade de usuário.
type GenerationScope struct {
	UserID             *string
	AuthContextType    string
	AuthContextID      string
	SecurityGeneration string
}

// ClosedGenerationProof é deliberadamente opaco. Não há construtor público:
// SealDrainedGeneration exige a prova do core fechado e executores drenados.
// Assim, um scope, bool ou marker do caller não vira prova de encerramento.
type ClosedGenerationProof struct {
	scope  GenerationScope
	marker string
}

type closedGenerationRow struct {
	ID                 string    `gorm:"primaryKey;type:text"`
	UserID             *string   `gorm:"index:ux_command_closed_generation,priority:1"`
	AuthContextType    string    `gorm:"type:text;not null;index:ux_command_closed_generation,priority:2"`
	AuthContextID      string    `gorm:"type:text;not null;index:ux_command_closed_generation,priority:3"`
	SecurityGeneration string    `gorm:"type:text;not null;index:ux_command_closed_generation,priority:4"`
	ClosedAt           time.Time `gorm:"not null"`
}

func (closedGenerationRow) TableName() string { return "command_closed_generations" }

// MaintenanceBatch é o resultado bounded das operações de manutenção.
type MaintenanceBatch struct {
	Processed int
	More      bool
}

func validGenerationScope(scope GenerationScope) error {
	if scope.AuthContextType != "local_session" && scope.AuthContextType != "system" {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(scope.AuthContextID) == "" || strings.TrimSpace(scope.AuthContextID) != scope.AuthContextID || len(scope.AuthContextID) > 256 {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(scope.SecurityGeneration) == "" || strings.TrimSpace(scope.SecurityGeneration) != scope.SecurityGeneration || len(scope.SecurityGeneration) > 256 {
		return ErrInvalidRequest
	}
	if scope.AuthContextType == "system" {
		if scope.UserID != nil {
			return ErrInvalidRequest
		}
		return nil
	}
	if scope.UserID == nil || !validUUID(*scope.UserID) || !validUUID(scope.AuthContextID) {
		return ErrInvalidRequest
	}
	return nil
}

// RecoverClosedGenerationWithProof converte, em lote, somente invocações do
// owner/ciclo que a própria Store marcou como fechado. Ledger e auditoria são
// atualizados na mesma transação. System usa user_id IS NULL e não exige que
// o auth_context_id antigo seja o epoch atual.
func (s *Store) RecoverClosedGenerationWithProof(ctx context.Context, proof ClosedGenerationProof, limit int) (MaintenanceBatch, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil || proof.marker == "" || limit < 1 || limit > 128 || validGenerationScope(proof.scope) != nil {
		return MaintenanceBatch{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return MaintenanceBatch{}, err
	}
	var result MaintenanceBatch
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var marker closedGenerationRow
		if err := tx.Where("id = ?", proof.marker).First(&marker).Error; err != nil {
			return ErrInvalidRequest
		}
		if marker.AuthContextType != proof.scope.AuthContextType || marker.AuthContextID != proof.scope.AuthContextID || marker.SecurityGeneration != proof.scope.SecurityGeneration || (marker.UserID == nil) != (proof.scope.UserID == nil) || (marker.UserID != nil && *marker.UserID != *proof.scope.UserID) {
			return ErrInvalidRequest
		}
		query := tx.Where("auth_context_type = ? AND auth_context_id = ? AND security_generation = ? AND status IN ?", proof.scope.AuthContextType, proof.scope.AuthContextID, proof.scope.SecurityGeneration, []Status{Evaluating, Queued, Running})
		if proof.scope.UserID == nil {
			query = query.Where("user_id IS NULL")
		} else {
			query = query.Where("user_id = ?", *proof.scope.UserID)
		}
		var rows []invocationRow
		if err := query.Order("invocation_id").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		result.More = len(rows) > limit
		if result.More {
			rows = rows[:limit]
		}
		now := s.now().UTC()
		if now.IsZero() {
			return ErrInvalidRequest
		}
		for _, row := range rows {
			// A prova fecha o ciclo, mas não torna um par divergente consistente.
			// Compare também a identidade imutável antes de avançar qualquer lado.
			ledgerQuery := tx.Model(&ledgerRow{}).Where("invocation_id = ? AND auth_context_type = ? AND auth_context_id = ? AND request_fingerprint_version = ? AND request_fingerprint = ? AND status = ?", row.InvocationID, proof.scope.AuthContextType, row.AuthContextID, row.RequestFingerprintVersion, row.RequestFingerprint, row.Status)
			if row.SourceType == nil {
				ledgerQuery = ledgerQuery.Where("source_type IS NULL")
			} else {
				ledgerQuery = ledgerQuery.Where("source_type = ?", *row.SourceType)
			}
			if proof.scope.UserID == nil {
				ledgerQuery = ledgerQuery.Where("user_id IS NULL")
			} else {
				ledgerQuery = ledgerQuery.Where("user_id = ?", *proof.scope.UserID)
			}
			changed := ledgerQuery.Updates(map[string]any{"status": OutcomeUnknown, "result_summary": "{}"})
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected != 1 {
				return ErrInconsistent
			}
			invQuery := tx.Model(&invocationRow{}).Where("invocation_id = ? AND auth_context_type = ? AND auth_context_id = ? AND security_generation = ? AND status = ?", row.InvocationID, proof.scope.AuthContextType, proof.scope.AuthContextID, proof.scope.SecurityGeneration, row.Status)
			if proof.scope.UserID == nil {
				invQuery = invQuery.Where("user_id IS NULL")
			} else {
				invQuery = invQuery.Where("user_id = ?", *proof.scope.UserID)
			}
			changed = invQuery.Updates(map[string]any{"status": OutcomeUnknown, "result_summary": "{}", "completed_at": now, "error_code": errorCode(OutcomeUnknown)})
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected != 1 {
				return ErrInconsistent
			}
			result.Processed++
		}
		return ctx.Err()
	})
	if err != nil {
		return MaintenanceBatch{}, err
	}
	return result, nil
}

// InvocationRetentionPolicy controla somente auditoria detalhada. Ledgers
// ficam protegidos por expires_at e registros avaliando/queued/running nunca
// são removidos.
type InvocationRetentionPolicy struct {
	Now         time.Time
	MaxAge      time.Duration
	PerUserKeep int
	SystemKeep  int
	BatchSize   int
}

type RetentionResult struct {
	InvocationsDeleted int64
	LedgersDeleted     int64
	More               bool
}

// MaintenanceService é uma capacidade de manutenção da instância, separada
// do Store usado pelas operações de usuário. A fábrica só entrega o serviço
// quando o selo privado pertence ao Store específico; nenhuma identidade do
// contexto ou limite vindo da UI participa dessa decisão. O bootstrap deve
// criar uma instância e mantê-la fora das portas comuns de usuário.
type MaintenanceService struct {
	store *Store
	seal  *maintenanceSeal
}

// NewMaintenanceService cria explicitamente a porta de retenção para este
// Store. O selo não é serializável nem substituível por um booleano/callback.
func (s *Store) NewMaintenanceService() (*MaintenanceService, error) {
	if s == nil || s.db == nil || s.now == nil || s.maintenanceSeal == nil {
		return nil, ErrInvalidRequest
	}
	return &MaintenanceService{store: s, seal: s.maintenanceSeal}, nil
}

func (m *MaintenanceService) valid() bool {
	return m != nil && m.store != nil && m.store.db != nil && m.store.now != nil && m.seal != nil && m.store.maintenanceSeal == m.seal
}

// Retain remove auditoria terminal antiga/excedente e, separadamente, ledgers
// terminais cujo expires_at já venceu. Não usa UUID como relógio, não remove
// source-event antes do deadline e nunca remove estado ativo. Cada chamada é
// limitada a um lote: More pede uma chamada posterior, não um loop interno.
func (m *MaintenanceService) Retain(ctx context.Context, policy InvocationRetentionPolicy) (RetentionResult, error) {
	if !m.valid() || ctx == nil || policy.Now.IsZero() || policy.MaxAge < 0 || policy.PerUserKeep <= 0 || policy.SystemKeep <= 0 || policy.BatchSize < 0 || policy.BatchSize > 128 {
		return RetentionResult{}, ErrInvalidRequest
	}
	if policy.BatchSize == 0 {
		policy.BatchSize = 128
	}
	s := m.store
	cutoff := policy.Now.UTC().Add(-policy.MaxAge)
	var result RetentionResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A idade é aplicada apenas a terminais; o cap é por owner e também só
		// seleciona auditoria. O ledger permanece até seu próprio expires_at.
		terminalStatuses := []Status{Succeeded, Failed, Denied, Cancelled, CancelledStale, TimedOut, OutcomeUnknown}
		var invocationIDs []string
		if err := tx.Raw(`
SELECT invocation_id
FROM (
    SELECT invocation_id, user_id, received_at,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY received_at DESC, invocation_id DESC
           ) AS keep_rank
    FROM command_invocations
    WHERE status IN ?
      AND (source_event_id IS NULL OR (source_replay_deadline IS NOT NULL AND source_replay_deadline <= ?))
)
WHERE (user_id IS NULL AND keep_rank > ?)
   OR (user_id IS NOT NULL AND keep_rank > ?)
   OR received_at < ?
ORDER BY invocation_id
LIMIT ?`, terminalStatuses, policy.Now.UTC(), policy.SystemKeep, policy.PerUserKeep, cutoff, policy.BatchSize+1).Scan(&invocationIDs).Error; err != nil {
			return err
		}
		if len(invocationIDs) > policy.BatchSize {
			result.More = true
			invocationIDs = invocationIDs[:policy.BatchSize]
		}
		if len(invocationIDs) > 0 {
			deleted := tx.Where("invocation_id IN ?", invocationIDs).Delete(&invocationRow{})
			if deleted.Error != nil {
				return deleted.Error
			}
			result.InvocationsDeleted = deleted.RowsAffected
		}
		// Ledgers sem auditoria também podem ser compactados, mas somente após
		// o prazo persistido e se não representam estado recuperável.
		var ledgerIDs []string
		if err := tx.Model(&ledgerRow{}).
			Where("expires_at <= ? AND (source_event_id IS NULL OR (source_replay_deadline IS NOT NULL AND source_replay_deadline <= ?)) AND status NOT IN ?", policy.Now.UTC(), policy.Now.UTC(), []Status{Evaluating, Queued, Running}).
			Order("id").Limit(policy.BatchSize+1).Pluck("id", &ledgerIDs).Error; err != nil {
			return err
		}
		if len(ledgerIDs) > policy.BatchSize {
			result.More = true
			ledgerIDs = ledgerIDs[:policy.BatchSize]
		}
		if len(ledgerIDs) > 0 {
			deleted := tx.Where("id IN ?", ledgerIDs).Delete(&ledgerRow{})
			if deleted.Error != nil {
				return deleted.Error
			}
			result.LedgersDeleted = deleted.RowsAffected
		}
		return ctx.Err()
	})
	if err != nil {
		return RetentionResult{}, err
	}
	return result, nil
}
