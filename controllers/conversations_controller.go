package controllers

import (
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/toolinvocations"
	"context"
	"fmt"
	"strings"
)

// ConversationsController orquestra persistência de conversas/mensagens (AEP-0088).
// Side-effects do App (reset de escopo, questionário, modelo do perfil) entram via hooks.
type ConversationsControllerConfig struct {
	MsgRepo               chat.MessageRepository
	Emitter               ports.Emitter
	ResetScopedState      func(ctx context.Context, conversationID string)
	ConfirmDeleteMessage  func() error
	GetEffectiveModelFunc func() (string, error)
}

// ConversationsController é o orquestrador do domínio conversations.
type ConversationsController struct {
	msgRepo              chat.MessageRepository
	emitter              ports.Emitter
	resetScopedState     func(ctx context.Context, conversationID string)
	confirmDeleteMessage func() error
	getEffectiveModel    func() (string, error)
}

// NewConversationsController monta o controller a partir da config.
func NewConversationsController(cfg ConversationsControllerConfig) *ConversationsController {
	return &ConversationsController{
		msgRepo:              cfg.MsgRepo,
		emitter:              cfg.Emitter,
		resetScopedState:     cfg.ResetScopedState,
		confirmDeleteMessage: cfg.ConfirmDeleteMessage,
		getEffectiveModel:    cfg.GetEffectiveModelFunc,
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

// GetMessages retorna mensagens com filtro por parent (lazy loading).
func (c *ConversationsController) GetMessages(ctx context.Context, conversationID string, parentID *string) ([]chat.MessageNode, error) {
	if c.msgRepo == nil {
		return nil, fmt.Errorf("message repository não inicializado")
	}
	messages, err := c.msgRepo.GetMessages(ctx, conversationID, parentID)
	if err != nil {
		return nil, err
	}
	return buildMessageNodesWithInvocationFallback(ctx, messages, parentID), nil
}

// GetRecentMessages retorna as mensagens raiz mais recentes de uma conversa.
func (c *ConversationsController) GetRecentMessages(ctx context.Context, conversationID string, limit int) ([]chat.MessageNode, error) {
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []chat.MessageNode{}, nil
	}
	rawLimit := limit
	if rawLimit < 10 {
		rawLimit = 10
	}
	if rawLimit > 5000 {
		rawLimit = 5000
	}
	var lastNodes []chat.MessageNode
	for {
		messages, err := database.GetRecentRootMessagesWithContext(ctx, conversationID, rawLimit)
		if err != nil {
			return nil, err
		}
		nodes := buildMessageNodesWithInvocationFallback(ctx, messages, nil)
		lastNodes = nodes
		if len(nodes) >= limit || len(messages) < rawLimit {
			if len(nodes) > limit {
				return nodes[len(nodes)-limit:], nil
			}
			return nodes, nil
		}
		if rawLimit >= 5000 {
			break
		}
		rawLimit *= 2
		if rawLimit > 5000 {
			rawLimit = 5000
		}
	}
	if len(lastNodes) > limit {
		return lastNodes[len(lastNodes)-limit:], nil
	}
	return lastNodes, nil
}

// GetMessagesBefore retorna mensagens raiz anteriores ao cursor informado.
func (c *ConversationsController) GetMessagesBefore(ctx context.Context, conversationID string, beforeID string, limit int) ([]chat.MessageNode, error) {
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []chat.MessageNode{}, nil
	}
	rawLimit := limit
	if rawLimit < 10 {
		rawLimit = 10
	}
	if rawLimit > 5000 {
		rawLimit = 5000
	}
	var lastNodes []chat.MessageNode
	for {
		messages, err := database.GetRootMessagesBeforeWithContext(ctx, conversationID, beforeID, rawLimit)
		if err != nil {
			return nil, err
		}
		nodes := buildMessageNodesWithInvocationFallback(ctx, messages, nil)
		lastNodes = nodes
		if len(nodes) >= limit || len(messages) < rawLimit {
			if len(nodes) > limit {
				return nodes[len(nodes)-limit:], nil
			}
			return nodes, nil
		}
		if rawLimit >= 5000 {
			break
		}
		rawLimit *= 2
		if rawLimit > 5000 {
			rawLimit = 5000
		}
	}
	if len(lastNodes) > limit {
		return lastNodes[len(lastNodes)-limit:], nil
	}
	return lastNodes, nil
}

