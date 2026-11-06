package controllers

import (
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"context"
	"errors"
	"fmt"
)

// ConversationsController orquestra persistência de conversas/mensagens (AEP-0088).
// Side-effects do App (reset de escopo, questionário, modelo do perfil) entram via hooks.
type ConversationsControllerConfig struct {
	MutateTimeline              func(context.Context, string, []string, func() ([]string, error)) error
	MsgRepo                     chat.MessageRepository
	Emitter                     ports.Emitter
	ResetScopedState            func(ctx context.Context, conversationID string)
	PrepareBatchDelete          func(ctx context.Context, conversationIDs []string) (finalize func(committed bool), err error)
	ValidateBatchDelete         func(ctx context.Context, conversationIDs []string) ([]string, error)
	DeleteTimeline              func(context.Context, []string, func() ([]string, error)) ([]string, error)
	ListConversations           func(ctx context.Context) ([]database.Conversation, error)
	WithMaintenance             func(ctx context.Context, fn func() error) error
	DeleteWithinMaintenance     func(ctx context.Context, conversationIDs []string) ([]string, error)
	ConfirmDeleteMessage        func() error
	GetEffectiveModelFunc       func() (string, error)
	UpdateConversationTitleFunc func(ctx context.Context, conversationID, title, model string) error
}

// ConversationsController é o orquestrador do domínio conversations.
type ConversationsController struct {
	mutateTimeline          func(context.Context, string, []string, func() ([]string, error)) error
	msgRepo                 chat.MessageRepository
	emitter                 ports.Emitter
	resetScopedState        func(ctx context.Context, conversationID string)
	prepareBatchDelete      func(ctx context.Context, conversationIDs []string) (finalize func(committed bool), err error)
	validateBatchDelete     func(ctx context.Context, conversationIDs []string) ([]string, error)
	deleteTimeline          func(context.Context, []string, func() ([]string, error)) ([]string, error)
	listConversations       func(ctx context.Context) ([]database.Conversation, error)
	withMaintenance         func(ctx context.Context, fn func() error) error
	deleteWithinMaintenance func(ctx context.Context, conversationIDs []string) ([]string, error)
	confirmDeleteMessage    func() error
	getEffectiveModel       func() (string, error)
	updateConversationTitle func(ctx context.Context, conversationID, title, model string) error
}

var (
	errClearConversationSnapshotChanged = errors.New("conversation snapshot changed during clear")
	ErrClearConversationsContended      = errors.New("conversation set kept changing during clear")
)

const clearConversationsMaxAttempts = 8

// NewConversationsController monta o controller a partir da config.
func NewConversationsController(cfg ConversationsControllerConfig) *ConversationsController {

	if cfg.ValidateBatchDelete == nil {
		cfg.ValidateBatchDelete = database.ValidateOwnedConversationIDsWithContext
	}
	if cfg.ListConversations == nil {
		cfg.ListConversations = database.GetConversationsWithContext
	}
	if cfg.WithMaintenance == nil {
		cfg.WithMaintenance = database.WithSQLiteMaintenance
	}
	if cfg.DeleteWithinMaintenance == nil {
		cfg.DeleteWithinMaintenance = database.DeleteConversationsWithinMaintenanceWithContext
	}
	return &ConversationsController{
		mutateTimeline:          cfg.MutateTimeline,
		msgRepo:                 cfg.MsgRepo,
		emitter:                 cfg.Emitter,
		resetScopedState:        cfg.ResetScopedState,
		prepareBatchDelete:      cfg.PrepareBatchDelete,
		validateBatchDelete:     cfg.ValidateBatchDelete,
		deleteTimeline:          cfg.DeleteTimeline,
		listConversations:       cfg.ListConversations,
		withMaintenance:         cfg.WithMaintenance,
		deleteWithinMaintenance: cfg.DeleteWithinMaintenance,
		confirmDeleteMessage:    cfg.ConfirmDeleteMessage,
		getEffectiveModel:       cfg.GetEffectiveModelFunc,
		updateConversationTitle: cfg.UpdateConversationTitleFunc,
	}
}

func (c *ConversationsController) emit(event string, data any) {
	if c != nil && c.emitter != nil {
		c.emitter.Emit(event, data)
	}
}

func (c *ConversationsController) resetScoped(ctx context.Context, conversationID string) {
	if c != nil && c.resetScopedState != nil {
		c.resetScopedState(ctx, conversationID)
	}
}

