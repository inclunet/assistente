package app

import (
	"fmt"
	"sort"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
)

// Re-exporta tipos do pacote database para manter compatibilidade
type Conversation = database.Conversation
type ChatMessage = database.ChatMessage

// Re-exporta funções que não dependem de App
var (
	InitDatabase  = database.Init
	GenerateTitle = database.GenerateTitle
)

const maxExpandedMessageWindowRows = 240

// ==================== Conversation ====================

func (a *App) CreateConversation(title, model string) (*Conversation, error) {
	return database.CreateConversation(title, model)
}

func (a *App) GetConversations() ([]Conversation, error) {
	return database.GetConversations()
}

func (a *App) GetConversation(id string) (*Conversation, error) {
	return database.GetConversation(id)
}

// EnsureConversation cria ou recicla uma conversa vazia e retorna.
// Usada pelo frontend quando uma aba de chat do workspace é criada sem contentId.
func (a *App) EnsureConversation(title string) (*Conversation, error) {
	if title == "" {
		title = "Nova Conversa"
	}
	conv, err := database.RecycleOrCreateConversation(title)
	if err != nil {
		return nil, fmt.Errorf("erro ao garantir conversa: %w", err)
	}
	return conv, nil
}

// GetMessages retorna mensagens com filtro por parent (API unificada com LAZY LOADING)
func (a *App) GetMessages(conversationID string, parentID *string) ([]chat.MessageNode, error) {
	messages, err := database.GetMessages(conversationID, parentID)
	if err != nil {
		return nil, err
	}

	return buildMessageNodes(messages, parentID), nil
}

func buildMessageNodes(messages []database.ChatMessage, parentID *string) []chat.MessageNode {
	msgIDs := make([]string, len(messages))
	for i, msg := range messages {
		msgIDs[i] = msg.ID
	}
	childCounts, err := database.CountChildren(msgIDs)
	if err != nil {
		childCounts = make(map[string]int)
	}
	return chat.BuildMessageNodes(messages, childCounts, parentID)
}

func assignMessageNodeOriginalIndexes(nodes []chat.MessageNode, indexesByID map[string]int) []chat.MessageNode {
	if len(indexesByID) == 0 {
		return nodes
	}
	for i := range nodes {
		if index, ok := indexesByID[nodes[i].Message.ID]; ok {
			value := index
			nodes[i].OriginalIndex = &value
		}
		if len(nodes[i].Children) > 0 {
			nodes[i].Children = assignMessageNodeOriginalIndexes(nodes[i].Children, indexesByID)
		}
	}
	return nodes
}

func expandWindowTurnMessages(conversationID string, parentID *string, messages []database.ChatMessage, maxRows int) ([]database.ChatMessage, error) {
	turnIDs := make([]string, 0)
	seenTurns := make(map[string]bool)
	addBoundaryTurn := func(message database.ChatMessage) {
		if message.TurnID == nil || *message.TurnID == "" || seenTurns[*message.TurnID] {
			return
		}
		seenTurns[*message.TurnID] = true
		turnIDs = append(turnIDs, *message.TurnID)
	}
	if len(messages) > 0 {
		addBoundaryTurn(messages[0])
		addBoundaryTurn(messages[len(messages)-1])
	}
	if len(turnIDs) == 0 {
		return messages, nil
	}
	if maxRows <= len(messages) {
		return messages, nil
	}
	byID := make(map[string]database.ChatMessage, maxRows)
	for _, message := range messages {
		byID[message.ID] = message
	}
	for _, turnID := range turnIDs {
		turnMessages, err := database.GetMessagesByTurnID(conversationID, parentID, turnID, maxRows+1)
		if err != nil {
			return nil, err
		}
		missingCount := 0
		for _, message := range turnMessages {
			if _, ok := byID[message.ID]; !ok {
				missingCount++
			}
		}
		if len(byID)+missingCount <= maxRows {
			for _, message := range turnMessages {
				byID[message.ID] = message
			}
			continue
		}
		for _, message := range turnMessages {
			if len(byID) >= maxRows {
				break
			}
			if message.Role == "tool" {
				continue
			}
			byID[message.ID] = message
		}
	}
	expanded := make([]database.ChatMessage, 0, len(byID))
	for _, message := range byID {
		expanded = append(expanded, message)
	}
	sort.Slice(expanded, func(i, j int) bool {
		if expanded[i].CreatedAt.Equal(expanded[j].CreatedAt) {
			return expanded[i].ID < expanded[j].ID
		}
		return expanded[i].CreatedAt.Before(expanded[j].CreatedAt)
	})
	return expanded, nil
}

// GetRecentMessages retorna as mensagens raiz mais recentes de uma conversa.
func (a *App) GetRecentMessages(conversationID string, limit int) ([]chat.MessageNode, error) {
	messages, err := database.GetRecentRootMessages(conversationID, limit)
	if err != nil {
		return nil, err
	}
	return buildMessageNodes(messages, nil), nil
}

