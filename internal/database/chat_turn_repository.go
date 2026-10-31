package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrChatTurnActive  = errors.New("turno possui tentativa ativa")
	ErrChatTurnStale   = errors.New("tentativa não é a execução ativa do turno")
	ErrChatTurnInvalid = errors.New("transição ou vínculo de turno inválido")
)

type ChatTurnRepository struct{ db *gorm.DB }

// NewChatTurnRepository aceita também a transação do chamador: Finish deve
// compartilhar a transação que grava resposta/usage quando o pipeline for integrado.
func NewChatTurnRepository(db *gorm.DB) *ChatTurnRepository { return &ChatTurnRepository{db: db} }

func (r *ChatTurnRepository) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, r.db, "chat.turn", fn)
}

func scopedChatTurn(ctx context.Context, db *gorm.DB, conversationID, turnID string) (*ChatTurn, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var turn ChatTurn
	err = db.WithContext(ctx).Table("chat_turns").Select("chat_turns.*").
		Joins("JOIN conversations c ON c.id = chat_turns.conversation_id").
		Where("c.user_id = ? AND c.id = ? AND chat_turns.user_message_id = ?", userID, conversationID, turnID).Take(&turn).Error
	return &turn, err
}

func (r *ChatTurnRepository) Get(ctx context.Context, conversationID, turnID string) (*ChatTurn, error) {
	return scopedChatTurn(ctx, r.db, conversationID, turnID)
}

// GetMany lê metadados nullable sem eleger outra resposta para turnos sem
// representante. Ausência da chave significa histórico sem registro de turno.
func (r *ChatTurnRepository) GetMany(ctx context.Context, conversationID string, turnIDs []string) (map[string]*ChatTurn, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*ChatTurn, len(turnIDs))
	const batchSize = 400
	for start := 0; start < len(turnIDs); start += batchSize {
		var turns []ChatTurn
		if err := r.db.WithContext(ctx).Table("chat_turns").Select("chat_turns.*").
			Joins("JOIN conversations c ON c.id=chat_turns.conversation_id").
			Where("c.id=? AND c.user_id=? AND chat_turns.user_message_id IN ?", conversationID, userID, turnIDs[start:min(start+batchSize, len(turnIDs))]).Find(&turns).Error; err != nil {
			return nil, err
		}
		for i := range turns {
			result[turns[i].UserMessageID] = &turns[i]
		}
	}
	return result, nil
}

