package main

import (
	"fmt"
	"log"

	"assistente/internal/config"
	"assistente/internal/database"
)

// ============================================================================
// Token Stats API
// ============================================================================

// ToolUsageBreakdownResult contém informações de uso de uma ferramenta
type ToolUsageBreakdownResult struct {
	ToolName              string `json:"toolName"`
	CallCount             int    `json:"callCount"`
	TotalPromptTokens     int    `json:"totalPromptTokens"`
	TotalCompletionTokens int    `json:"totalCompletionTokens"`
	TotalTokens           int    `json:"totalTokens"`
}

// TokenStatsResult representa estatísticas de tokens para o frontend
type TokenStatsResult struct {
	ConversationID   uint    `json:"conversationId"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	MessageCount     int     `json:"messageCount"`
	Model            string  `json:"model"`
	MostUsedModel    string  `json:"mostUsedModel"`
	ContextUsage     float64 `json:"contextUsage"` // Porcentagem de uso do contexto (0-100)
	ContextLimit     int     `json:"contextLimit"` // Limite de tokens do modelo
	IsNearLimit      bool    `json:"isNearLimit"`  // True se >= 80% do limite
	IsCritical       bool    `json:"isCritical"`   // True se >= 95% do limite

	// Breakdown detalhado de tokens
	SystemPromptEstimatedTokens int                        `json:"systemPromptEstimatedTokens"`
	SummaryTokens               int                        `json:"summaryTokens"`
	MessagesInContextCount      int                        `json:"messagesInContextCount"`
	MessagesInContextTokens     int                        `json:"messagesInContextTokens"`
	MessagesOutOfContextCount   int                        `json:"messagesOutOfContextCount"`
	MessagesOutOfContextTokens  int                        `json:"messagesOutOfContextTokens"`
	ToolsUsedCount              int                        `json:"toolsUsedCount"`
	ToolBreakdown               []ToolUsageBreakdownResult `json:"toolBreakdown"`
}

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa
func (a *App) GetConversationTokenStats(conversationID uint) (*TokenStatsResult, error) {
	// Recuperar summaryUpToMessageID para cálculo de mensagens in/out of context
	summaryUpToMessageID := uint(0)
	summary, upToID, _ := database.GetConversationSummary(conversationID)
	if summary != "" {
		summaryUpToMessageID = upToID
	}

	detailedStats, err := database.GetDetailedTokenStats(conversationID, summaryUpToMessageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas de tokens: %w", err)
	}

	// Map tool usage breakdown
	toolBreakdown := make([]ToolUsageBreakdownResult, len(detailedStats.ToolBreakdown))
	for i, tool := range detailedStats.ToolBreakdown {
		toolBreakdown[i] = ToolUsageBreakdownResult{
			ToolName:              tool.ToolName,
			CallCount:             tool.CallCount,
			TotalPromptTokens:     tool.TotalPromptTokens,
			TotalCompletionTokens: tool.TotalCompletionTokens,
			TotalTokens:           tool.TotalTokens,
		}
	}

	result := &TokenStatsResult{
		ConversationID:              conversationID,
		PromptTokens:                detailedStats.PromptTokens,
		CompletionTokens:            detailedStats.CompletionTokens,
		TotalTokens:                 detailedStats.TotalTokens,
		MessageCount:                detailedStats.MessageCount,
		Model:                       detailedStats.Model,
		MostUsedModel:               detailedStats.Model,
		SystemPromptEstimatedTokens: detailedStats.SystemPromptEstimatedTokens,
		SummaryTokens:               detailedStats.SummaryTokens,
		MessagesInContextCount:      detailedStats.MessagesInContextCount,
		MessagesInContextTokens:     detailedStats.MessagesInContextTokens,
		MessagesOutOfContextCount:   detailedStats.MessagesOutOfContextCount,
		MessagesOutOfContextTokens:  detailedStats.MessagesOutOfContextTokens,
		ToolsUsedCount:              detailedStats.ToolsUsedCount,
		ToolBreakdown:               toolBreakdown,
	}

	// Busca informações do perfil ativo para obter o limite de contexto
	profile, err := a.profileManager.GetActive()
	if err == nil && profile != nil && profile.Chat.ContextWindow > 0 {
		contextLimit := profile.Chat.ContextWindow
		percentage, _, err := database.GetContextWindowUsage(conversationID, contextLimit)
		if err == nil {
			result.ContextUsage = percentage
			result.ContextLimit = contextLimit
			result.IsNearLimit = percentage >= 80.0
			result.IsCritical = percentage >= 95.0
		}
	}

	return result, nil
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico
func (a *App) GetTurnTokenStats(conversationID uint, turnID uint) (*TokenStatsResult, error) {
	stats, err := database.GetTurnTokenStats(conversationID, turnID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas do turno: %w", err)
	}

	return &TokenStatsResult{
		PromptTokens:     stats.PromptTokens,
		CompletionTokens: stats.CompletionTokens,
		TotalTokens:      stats.TotalTokens,
		MessageCount:     stats.MessageCount,
	}, nil
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes
// Útil para estimar quanto contexto será enviado na próxima requisição
func (a *App) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error) {
	return database.GetRecentMessagesTokenCount(conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto
// Retorna true e a porcentagem se estiver acima do threshold (padrão 80%)
func (a *App) CheckContextWindowThreshold(conversationID uint, threshold float64) (bool, float64, error) {
	if threshold <= 0 {
		threshold = 80.0 // Padrão: 80%
	}

	// Busca informações do perfil ativo para obter o limite de contexto
	profile, err := a.profileManager.GetActive()
	if err != nil {
		return false, 0, fmt.Errorf("erro ao obter perfil ativo: %w", err)
	}

	if profile == nil || profile.Chat.ContextWindow <= 0 {
		return false, 0, fmt.Errorf("limite de contexto não configurado no perfil")
	}

	contextLimit := profile.Chat.ContextWindow
	percentage, totalTokens, err := database.GetContextWindowUsage(conversationID, contextLimit)
	if err != nil {
		return false, 0, fmt.Errorf("erro ao calcular uso do contexto: %w", err)
	}

	log.Printf("[TokenStats] Conversa %d: %d tokens de %d (%0.1f%%)",
		conversationID, totalTokens, contextLimit, percentage)

	return percentage >= threshold, percentage, nil
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