// GetConversationMessageWindow é a API canônica de carregamento incremental.
func (c *ConversationsController) GetConversationMessageWindow(ctx context.Context, req chat.MessageWindowRequest) (*chat.MessageWindow, error) {
	conversationID := strings.TrimSpace(req.ConversationID)
	if conversationID == "" {
		return nil, fmt.Errorf("conversationId é obrigatório")
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = chat.MessageWindowScopeConversation
	}
	if scope != chat.MessageWindowScopeConversation && scope != chat.MessageWindowScopeThread {
		return nil, fmt.Errorf("scope de janela de mensagens inválido: %s", req.Scope)
	}
	if req.Limit <= 0 {
		return nil, fmt.Errorf("limit deve ser maior que zero")
	}
	limit := req.Limit
	if limit > database.MaxMessageWindowRows {
		limit = database.MaxMessageWindowRows
	}

	anchor := strings.TrimSpace(req.Anchor)
	anchorMessageID := strings.TrimSpace(req.AnchorMessageID)
	direction := strings.TrimSpace(req.Direction)
	if direction == "" {
		direction = chat.MessageWindowDirectionBefore
	}
	if direction != chat.MessageWindowDirectionBefore &&
		direction != chat.MessageWindowDirectionAfter &&
		direction != chat.MessageWindowDirectionAround {
		return nil, fmt.Errorf("direction de janela de mensagens inválido: %s", req.Direction)
	}
	if anchor != "" &&
		anchor != chat.MessageWindowAnchorStart &&
		anchor != chat.MessageWindowAnchorEnd {
		return nil, fmt.Errorf("anchor de janela de mensagens inválido: %s", req.Anchor)
	}
	if anchor != "" && anchorMessageID != "" {
		return nil, fmt.Errorf("anchor e anchorMessageId são mutuamente exclusivos")
	}
	if anchor == chat.MessageWindowAnchorStart && direction == chat.MessageWindowDirectionBefore {
		return nil, fmt.Errorf("anchor=start não aceita direction=before")
	}
	if anchor == chat.MessageWindowAnchorEnd && direction == chat.MessageWindowDirectionAfter {
		return nil, fmt.Errorf("anchor=end não aceita direction=after")
	}
	if direction == chat.MessageWindowDirectionAround && anchorMessageID == "" {
		return nil, fmt.Errorf("direction=around exige anchorMessageId")
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}

	var parentID *string
	threadParentID := ""
	if scope == chat.MessageWindowScopeThread {
		threadParentID = strings.TrimSpace(req.ThreadParentID)
		if threadParentID == "" {
			return nil, fmt.Errorf("threadParentId é obrigatório para scope=thread")
		}
		parentMessage, err := database.GetMessageWithContext(ctx, threadParentID)
		if err != nil {
			return nil, fmt.Errorf("threadParentId inválido: %w", err)
		}
		if parentMessage.ConversationID != conversationID {
			return nil, fmt.Errorf("threadParentId não pertence à conversa solicitada")
		}
		if parentMessage.ParentID != nil {
			return nil, fmt.Errorf("threadParentId deve apontar para uma mensagem raiz")
		}
		parentID = &threadParentID
	}

	window, err := database.GetMessageWindowWithContext(ctx, database.MessageWindowQuery{
		ConversationID:  conversationID,
		ParentID:        parentID,
		Anchor:          anchor,
		AnchorMessageID: anchorMessageID,
		Direction:       direction,
		Limit:           limit,
	})
	if err != nil {
		return nil, err
	}
	nodes := buildTimelineMessageNodes(ctx, window.Items, window.Messages, parentID)

	return &chat.MessageWindow{
		Scope:          scope,
		ConversationID: conversationID,
		ThreadParentID: threadParentID,
		Nodes:          nodes,
		TotalCount:     window.TotalCount,
		StartIndex:     window.StartIndex,
		EndIndex:       window.EndIndex,
		HasBefore:      window.HasBefore,
		HasAfter:       window.HasAfter,
	}, nil
}

// GetConversationInfo retorna metadados da conversa (sem mensagens).
func (c *ConversationsController) GetConversationInfo(ctx context.Context, id string) (*database.Conversation, error) {
	return database.GetConversationInfoWithContext(ctx, id)
}

