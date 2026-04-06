package chat

import "assistente/internal/database"

// MessageRepository abstrai operações de persistência de mensagens e stats de tokens.
// Implementado por DBMessageStore; pode ser mockado em testes.
type MessageRepository interface {
	// CreateMessage persiste uma mensagem no banco.
	CreateMessage(opts database.MessageOptions) (*database.ChatMessage, error)

	// GetMessages retorna mensagens de uma conversa, opcionalmente filtradas por parentID.
	GetMessages(conversationID uint, parentID *uint) ([]database.ChatMessage, error)

	// GetConversationSummary retorna o resumo salvo e o ID da última mensagem resumida.
	GetConversationSummary(conversationID uint) (summary string, upToMessageID uint, err error)

	// GetDetailedTokenStats retorna estatísticas detalhadas de uso de tokens.
	GetDetailedTokenStats(conversationID uint, summaryUpToMessageID uint) (*database.DetailedTokenStats, error)

	// GetContextWindowUsage retorna o percentual de uso da janela de contexto.
	GetContextWindowUsage(conversationID uint, contextLimit int) (percentage float64, totalTokens int, err error)

	// GetRecentMessagesTokenCount retorna a contagem de tokens das N mensagens mais recentes.
	GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error)

	// GetTurnTokenStats retorna estatísticas de tokens de um turno específico.
	GetTurnTokenStats(conversationID uint, turnID uint) (*database.TokenStats, error)

	// AddAssistantToolMessage persiste mensagem de assistant com tool_calls JSON.
	AddAssistantToolMessage(conversationID, turnID uint, content, toolCalls, reasoning, model string) (*database.ChatMessage, error)

	// AddToolResultMessage persiste o resultado de uma tool call.
	AddToolResultMessage(conversationID, turnID uint, content, toolCallID string) (*database.ChatMessage, error)
}

// ConversationRepository abstrai operações sobre conversas.
// Implementado por DBConversationStore; pode ser mockado em testes.
type ConversationRepository interface {
	// GetConversationInfo retorna os metadados de uma conversa pelo ID.
	GetConversationInfo(id uint) (*database.Conversation, error)

	// UpdateConversation atualiza título e/ou modelo de uma conversa.
	UpdateConversation(id uint, title, model string) error

	// UpdateConversationChannel associa/desassocia um canal de mensageria a uma conversa.
	UpdateConversationChannel(id uint, channel, contactID string) error
}