// CreateConversation cria uma conversa e limpa estado efêmero do ID.
func (c *ConversationsController) CreateConversation(ctx context.Context, title, model string) (*database.Conversation, error) {
	conv, err := database.CreateConversationWithContext(ctx, title, model)
	if err != nil {
		return nil, err
	}
	c.resetScoped(ctx, conv.ID)
	return conv, nil
}

// GetConversations lista conversas do usuário.
func (c *ConversationsController) GetConversations(ctx context.Context) ([]database.Conversation, error) {
	return database.GetConversationsWithContext(ctx)
}

// GetConversationsPage lista conversas paginadas.
func (c *ConversationsController) GetConversationsPage(ctx context.Context, limit, offset int) (database.ConversationListResult, error) {
	limit, offset = normalizeConversationPageRequest(limit, offset)
	return database.GetConversationsPageWithContext(ctx, limit, offset)
}

func normalizeConversationPageRequest(limit, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 && offset > 0 {
		limit = database.DefaultConversationPageLimit
	}
	return limit, offset
}

// GetConversationsByIDs retorna conversas pelos IDs informados.
func (c *ConversationsController) GetConversationsByIDs(ctx context.Context, ids []string) ([]database.Conversation, error) {
	return database.GetConversationsByIDsWithContext(ctx, ids)
}

// GetConversation retorna uma conversa pelo id.
func (c *ConversationsController) GetConversation(ctx context.Context, id string) (*database.Conversation, error) {
	return database.GetConversationWithContext(ctx, id)
}

// EnsureConversation cria ou recicla uma conversa vazia.
func (c *ConversationsController) EnsureConversation(ctx context.Context, title string) (*database.Conversation, error) {
	if title == "" {
		title = "Nova Conversa"
	}
	conv, err := database.RecycleOrCreateConversationWithContext(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("erro ao garantir conversa: %w", err)
	}
	c.resetScoped(ctx, conv.ID)
	return conv, nil
}

// GetToolInvocationDetails carrega payloads técnicos sob demanda em um único
// lote user-scoped. A timeline recebe apenas Summary.
func (c *ConversationsController) GetToolInvocationDetails(ctx context.Context, invocationIDs []string) ([]toolinvocations.Detail, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	return toolinvocations.LoadDetailsWithUser(ctx, userID, invocationIDs)
}

// GetConversationInfo retorna metadados da conversa (sem mensagens).
func (c *ConversationsController) GetConversationInfo(ctx context.Context, id string) (*database.Conversation, error) {
	return database.GetConversationInfoWithContext(ctx, id)
}

// UpdateConversation atualiza título/modelo e emite rename se houver título.
func (c *ConversationsController) UpdateConversation(ctx context.Context, id string, title, model string) error {
	update := c.updateConversationTitle
	if update == nil {
		update = database.UpdateConversationWithContext
	}
	if err := update(ctx, id, title, model); err != nil {
		return err
	}
	if title != "" {
		c.emit("conversation:renamed", ports.ConversationRenamedEvent{
			ConversationID: id,
			NewTitle:       title,
		})
	}
	return nil
}

// DeleteConversation delega ao pipeline batch canônico.
func (c *ConversationsController) DeleteConversation(ctx context.Context, id string) error {
	_, err := c.DeleteConversations(ctx, []string{id})
	return err
}

// DeleteConversations remove atomicamente as conversas e só então limpa estado
// efêmero e publica um evento por ID normalizado.
func (c *ConversationsController) DeleteConversations(ctx context.Context, ids []string) ([]string, error) {
	normalizedIDs, err := c.validateBatchDelete(ctx, ids)
	if err != nil {
		return nil, err
	}
	return c.deletePreparedConversations(ctx, normalizedIDs, func(ctx context.Context, ids []string) ([]string, error) {
		var deleted []string
		err := c.withMaintenance(ctx, func() error {
			var err error
			deleted, err = c.deleteTimelineWithinMaintenance(ctx, ids)
			return err
		})
		return deleted, err
	})
}

