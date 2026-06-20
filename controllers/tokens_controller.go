package controllers

import (
	"context"
	"fmt"
	"log"

	"assistente/internal/chat"
	"assistente/internal/profiles"
)

// TokensControllerConfig agrupa dependências do TokensController.
type TokensControllerConfig struct {
	ProfileMgr *profiles.Manager
	TokenSvc   *chat.TokenService
}

// TokensController expõe operações de contagem e estatísticas de tokens.
type TokensController struct {
	profileMgr *profiles.Manager
	tokenSvc   *chat.TokenService
}

// NewTokensController cria um TokensController com as dependências fornecidas.
func NewTokensController(cfg TokensControllerConfig) *TokensController {
	return &TokensController{
		profileMgr: cfg.ProfileMgr,
		tokenSvc:   cfg.TokenSvc,
	}
}

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa.
func (c *TokensController) GetConversationTokenStats(ctx context.Context, conversationID string) (*chat.TokenStats, error) {
	contextLimit := 0
	if profile, err := c.profileMgr.GetActive(); err == nil && profile != nil {
		contextLimit = profile.Chat.ContextWindow
	}
	return c.tokenSvc.GetConversationStats(ctx, conversationID, contextLimit)
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico.
func (c *TokensController) GetTurnTokenStats(ctx context.Context, conversationID string, turnID string) (*chat.TokenStats, error) {
	return c.tokenSvc.GetTurnStats(ctx, conversationID, turnID)
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes.
func (c *TokensController) GetRecentMessagesTokenCount(ctx context.Context, conversationID string, messageLimit int) (int, error) {
	return c.tokenSvc.GetRecentTokenCount(ctx, conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto.
func (c *TokensController) CheckContextWindowThreshold(ctx context.Context, conversationID string, threshold float64) (bool, float64, error) {
	if threshold <= 0 {
		threshold = 80.0
	}

	profile, err := c.profileMgr.GetActive()
	if err != nil {
		return false, 0, fmt.Errorf("erro ao obter perfil ativo: %w", err)
	}
	if profile == nil || profile.Chat.ContextWindow <= 0 {
		return false, 0, fmt.Errorf("limite de contexto não configurado no perfil")
	}

	contextLimit := profile.Chat.ContextWindow
	above, percentage, err := c.tokenSvc.CheckContextThreshold(ctx, conversationID, contextLimit, threshold)
	if err != nil {
		return false, 0, err
	}

	log.Printf("[TokenStats] Conversa %s: %.1f%% de %d tokens", conversationID, percentage, contextLimit)
	return above, percentage, nil
}
