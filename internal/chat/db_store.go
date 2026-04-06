package chat

import "assistente/internal/database"

// DBMessageStore implementa MessageRepository usando o banco de dados SQLite via GORM.
type DBMessageStore struct{}

// NewDBMessageStore cria um DBMessageStore pronto para uso.
func NewDBMessageStore() *DBMessageStore { return &DBMessageStore{} }

func (s *DBMessageStore) CreateMessage(opts database.MessageOptions) (*database.ChatMessage, error) {
	return database.CreateMessage(opts)
}

func (s *DBMessageStore) GetMessages(conversationID uint, parentID *uint) ([]database.ChatMessage, error) {
	return database.GetMessages(conversationID, parentID)
}

func (s *DBMessageStore) GetConversationSummary(conversationID uint) (string, uint, error) {
	return database.GetConversationSummary(conversationID)
}

func (s *DBMessageStore) GetDetailedTokenStats(conversationID uint, summaryUpToMessageID uint) (*database.DetailedTokenStats, error) {
	return database.GetDetailedTokenStats(conversationID, summaryUpToMessageID)
}

func (s *DBMessageStore) GetContextWindowUsage(conversationID uint, contextLimit int) (float64, int, error) {
	return database.GetContextWindowUsage(conversationID, contextLimit)
}

func (s *DBMessageStore) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error) {
	return database.GetRecentMessagesTokenCount(conversationID, messageLimit)
}

func (s *DBMessageStore) GetTurnTokenStats(conversationID uint, turnID uint) (*database.TokenStats, error) {
	return database.GetTurnTokenStats(conversationID, turnID)
}

// DBConversationStore implementa ConversationRepository usando o banco de dados SQLite via GORM.
type DBConversationStore struct{}

// NewDBConversationStore cria um DBConversationStore pronto para uso.
func NewDBConversationStore() *DBConversationStore { return &DBConversationStore{} }

func (s *DBConversationStore) GetConversationInfo(id uint) (*database.Conversation, error) {
	return database.GetConversationInfo(id)
}

func (s *DBConversationStore) UpdateConversation(id uint, title, model string) error {
	return database.UpdateConversation(id, title, model)
}

func (s *DBConversationStore) UpdateConversationChannel(id uint, channel, contactID string) error {
	return database.UpdateConversationChannel(id, channel, contactID)
}
