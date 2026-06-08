package chat

import (
	"context"

	"assistente/internal/database"
)

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
	CreateMessage(ctx context.Context, opts MessageOptions) (*Message, error)

	// UpdateMessageContentAndReasoning atualiza conteúdo, reasoning e tokens de uma mensagem.
	UpdateMessageContentAndReasoning(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, model string) error

	// GetMessage retorna uma mensagem completa pelo ID.
	GetMessage(ctx context.Context, messageID string) (*Message, error)

	// GetMessages retorna mensagens de uma conversa, opcionalmente filtradas por parentID.
	GetMessages(ctx context.Context, conversationID string, parentID *string) ([]Message, error)

	// GetMessagesByTurnID retorna mensagens de um turno específico pertencentes ao usuário do contexto.
	// Mantém o mesmo escopo de parent da janela para não misturar raiz e threads.
	GetMessagesByTurnID(ctx context.Context, conversationID string, parentID *string, turnID string, limit int) ([]Message, error)

	// GetConversationSummary retorna o resumo salvo e o ID da última mensagem resumida.
	GetConversationSummary(ctx context.Context, conversationID string) (summary string, upToMessageID string, err error)

	// GetDetailedTokenStats retorna estatísticas detalhadas de uso de tokens.
	GetDetailedTokenStats(ctx context.Context, conversationID string, summaryUpToMessageID string) (*DetailedTokenStats, error)

	// GetContextWindowUsage retorna o percentual de uso da janela de contexto.
	GetContextWindowUsage(ctx context.Context, conversationID string, contextLimit int) (percentage float64, totalTokens int, err error)

	// GetRecentMessagesTokenCount retorna a contagem de tokens das N mensagens mais recentes.
	GetRecentMessagesTokenCount(ctx context.Context, conversationID string, messageLimit int) (int, error)

	// GetTurnTokenStats retorna estatísticas de tokens de um turno específico.
	// Retorna database.TokenStats (tipo de infraestrutura com 5 campos) — não confundir com
	// chat.TokenStats que é o tipo enriquecido para o frontend.
	GetTurnTokenStats(ctx context.Context, conversationID string, turnID string) (*database.TokenStats, error)

	// AddAssistantToolMessage persiste mensagem de assistant com tool_calls JSON.
	AddAssistantToolMessage(ctx context.Context, conversationID string, turnID string, content, toolCalls, reasoning, model string) (*Message, error)

	// AddToolResultMessage persiste o resultado de uma tool call.
	AddToolResultMessage(ctx context.Context, conversationID string, turnID string, content, toolCallID string) (*Message, error)

	// SearchMessages busca mensagens usando full-text search (FTS5).
	SearchMessages(ctx context.Context, query string, limit int) ([]MessageSearchResult, error)
}

// ConversationRepository abstrai operações sobre conversas.
// Implementado por DBConversationStore; pode ser mockado em testes.
type ConversationRepository interface {
	// GetConversationInfo retorna os metadados de uma conversa pelo ID.
	GetConversationInfo(ctx context.Context, id string) (*Conversation, error)

	// UpdateConversation atualiza título e/ou modelo de uma conversa.
	UpdateConversation(ctx context.Context, id string, title, model string) error

	// UpdateConversationChannel associa/desassocia um canal de mensageria a uma conversa.
	UpdateConversationChannel(ctx context.Context, id string, channel, contactID string) error
}
