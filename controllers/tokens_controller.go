package controllers

import (
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/logging"
	"assistente/internal/profiles"
	"context"
	"fmt"
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
func (c *TokensController) GetConversationTokenStats(ctx context.Context, conversationID string) (*apidto.TokenStats, error) {
	contextLimit := 0
	var promptCacheEnabled *bool
	if profile, err := c.profileMgr.GetActive(); err == nil && profile != nil {
		contextLimit = profile.Chat.ContextWindow
		enabled := profile.Chat.PromptCache.Enabled
		promptCacheEnabled = &enabled
	}
	stats, err := c.tokenSvc.GetConversationStats(ctx, conversationID, contextLimit)
	if err != nil {
		return nil, err
	}
	applyPromptCacheProfileState(stats, promptCacheEnabled)
	return stats, nil
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico.
func (c *TokensController) GetTurnTokenStats(ctx context.Context, conversationID string, turnID string) (*apidto.TokenStats, error) {
	stats, err := c.tokenSvc.GetTurnStats(ctx, conversationID, turnID)
	if err != nil {
		return nil, err
	}
	var promptCacheEnabled *bool
	if profile, err := c.profileMgr.GetActive(); err == nil && profile != nil {
		enabled := profile.Chat.PromptCache.Enabled
		promptCacheEnabled = &enabled
	}
	applyPromptCacheProfileState(stats, promptCacheEnabled)
	return stats, nil
}

func applyPromptCacheProfileState(stats *apidto.TokenStats, promptCacheEnabled *bool) {
	if stats == nil {
		return
	}
	stats.PromptCacheEnabled = promptCacheEnabled
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

	logging.Infof(ctx, "controllers.tokens-controller", "[TokenStats] Conversa %s: %.1f%% de %d tokens", conversationID, percentage, contextLimit)
	return above, percentage, nil
}
