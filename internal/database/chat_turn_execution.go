package database

import (
	"assistente/internal/logging"
	"assistente/internal/turnlease"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ReconcileImportedConversationInTransaction exige a transação do importador.
// Reconcilia apenas legado sem tentativas; importação de execuções é explícita
// no serviço de portabilidade. Falhas revertem também as mensagens.
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

// SaveIntermediate transfere a fonte de conteúdo para a mensagem intermediária
// na mesma transação. O representante mantém sua identidade, sem a cópia inicial
// do prefixo de continuação, mesmo se o processo terminar antes da próxima iteração.
func (r *ChatTurnRepository) SaveIntermediate(ctx context.Context, executionID, responseID string, opts MessageOptions) (*ChatMessage, error) {
	if opts.TurnID == nil || opts.Role != "assistant" || opts.ChatExecutionID == nil || *opts.ChatExecutionID != executionID {
		return nil, ErrChatTurnInvalid
	}
	var message *ChatMessage
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		turn, err := scopedChatTurn(ctx, tx, opts.ConversationID, *opts.TurnID)
		if err != nil {
			return err
		}
		if turn.ActiveExecutionID == nil || *turn.ActiveExecutionID != executionID || turn.ResponseMessageID == nil || *turn.ResponseMessageID != responseID {
			return ErrChatTurnStale
		}
		var response ChatMessage
		if err := tx.First(&response, "id = ?", responseID).Error; err != nil {
			return err
		}
		message = &ChatMessage{ConversationID: opts.ConversationID, TurnID: opts.TurnID,
			ChatExecutionID: opts.ChatExecutionID, ParentID: response.ParentID, Role: "assistant",
			Content: opts.Content, Reasoning: opts.Reasoning, Model: opts.Model}
		if err := CreateChatMessageInTransaction(tx, message); err != nil {
			return err
		}
		if err := tx.Model(&ChatMessage{}).Where("id = ?", responseID).
			Updates(map[string]any{"content": "", "reasoning": ""}).Error; err != nil {
			return err
		}
		return ScopeByUser(ctx, tx.Model(&Conversation{}), "user_id").
			Where("id = ?", opts.ConversationID).Update("updated_at", time.Now()).Error
	})
	if err != nil {
		return nil, err
	}
	return message, nil
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
		var attempt ChatTurnAttempt
		if err := tx.First(&attempt, "execution_id=?", executionID).Error; err != nil {
			return err
		}
		response := &ChatMessage{ConversationID: conversationID, TurnID: &turnID, ParentID: question.ParentID, Role: "assistant", ChatExecutionID: &executionID}
		if attempt.Mode == TurnModeContinue {
			if attempt.BaseResponseMessageID == nil {
				return ErrChatTurnInvalid
			}
			var base ChatMessage
			if err := tx.First(&base, "id=?", *attempt.BaseResponseMessageID).Error; err != nil {
				return err
			}
			response.Content, response.Reasoning = base.Content, base.Reasoning
		}
		if err := CreateChatMessageInTransaction(tx, response); err != nil {
			return err
		}
		responseID = response.ID
		if err := tx.Table("chat_turn_attempts").Where("execution_id=?", executionID).Update("response_message_id", responseID).Error; err != nil {
			return err
		}
		return tx.Model(&ChatTurn{}).Where("user_message_id = ?", turnID).Update("response_message_id", responseID).Error
	})
	return responseID, err
}

// FinishResponse confirma conteúdo, usage e terminal na mesma transação.
// A checagem da execução antecede qualquer escrita da resposta.
func (r *ChatTurnRepository) FinishResponse(ctx context.Context, conversationID, turnID, executionID, state, reason, responseID string, response *MessageOptions) error {
	return r.finishResponse(ctx, conversationID, turnID, executionID, state, reason, responseID, response, nil)
}

// FinishResponseWithVersion captura a versão na própria transação terminal.
// A recuperação de startup mantém FinishResponse, inclusive antes da migração v43.
func (r *ChatTurnRepository) FinishResponseWithVersion(ctx context.Context, conversationID, turnID, executionID, state, reason, responseID string, response *MessageOptions) (ChatTimelineVersion, error) {
	var version ChatTimelineVersion
	err := r.finishResponse(ctx, conversationID, turnID, executionID, state, reason, responseID, response, func(tx *gorm.DB) error {
		var readErr error
		version, readErr = ReadChatTimelineVersion(ctx, tx, conversationID)
		return readErr
	})
	return version, err
}

func (r *ChatTurnRepository) finishResponse(ctx context.Context, conversationID, turnID, executionID, state, reason, responseID string, response *MessageOptions, afterWrite func(*gorm.DB) error) error {
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
		if err := NewChatTurnRepository(tx).Finish(ctx, conversationID, turnID, executionID, state, reason); err != nil {
			return err
		}
		if afterWrite != nil {
			return afterWrite(tx)
		}
		return nil
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
	if !versions[41] || !versions[42] || !versions[43] || !versions[44] || !versions[45] || !versions[46] {
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
			logging.Warnf(context.Background(), "database.chat-turns", "posse da execução %s indisponível; tentativa preservada para restauração dos dados: %v", turn.ActiveExecutionID, err)
			continue
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