// ClearConversations remove todas as conversas do usuário mantendo o gate de
// manutenção entre o snapshot e o commit. Assim, criadores não atravessam o
// snapshot e a operação não deixa conversas que existiam quando começou.
func (c *ConversationsController) ClearConversations(ctx context.Context) ([]string, error) {
	for attempt := 1; attempt <= clearConversationsMaxAttempts; attempt++ {
		var conversations []database.Conversation
		err := c.withMaintenance(ctx, func() error {
			return database.WithSQLiteBusyRetry(ctx, "conversations.clear_snapshot", func() error {
				var err error
				conversations, err = c.listConversations(ctx)
				return err
			})
		})
		if err != nil {
			return nil, err
		}
		if len(conversations) == 0 {
			return nil, nil
		}
		ids := make([]string, 0, len(conversations))
		for _, conversation := range conversations {
			ids = append(ids, conversation.ID)
		}

		deletedIDs, err := c.deletePreparedConversations(
			ctx,
			ids,
			func(ctx context.Context, snapshot []string) ([]string, error) {
				var deleted []string
				err := c.withMaintenance(ctx, func() error {
					var current []database.Conversation
					if err := database.WithSQLiteBusyRetry(ctx, "conversations.clear_revalidate", func() error {
						var err error
						current, err = c.listConversations(ctx)
						return err
					}); err != nil {
						return err
					}
					if !sameConversationIDs(snapshot, current) {
						return errClearConversationSnapshotChanged
					}
					deleted, err = c.deleteTimelineWithinMaintenance(ctx, snapshot)
					return err
				})
				return deleted, err
			},
		)
		if errors.Is(err, errClearConversationSnapshotChanged) {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt == clearConversationsMaxAttempts {
				return nil, ErrClearConversationsContended
			}
			continue
		}
		return deletedIDs, err
	}
	return nil, ErrClearConversationsContended
}

func sameConversationIDs(ids []string, conversations []database.Conversation) bool {
	if len(ids) != len(conversations) {
		return false
	}
	expected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		expected[id] = struct{}{}
	}
	for _, conversation := range conversations {
		if _, ok := expected[conversation.ID]; !ok {
			return false
		}
	}
	return true
}

