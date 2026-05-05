package controllers

import (
	"fmt"
	"log"

	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/profiles"
)

// LLMSettings contém configurações da API LLM (DTO para o frontend).
type LLMSettings struct {
	APIKey  string
	BaseURL string
}

// TokensControllerConfig agrupa dependências do TokensController.
type TokensControllerConfig struct {
	ProfileMgr  *profiles.Manager
	TokenSvc    *chat.TokenService
	SettingsSvc *config.SettingsService
}

// TokensController expõe operações de contagem e estatísticas de tokens.
type TokensController struct {
	profileMgr  *profiles.Manager
	tokenSvc    *chat.TokenService
	settingsSvc *config.SettingsService
}

// NewTokensController cria um TokensController com as dependências fornecidas.
func NewTokensController(cfg TokensControllerConfig) *TokensController {
	return &TokensController{
		profileMgr:  cfg.ProfileMgr,
		tokenSvc:    cfg.TokenSvc,
		settingsSvc: cfg.SettingsSvc,
	}
}

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa.
func (c *TokensController) GetConversationTokenStats(conversationID uint) (*chat.TokenStats, error) {
	contextLimit := 0
	if profile, err := c.profileMgr.GetActive(); err == nil && profile != nil {
		contextLimit = profile.Chat.ContextWindow
	}
	return c.tokenSvc.GetConversationStats(conversationID, contextLimit)
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico.
func (c *TokensController) GetTurnTokenStats(conversationID uint, turnID uint) (*chat.TokenStats, error) {
	return c.tokenSvc.GetTurnStats(conversationID, turnID)
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes.
func (c *TokensController) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error) {
	return c.tokenSvc.GetRecentTokenCount(conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto.
func (c *TokensController) CheckContextWindowThreshold(conversationID uint, threshold float64) (bool, float64, error) {
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
	above, percentage, err := c.tokenSvc.CheckContextThreshold(conversationID, contextLimit, threshold)
	if err != nil {
		return false, 0, err
	}

	log.Printf("[TokenStats] Conversa %d: %.1f%% de %d tokens", conversationID, percentage, contextLimit)
	return above, percentage, nil
}

// GetLLMSettings retorna as configurações atuais da API LLM.
func (c *TokensController) GetLLMSettings() (*LLMSettings, error) {
	cfg, err := c.settingsSvc.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar config: %w", err)
	}
	return &LLMSettings{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.APIBaseURL,
	}, nil
}
