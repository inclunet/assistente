package chat

// TokenStats representa estatísticas de tokens de uma conversa ou turno — usada como
// resposta da API Wails para o frontend.
type TokenStats struct {
	ConversationID   uint    `json:"conversationId"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	MessageCount     int     `json:"messageCount"`
	Model            string  `json:"model"`
	MostUsedModel    string  `json:"mostUsedModel"`
	ContextUsage     float64 `json:"contextUsage"` // Percentual de uso da janela de contexto (0-100)
	ContextLimit     int     `json:"contextLimit"` // Limite de tokens do modelo
	IsNearLimit      bool    `json:"isNearLimit"`  // true se >= 80%
	IsCritical       bool    `json:"isCritical"`   // true se >= 95%

	// Breakdown detalhado
	SystemPromptEstimatedTokens int                  `json:"systemPromptEstimatedTokens"`
	SummaryTokens               int                  `json:"summaryTokens"`
	MessagesInContextCount      int                  `json:"messagesInContextCount"`
	MessagesInContextTokens     int                  `json:"messagesInContextTokens"`
	MessagesOutOfContextCount   int                  `json:"messagesOutOfContextCount"`
	MessagesOutOfContextTokens  int                  `json:"messagesOutOfContextTokens"`
	ToolsUsedCount              int                  `json:"toolsUsedCount"`
	ToolBreakdown               []ToolUsageBreakdown `json:"toolBreakdown"`
}

// ToolUsageBreakdown contém estatísticas de uso de uma ferramenta específica.
type ToolUsageBreakdown struct {
	ToolName              string `json:"toolName"`
	CallCount             int    `json:"callCount"`
	TotalPromptTokens     int    `json:"totalPromptTokens"`
	TotalCompletionTokens int    `json:"totalCompletionTokens"`
	TotalTokens           int    `json:"totalTokens"`
}
