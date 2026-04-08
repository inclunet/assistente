package main

import (
	"fmt"
	"log"

	"assistente/internal/chat"
	"assistente/internal/config"
)

// ============================================================================
// Token Stats API
// ============================================================================

// ToolUsageBreakdownResult é alias de chat.ToolUsageBreakdown para compatibilidade com o frontend.
type ToolUsageBreakdownResult = chat.ToolUsageBreakdown

// TokenStatsResult é alias de chat.TokenStats para compatibilidade com o frontend.
type TokenStatsResult = chat.TokenStats

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa.
func (a *App) GetConversationTokenStats(conversationID uint) (*TokenStatsResult, error) {
	contextLimit := 0
	if profile, err := a.profileManager.GetActive(); err == nil && profile != nil {
		contextLimit = profile.Chat.ContextWindow
	}
	return a.tokenSvc.GetConversationStats(conversationID, contextLimit)
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico.
func (a *App) GetTurnTokenStats(conversationID uint, turnID uint) (*TokenStatsResult, error) {
	return a.tokenSvc.GetTurnStats(conversationID, turnID)
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes.
func (a *App) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error) {
	return a.tokenSvc.GetRecentTokenCount(conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto.
// Retorna true e a porcentagem se estiver acima do threshold (padrão 80%).
func (a *App) CheckContextWindowThreshold(conversationID uint, threshold float64) (bool, float64, error) {
	if threshold <= 0 {
		threshold = 80.0
	}

	profile, err := a.profileManager.GetActive()
	if err != nil {
		return false, 0, fmt.Errorf("erro ao obter perfil ativo: %w", err)
	}
	if profile == nil || profile.Chat.ContextWindow <= 0 {
		return false, 0, fmt.Errorf("limite de contexto não configurado no perfil")
	}

	contextLimit := profile.Chat.ContextWindow
	above, percentage, err := a.tokenSvc.CheckContextThreshold(conversationID, contextLimit, threshold)
	if err != nil {
		return false, 0, err
	}

	log.Printf("[TokenStats] Conversa %d: %.1f%% de %d tokens", conversationID, percentage, contextLimit)
	return above, percentage, nil
}

// GetLLMSettings retorna as configurações atuais da API LLM
func (a *App) GetLLMSettings() (*LLMSettings, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar config: %w", err)
	}

	return &LLMSettings{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.APIBaseURL,
	}, nil
}
