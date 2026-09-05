package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
	"context"
	"sync"
)

// Conversations é o bind Wails do domínio conversations/persistência (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
// Helpers de reset de escopo permanecem no *App; SendMessage/RetryMessage em wailsapi.Chat.
type Conversations struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.ConversationsController
}

// NewConversations cria o bind vazio; AttachConversations preenche deps no startup.
func NewConversations() *Conversations {
	return &Conversations{}
}

// AttachConversations associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachConversations(api *Conversations, session Session, ctrl *controllers.ConversationsController) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
}

func (api *Conversations) deps() (Session, *controllers.ConversationsController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, ErrConversationsNotWired
	}
	return api.session, api.ctrl, nil
}

// CreateConversation cria uma conversa.
func (api *Conversations) CreateConversation(title, model string) (*database.Conversation, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.Conversation, error) {
		return ctrl.CreateConversation(ctx, title, model)
	})
}

// GetConversations lista conversas do usuário.
func (api *Conversations) GetConversations() ([]database.Conversation, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.Conversation, error) {
		return ctrl.GetConversations(ctx)
	})
}

// GetConversationsPage lista conversas paginadas.
func (api *Conversations) GetConversationsPage(limit, offset int) (database.ConversationListResult, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return database.ConversationListResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (database.ConversationListResult, error) {
		return ctrl.GetConversationsPage(ctx, limit, offset)
	})
}

// GetConversationsByIDs retorna conversas pelos IDs.
func (api *Conversations) GetConversationsByIDs(ids []string) ([]database.Conversation, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.Conversation, error) {
		return ctrl.GetConversationsByIDs(ctx, ids)
	})
}

// GetConversation retorna uma conversa pelo id.
func (api *Conversations) GetConversation(id string) (*database.Conversation, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.Conversation, error) {
		return ctrl.GetConversation(ctx, id)
	})
}

// EnsureConversation cria ou recicla uma conversa vazia.
func (api *Conversations) EnsureConversation(title string) (*database.Conversation, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.Conversation, error) {
		return ctrl.EnsureConversation(ctx, title)
	})
}

// GetMessages retorna mensagens com filtro por parent.
func (api *Conversations) GetMessages(conversationID string, parentID *string) ([]chat.MessageNode, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]chat.MessageNode, error) {
		return ctrl.GetMessages(ctx, conversationID, parentID)
	})
}

// GetRecentMessages retorna as mensagens raiz mais recentes.
func (api *Conversations) GetRecentMessages(conversationID string, limit int) ([]chat.MessageNode, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]chat.MessageNode, error) {
		return ctrl.GetRecentMessages(ctx, conversationID, limit)
	})
}

// GetMessagesBefore retorna mensagens raiz anteriores ao cursor.
func (api *Conversations) GetMessagesBefore(conversationID string, beforeID string, limit int) ([]chat.MessageNode, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]chat.MessageNode, error) {
		return ctrl.GetMessagesBefore(ctx, conversationID, beforeID, limit)
	})
}

// GetConversationMessageWindow carrega janela incremental de mensagens.
func (api *Conversations) GetConversationMessageWindow(req chat.MessageWindowRequest) (*chat.MessageWindow, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*chat.MessageWindow, error) {
		return ctrl.GetConversationMessageWindow(ctx, req)
	})
}

// GetConversationInfo retorna metadados da conversa.
func (api *Conversations) GetConversationInfo(id string) (*database.Conversation, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.Conversation, error) {
		return ctrl.GetConversationInfo(ctx, id)
	})
}

// GetConversationWithThreads retorna conversa com threads raiz.
func (api *Conversations) GetConversationWithThreads(id string) (*chat.ConversationWithThreads, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*chat.ConversationWithThreads, error) {
		return ctrl.GetConversationWithThreads(ctx, id)
	})
}

// GetMessageChildren retorna filhos de uma mensagem.
func (api *Conversations) GetMessageChildren(messageID string) ([]chat.MessageNode, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]chat.MessageNode, error) {
		return ctrl.GetMessageChildren(ctx, messageID)
	})
}

// UpdateConversation atualiza título/modelo.
func (api *Conversations) UpdateConversation(id string, title, model string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateConversation(ctx, id, title, model)
	})
	return err
}

// DeleteConversation remove a conversa.
func (api *Conversations) DeleteConversation(id string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteConversation(ctx, id)
	})
	return err
}

// DeleteMessage exclui mensagem (com confirmação no controller).
func (api *Conversations) DeleteMessage(messageID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteMessage(ctx, messageID)
	})
	return err
}

