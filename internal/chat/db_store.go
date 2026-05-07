package chat

import (
	"context"

	"assistente/internal/database"
)

// DBMessageStore implementa MessageRepository usando o banco de dados SQLite via GORM.
type DBMessageStore struct {
	ctxProvider func() context.Context
	requireUser bool
}

// NewDBMessageStore cria um DBMessageStore pronto para uso.
func NewDBMessageStore() *DBMessageStore { return &DBMessageStore{} }

func NewScopedDBMessageStore(ctxProvider func() context.Context) *DBMessageStore {
	return &DBMessageStore{ctxProvider: ctxProvider, requireUser: true}
}

func (s *DBMessageStore) ctx() (context.Context, error) {
	ctx := context.Background()
	if s.ctxProvider != nil {
		ctx = s.ctxProvider()
	}
	if s.requireUser {
		if _, err := database.RequireUserID(ctx); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

func (s *DBMessageStore) CreateMessage(opts database.MessageOptions) (*database.ChatMessage, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.CreateMessageWithContext(ctx, opts)
}

func (s *DBMessageStore) GetMessage(messageID string) (*database.ChatMessage, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetMessageWithContext(ctx, messageID)
}

func (s *DBMessageStore) GetMessages(conversationID string, parentID *string) ([]database.ChatMessage, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetMessagesWithContext(ctx, conversationID, parentID)
}

func (s *DBMessageStore) GetConversationSummary(conversationID string) (string, string, error) {
	ctx, err := s.ctx()
	if err != nil {
		return "", "", err
	}
	return database.GetConversationSummaryWithContext(ctx, conversationID)
}

func (s *DBMessageStore) GetDetailedTokenStats(conversationID string, summaryUpToMessageID string) (*database.DetailedTokenStats, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetDetailedTokenStatsWithContext(ctx, conversationID, summaryUpToMessageID)
}

func (s *DBMessageStore) GetContextWindowUsage(conversationID string, contextLimit int) (float64, int, error) {
	ctx, err := s.ctx()
	if err != nil {
		return 0, 0, err
	}
	return database.GetContextWindowUsageWithContext(ctx, conversationID, contextLimit)
}

func (s *DBMessageStore) GetRecentMessagesTokenCount(conversationID string, messageLimit int) (int, error) {
	ctx, err := s.ctx()
	if err != nil {
		return 0, err
	}
	return database.GetRecentMessagesTokenCountWithContext(ctx, conversationID, messageLimit)
}

func (s *DBMessageStore) GetTurnTokenStats(conversationID string, turnID string) (*database.TokenStats, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTurnTokenStatsWithContext(ctx, conversationID, turnID)
}

func (s *DBMessageStore) AddAssistantToolMessage(conversationID, turnID string, content, toolCalls, reasoning, model string) (*database.ChatMessage, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.AddAssistantToolMessageWithContext(ctx, conversationID, turnID, content, toolCalls, reasoning, model)
}

func (s *DBMessageStore) AddToolResultMessage(conversationID, turnID string, content, toolCallID string) (*database.ChatMessage, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.AddToolResultMessageWithContext(ctx, conversationID, turnID, content, toolCallID)
}

func (s *DBMessageStore) SearchMessages(query string, limit int) ([]database.MessageSearchResult, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.SearchMessageContentWithContext(ctx, query, limit)
}

func (s *DBMessageStore) SearchMessagesWithContext(ctx context.Context, query string, limit int) ([]database.MessageSearchResult, error) {
	if s.requireUser {
		if _, err := database.RequireUserID(ctx); err != nil {
			return nil, err
		}
	}
	return database.SearchMessageContentWithContext(ctx, query, limit)
}

// DBConversationStore implementa ConversationRepository usando o banco de dados SQLite via GORM.
type DBConversationStore struct {
	ctxProvider func() context.Context
	requireUser bool
}

// NewDBConversationStore cria um DBConversationStore pronto para uso.
func NewDBConversationStore() *DBConversationStore { return &DBConversationStore{} }

func NewScopedDBConversationStore(ctxProvider func() context.Context) *DBConversationStore {
	return &DBConversationStore{ctxProvider: ctxProvider, requireUser: true}
}

func (s *DBConversationStore) ctx() (context.Context, error) {
	ctx := context.Background()
	if s.ctxProvider != nil {
		ctx = s.ctxProvider()
	}
	if s.requireUser {
		if _, err := database.RequireUserID(ctx); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

func (s *DBConversationStore) GetConversationInfo(id string) (*database.Conversation, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetConversationInfoWithContext(ctx, id)
}

func (s *DBConversationStore) UpdateConversation(id string, title, model string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateConversationWithContext(ctx, id, title, model)
}

func (s *DBConversationStore) UpdateConversationChannel(id string, channel, contactID string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateConversationChannelWithContext(ctx, id, channel, contactID)
}
