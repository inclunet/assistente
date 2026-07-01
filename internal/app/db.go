package app

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
)

// Re-exporta tipos do pacote database para manter compatibilidade
type Conversation = database.Conversation
type ChatMessage = database.ChatMessage

// Re-exporta funções que não dependem de App
var (
	InitDatabase  = database.Init
	GenerateTitle = database.GenerateTitle
)

// ==================== Conversation ====================

func (a *App) CreateConversation(title, model string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.CreateConversationWithContext(ctx, title, model)
}

func (a *App) GetConversations() ([]Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetConversationsWithContext(ctx)
}

func (a *App) GetConversation(id string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetConversationWithContext(ctx, id)
}

// EnsureConversation cria ou recicla uma conversa vazia e retorna.
// Usada pelo frontend quando uma aba de chat do workspace é criada sem contentId.
func (a *App) EnsureConversation(title string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = "Nova Conversa"
	}
	conv, err := database.RecycleOrCreateConversationWithContext(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("erro ao garantir conversa: %w", err)
	}
	return conv, nil
}

// GetMessages retorna mensagens com filtro por parent (API unificada com LAZY LOADING)
func (a *App) GetMessages(conversationID string, parentID *string) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	messages, err := a.msgRepo.GetMessages(ctx, conversationID, parentID)
	if err != nil {
		return nil, err
	}

	return buildMessageNodesWithInvocationFallback(ctx, messages, parentID), nil
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
	invocationToolResults := loadChatToolInvocationResultsForTurnIDs(ctx, turnIDs)
	invocationToolCalls := loadChatToolInvocationDisplaysForTurnIDs(ctx, turnIDs)
	nodes := chat.BuildNodesWithTimelineConsolidation(messages, parentID, map[string]int{}, invocationToolResults, invocationToolCalls)
	return assignMessageNodeChildCounts(ctx, nodes)
}

func buildTimelineMessageNodes(ctx context.Context, items []database.MessageWindowItem, messages []database.ChatMessage, parentID *string) []chat.MessageNode {
	turnIDs := chat.CollectTurnIDsWithToolCalls(messages)
	invocationToolResults := loadChatToolInvocationResultsForTurnIDs(ctx, turnIDs)
	invocationToolCalls := loadChatToolInvocationDisplaysForTurnIDs(ctx, turnIDs)
	nodes := chat.BuildTimelineMessageNodes(items, messages, parentID, map[string]int{}, invocationToolResults, invocationToolCalls)
	return assignMessageNodeChildCounts(ctx, nodes)
}

func loadChatToolInvocationResultsForTurnIDs(ctx context.Context, turnIDs []string) map[string]map[string]string {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return map[string]map[string]string{}
	}
	return loadChatToolInvocationResultsForTurnIDsWithUser(ctx, userID, turnIDs)
}

func loadChatToolInvocationResultsForTurnIDsWithUser(ctx context.Context, userID string, turnIDs []string) map[string]map[string]string {
	if len(turnIDs) == 0 {
		return map[string]map[string]string{}
	}
	results, err := toolinvocations.LoadChatToolInvocationResultsForTurnIDsWithUser(ctx, userID, turnIDs)
	if err != nil {
		logging.Errorf(ctx, "app.db", "[Chat] load tool_invocations results failed: %v", err)
		return map[string]map[string]string{}
	}
	return results
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
		logging.Errorf(ctx, "app.db", "[Chat] load tool_invocations display failed: %v", err)
		return map[string][]chat.TurnSegmentToolCall{}
	}
	return toolInvocationDisplaysToTurnSegments(results)
}

func toolInvocationDisplaysToTurnSegments(displays map[string][]toolinvocations.ChatToolInvocationDisplay) map[string][]chat.TurnSegmentToolCall {
	out := make(map[string][]chat.TurnSegmentToolCall, len(displays))
	for turnID, calls := range displays {
		for _, call := range calls {
			out[turnID] = append(out[turnID], chat.TurnSegmentToolCall{
				ID:          call.ID,
				Type:        call.Type,
				Function:    chat.TurnSegmentToolFunction{Name: call.Name, Arguments: call.Arguments},
				Result:      call.Result,
				Origin:      call.Origin,
				ServerLabel: call.ServerLabel,
				Iteration:   call.Iteration,
				DurationMs:  call.DurationMs,
			})
		}
	}
	return out
}

