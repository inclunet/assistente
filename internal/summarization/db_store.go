package summarization

import (
	"assistente/internal/chat"
	"assistente/internal/database"
)

// DBSummarizationStore implementa SummarizationRepository usando SQLite via GORM.
type DBSummarizationStore struct{}

// NewDBStore cria um DBSummarizationStore pronto para uso.
func NewDBStore() *DBSummarizationStore { return &DBSummarizationStore{} }

func (s *DBSummarizationStore) GetMessages(conversationID uint) ([]chat.Message, error) {
	return database.GetMessages(conversationID, nil)
}

func (s *DBSummarizationStore) GetConversationSummary(conversationID uint) (string, uint, error) {
	return database.GetConversationSummary(conversationID)
}

func (s *DBSummarizationStore) IsSummarizingInProgress(conversationID uint) (bool, error) {
	return database.IsSummarizingInProgress(conversationID)
}

func (s *DBSummarizationStore) SetSummarizingInProgress(conversationID uint, inProgress bool) error {
	return database.SetSummarizingInProgress(conversationID, inProgress)
}

func (s *DBSummarizationStore) UpdateConversationSummary(conversationID uint, summary string, upToMessageID uint) error {
	return database.UpdateConversationSummary(conversationID, summary, upToMessageID)
}
