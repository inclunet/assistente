package main

import (
	"fmt"

	"assistente/internal/chat"
	"assistente/internal/config"
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

// ==================== Conversation ====================

func (a *App) CreateConversation(title, model string) (*Conversation, error) {
	return database.CreateConversation(title, model)
}

func (a *App) GetConversations() ([]Conversation, error) {
	return database.GetConversations()
}

func (a *App) GetConversation(id uint) (*Conversation, error) {
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
func (a *App) GetMessages(conversationID uint, parentID *uint) ([]chat.MessageNode, error) {
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

	return chat.BuildMessageNodes(messages, childCounts, parentID), nil
}

// GetConversationInfo retorna apenas metadados da conversa (sem mensagens)
func (a *App) GetConversationInfo(id uint) (*Conversation, error) {
	return database.GetConversationInfo(id)
}

// GetConversationWithThreads retorna conversa com mensagens raiz (lazy loading)
func (a *App) GetConversationWithThreads(id uint) (*chat.ConversationWithThreads, error) {
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
func (a *App) GetMessageChildren(messageID uint) ([]chat.MessageNode, error) {
	return a.GetMessages(0, &messageID)
}

func (a *App) UpdateConversation(id uint, title, model string) error {
	if err := database.UpdateConversation(id, title, model); err != nil {
		return err
	}

	if title != "" {
		a.emitter.Emit("conversation:renamed", map[string]interface{}{
			"conversation_id": id,
			"new_title":       title,
		})
	}

	return nil
}

func (a *App) DeleteConversation(id uint) error {
	if err := database.DeleteConversation(id); err != nil {
		return err
	}

	a.emitter.Emit("conversation:deleted", map[string]interface{}{
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

	a.emitter.Emit("message:deleted", map[string]interface{}{
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

	a.emitter.Emit("message:updated", map[string]interface{}{
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

// ==================== Rolling Context (Summary) ====================

type ConversationSummaryInfo struct {
	Summary              string `json:"summary"`
	SummaryUpToMessageID uint   `json:"summary_up_to_message_id"`
	SummarizingInProgress bool  `json:"summarizing_in_progress"`
}

func (a *App) GetConversationSummary(conversationID uint) (*ConversationSummaryInfo, error) {
	summary, upToID, err := database.GetConversationSummary(conversationID)
	if err != nil {
		return nil, err
	}
	inProgress, _ := database.IsSummarizingInProgress(conversationID)
	return &ConversationSummaryInfo{
		Summary:              summary,
		SummaryUpToMessageID: upToID,
		SummarizingInProgress: inProgress,
	}, nil
}

func (a *App) RenameConversation(conversationID uint, newTitle string) error {
	return a.UpdateConversation(conversationID, newTitle, "")
}

func (a *App) ClearConversation(conversationID uint) error {
	if err := database.DeleteAllMessages(conversationID); err != nil {
		return err
	}

	a.emitter.Emit("conversation:cleared", map[string]interface{}{
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


