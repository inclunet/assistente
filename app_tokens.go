package main

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
func (a *App) GetConversationTokenStats(conversationID uint) (*TokenStatsResult, error) {
	return a.tokensCtrl.GetConversationTokenStats(conversationID)
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico.
func (a *App) GetTurnTokenStats(conversationID uint, turnID uint) (*TokenStatsResult, error) {
	return a.tokensCtrl.GetTurnTokenStats(conversationID, turnID)
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes.
func (a *App) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error) {
	return a.tokensCtrl.GetRecentMessagesTokenCount(conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto.
func (a *App) CheckContextWindowThreshold(conversationID uint, threshold float64) (bool, float64, error) {
	return a.tokensCtrl.CheckContextWindowThreshold(conversationID, threshold)
}

// GetLLMSettings retorna as configurações atuais da API LLM.
func (a *App) GetLLMSettings() (*controllers.LLMSettings, error) {
	return a.tokensCtrl.GetLLMSettings()
}
