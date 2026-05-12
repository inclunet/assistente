package app

import (
	"assistente/controllers"
	"assistente/internal/chat"
)

// ============================================================================
// Token Stats API
// ============================================================================

// ToolUsageBreakdownResult é alias de chat.ToolUsageBreakdown para compatibilidade com o frontend.
type ToolUsageBreakdownResult = chat.ToolUsageBreakdown

// TokenStatsResult é alias de chat.TokenStats para compatibilidade com o frontend.
type TokenStatsResult = chat.TokenStats

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa.
func (a *App) GetConversationTokenStats(conversationID string) (*TokenStatsResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.tokensCtrl.GetConversationTokenStats(ctx, conversationID)
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico.
func (a *App) GetTurnTokenStats(conversationID string, turnID string) (*TokenStatsResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.tokensCtrl.GetTurnTokenStats(ctx, conversationID, turnID)
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes.
func (a *App) GetRecentMessagesTokenCount(conversationID string, messageLimit int) (int, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return 0, err
	}
	return a.tokensCtrl.GetRecentMessagesTokenCount(ctx, conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto.
func (a *App) CheckContextWindowThreshold(conversationID string, threshold float64) (bool, float64, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return false, 0, err
	}
	return a.tokensCtrl.CheckContextWindowThreshold(ctx, conversationID, threshold)
}

// GetLLMSettings retorna as configurações atuais da API LLM.
func (a *App) GetLLMSettings() (*controllers.LLMSettings, error) {
	return a.tokensCtrl.GetLLMSettings()
}