// GetRecentMessages retorna as mensagens raiz mais recentes de uma conversa.
func (a *App) GetRecentMessages(conversationID string, limit int) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []chat.MessageNode{}, nil
	}
	// A API legada pagina por mensagens, mas o renderer colapsa múltiplas linhas do mesmo turno
	// em um único representative. Faz overfetch incremental até preencher o limit.
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
func (a *App) GetMessagesBefore(conversationID string, beforeID string, limit int) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
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

// GetConversationMessageWindow é a API canônica de carregamento incremental de mensagens.
// Ela cobre conversa raiz e filhos diretos de thread com o mesmo contrato total-aware.
func (a *App) GetConversationMessageWindow(req chat.MessageWindowRequest) (*chat.MessageWindow, error) {
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
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
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

// GetConversationInfo retorna apenas metadados da conversa (sem mensagens)
func (a *App) GetConversationInfo(id string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetConversationInfoWithContext(ctx, id)
}

// GetConversationWithThreads retorna conversa com mensagens raiz (lazy loading)
func (a *App) GetConversationWithThreads(id string) (*chat.ConversationWithThreads, error) {
	conv, err := a.GetConversationInfo(id)
	if err != nil {
		return nil, err
	}

	threads, err := a.GetMessages(id, nil)
	if err != nil {
		return nil, err
	}

	return &chat.ConversationWithThreads{
		ID:      conv.ID,
		Title:   conv.Title,
		Threads: threads,
	}, nil
}

// GetMessageChildren retorna os filhos de uma mensagem (lazy loading)
func (a *App) GetMessageChildren(messageID string) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	parent, err := database.GetMessageWithContext(ctx, messageID)
	if err != nil {
		return nil, err
	}
	return a.GetMessages(parent.ConversationID, &messageID)
}

func (a *App) UpdateConversation(id string, title, model string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if err := database.UpdateConversationWithContext(ctx, id, title, model); err != nil {
		return err
	}

	if title != "" {
		a.emitter.Emit("conversation:renamed", ports.ConversationRenamedEvent{
			ConversationID: id,
			NewTitle:       title,
		})
	}

	return nil
}

func (a *App) DeleteConversation(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if err := database.DeleteConversationWithContext(ctx, id); err != nil {
		return err
	}

	a.emitter.Emit("conversation:deleted", map[string]interface{}{
		"conversation_id": id,
	})

	return nil
}

// DeleteMessage exclui uma mensagem e todas as suas filhas (respostas)
func (a *App) DeleteMessage(messageID string) error {
	// Solicita confirmação via questionário
	ctx := a.ctx
	if a.questionnaireMgr == nil {
		return fmt.Errorf("questionnaire manager não inicializado")
	}

	resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       "Excluir mensagem",
		Description: "Tem certeza que deseja excluir esta mensagem e todas as suas respostas? Esta ação não pode ser desfeita.",
		AllowCancel: true,
		SubmitLabel: "Excluir",
		CancelLabel: "Cancelar",
		Questions: []questionnaire.Question{
			{
				ID:       "confirm",
				Type:     "boolean",
				Prompt:   "Confirmar exclusão?",
				Required: true,
			},
		},
	})
	if err != nil {
		return err
	}
	if resp.Cancelled {
		return fmt.Errorf("exclusão cancelada pelo usuário")
	}
	confirmed, ok := resp.Answers["confirm"].(bool)
	if !ok || !confirmed {
		return fmt.Errorf("exclusão cancelada pelo usuário")
	}

	// Prossegue com a exclusão
	authCtx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	msg, err := database.GetMessageWithContext(authCtx, messageID)
	if err != nil {
		return err
	}
	convID := msg.ConversationID

	if err := database.DeleteMessageWithContext(authCtx, messageID); err != nil {
		return err
	}

	a.emitter.Emit("message:deleted", ports.MessageDeletedEvent{
		ConversationID: convID,
		MessageID:      messageID,
	})

	return nil
}