func (c *ConversationsController) deletePreparedConversations(
	ctx context.Context,
	normalizedIDs []string,
	deleteBatch func(context.Context, []string) ([]string, error),
) ([]string, error) {
	finalize := func(bool) {}
	if c.prepareBatchDelete != nil {
		var err error
		finalize, err = c.prepareBatchDelete(ctx, normalizedIDs)
		if err != nil {
			return nil, err
		}
		if finalize == nil {
			finalize = func(bool) {}
		}
	}
	committed := false
	defer func() {
		if !committed {
			finalize(false)
		}
	}()

	var deletedIDs []string
	err := database.WithConversationLifecycle(ctx, func() error {
		var err error
		deletedIDs, err = deleteBatch(ctx, normalizedIDs)
		if err != nil {
			return err
		}
		finalize(true)
		committed = true
		for _, id := range deletedIDs {
			c.resetScoped(ctx, id)
			c.emit("conversation:deleted", map[string]interface{}{
				"conversation_id": id,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return deletedIDs, nil
}

// DeleteMessage exclui uma mensagem (com confirmação via hook) e filhas.
func (c *ConversationsController) DeleteMessage(ctx context.Context, messageID string) error {
	if c.confirmDeleteMessage != nil {
		if err := c.confirmDeleteMessage(); err != nil {
			return err
		}
	}
	msg, err := database.GetMessageIdentityWithContext(ctx, messageID)
	if err != nil {
		return err
	}
	convID := msg.ConversationID
	if err := c.coordinateMessageMutation(ctx, convID, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(ctx, messageID) }); err != nil {
		return err
	}
	c.messageDeleted(convID, messageID)
	return nil
}

// UpdateMessage atualiza o conteúdo de uma mensagem existente.
func (c *ConversationsController) UpdateMessage(ctx context.Context, messageID string, newContent string) error {
	msg, err := database.GetMessageIdentityWithContext(ctx, messageID)
	if err != nil {
		return err
	}
	if err := c.coordinateMessageMutation(ctx, msg.ConversationID, messageTimelineIDs(msg), func() ([]string, error) {
		return nil, database.UpdateMessageContentWithContext(ctx,
			messageID,
			newContent,
			0, 0, 0, "",
		)
	}); err != nil {
		return err
	}
	c.messageUpdated(msg.ConversationID, messageID, newContent)
	return nil
}

// ToggleMessagePin alterna a fixação persistente e publica o novo estado.
func (c *ConversationsController) ToggleMessagePin(ctx context.Context, messageID string) (*database.ChatMessage, error) {
	msg, err := database.GetMessageIdentityWithContext(ctx, messageID)
	if err != nil {
		return nil, err
	}
	err = c.coordinateMessageMutation(ctx, msg.ConversationID, messageTimelineIDs(msg), func() ([]string, error) {
		var err error
		msg, err = database.ToggleMessagePinWithContext(ctx, messageID)
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	c.messagePinChanged(msg)
	return msg, nil
}

// GetPinnedMessages lista as mensagens fixadas da conversa atual.
func (c *ConversationsController) GetPinnedMessages(ctx context.Context, conversationID string) ([]database.ChatMessage, error) {
	return database.GetPinnedMessagesWithContext(ctx, conversationID)
}

// UpdateConversationModel altera só o modelo da conversa.
func (c *ConversationsController) UpdateConversationModel(ctx context.Context, id string, model string) error {
	return database.UpdateConversationWithContext(ctx, id, "", model)
}

// CreateMessage cria uma mensagem simples.
func (c *ConversationsController) CreateMessage(ctx context.Context, conversationID string, role, content string) (*database.ChatMessage, error) {
	return c.createMessage(ctx, database.MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

// AddMessage adiciona uma mensagem simples.
func (c *ConversationsController) AddMessage(ctx context.Context, conversationID string, role, content string) (*database.ChatMessage, error) {
	return c.createMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content})
}

// AddMessageWithMedia adiciona mensagem com mídia.
func (c *ConversationsController) AddMessageWithMedia(ctx context.Context, conversationID string, role, content, media string) (*database.ChatMessage, error) {
	return c.createMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content, Media: media})
}

// AddMessageWithTokens adiciona mensagem com tokens.
func (c *ConversationsController) AddMessageWithTokens(ctx context.Context, conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*database.ChatMessage, error) {
	return c.createMessage(ctx, database.MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// AddMessageWithTokensAndMedia adiciona mensagem com tokens e mídia.
func (c *ConversationsController) AddMessageWithTokensAndMedia(ctx context.Context, conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*database.ChatMessage, error) {
	return c.createMessage(ctx, database.MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		Media:            media,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// AddChildMessage adiciona mensagem filha.
func (c *ConversationsController) AddChildMessage(ctx context.Context, conversationID string, parentID string, role, content, model string) (*database.ChatMessage, error) {
	return c.createMessage(ctx, database.MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		Model:          model,
	})
}

// GetAllTokenStats retorna estatísticas agregadas de tokens.
func (c *ConversationsController) GetAllTokenStats(ctx context.Context) (map[string]int, error) {
	return database.GetAllTokenStatsWithContext(ctx)
}

// GetConversationSummary retorna o resumo rolling da conversa.
func (c *ConversationsController) GetConversationSummary(ctx context.Context, conversationID string) (*apidto.ConversationSummaryInfo, error) {
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	summary, upToID, err := database.GetConversationSummaryWithContext(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	inProgress, _ := database.IsSummarizingInProgressWithContext(ctx, conversationID)
	return &apidto.ConversationSummaryInfo{
		Summary:               summary,
		SummaryUpToMessageID:  upToID,
		SummarizingInProgress: inProgress,
	}, nil
}

// RenameConversation renomeia a conversa.
func (c *ConversationsController) RenameConversation(ctx context.Context, conversationID string, newTitle string) error {
	return c.UpdateConversation(ctx, conversationID, newTitle, "")
}

// ClearConversation apaga todas as mensagens e limpa estado efêmero.
func (c *ConversationsController) ClearConversation(ctx context.Context, conversationID string) error {
	return c.clearConversation(ctx, conversationID, func() error {
		return database.ClearConversationContentWithContext(ctx, conversationID)
	})
}

// GetConversationContentRevision captura o conteúdo antes da confirmação.
func (c *ConversationsController) GetConversationContentRevision(ctx context.Context, conversationID string) (string, error) {
	return database.GetConversationContentRevisionWithContext(ctx, conversationID)
}

// ClearConversationIfUnchanged compara e limpa atomicamente sob o lifecycle;
// ErrConversationContentChanged exige nova captura/confirmação pelo caller.
func (c *ConversationsController) ClearConversationIfUnchanged(ctx context.Context, conversationID, expectedRevision string) error {
	return c.clearConversation(ctx, conversationID, func() error {
		return database.ClearConversationContentIfUnchangedWithinMaintenanceWithContext(ctx, conversationID, expectedRevision)
	})
}

// ClearConversationIfUnchangedGuarded permite ao App adquirir auth/workspace
// DEPOIS de runtime/lifecycle. O guard deve chamar commit sincronamente no máximo
// uma vez, mantendo seus locks até retornar (incluindo os efeitos pós-commit).
func (c *ConversationsController) ClearConversationIfUnchangedGuarded(ctx context.Context, conversationID, expectedRevision string, guard func(commit func() error) error) error {
	if guard == nil {
		return errors.New("conversation clear guard required")
	}
	return c.clearConversationGuarded(ctx, conversationID, func() error {
		return database.ClearConversationContentIfUnchangedWithinMaintenanceWithContext(ctx, conversationID, expectedRevision)
	}, guard)
}

func (c *ConversationsController) clearConversation(ctx context.Context, conversationID string, clear func() error) error {
	return c.clearConversationGuarded(ctx, conversationID, clear, func(commit func() error) error { return commit() })
}

func (c *ConversationsController) clearConversationGuarded(ctx context.Context, conversationID string, clear func() error, guard func(func() error) error) error {
	// Valida ownership antes de fechar os gates runtime. Ordem igual ao delete:
	// runtime -> lifecycle -> guard -> manutenção -> projetor -> transação. Clear preserva a conversa, logo libera
	// sempre com false (sem tombstone), inclusive depois de um commit bem sucedido.
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	if c.prepareBatchDelete != nil {
		finalize, err := c.prepareBatchDelete(ctx, []string{conversationID})
		if err != nil {
			return err
		}
		if finalize != nil {
			defer finalize(false)
		}
	}
	return database.WithConversationLifecycle(ctx, func() error {
		return guard(func() error {
			err := c.withMaintenance(ctx, func() error {
				return c.coordinateMessageMutation(ctx, conversationID, nil, func() ([]string, error) {
					return nil, clear()
				})
			})
			if err != nil {
				return err
			}
			c.conversationCleared(ctx, conversationID)
			return nil
		})
	})
}

func (c *ConversationsController) conversationCleared(ctx context.Context, conversationID string) {
	c.resetScoped(ctx, conversationID)
	c.emit("conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})
}

// DeleteMessages remove mensagens específicas de uma conversa.
func (c *ConversationsController) DeleteMessages(ctx context.Context, conversationID string, messageIDs []string) error {
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	return c.coordinateMessageMutation(ctx, conversationID, nil, func() ([]string, error) {
		for _, msgID := range messageIDs {
			msg, err := database.GetMessageIdentityWithContext(ctx, msgID)
			if err != nil {
				return nil, fmt.Errorf("erro ao localizar mensagem %s: %w", msgID, err)
			}
			if msg.ConversationID != conversationID {
				return nil, fmt.Errorf("mensagem %s não pertence à conversa solicitada", msgID)
			}
			if err := database.DeleteMessageWithContext(ctx, msgID); err != nil {
				return nil, fmt.Errorf("erro ao deletar mensagem %s: %w", msgID, err)
			}
		}
		return nil, nil
	})
}

// SearchConversationHistory busca no conteúdo de mensagens (FTS5).
func (c *ConversationsController) SearchConversationHistory(ctx context.Context, query string, limit int) ([]database.MessageSearchResult, error) {
	return database.SearchMessageContentWithContext(ctx, query, limit)
}

// RebuildSearchIndex reconstrói o índice FTS (instance-wide; exige sessão).
func (c *ConversationsController) RebuildSearchIndex(ctx context.Context) error {
	return database.RebuildFTSIndex(ctx)
}

// SetConversationModel define o modelo da conversa.
func (c *ConversationsController) SetConversationModel(ctx context.Context, conversationID string, model string) error {
	return database.UpdateConversationWithContext(ctx, conversationID, "", model)
}

// GetEffectiveModel retorna o modelo do perfil ativo (hook).
func (c *ConversationsController) GetEffectiveModel(_ context.Context) (string, error) {
	if c.getEffectiveModel == nil {
		return "", nil
	}
	return c.getEffectiveModel()
}
