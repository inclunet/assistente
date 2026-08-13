package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/chat"
	"context"
	"sync"
)

// Tokens é o bind Wails do domínio tokens (AEP-0088 piloto).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Tokens struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.TokensController
}

// NewTokens cria o bind vazio; AttachTokens preenche session + controller no startup.
func NewTokens() *Tokens {
	return &Tokens{}
}

// AttachTokens associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachTokens(t *Tokens, session Session, ctrl *controllers.TokensController) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.session = session
	t.ctrl = ctrl
}

func (t *Tokens) deps() (Session, *controllers.TokensController, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.session == nil || t.ctrl == nil {
		return nil, nil, ErrTokensNotWired
	}
	return t.session, t.ctrl, nil
}

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa.
func (t *Tokens) GetConversationTokenStats(conversationID string) (*chat.TokenStats, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*chat.TokenStats, error) {
		return ctrl.GetConversationTokenStats(ctx, conversationID)
	})
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico.
func (t *Tokens) GetTurnTokenStats(conversationID string, turnID string) (*chat.TokenStats, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*chat.TokenStats, error) {
		return ctrl.GetTurnTokenStats(ctx, conversationID, turnID)
	})
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes.
func (t *Tokens) GetRecentMessagesTokenCount(conversationID string, messageLimit int) (int, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return 0, err
	}
	return WithUser(session, func(ctx context.Context) (int, error) {
		return ctrl.GetRecentMessagesTokenCount(ctx, conversationID, messageLimit)
	})
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto.
func (t *Tokens) CheckContextWindowThreshold(conversationID string, threshold float64) (bool, float64, error) {
	session, ctrl, err := t.deps()
	if err != nil {
		return false, 0, err
	}
	return WithUser2(session, func(ctx context.Context) (bool, float64, error) {
		return ctrl.CheckContextWindowThreshold(ctx, conversationID, threshold)
	})
}
