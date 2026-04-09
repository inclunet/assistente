package chat

import "assistente/internal/database"

// MessageOptions é um alias para database.MessageOptions, exposto no domínio chat
// para que os chamadores não precisem importar o pacote de infraestrutura diretamente.
type MessageOptions = database.MessageOptions

// Tipos de domínio do pacote chat — aliases para os modelos de infraestrutura.
// Callers devem referenciar chat.Message, chat.Conversation, etc. em vez de database.*.
// Nota: TokenStats e ToolUsageBreakdown NÃO são aliases — o pacote chat define suas
// próprias versões enriquecidas (token_stats.go) para a API Wails.
type Message = database.ChatMessage
type Conversation = database.Conversation
type DetailedTokenStats = database.DetailedTokenStats
type MessageSearchResult = database.MessageSearchResult

// ErrConversationDeleted e ErrParentMessageDeleted são re-exportados aqui como erros de
// domínio do pacote chat, para que os chamadores não precisem importar internal/database.
var ErrConversationDeleted = database.ErrConversationDeleted
var ErrParentMessageDeleted = database.ErrParentMessageDeleted

// MessageRepository abstrai operações de persistência de mensagens e stats de tokens.
// Implementado por DBMessageStore; pode ser mockado em testes.
type MessageRepository interface {
	// CreateMessage persiste uma mensagem no banco.
	CreateMessage(opts MessageOptions) (*Message, error)

	// GetMessages retorna mensagens de uma conversa, opcionalmente filtradas por parentID.
	GetMessages(conversationID uint, parentID *uint) ([]Message, error)

	// GetConversationSummary retorna o resumo salvo e o ID da última mensagem resumida.
	GetConversationSummary(conversationID uint) (summary string, upToMessageID uint, err error)

	// GetDetailedTokenStats retorna estatísticas detalhadas de uso de tokens.
	GetDetailedTokenStats(conversationID uint, summaryUpToMessageID uint) (*DetailedTokenStats, error)

	// GetContextWindowUsage retorna o percentual de uso da janela de contexto.
	GetContextWindowUsage(conversationID uint, contextLimit int) (percentage float64, totalTokens int, err error)

	// GetRecentMessagesTokenCount retorna a contagem de tokens das N mensagens mais recentes.
	GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error)

	// GetTurnTokenStats retorna estatísticas de tokens de um turno específico.
	// Retorna database.TokenStats (tipo de infraestrutura com 5 campos) — não confundir com
	// chat.TokenStats que é o tipo enriquecido para o frontend.
	GetTurnTokenStats(conversationID uint, turnID uint) (*database.TokenStats, error)

	// AddAssistantToolMessage persiste mensagem de assistant com tool_calls JSON.
	AddAssistantToolMessage(conversationID, turnID uint, content, toolCalls, reasoning, model string) (*Message, error)

	// AddToolResultMessage persiste o resultado de uma tool call.
	AddToolResultMessage(conversationID, turnID uint, content, toolCallID string) (*Message, error)

	// SearchMessages busca mensagens usando full-text search (FTS5).
	SearchMessages(query string, limit int) ([]MessageSearchResult, error)
}

// ConversationRepository abstrai operações sobre conversas.
// Implementado por DBConversationStore; pode ser mockado em testes.
type ConversationRepository interface {
	// GetConversationInfo retorna os metadados de uma conversa pelo ID.
	GetConversationInfo(id uint) (*Conversation, error)

	// UpdateConversation atualiza título e/ou modelo de uma conversa.
	UpdateConversation(id uint, title, model string) error

	// UpdateConversationChannel associa/desassocia um canal de mensageria a uma conversa.
	UpdateConversationChannel(id uint, channel, contactID string) error
}
