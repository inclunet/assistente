package summarization

import (
	"context"

	"assistente/internal/chat"
	"assistente/internal/database"
)

// DBSummarizationStore implementa SummarizationRepository usando SQLite via GORM.
type DBSummarizationStore struct{}

// NewDBStore cria um DBSummarizationStore pronto para uso.
func NewDBStore() *DBSummarizationStore { return &DBSummarizationStore{} }

func (s *DBSummarizationStore) GetMessages(ctx context.Context, conversationID string) ([]chat.Message, error) {
	return database.GetMessagesWithContext(ctx, conversationID, nil)
}

func (s *DBSummarizationStore) GetConversationSummary(ctx context.Context, conversationID string) (string, string, error) {
	return database.GetConversationSummaryWithContext(ctx, conversationID)
}

func (s *DBSummarizationStore) IsSummarizingInProgress(ctx context.Context, conversationID string) (bool, error) {
	return database.IsSummarizingInProgressWithContext(ctx, conversationID)
}

func (s *DBSummarizationStore) SetSummarizingInProgress(ctx context.Context, conversationID string, inProgress bool) error {
	return database.SetSummarizingInProgressWithContext(ctx, conversationID, inProgress)
}

func (s *DBSummarizationStore) UpdateConversationSummary(ctx context.Context, conversationID string, summary string, upToMessageID string) error {
	return database.UpdateConversationSummaryWithContext(ctx, conversationID, summary, upToMessageID)
}