// GetConversationWithThreads retorna conversa com mensagens raiz (lazy loading).
func (c *ConversationsController) GetConversationWithThreads(ctx context.Context, id string) (*chat.ConversationWithThreads, error) {
	conv, err := c.GetConversationInfo(ctx, id)
	if err != nil {
		return nil, err
	}
	threads, err := c.GetMessages(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	return &chat.ConversationWithThreads{
		ID:      conv.ID,
		Title:   conv.Title,
		Threads: threads,
	}, nil
}

// GetMessageChildren retorna os filhos de uma mensagem (lazy loading).
func (c *ConversationsController) GetMessageChildren(ctx context.Context, messageID string) ([]chat.MessageNode, error) {
	parent, err := database.GetMessageWithContext(ctx, messageID)
	if err != nil {
		return nil, err
	}
	return c.GetMessages(ctx, parent.ConversationID, &messageID)
}

// UpdateConversation atualiza título/modelo e emite rename se houver título.
func (c *ConversationsController) UpdateConversation(ctx context.Context, id string, title, model string) error {
	if err := database.UpdateConversationWithContext(ctx, id, title, model); err != nil {
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

// DeleteConversation remove a conversa e limpa estado efêmero.
func (c *ConversationsController) DeleteConversation(ctx context.Context, id string) error {
	if err := database.DeleteConversationWithContext(ctx, id); err != nil {
		return err
	}
	c.resetScoped(ctx, id)
	c.emit("conversation:deleted", map[string]interface{}{
		"conversation_id": id,
	})
	return nil
}

// DeleteMessage exclui uma mensagem (com confirmação via hook) e filhas.
func (c *ConversationsController) DeleteMessage(ctx context.Context, messageID string) error {
	if c.confirmDeleteMessage != nil {
		if err := c.confirmDeleteMessage(); err != nil {
			return err
		}
	}
	msg, err := database.GetMessageWithContext(ctx, messageID)
	if err != nil {
		return err
	}
	convID := msg.ConversationID
	if err := database.DeleteMessageWithContext(ctx, messageID); err != nil {
		return err
	}
	c.emit("message:deleted", ports.MessageDeletedEvent{
		ConversationID: convID,
		MessageID:      messageID,
	})
	return nil
}

// UpdateMessage atualiza o conteúdo de uma mensagem existente.
func (c *ConversationsController) UpdateMessage(ctx context.Context, messageID string, newContent string) error {
	msg, err := database.GetMessageWithContext(ctx, messageID)
	if err != nil {
		return err
	}
	if err := database.UpdateMessageContentWithContext(ctx,
		messageID,
		newContent,
		0, 0, 0, "",
	); err != nil {
		return err
	}
	c.emit("message:updated", ports.MessageUpdatedEvent{
		ConversationID: msg.ConversationID,
		MessageID:      messageID,
		Content:        newContent,
	})
	return nil
}

// UpdateConversationModel altera só o modelo da conversa.
func (c *ConversationsController) UpdateConversationModel(ctx context.Context, id string, model string) error {
	return database.UpdateConversationWithContext(ctx, id, "", model)
}

// CreateMessage cria uma mensagem simples.
func (c *ConversationsController) CreateMessage(ctx context.Context, conversationID string, role, content string) (*database.ChatMessage, error) {
	if c.msgRepo == nil {
		return nil, fmt.Errorf("message repository não inicializado")
	}
	return c.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

// AddMessage adiciona uma mensagem simples.
func (c *ConversationsController) AddMessage(ctx context.Context, conversationID string, role, content string) (*database.ChatMessage, error) {
	if c.msgRepo == nil {
		return nil, fmt.Errorf("message repository não inicializado")
	}
	return c.msgRepo.CreateMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content})
}

// AddMessageWithMedia adiciona mensagem com mídia.
func (c *ConversationsController) AddMessageWithMedia(ctx context.Context, conversationID string, role, content, media string) (*database.ChatMessage, error) {
	if c.msgRepo == nil {
		return nil, fmt.Errorf("message repository não inicializado")
	}
	return c.msgRepo.CreateMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content, Media: media})
}

