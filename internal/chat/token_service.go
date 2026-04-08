package chat

import "fmt"

// TokenService encapsula a lógica de consulta de estatísticas de tokens.
// Depende apenas de MessageRepository — zero acoplamento ao banco de dados.
type TokenService struct {
	msgRepo MessageRepository
}

// NewTokenService cria um TokenService com o repositório fornecido.
func NewTokenService(msgRepo MessageRepository) *TokenService {
	return &TokenService{msgRepo: msgRepo}
}

// GetConversationStats retorna estatísticas completas de tokens para uma conversa.
// contextLimit <= 0 desativa o cálculo de uso da janela de contexto.
func (s *TokenService) GetConversationStats(conversationID uint, contextLimit int) (*TokenStats, error) {
	// Determina summaryUpToMessageID para distinguir mensagens in/out of context.
	summaryUpToMessageID := uint(0)
	summary, upToID, _ := s.msgRepo.GetConversationSummary(conversationID)
	if summary != "" {
		summaryUpToMessageID = upToID
	}

	detailedStats, err := s.msgRepo.GetDetailedTokenStats(conversationID, summaryUpToMessageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas de tokens: %w", err)
	}

	toolBreakdown := make([]ToolUsageBreakdown, len(detailedStats.ToolBreakdown))
	for i, t := range detailedStats.ToolBreakdown {
		toolBreakdown[i] = ToolUsageBreakdown{
			ToolName:              t.ToolName,
			CallCount:             t.CallCount,
			TotalPromptTokens:     t.TotalPromptTokens,
			TotalCompletionTokens: t.TotalCompletionTokens,
			TotalTokens:           t.TotalTokens,
		}
	}

	result := &TokenStats{
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

	if contextLimit > 0 {
		percentage, _, err := s.msgRepo.GetContextWindowUsage(conversationID, contextLimit)
		if err == nil {
			result.ContextUsage = percentage
			result.ContextLimit = contextLimit
			result.IsNearLimit = percentage >= 80.0
			result.IsCritical = percentage >= 95.0
		}
	}

	return result, nil
}

// GetTurnStats retorna estatísticas de tokens de um turno específico.
func (s *TokenService) GetTurnStats(conversationID, turnID uint) (*TokenStats, error) {
	stats, err := s.msgRepo.GetTurnTokenStats(conversationID, turnID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas do turno: %w", err)
	}
	return &TokenStats{
		PromptTokens:     stats.PromptTokens,
		CompletionTokens: stats.CompletionTokens,
		TotalTokens:      stats.TotalTokens,
		MessageCount:     stats.MessageCount,
	}, nil
}

// GetRecentTokenCount retorna o total de tokens das N mensagens mais recentes.
func (s *TokenService) GetRecentTokenCount(conversationID uint, messageLimit int) (int, error) {
	return s.msgRepo.GetRecentMessagesTokenCount(conversationID, messageLimit)
}

// CheckContextThreshold verifica se a conversa ultrapassou o threshold de uso de contexto.
// Retorna (acimaDoThreshold, percentual, erro). contextLimit deve ser > 0.
func (s *TokenService) CheckContextThreshold(conversationID uint, contextLimit int, threshold float64) (bool, float64, error) {
	percentage, totalTokens, err := s.msgRepo.GetContextWindowUsage(conversationID, contextLimit)
	if err != nil {
		return false, 0, fmt.Errorf("erro ao calcular uso do contexto: %w", err)
	}
	_ = totalTokens
	return percentage >= threshold, percentage, nil
}
