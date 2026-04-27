package chat

import "assistente/internal/database"

// DBMessageStore implementa MessageRepository usando o banco de dados SQLite via GORM.
type DBMessageStore struct{}

// NewDBMessageStore cria um DBMessageStore pronto para uso.
func NewDBMessageStore() *DBMessageStore { return &DBMessageStore{} }

func (s *DBMessageStore) CreateMessage(opts database.MessageOptions) (*database.ChatMessage, error) {
	return database.CreateMessage(opts)
}

func (s *DBMessageStore) GetMessage(messageID string) (*database.ChatMessage, error) {
	return database.GetMessage(messageID)
}

func (s *DBMessageStore) GetMessages(conversationID string, parentID *string) ([]database.ChatMessage, error) {
	return database.GetMessages(conversationID, parentID)
}

func (s *DBMessageStore) GetConversationSummary(conversationID string) (string, string, error) {
	return database.GetConversationSummary(conversationID)
}

func (s *DBMessageStore) GetDetailedTokenStats(conversationID string, summaryUpToMessageID string) (*database.DetailedTokenStats, error) {
	return database.GetDetailedTokenStats(conversationID, summaryUpToMessageID)
}

func (s *DBMessageStore) GetContextWindowUsage(conversationID string, contextLimit int) (float64, int, error) {
	return database.GetContextWindowUsage(conversationID, contextLimit)
}

func (s *DBMessageStore) GetRecentMessagesTokenCount(conversationID string, messageLimit int) (int, error) {
	return database.GetRecentMessagesTokenCount(conversationID, messageLimit)
}

func (s *DBMessageStore) GetTurnTokenStats(conversationID string, turnID string) (*database.TokenStats, error) {
	return database.GetTurnTokenStats(conversationID, turnID)
}

func (s *DBMessageStore) AddAssistantToolMessage(conversationID, turnID string, content, toolCalls, reasoning, model string) (*database.ChatMessage, error) {
	return database.AddAssistantToolMessage(conversationID, turnID, content, toolCalls, reasoning, model)
}

func (s *DBMessageStore) AddToolResultMessage(conversationID, turnID string, content, toolCallID string) (*database.ChatMessage, error) {
	return database.AddToolResultMessage(conversationID, turnID, content, toolCallID)
}

func (s *DBMessageStore) SearchMessages(query string, limit int) ([]database.MessageSearchResult, error) {
	return database.SearchMessageContent(query, limit)
}

// DBConversationStore implementa ConversationRepository usando o banco de dados SQLite via GORM.
type DBConversationStore struct{}

// NewDBConversationStore cria um DBConversationStore pronto para uso.
func NewDBConversationStore() *DBConversationStore { return &DBConversationStore{} }

func (s *DBConversationStore) GetConversationInfo(id string) (*database.Conversation, error) {
	return database.GetConversationInfo(id)
}

func (s *DBConversationStore) UpdateConversation(id string, title, model string) error {
	return database.UpdateConversation(id, title, model)
}

func (s *DBConversationStore) UpdateConversationChannel(id string, channel, contactID string) error {
	return database.UpdateConversationChannel(id, channel, contactID)
}