// AddMessageWithTokens adiciona mensagem com tokens.
func (c *ConversationsController) AddMessageWithTokens(ctx context.Context, conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*database.ChatMessage, error) {
	if c.msgRepo == nil {
		return nil, fmt.Errorf("message repository não inicializado")
	}
	return c.msgRepo.CreateMessage(ctx, database.MessageOptions{
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
	if c.msgRepo == nil {
		return nil, fmt.Errorf("message repository não inicializado")
	}
	return c.msgRepo.CreateMessage(ctx, database.MessageOptions{
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
	if c.msgRepo == nil {
		return nil, fmt.Errorf("message repository não inicializado")
	}
	return c.msgRepo.CreateMessage(ctx, database.MessageOptions{
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
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	if err := database.DeleteAllMessagesWithContext(ctx, conversationID); err != nil {
		return err
	}
	c.resetScoped(ctx, conversationID)
	c.emit("conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})
	return nil
}

// DeleteMessages remove mensagens específicas de uma conversa.
func (c *ConversationsController) DeleteMessages(ctx context.Context, conversationID string, messageIDs []string) error {
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	for _, msgID := range messageIDs {
		msg, err := database.GetMessageWithContext(ctx, msgID)
		if err != nil {
			return fmt.Errorf("erro ao localizar mensagem %s: %w", msgID, err)
		}
		if msg.ConversationID != conversationID {
			return fmt.Errorf("mensagem %s não pertence à conversa solicitada", msgID)
		}
		if err := database.DeleteMessageWithContext(ctx, msgID); err != nil {
			return fmt.Errorf("erro ao deletar mensagem %s: %w", msgID, err)
		}
	}
	return nil
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

func assignMessageNodeChildCounts(ctx context.Context, nodes []chat.MessageNode) []chat.MessageNode {
	if len(nodes) == 0 {
		return nodes
	}
	msgIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		msgIDs = append(msgIDs, node.Message.ID)
	}
	childCounts, err := database.CountChildrenWithContext(ctx, msgIDs)
	if err != nil {
		childCounts = make(map[string]int)
	}
	for i := range nodes {
		nodes[i].ChildCount = childCounts[nodes[i].Message.ID]
	}
	return nodes
}

func buildMessageNodesWithInvocationFallback(ctx context.Context, messages []database.ChatMessage, parentID *string) []chat.MessageNode {
	if len(messages) == 0 {
		return []chat.MessageNode{}
	}
	turnIDs := chat.CollectTurnIDsWithToolCalls(messages)
	invocationToolCalls := loadChatToolInvocationDisplaysForTurnIDs(ctx, turnIDs)
	invocationToolResults := toolInvocationResultsFromTurnSegments(invocationToolCalls)
	nodes := chat.BuildNodesWithTimelineConsolidation(messages, parentID, map[string]int{}, invocationToolResults, invocationToolCalls)
	return assignMessageNodeChildCounts(ctx, nodes)
}

func buildTimelineMessageNodes(ctx context.Context, items []database.MessageWindowItem, messages []database.ChatMessage, parentID *string) []chat.MessageNode {
	turnIDs := chat.CollectTurnIDsWithToolCalls(messages)
	invocationToolCalls := loadChatToolInvocationDisplaysForTurnIDs(ctx, turnIDs)
	invocationToolResults := toolInvocationResultsFromTurnSegments(invocationToolCalls)
	nodes := chat.BuildTimelineMessageNodes(items, messages, parentID, map[string]int{}, invocationToolResults, invocationToolCalls)
	return assignMessageNodeChildCounts(ctx, nodes)
}

func loadChatToolInvocationDisplaysForTurnIDs(ctx context.Context, turnIDs []string) map[string][]chat.TurnSegmentToolCall {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return map[string][]chat.TurnSegmentToolCall{}
	}
	if len(turnIDs) == 0 {
		return map[string][]chat.TurnSegmentToolCall{}
	}
	results, err := toolinvocations.LoadChatToolInvocationDisplaysForTurnIDsWithUser(ctx, userID, turnIDs)
	if err != nil {
		logging.Errorf(ctx, "controllers.conversations", "[Chat] load tool_invocations display failed: %v", err)
		return map[string][]chat.TurnSegmentToolCall{}
	}
	return toolInvocationDisplaysToTurnSegments(results)
}

func toolInvocationResultsFromTurnSegments(callsByTurn map[string][]chat.TurnSegmentToolCall) map[string]map[string]string {
	out := make(map[string]map[string]string, len(callsByTurn))
	for turnID, calls := range callsByTurn {
		for _, call := range calls {
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
				continue
			}
			byCall := out[turnID]
			if byCall == nil {
				byCall = map[string]string{}
				out[turnID] = byCall
			}
			if _, ok := byCall[callID]; ok {
				continue
			}
			byCall[callID] = call.Result
		}
	}
	return out
}

func toolInvocationDisplaysToTurnSegments(displays map[string][]toolinvocations.ChatToolInvocationDisplay) map[string][]chat.TurnSegmentToolCall {
	out := make(map[string][]chat.TurnSegmentToolCall, len(displays))
	for turnID, calls := range displays {
		for _, call := range calls {
			out[turnID] = append(out[turnID], chat.TurnSegmentToolCall{
				ID:                 call.ID,
				Type:               call.Type,
				Function:           chat.TurnSegmentToolFunction{Name: call.Name, Arguments: call.Arguments},
				Result:             call.Result,
				Origin:             call.Origin,
				ServerLabel:        call.ServerLabel,
				Iteration:          call.Iteration,
				DurationMs:         call.DurationMs,
				AssistantMessageID: call.AssistantMessageID,
			})
		}
	}
	return out
}
