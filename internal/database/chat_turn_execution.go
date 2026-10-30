package database

import (
	"assistente/internal/turnlease"
	"context"
	"errors"
	"path/filepath"
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

// CreateExecutionLease usa armazenamento persistente adjacente ao banco. O
// caminho exato vai na tentativa e independe do ambiente/path usado no próximo boot.
func (r *ChatTurnRepository) CreateExecutionLease(ctx context.Context, executionID string) (*turnlease.Lease, error) {
	sqlDB, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	path, err := sqliteMainDatabasePath(sqlDB)
	if err != nil {
		return nil, err
	}
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("banco de chat precisa de caminho persistente")
	}
	return turnlease.Create(ctx, filepath.Join(filepath.Dir(path), ".assistente-chat-executions"), executionID)
}

// CheckReady bloqueia o envio antes da pergunta quando o upgrade depende da
// adoção dos dados legados. O login permanece disponível para concluir a adoção.
func (r *ChatTurnRepository) CheckReady(ctx context.Context) error {
	versions, err := appliedMigrationVersions(r.db.WithContext(ctx))
	if err != nil {
		return err
	}
	if !versions[41] || !versions[42] {
		return ErrChatTurnMigrationPending
	}
	return nil
}

var ErrChatTurnMigrationPending = errors.New("migração do histórico de chat pendente; conclua a adoção dos dados no login e reinicie o aplicativo")

// RecoverChatTurnsBeforeWorkers recupera somente execuções sem lease vivo.
// A composição chama antes de admitir workers; CLI e desktop usam a mesma prova.
func RecoverChatTurnsBeforeWorkers() error { return initializeChatTurnsBeforeWorkers(db) }

// initializeChatTurnsBeforeWorkers só roda ao abrir o banco, antes de admitir
// execuções. Nunca usar como limpeza periódica de uma instância ativa.
func initializeChatTurnsBeforeWorkers(db *gorm.DB) error {
	versions, err := appliedMigrationVersions(db)
	if err != nil {
		return err
	}
	if !versions[41] || !versions[42] {
		return nil
	} // a adoção do usuário ainda pode adiar v19/v41/v42
	var active []struct {
		UserID            string
		ConversationID    string
		UserMessageID     string
		ActiveExecutionID string
		LeasePath         string
	}
	if err := db.Raw(`SELECT c.user_id,t.conversation_id,t.user_message_id,t.active_execution_id,a.lease_path FROM conversations c JOIN chat_turns t ON t.conversation_id=c.id JOIN chat_turn_attempts a ON a.execution_id=t.active_execution_id WHERE t.active_execution_id IS NOT NULL`).Scan(&active).Error; err != nil {
		return err
	}
	for _, turn := range active {
		lease, err := turnlease.Open(context.Background(), turn.LeasePath, turn.ActiveExecutionID)
		if errors.Is(err, turnlease.ErrBusy) {
			continue
		}
		if err != nil {
			return err
		}
		ctx := WithUserID(context.Background(), turn.UserID)
		finishErr := NewChatTurnRepository(db).FinishResponse(ctx, turn.ConversationID, turn.UserMessageID, turn.ActiveExecutionID, TurnInterrupted, "restart", "", nil)
		closeErr := lease.Close()
		if finishErr != nil && !errors.Is(finishErr, ErrChatTurnStale) {
			return finishErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return db.Transaction(func(tx *gorm.DB) error { return reconcileLegacyChatTurns(tx, "", "") })
}
