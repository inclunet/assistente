package main

import (
	"fmt"
	"log"
	"sort"

	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/questionnaire"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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

// enrichMessage converte ChatMessage para EnrichedMessage com campos calculados
func (a *App) enrichMessage(msg database.ChatMessage) EnrichedMessage {
	// Converte ParentID *uint para *string
	var parentIDStr *string
	if msg.ParentID != nil {
		pidStr := fmt.Sprintf("%d", *msg.ParentID)
		parentIDStr = &pidStr
	}

	enriched := EnrichedMessage{
		// Campos do ChatMessage
		ID:               fmt.Sprintf("%d", msg.ID),
		ConversationID:   msg.ConversationID,
		ParentID:         parentIDStr,
		TurnID:           msg.TurnID,
		Role:             msg.Role,
		Content:          msg.Content,
		Reasoning:        msg.Reasoning,
		Media:            msg.Media,
		ToolCalls:        msg.ToolCalls,
		ToolCallID:       msg.ToolCallID,
		PromptTokens:     msg.PromptTokens,
		CompletionTokens: msg.CompletionTokens,
		TotalTokens:      msg.TotalTokens,
		Model:            msg.Model,
		Source:           msg.Source,
		CreatedAt:        msg.CreatedAt,
		// Campos derivados
		Timestamp:   msg.CreatedAt.UnixMilli(),
		IsStreaming: false,
		Internal:    msg.ParentID != nil || msg.Role == "tool",
	}

	return enriched
}

func (a *App) CreateConversation(title, model string) (*Conversation, error) {
	return database.CreateConversation(title, model)
}

func (a *App) GetConversations() ([]Conversation, error) {
	return database.GetConversations()
}

func (a *App) GetConversation(id uint) (*Conversation, error) {
	return database.GetConversation(id)
}

// GetMessages retorna mensagens com filtro por parent (API unificada com LAZY LOADING)
func (a *App) GetMessages(conversationID uint, parentID *uint) ([]MessageNode, error) {
	messages, err := database.GetMessages(conversationID, parentID)
	if err != nil {
		return nil, err
	}

	msgIDs := make([]uint, len(messages))
	for i, msg := range messages {
		msgIDs[i] = msg.ID
	}

	childCounts, err := database.CountChildren(msgIDs)
	if err != nil {
		childCounts = make(map[uint]int)
	}

	level := 0
	if parentID != nil {
		level = 1
	}

	result := make([]MessageNode, 0, len(messages))
	for _, msg := range messages {
		childCount := childCounts[msg.ID]
		node := MessageNode{
			Message:    a.enrichMessage(msg),
			Children:   nil,
			Level:      level,
			ChildCount: childCount,
		}
		result = append(result, node)
	}

	return result, nil
}

// GetConversationInfo retorna apenas metadados da conversa (sem mensagens)
func (a *App) GetConversationInfo(id uint) (*Conversation, error) {
	return database.GetConversationInfo(id)
}

// GetConversationWithThreads retorna conversa com mensagens raiz (lazy loading)
func (a *App) GetConversationWithThreads(id uint) (*ConversationWithThreads, error) {
	conv, err := database.GetConversationInfo(id)
	if err != nil {
		return nil, err
	}

	threads, err := a.GetMessages(id, nil)
	if err != nil {
		return nil, err
	}

	return &ConversationWithThreads{
		ID:      conv.ID,
		Title:   conv.Title,
		Threads: threads,
	}, nil
}

// GetMessageChildren retorna os filhos de uma mensagem (lazy loading)
func (a *App) GetMessageChildren(messageID uint) ([]MessageNode, error) {
	return a.GetMessages(0, &messageID)
}

// buildMessageTree organiza mensagens planas em uma árvore hierárquica
func (a *App) buildMessageTree(messages []database.ChatMessage) []MessageNode {
	fmt.Printf("[TREE] Construindo árvore com %d mensagens\n", len(messages))

	childrenMap := make(map[uint][]database.ChatMessage)
	var rootMessages []database.ChatMessage

	for _, msg := range messages {
		if msg.ParentID == nil {
			rootMessages = append(rootMessages, msg)
		} else {
			childrenMap[*msg.ParentID] = append(childrenMap[*msg.ParentID], msg)
		}
	}

	sort.Slice(rootMessages, func(i, j int) bool {
		return rootMessages[i].ID < rootMessages[j].ID
	})
	for parentID := range childrenMap {
		sort.Slice(childrenMap[parentID], func(i, j int) bool {
			return childrenMap[parentID][i].ID < childrenMap[parentID][j].ID
		})
	}

	var buildNode func(msg database.ChatMessage, level int) MessageNode
	buildNode = func(msg database.ChatMessage, level int) MessageNode {
		node := MessageNode{
			Message:  a.enrichMessage(msg),
			Children: []MessageNode{},
			Level:    level,
		}

		children := childrenMap[msg.ID]
		node.ChildCount = len(children)

		for _, child := range children {
			childNode := buildNode(child, level+1)
			node.Children = append(node.Children, childNode)
		}

		return node
	}

	result := make([]MessageNode, 0, len(rootMessages))
	for _, rootMsg := range rootMessages {
		node := buildNode(rootMsg, 0)
		result = append(result, node)
	}

	fmt.Printf("[TREE] Resultado: %d raízes\n", len(result))

	return result
}

func (a *App) UpdateConversation(id uint, title, model string) error {
	if err := database.UpdateConversation(id, title, model); err != nil {
		return err
	}

	if title != "" {
		runtime.EventsEmit(a.ctx, "conversation:renamed", map[string]interface{}{
			"conversation_id": id,
			"new_title":       title,
		})
	}

	return nil
}

func (a *App) DeleteConversation(id uint) error {
	fmt.Printf("[DeleteConversation] Iniciando deleção da conversa %d...\n", id)

	tabs, err := database.GetAllTabs()
	if err == nil {
		for _, tab := range tabs {
			if tab.ConversationID != nil && *tab.ConversationID == id {
				fmt.Printf("[DeleteConversation] Limpando tab %d que referencia conversa %d\n", tab.ID, id)
				database.ClearTab(tab.ID)
			}
		}
	}

	if err := database.DeleteConversation(id); err != nil {
		fmt.Printf("[DeleteConversation] ERRO ao deletar: %v\n", err)
		return err
	}

	runtime.EventsEmit(a.ctx, "conversation:deleted", map[string]interface{}{
		"conversation_id": id,
	})

	return nil
}

// DeleteMessage exclui uma mensagem e todas as suas filhas (respostas)
func (a *App) DeleteMessage(messageID uint) error {
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
	if err := database.DeleteMessage(messageID); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "message:deleted", map[string]interface{}{
		"message_id": messageID,
	})

	return nil
}

