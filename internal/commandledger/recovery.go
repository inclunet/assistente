package commandledger

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

// RecoverClosedGeneration é uma operação interna de manutenção, não endpoint.
// O host deve comprovar que a geração de segurança terminou e impedir qualquer
// admissão/handler dessa geração antes da chamada. Não infere encerramento por
// tempo, expiração ou diferença com a geração atual. Não autentica Owner.
// Suporta apenas o subconjunto local_session deste pacote; não recupera system.
// Todos os pares selecionados mudam na mesma transação, sem repetir efeitos.
func (s *Store) RecoverClosedGeneration(ctx context.Context, owner Owner, closedSecurityGeneration string) (int64, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil || validateOwner(owner) != nil || strings.TrimSpace(closedSecurityGeneration) == "" || strings.TrimSpace(closedSecurityGeneration) != closedSecurityGeneration {
		return 0, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var recovered int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []invocationRow
		err := tx.Where("user_id = ? AND auth_context_type = ? AND auth_context_id = ? AND security_generation = ? AND status IN ?", owner.UserID, "local_session", owner.AuthContextID, closedSecurityGeneration, []Status{Evaluating, Queued, Running}).Find(&rows).Error
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if now.IsZero() {
			return ErrInvalidRequest
		}
		for _, row := range rows {
			// Não permite recuperar linhas futuras/externas nem um par com identidade divergente.
			if row.SourceType == nil || (*row.SourceType != "palette" && *row.SourceType != "ui.action" && *row.SourceType != "cli") {
				return ErrInconsistent
			}
			ledger := tx.Model(&ledgerRow{}).Where("invocation_id = ? AND user_id = ? AND auth_context_type = ? AND auth_context_id = ? AND source_type = ? AND request_fingerprint_version = ? AND request_fingerprint = ? AND status = ?", row.InvocationID, owner.UserID, "local_session", owner.AuthContextID, *row.SourceType, row.RequestFingerprintVersion, row.RequestFingerprint, row.Status).Updates(map[string]any{"status": OutcomeUnknown, "result_summary": "{}"})
			if ledger.Error != nil {
				return ledger.Error
			}
			if ledger.RowsAffected != 1 {
				return ErrInconsistent
			}
			audit := tx.Model(&invocationRow{}).Where("invocation_id = ? AND status = ? AND security_generation = ?", row.InvocationID, row.Status, closedSecurityGeneration).Updates(map[string]any{"status": OutcomeUnknown, "result_summary": "{}", "completed_at": now, "error_code": errorCode(OutcomeUnknown)})
			if audit.Error != nil {
				return audit.Error
			}
			if audit.RowsAffected != 1 {
				return ErrInconsistent
			}
			recovered++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return recovered, nil
}