// UpdateMessage atualiza conteúdo de mensagem.
func (api *Conversations) UpdateMessage(messageID string, newContent string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateMessage(ctx, messageID, newContent)
	})
	return err
}

// ToggleMessagePin alterna a fixação persistente de uma mensagem.
func (api *Conversations) ToggleMessagePin(messageID string) (*database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.ChatMessage, error) {
		return ctrl.ToggleMessagePin(ctx, messageID)
	})
}

// GetPinnedMessages lista as mensagens fixadas de uma conversa.
func (api *Conversations) GetPinnedMessages(conversationID string) ([]database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.ChatMessage, error) {
		return ctrl.GetPinnedMessages(ctx, conversationID)
	})
}

// UpdateConversationModel altera o modelo da conversa.
func (api *Conversations) UpdateConversationModel(id string, model string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateConversationModel(ctx, id, model)
	})
	return err
}

// CreateMessage cria uma mensagem.
func (api *Conversations) CreateMessage(conversationID string, role, content string) (*database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.ChatMessage, error) {
		return ctrl.CreateMessage(ctx, conversationID, role, content)
	})
}

// AddMessage adiciona uma mensagem.
func (api *Conversations) AddMessage(conversationID string, role, content string) (*database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.ChatMessage, error) {
		return ctrl.AddMessage(ctx, conversationID, role, content)
	})
}

// AddMessageWithMedia adiciona mensagem com mídia.
func (api *Conversations) AddMessageWithMedia(conversationID string, role, content, media string) (*database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.ChatMessage, error) {
		return ctrl.AddMessageWithMedia(ctx, conversationID, role, content, media)
	})
}

// AddMessageWithTokens adiciona mensagem com tokens.
func (api *Conversations) AddMessageWithTokens(conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.ChatMessage, error) {
		return ctrl.AddMessageWithTokens(ctx, conversationID, role, content, promptTokens, completionTokens, totalTokens, model)
	})
}

// AddMessageWithTokensAndMedia adiciona mensagem com tokens e mídia.
func (api *Conversations) AddMessageWithTokensAndMedia(conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.ChatMessage, error) {
		return ctrl.AddMessageWithTokensAndMedia(ctx, conversationID, role, content, media, promptTokens, completionTokens, totalTokens, model)
	})
}

// AddChildMessage adiciona mensagem filha.
func (api *Conversations) AddChildMessage(conversationID string, parentID string, role, content, model string) (*database.ChatMessage, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.ChatMessage, error) {
		return ctrl.AddChildMessage(ctx, conversationID, parentID, role, content, model)
	})
}

// GetAllTokenStats retorna estatísticas agregadas de tokens.
func (api *Conversations) GetAllTokenStats() (map[string]int, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]int, error) {
		return ctrl.GetAllTokenStats(ctx)
	})
}

// GetConversationSummary retorna o resumo rolling.
func (api *Conversations) GetConversationSummary(conversationID string) (*apidto.ConversationSummaryInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.ConversationSummaryInfo, error) {
		return ctrl.GetConversationSummary(ctx, conversationID)
	})
}

// RenameConversation renomeia a conversa.
func (api *Conversations) RenameConversation(conversationID string, newTitle string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RenameConversation(ctx, conversationID, newTitle)
	})
	return err
}

// ClearConversation limpa mensagens da conversa.
func (api *Conversations) ClearConversation(conversationID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ClearConversation(ctx, conversationID)
	})
	return err
}

// DeleteMessages remove mensagens específicas.
func (api *Conversations) DeleteMessages(conversationID string, messageIDs []string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteMessages(ctx, conversationID, messageIDs)
	})
	return err
}

// SearchConversationHistory busca no histórico (FTS5).
func (api *Conversations) SearchConversationHistory(query string, limit int) ([]database.MessageSearchResult, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.MessageSearchResult, error) {
		return ctrl.SearchConversationHistory(ctx, query, limit)
	})
}

// RebuildSearchIndex reconstrói o índice FTS.
func (api *Conversations) RebuildSearchIndex() error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RebuildSearchIndex(ctx)
	})
	return err
}

// SetConversationModel define o modelo da conversa.
func (api *Conversations) SetConversationModel(conversationID string, model string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetConversationModel(ctx, conversationID, model)
	})
	return err
}

// GetEffectiveModel retorna o modelo efetivo do perfil ativo.
func (api *Conversations) GetEffectiveModel() (string, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.GetEffectiveModel(ctx)
	})
}
