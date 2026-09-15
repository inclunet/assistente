package commanddecision

import (
	"context"
	"strings"
	"unicode/utf8"

	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

const MaxRecoveryBatch = 128

type RecoveryResult struct {
	Closed int
	More   bool
}

// ReconcileSession encerra somente pending/accepted vencidos ou de um epoch
// anterior da MESMA sessão. O chamador autenticado deve manter o gate exclusivo
// e revalidar current antes da chamada. Não autoriza, apresenta, consome nem
// executa efeitos. A recuperação não apaga histórico e não revive decisões.
// More indica outras linhas elegíveis no snapshot do lote, não um cursor global.
func (s *Store) ReconcileSession(ctx context.Context, current commandsecurity.EpochSnapshot, limit int) (RecoveryResult, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil || limit < 1 || limit > MaxRecoveryBatch ||
		!validID(current.UserID) || !validID(current.SessionID) {
		return RecoveryResult{}, ErrInvalid
	}
	for _, v := range []string{current.AuthGeneration, current.SecurityGeneration} {
		if v == "" || strings.TrimSpace(v) != v || len(v) > 256 || !utf8.ValidString(v) {
			return RecoveryResult{}, ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return RecoveryResult{}, err
	}
	now := s.now()
	if now.IsZero() || now.UnixMilli() <= 0 {
		return RecoveryResult{}, ErrInvalid
	}
	var result RecoveryResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []receiptRow
		query := tx.Where("user_id = ? AND auth_context_id = ? AND auth_context_type = ? AND subject_type = ? AND status IN ? AND (expires_at <= ? OR auth_generation <> ? OR security_generation <> ?)",
			current.UserID, current.SessionID, "local_session", "config_mutation", []string{Pending, Accepted}, now.UnixMilli(), current.AuthGeneration, current.SecurityGeneration)
		if err := query.Order("decision_id").Limit(limit + 1).Find(&rows).Error; err != nil {
			return err
		}
		result.More = len(rows) > limit
		if result.More {
			rows = rows[:limit]
		}
		for _, row := range rows {
			if !validID(row.ID) {
				return ErrInvalid
			}
			state := Cancelled
			if row.ExpiresMS <= now.UnixMilli() {
				state = Expired
			}
			updates := map[string]any{"status": state, "accepted_action_id": nil}
			// Preserve quando o usuário respondeu; pending ganha o instante de
			// encerramento. O evento registra separadamente a recuperação.
			if row.RespondedAt == nil {
				updates["responded_at"] = now.UnixMilli()
			}
			changed := tx.Model(&receiptRow{}).Where("decision_id = ? AND user_id = ? AND auth_context_id = ? AND status = ? AND auth_generation = ? AND security_generation = ? AND expires_at = ? AND consumed_at IS NULL",
				row.ID, current.UserID, current.SessionID, row.State, row.AuthGeneration, row.SecurityGeneration, row.ExpiresMS).Updates(updates)
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected != 1 {
				return ErrStale
			}
			if err := appendEvent(tx, row.ID, state, now); err != nil {
				return err
			}
			result.Closed++
		}
		return ctx.Err()
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	return result, nil
}
