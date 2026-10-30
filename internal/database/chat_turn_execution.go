package database

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// ReconcileImportedConversationInTransaction exige a transação do importador.
// Nenhum executionId remoto é restaurado; falhas revertem também as mensagens.
func (r *ChatTurnRepository) ReconcileImportedConversationInTransaction(ctx context.Context, conversationID string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	tx := r.db.WithContext(ctx)
	var conversation Conversation
	if err := ScopeByUser(ctx, tx, "user_id").Select("id").First(&conversation, "id = ?", conversationID).Error; err != nil {
		return err
	}
	if err := reconcileLegacyChatTurns(tx, "", conversationID); err != nil {
		return err
	}
	return tx.Model(&ChatTurn{}).Where("conversation_id = ? AND NOT EXISTS (SELECT 1 FROM chat_turn_attempts a WHERE a.turn_id = chat_turns.user_message_id)", conversationID).Update("origin", "imported").Error
}

// WritePartial preserva os contadores confirmados e rejeita writes atrasados
// antes de alterar o conteúdo. Só é usado no encerramento, nunca por delta.
func (r *ChatTurnRepository) WritePartial(ctx context.Context, executionID, messageID, content, reasoning string, onlyWhenEmpty bool) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		var message ChatMessage
		err := scopedMessageQuery(ctx, tx.Model(&ChatMessage{})).Select("chat_messages.*").
			Joins("JOIN chat_turns t ON t.response_message_id=chat_messages.id").
			Where("chat_messages.id = ? AND t.active_execution_id = ?", messageID, executionID).Take(&message).Error
		if err == gorm.ErrRecordNotFound {
			return ErrChatTurnStale
		}
		if err != nil {
			return err
		}
		if onlyWhenEmpty && strings.TrimSpace(message.Content) != "" {
			return nil
		}
		if strings.TrimSpace(reasoning) == "" {
			reasoning = message.Reasoning
		}
		return NewMessageRepository(tx).UpdateMessageContentAndReasoningWithContext(ctx, messageID, content, reasoning, message.PromptTokens, message.CompletionTokens, message.TotalTokens, message.Model)
	})
}

// EnsureResponse vincula a única resposta editável à execução ativa. Nunca
// escolhe o primeiro assistant do histórico depois que o turno é canônico.
func (r *ChatTurnRepository) EnsureResponse(ctx context.Context, conversationID, turnID, executionID string) (string, error) {
	var responseID string
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		turn, err := scopedChatTurn(ctx, tx, conversationID, turnID)
		if err != nil {
			return err
		}
		if turn.ActiveExecutionID == nil || *turn.ActiveExecutionID != executionID {
			return ErrChatTurnStale
		}
		if turn.ResponseMessageID != nil {
			responseID = *turn.ResponseMessageID
			return nil
		}
		var question ChatMessage
		if err := tx.Select("parent_id").First(&question, "id = ?", turnID).Error; err != nil {
			return err
		}
		response := &ChatMessage{ConversationID: conversationID, TurnID: &turnID, ParentID: question.ParentID, Role: "assistant"}
		if err := tx.Create(response).Error; err != nil {
			return err
		}
		responseID = response.ID
		return tx.Model(&ChatTurn{}).Where("user_message_id = ?", turnID).Update("response_message_id", responseID).Error
	})
	return responseID, err
}

// FinishResponse confirma conteúdo, usage e terminal na mesma transação.
// A checagem da execução antecede qualquer escrita da resposta.
func (r *ChatTurnRepository) FinishResponse(ctx context.Context, conversationID, turnID, executionID, state, reason, responseID string, response *MessageOptions) error {
	return r.transaction(ctx, func(tx *gorm.DB) error {
		turn, err := scopedChatTurn(ctx, tx, conversationID, turnID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChatTurnStale
		}
		if err != nil {
			return err
		}
		if turn.ActiveExecutionID == nil || *turn.ActiveExecutionID != executionID {
			return ErrChatTurnStale
		}
		if response != nil {
			if turn.ResponseMessageID == nil || *turn.ResponseMessageID != responseID {
				return ErrChatTurnInvalid
			}
			err := NewMessageRepository(tx).UpdateMessageContentReasoningAndUsageWithContext(ctx, *turn.ResponseMessageID, response.Content, response.Reasoning, response.PromptTokens, response.CompletionTokens, response.TotalTokens, response.CacheReadTokens, response.CacheWriteTokens, response.CacheMissTokens, response.Model)
			if err != nil {
				return err
			}
		}
		return NewChatTurnRepository(tx).Finish(ctx, conversationID, turnID, executionID, state, reason)
	})
}

// initializeChatTurnsBeforeWorkers só roda ao abrir o banco, antes de admitir
// execuções. Nunca usar como limpeza periódica de uma instância ativa.
func initializeChatTurnsBeforeWorkers(db *gorm.DB) error {
	versions, err := appliedMigrationVersions(db)
	if err != nil {
		return err
	}
	if !versions[41] {
		return nil
	} // a adoção do usuário ainda pode adiar v19/v41
	return db.Transaction(func(tx *gorm.DB) error {
		var owners []string
		if err := tx.Raw(`SELECT DISTINCT c.user_id FROM conversations c JOIN chat_turns t ON t.conversation_id=c.id WHERE t.active_execution_id IS NOT NULL`).Scan(&owners).Error; err != nil {
			return err
		}
		for _, owner := range owners {
			if err := NewChatTurnRepository(tx).RecoverInterruptedBeforeStartup(WithUserID(context.Background(), owner)); err != nil {
				return err
			}
		}
		return reconcileLegacyChatTurns(tx, "", "")
	})
}
