package chat

import (
	"context"

	"assistente/internal/database"
)

// DBMessageStore implementa MessageRepository usando o banco de dados SQLite via GORM.
type DBMessageStore struct{}

// NewDBMessageStore cria um DBMessageStore pronto para uso.
func NewDBMessageStore() *DBMessageStore { return &DBMessageStore{} }

func (s *DBMessageStore) CreateMessage(ctx context.Context, opts database.MessageOptions) (*database.ChatMessage, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.CreateMessageWithContext(ctx, opts)
}

func (s *DBMessageStore) UpdateMessageContentAndReasoning(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, model string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.UpdateMessageContentAndReasoningWithContext(ctx, messageID, content, reasoning, promptTokens, completionTokens, totalTokens, model)
}

func (s *DBMessageStore) UpdateMessageContentReasoningAndUsage(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, cacheReadTokens, cacheWriteTokens, cacheMissTokens int, model string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.UpdateMessageContentReasoningAndUsageWithContext(ctx, messageID, content, reasoning, promptTokens, completionTokens, totalTokens, cacheReadTokens, cacheWriteTokens, cacheMissTokens, model)
}

func (s *DBMessageStore) UpdateMessageCacheTokens(ctx context.Context, messageID string, cacheReadTokens, cacheWriteTokens, cacheMissTokens int) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.UpdateMessageCacheTokensWithContext(ctx, messageID, cacheReadTokens, cacheWriteTokens, cacheMissTokens)
}

func (s *DBMessageStore) GetMessage(ctx context.Context, messageID string) (*database.ChatMessage, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.GetMessageWithContext(ctx, messageID)
}

func (s *DBMessageStore) GetMessages(ctx context.Context, conversationID string, parentID *string) ([]database.ChatMessage, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.GetMessagesWithContext(ctx, conversationID, parentID)
}

func (s *DBMessageStore) GetMessagesByTurnID(ctx context.Context, conversationID string, parentID *string, turnID string, limit int) ([]database.ChatMessage, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.GetMessagesByTurnIDWithContext(ctx, conversationID, parentID, turnID, limit)
}

func (s *DBMessageStore) GetConversationSummary(ctx context.Context, conversationID string) (string, string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", "", err
	}
	return database.GetConversationSummaryWithContext(ctx, conversationID)
}

func (s *DBMessageStore) GetDetailedTokenStats(ctx context.Context, conversationID string, summaryUpToMessageID string) (*database.DetailedTokenStats, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.GetDetailedTokenStatsWithContext(ctx, conversationID, summaryUpToMessageID)
}

func (s *DBMessageStore) GetContextWindowUsage(ctx context.Context, conversationID string, contextLimit int) (float64, int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, 0, err
	}
	return database.GetContextWindowUsageWithContext(ctx, conversationID, contextLimit)
}

func (s *DBMessageStore) GetRecentMessagesTokenCount(ctx context.Context, conversationID string, messageLimit int) (int, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return 0, err
	}
	return database.GetRecentMessagesTokenCountWithContext(ctx, conversationID, messageLimit)
}

func (s *DBMessageStore) GetTurnTokenStats(ctx context.Context, conversationID string, turnID string) (*database.TokenStats, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.GetTurnTokenStatsWithContext(ctx, conversationID, turnID)
}

func (s *DBMessageStore) AddAssistantToolMessage(ctx context.Context, conversationID, turnID string, content, toolCalls, reasoning, model string) (*database.ChatMessage, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.AddAssistantToolMessageWithContext(ctx, conversationID, turnID, content, toolCalls, reasoning, model)
}

func (s *DBMessageStore) AddToolResultMessage(ctx context.Context, conversationID, turnID string, content, toolCallID string) (*database.ChatMessage, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.AddToolResultMessageWithContext(ctx, conversationID, turnID, content, toolCallID)
}

func (s *DBMessageStore) SearchMessages(ctx context.Context, query string, limit int) ([]database.MessageSearchResult, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.SearchMessageContentWithContext(ctx, query, limit)
}

// SearchMessagesInConversation restringe a busca FTS a uma conversa do
// usuário autenticado. O repository mantém o filtro por user_id mesmo com o
// conversationID explícito.
func (s *DBMessageStore) SearchMessagesInConversation(ctx context.Context, query, conversationID string, limit int) ([]database.MessageSearchResult, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.SearchMessageContentInConversationWithContext(ctx, query, conversationID, limit)
}

// DBConversationStore implementa ConversationRepository usando o banco de dados SQLite via GORM.
type DBConversationStore struct{}

// NewDBConversationStore cria um DBConversationStore pronto para uso.
func NewDBConversationStore() *DBConversationStore { return &DBConversationStore{} }

func (s *DBConversationStore) GetConversationInfo(ctx context.Context, id string) (*database.Conversation, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.GetConversationInfoWithContext(ctx, id)
}

func (s *DBConversationStore) UpdateConversation(ctx context.Context, id string, title, model string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.UpdateConversationWithContext(ctx, id, title, model)
}

func (s *DBConversationStore) UpdateConversationChannel(ctx context.Context, id string, channel, contactID string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.UpdateConversationChannelWithContext(ctx, id, channel, contactID)
}