// UpdateMessage atualiza o conteúdo de uma mensagem existente
func (a *App) UpdateMessage(messageID uint, newContent string) error {
	if err := database.UpdateMessageContent(
		messageID,
		newContent,
		0, 0, 0, "",
	); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "message:updated", map[string]interface{}{
		"message_id": messageID,
		"content":    newContent,
	})

	return nil
}

func (a *App) UpdateConversationModel(id uint, model string) error {
	return database.UpdateConversation(id, "", model)
}

// ==================== ChatMessage ====================

func (a *App) CreateMessage(conversationID uint, role, content string) (*ChatMessage, error) {
	return database.CreateMessage(database.MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

func (a *App) AddMessage(conversationID uint, role, content string) (*ChatMessage, error) {
	return database.AddMessage(conversationID, role, content)
}

func (a *App) AddMessageWithMedia(conversationID uint, role, content, media string) (*ChatMessage, error) {
	return database.AddMessageWithMedia(conversationID, role, content, media)
}

func (a *App) AddMessageWithTokens(conversationID uint, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokens(conversationID, role, content, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) AddMessageWithTokensAndMedia(conversationID uint, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return database.AddMessageWithTokensAndMedia(conversationID, role, content, media, promptTokens, completionTokens, totalTokens, model)
}

func (a *App) AddChildMessage(conversationID uint, parentID uint, role, content, model string) (*ChatMessage, error) {
	return database.AddChildMessage(conversationID, parentID, role, content, model)
}

func (a *App) GetAllTokenStats() (map[string]int, error) {
	return database.GetAllTokenStats()
}

// ==================== Chat Tabs ====================

type TabsResponse struct {
	Tabs        []database.ChatTab `json:"tabs"`
	ActiveTabId uint               `json:"active_tab_id"`
}

func (a *App) GetTabs() (TabsResponse, error) {
	tabs, err := database.GetAllTabs()
	if err != nil {
		return TabsResponse{}, err
	}

	if len(tabs) == 0 {
		if err := database.InitializeDefaultTab(); err != nil {
			return TabsResponse{}, err
		}
		tabs, err = database.GetAllTabs()
		if err != nil {
			return TabsResponse{}, err
		}
	}

	var activeId uint
	for _, tab := range tabs {
		if tab.IsActive {
			activeId = tab.ID
			break
		}
	}

	return TabsResponse{
		Tabs:        tabs,
		ActiveTabId: activeId,
	}, nil
}

func (a *App) SwitchToTab(tabID uint) error {
	return database.SetActiveTab(tabID)
}

func (a *App) OpenConversationInNewTab(conversationID uint) (uint, error) {
	conv, err := database.GetConversation(conversationID)
	if err != nil {
		return 0, fmt.Errorf("conversa não encontrada: %w", err)
	}

	tab, err := database.CreateTab(conv.Title, "💬", true)
	if err != nil {
		return 0, fmt.Errorf("erro ao criar aba: %w", err)
	}

	if err := database.LoadConversationInTab(tab.ID, conversationID); err != nil {
		return 0, fmt.Errorf("erro ao carregar conversa: %w", err)
	}

	return tab.ID, nil
}

func (a *App) OpenConversationInCurrentTab(conversationID uint) error {
	_, err := database.GetConversation(conversationID)
	if err != nil {
		return fmt.Errorf("conversa não encontrada: %w", err)
	}

	tabs, err := database.GetAllTabs()
	if err != nil {
		return fmt.Errorf("erro ao obter abas: %w", err)
	}

	var activeTabID uint
	for _, tab := range tabs {
		if tab.IsActive {
			activeTabID = tab.ID
			break
		}
	}

	if activeTabID == 0 {
		return fmt.Errorf("nenhuma aba ativa encontrada")
	}

	return database.LoadConversationInTab(activeTabID, conversationID)
}

func (a *App) RenameTab(tabID uint, newTitle string) error {
	return a.UpdateTabTitle(tabID, newTitle)
}

func (a *App) GetCurrentTabID() (uint, error) {
	tabs, err := database.GetAllTabs()
	if err != nil {
		return 0, err
	}

	for _, tab := range tabs {
		if tab.IsActive {
			return tab.ID, nil
		}
	}

	return 0, fmt.Errorf("nenhuma aba ativa encontrada")
}

func (a *App) GetCurrentConversationID() (uint, error) {
	tabs, err := database.GetAllTabs()
	if err != nil {
		return 0, err
	}

	for _, tab := range tabs {
		if tab.IsActive {
			if tab.ConversationID != nil && *tab.ConversationID > 0 {
				return *tab.ConversationID, nil
			}
			return 0, fmt.Errorf("aba ativa não tem conversa associada")
		}
	}

	return 0, fmt.Errorf("nenhuma aba ativa encontrada")
}

func (a *App) CreateNewConversation(title string) (uint, error) {
	conv, err := database.CreateConversation(title, "gpt-4o-mini")
	if err != nil {
		return 0, fmt.Errorf("erro ao criar conversa: %w", err)
	}

	tab, err := database.CreateTab(title, "💬", true)
	if err != nil {
		return 0, fmt.Errorf("erro ao criar aba: %w", err)
	}

	if err := database.LoadConversationInTab(tab.ID, conv.ID); err != nil {
		return 0, fmt.Errorf("erro ao carregar conversa: %w", err)
	}

	return conv.ID, nil
}

func (a *App) RenameConversation(conversationID uint, newTitle string) error {
	return a.UpdateConversation(conversationID, newTitle, "")
}

func (a *App) ClearConversation(conversationID uint) error {
	if err := database.DeleteAllMessages(conversationID); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})

	return nil
}

func (a *App) DeleteMessages(conversationID uint, messageIDs []uint) error {
	for _, msgID := range messageIDs {
		if err := database.DeleteMessage(msgID); err != nil {
			return fmt.Errorf("erro ao deletar mensagem %d: %w", msgID, err)
		}
	}
	return nil
}

// ==================== Model ====================

// SetConversationModel define o modelo para uma conversa
func (a *App) SetConversationModel(conversationID uint, model string) error {
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
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.DefaultModel, nil
}

// ==================== Unused but kept for build compat ====================

// Funcao auxiliar que nao referencia mais nada - usada apenas para evitar import nao usado
var _ = log.Printf