// Begin gera executionId no backend. Retry/continue preservam a identidade do turno.
func (r *ChatTurnRepository) Begin(ctx context.Context, conversationID, turnID, responseID, mode string) (*ChatTurnAttempt, error) {
	if mode != TurnModeNew && mode != TurnModeRetry && mode != TurnModeContinue {
		return nil, ErrChatTurnInvalid
	}
	if strings.TrimSpace(turnID) == "" || strings.TrimSpace(conversationID) == "" {
		return nil, ErrChatTurnInvalid
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	return r.BeginExecution(ctx, conversationID, turnID, responseID, mode, id.String(), "")
}

// BeginExecution admite o ID cujo lease o worker já adquiriu. Begin é a API
// de armazenamento isolado; o pipeline usa BeginExecution com posse comprovada.
func (r *ChatTurnRepository) BeginExecution(ctx context.Context, conversationID, turnID, responseID, mode, executionID, leasePath string) (*ChatTurnAttempt, error) {
	if mode != TurnModeNew && mode != TurnModeRetry && mode != TurnModeContinue {
		return nil, ErrChatTurnInvalid
	}
	parsed, err := uuid.Parse(executionID)
	if err != nil || parsed.Version() != 7 || parsed.String() != executionID {
		return nil, ErrChatTurnInvalid
	}
	attempt := &ChatTurnAttempt{ExecutionID: executionID, TurnID: turnID, Mode: mode, State: TurnPreparing, StartedAt: time.Now().UTC()}
	err = r.transaction(ctx, func(tx *gorm.DB) error {
		userID, _ := RequireUserID(ctx)
		var question ChatMessage
		if err := tx.Table("chat_messages").Select("chat_messages.*").Joins("JOIN conversations c ON c.id = chat_messages.conversation_id").
			Where("c.user_id = ? AND c.id = ? AND chat_messages.id = ? AND chat_messages.role = 'user'", userID, conversationID, turnID).Take(&question).Error; err != nil {
			return err
		}
		if mode != TurnModeNew {
			if err := reconcileLegacyChatTurns(tx, turnID, conversationID); err != nil {
				return err
			}
		}
		turn, err := scopedChatTurn(ctx, tx, conversationID, turnID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			origin := "new"
			if mode != TurnModeNew {
				origin = "legacy"
			}
			turn = &ChatTurn{UserMessageID: turnID, ConversationID: conversationID, State: TurnHistoricalUnknown, Origin: origin}
			if err := tx.Create(turn).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if turn.ActiveExecutionID != nil {
			return ErrChatTurnActive
		}
		if mode == TurnModeNew {
			var count int64
			if err := tx.Model(&ChatTurnAttempt{}).Where("turn_id = ?", turnID).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return ErrChatTurnInvalid
			}
		}
		if responseID != "" && (turn.ResponseMessageID == nil || *turn.ResponseMessageID != responseID) {
			return ErrChatTurnInvalid
		}
		if mode == TurnModeContinue {
			if turn.ResponseMessageID == nil {
				return ErrChatTurnInvalid
			}
			attempt.BaseExecutionID = turn.CurrentExecutionID
			attempt.BaseResponseMessageID = turn.ResponseMessageID
		}

		if err := tx.Table("chat_turn_attempts").Create(&struct {
			*ChatTurnAttempt
			BaseExecutionID       *string
			BaseResponseMessageID *string
		}{attempt, attempt.BaseExecutionID, attempt.BaseResponseMessageID}).Error; err != nil {
			return err
		}
		if leasePath != "" {
			if err := tx.Exec("UPDATE chat_turn_attempts SET lease_path = ? WHERE execution_id = ?", leasePath, executionID).Error; err != nil {
				return err
			}
		}
		if mode != TurnModeNew {
			if err := tx.Model(&Conversation{}).Where("id=? AND user_id=?", conversationID, userID).Updates(map[string]any{"summary": "", "summary_up_to_message_id": ""}).Error; err != nil {
				return err
			}
		}
		return tx.Table("chat_turns").Where("user_message_id = ?", turnID).Updates(map[string]any{
			"state": TurnPreparing, "active_execution_id": attempt.ExecutionID, "current_execution_id": attempt.ExecutionID, "response_message_id": nil,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

// MarkStreaming é idempotente só enquanto a mesma execução permanece ativa.
func (r *ChatTurnRepository) MarkStreaming(ctx context.Context, conversationID, turnID, executionID string) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		turn, err := scopedChatTurn(ctx, tx, conversationID, turnID)
		if err != nil {
			return err
		}
		if turn.ActiveExecutionID == nil || *turn.ActiveExecutionID != executionID {
			return ErrChatTurnStale
		}
		result := tx.Model(&ChatTurnAttempt{}).Where("turn_id = ? AND execution_id = ? AND state IN ?", turnID, executionID, []string{TurnPreparing, TurnStreaming}).Update("state", TurnStreaming)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrChatTurnStale
		}
		return tx.Model(&ChatTurn{}).Where("user_message_id = ? AND active_execution_id = ?", turnID, executionID).Update("state", TurnStreaming).Error
	})
}

// Finish rejeita terminais repetidos/antigos: nunca modifica uma tentativa nova.
// reason é um código controlado, não mensagem livre ou erro do provedor.
func (r *ChatTurnRepository) Finish(ctx context.Context, conversationID, turnID, executionID, state, reason string) error {
	if !validTurnTerminal(state, reason) {
		return ErrChatTurnInvalid
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		turn, err := scopedChatTurn(ctx, tx, conversationID, turnID)
		if err != nil {
			return err
		}
		if turn.ActiveExecutionID == nil || *turn.ActiveExecutionID != executionID {
			return ErrChatTurnStale
		}
		result := tx.Model(&ChatTurnAttempt{}).Where("turn_id = ? AND execution_id = ? AND state IN ?", turnID, executionID, []string{TurnPreparing, TurnStreaming}).
			Updates(map[string]any{"state": state, "terminal_reason": reason, "finished_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrChatTurnStale
		}
		return tx.Model(&ChatTurn{}).Where("user_message_id = ? AND active_execution_id = ?", turnID, executionID).
			Updates(map[string]any{"state": state, "active_execution_id": nil}).Error
	})
}

func validTurnTerminal(state, reason string) bool {
	switch state {
	case TurnCompleted:
		return reason == "completed"
	case TurnInterrupted:
		return reason == "cancelled" || reason == "restart" || reason == "output_limit" || reason == "limit_reached"
	case TurnFailed:
		return reason == "provider_error" || reason == "persistence_error" || reason == "internal_error"
	}
	return false
}

// RecoverInterruptedBeforeStartup deve ser chamado antes da admissão de workers
// do usuário. Não é limpeza periódica: uma execução viva nunca deve ser varrida.
func (r *ChatTurnRepository) RecoverInterruptedBeforeStartup(ctx context.Context) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		userID, _ := RequireUserID(ctx)
		var turns []ChatTurn
		if err := tx.Table("chat_turns").Select("chat_turns.*").Joins("JOIN conversations c ON c.id = chat_turns.conversation_id").
			Where("c.user_id = ? AND active_execution_id IS NOT NULL", userID).Find(&turns).Error; err != nil {
			return err
		}
		for _, turn := range turns {
			result := tx.Model(&ChatTurnAttempt{}).Where("execution_id = ? AND turn_id = ? AND state IN ?", *turn.ActiveExecutionID, turn.UserMessageID, []string{TurnPreparing, TurnStreaming}).
				Updates(map[string]any{"state": TurnInterrupted, "terminal_reason": "restart", "finished_at": time.Now().UTC()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("%w: tentativa ativa inconsistente", ErrChatTurnInvalid)
			}
			if err := tx.Model(&ChatTurn{}).Where("user_message_id = ?", turn.UserMessageID).
				Updates(map[string]any{"state": TurnInterrupted, "active_execution_id": nil}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