// UpdateMessage atualiza o conteúdo de uma mensagem existente
func (a *App) UpdateMessage(messageID string, newContent string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
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

	a.emitter.Emit("message:updated", ports.MessageUpdatedEvent{
		ConversationID: msg.ConversationID,
		MessageID:      messageID,
		Content:        newContent,
	})

	return nil
}

func (a *App) UpdateConversationModel(id string, model string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return database.UpdateConversationWithContext(ctx, id, "", model)
}

// ==================== ChatMessage ====================

func (a *App) CreateMessage(conversationID string, role, content string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

func (a *App) AddMessage(conversationID string, role, content string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content})
}

func (a *App) AddMessageWithMedia(conversationID string, role, content, media string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content, Media: media})
}

func (a *App) AddMessageWithTokens(conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

func (a *App) AddMessageWithTokensAndMedia(conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
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

func (a *App) AddChildMessage(conversationID string, parentID string, role, content, model string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		Model:          model,
	})
}

func (a *App) GetAllTokenStats() (map[string]int, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetAllTokenStatsWithContext(ctx)
}

// ==================== Rolling Context (Summary) ====================

type ConversationSummaryInfo struct {
	Summary               string `json:"summary"`
	SummaryUpToMessageID  string `json:"summary_up_to_message_id"`
	SummarizingInProgress bool   `json:"summarizing_in_progress"`
}

func (a *App) GetConversationSummary(conversationID string) (*ConversationSummaryInfo, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	summary, upToID, err := database.GetConversationSummaryWithContext(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	inProgress, _ := database.IsSummarizingInProgressWithContext(ctx, conversationID)
	return &ConversationSummaryInfo{
		Summary:               summary,
		SummaryUpToMessageID:  upToID,
		SummarizingInProgress: inProgress,
	}, nil
}

func (a *App) RenameConversation(conversationID string, newTitle string) error {
	return a.UpdateConversation(conversationID, newTitle, "")
}

func (a *App) ClearConversation(conversationID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	if err := database.DeleteAllMessagesWithContext(ctx, conversationID); err != nil {
		return err
	}

	a.emitter.Emit("conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})

	return nil
}

func (a *App) DeleteMessages(conversationID string, messageIDs []string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
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

// ==================== Search ====================

// MessageSearchResult re-exporta o tipo do database
type MessageSearchResult = database.MessageSearchResult

// SearchConversationHistory busca no conteúdo de todas as mensagens usando FTS5.
// Suporta palavras, "frases exatas", prefixo*, operadores OR/AND/NOT.
func (a *App) SearchConversationHistory(query string, limit int) ([]MessageSearchResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.SearchMessageContentWithContext(ctx, query, limit)
}

// RebuildSearchIndex reconstrói o índice de busca full-text.
//
// Operação instance-wide (afeta o índice de todos os usuários), mas o
// binding Wails exige autenticação: AEP-0052 mantém o invariante de que
// nenhum entry point publico ao frontend roda sem sessão ativa, mesmo
// quando a operação interna nao tem escopo de usuário (defesa em
// profundidade contra disparos pré-login).
func (a *App) RebuildSearchIndex() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return database.RebuildFTSIndex(ctx)
}

// ==================== Model ====================

// SetConversationModel define o modelo para uma conversa
func (a *App) SetConversationModel(conversationID string, model string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return database.UpdateConversationWithContext(ctx, conversationID, "", model)
}

// GetEffectiveModel retorna o modelo efetivo a partir do perfil ativo.
// Não há fallback para config.json legado (AEP-0074): o modelo vem do perfil.
func (a *App) GetEffectiveModel() (string, error) {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil {
		return "", err
	}
	if activeProfile == nil {
		return "", nil
	}
	return activeProfile.Chat.Model, nil
}