// GetMessagesBefore retorna mensagens raiz anteriores ao cursor informado.
func (a *App) GetMessagesBefore(conversationID string, beforeID string, limit int) ([]chat.MessageNode, error) {
	messages, err := database.GetRootMessagesBefore(conversationID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	return buildMessageNodes(messages, nil), nil
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
	if limit > maxExpandedMessageWindowRows {
		limit = maxExpandedMessageWindowRows
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

	var parentID *string
	threadParentID := ""
	if scope == chat.MessageWindowScopeThread {
		threadParentID = strings.TrimSpace(req.ThreadParentID)
		if threadParentID == "" {
			return nil, fmt.Errorf("threadParentId é obrigatório para scope=thread")
		}
		parentMessage, err := database.GetMessage(threadParentID)
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

	window, err := database.GetMessageWindow(database.MessageWindowQuery{
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
	originalIndexesByMessageID := make(map[string]int, len(window.Messages))
	for index, message := range window.Messages {
		originalIndexesByMessageID[message.ID] = window.StartIndex + index
	}
	messages, err := expandWindowTurnMessages(conversationID, parentID, window.Messages, maxExpandedMessageWindowRows)
	if err != nil {
		return nil, err
	}
	nodes := assignMessageNodeOriginalIndexes(buildMessageNodes(messages, parentID), originalIndexesByMessageID)

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
	return database.GetConversationInfo(id)
}

// GetConversationWithThreads retorna conversa com mensagens raiz (lazy loading)
func (a *App) GetConversationWithThreads(id string) (*chat.ConversationWithThreads, error) {
	conv, err := database.GetConversationInfo(id)
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
	return a.GetMessages("", &messageID)
}

func (a *App) UpdateConversation(id string, title, model string) error {
	if err := database.UpdateConversation(id, title, model); err != nil {
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
	if err := database.DeleteConversation(id); err != nil {
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
	var msg database.ChatMessage
	if err := database.DB().First(&msg, "id = ?", messageID).Error; err != nil {
		return err
	}
	convID := msg.ConversationID

	if err := database.DeleteMessage(messageID); err != nil {
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
	var msg database.ChatMessage
	if err := database.DB().Select("conversation_id").First(&msg, "id = ?", messageID).Error; err != nil {
		return err
	}

	if err := database.UpdateMessageContent(
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
	return database.UpdateConversation(id, "", model)
}

// ==================== ChatMessage ====================

func (a *App) CreateMessage(conversationID string, role, content string) (*ChatMessage, error) {
	return database.CreateMessage(database.MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

func (a *App) AddMessage(conversationID string, role, content string) (*ChatMessage, error) {
	return database.AddMessage(conversationID, role, content)
}

func (a *App) AddMessageWithMedia(conversationID string, role, content, media string) (*ChatMessage, error) {
	return database.AddMessageWithMedia(conversationID, role, content, media)
}

func (a *App) AddMessageWithTokens(conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokens(conversationID, role, content, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) AddMessageWithTokensAndMedia(conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokensAndMedia(conversationID, role, content, media, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) AddChildMessage(conversationID string, parentID string, role, content, model string) (*ChatMessage, error) {
	return database.AddChildMessage(conversationID, parentID, role, content, model)
}

func (a *App) GetAllTokenStats() (map[string]int, error) {
	return database.GetAllTokenStats()
}

// ==================== Rolling Context (Summary) ====================

type ConversationSummaryInfo struct {
	Summary               string `json:"summary"`
	SummaryUpToMessageID  string `json:"summary_up_to_message_id"`
	SummarizingInProgress bool   `json:"summarizing_in_progress"`
}

func (a *App) GetConversationSummary(conversationID string) (*ConversationSummaryInfo, error) {
	summary, upToID, err := database.GetConversationSummary(conversationID)
	if err != nil {
		return nil, err
	}
	inProgress, _ := database.IsSummarizingInProgress(conversationID)
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
	if err := database.DeleteAllMessages(conversationID); err != nil {
		return err
	}

	a.emitter.Emit("conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})

	return nil
}

func (a *App) DeleteMessages(conversationID string, messageIDs []string) error {
	for _, msgID := range messageIDs {
		if err := database.DeleteMessage(msgID); err != nil {
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
	return database.SearchMessageContent(query, limit)
}

// RebuildSearchIndex reconstrói o índice de busca full-text.
func (a *App) RebuildSearchIndex() error {
	return database.RebuildFTSIndex()
}

// ==================== Model ====================

// SetConversationModel define o modelo para uma conversa
func (a *App) SetConversationModel(conversationID string, model string) error {
	return database.UpdateConversation(conversationID, "", model)
}

// GetEffectiveModel retorna o modelo efetivo (perfil ativo > config padrão)
func (a *App) GetEffectiveModel() (string, error) {
	// Tenta obter do perfil ativo
	activeProfile, err := a.profileManager.GetActive()
	if err == nil && activeProfile != nil && activeProfile.Chat.Model != "" {
		return activeProfile.Chat.Model, nil
	}

	// Fallback para config
	cfg, err := a.settingsSvc.GetConfig()
	if err != nil {
		return "", err
	}
	return cfg.DefaultModel, nil
}
