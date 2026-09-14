package commandledger

import (
	"context"
	"gorm.io/gorm"
)

// OutcomeVerifier consulta o executor/recurso confiável, sem executar efeitos.
// O host deve selecionar o verificador pelo contrato do comando e reaplicar
// autenticação/autorização; não pode aceitar status alegado pelo cliente.
// A API inicial suporta somente leitura sem retorno (resumo {}), não provas
// externas, resultados arbitrários ou referências fornecidas pelo usuário.
type OutcomeVerifier func(context.Context, Record) (Status, error)

// Reconcile consulta fora da transação e só depois tenta CAS auditado.
// A incerteza é preservada em erro, cancelamento ou resultado inconclusivo.
// É infraestrutura interna: não registra verificadores nem expõe endpoint.
func (s *Store) Reconcile(ctx context.Context, owner Owner, id string, verify OutcomeVerifier) (bool, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil || verify == nil {
		return false, ErrInvalidRequest
	}
	record, err := s.Get(ctx, owner, id)
	if err != nil {
		return false, err
	}
	if record.Status != OutcomeUnknown {
		return false, nil
	}
	if record.SourceType != "palette" && record.SourceType != "ui.action" && record.SourceType != "cli" {
		return false, ErrInconsistent
	}
	outcome, err := verify(ctx, record)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if outcome != Succeeded && outcome != Failed {
		return false, ErrInvalidTransition
	}
	var changed bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ledger := tx.Model(&ledgerRow{}).Where("id = ? AND invocation_id = ? AND user_id = ? AND auth_context_type = ? AND auth_context_id = ? AND source_type = ? AND request_fingerprint_version = ? AND request_fingerprint = ? AND status = ?", record.ID, id, owner.UserID, "local_session", owner.AuthContextID, record.SourceType, record.RequestFingerprintVersion, record.RequestFingerprint, OutcomeUnknown).Updates(map[string]any{"status": outcome, "result_summary": "{}"})
		if ledger.Error != nil {
			return ledger.Error
		}
		if ledger.RowsAffected == 0 {
			return nil
		}
		now := s.now().UTC()
		if now.IsZero() {
			return ErrInvalidRequest
		}
		var errorValue any
		if outcome == Failed {
			errorValue = errorCode(Failed)
		}
		audit := tx.Model(&invocationRow{}).Where("invocation_id = ? AND user_id = ? AND auth_context_type = ? AND auth_context_id = ? AND source_type = ? AND request_fingerprint_version = ? AND request_fingerprint = ? AND status = ?", id, owner.UserID, "local_session", owner.AuthContextID, record.SourceType, record.RequestFingerprintVersion, record.RequestFingerprint, OutcomeUnknown).Updates(map[string]any{"status": outcome, "result_summary": "{}", "completed_at": now, "error_code": errorValue})
		if audit.Error != nil {
			return audit.Error
		}
		if audit.RowsAffected != 1 {
			return ErrInconsistent
		}
		changed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}
