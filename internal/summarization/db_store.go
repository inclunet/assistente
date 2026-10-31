package summarization

import (
	"context"

	"assistente/internal/chat"
	"assistente/internal/database"
)

// DBSummarizationStore implementa SummarizationRepository usando SQLite via GORM.
//
// SECURITY (AEP-0052): cada método chama database.RequireUserID antes de
// delegar para a camada database/. As funções *WithContext em database.go
// agora são fail-closed por si mesmas, mas a guarda extra aqui mantém a
// defesa em camadas igual a chat/tasklist/providers DBStores e garante uma
// mensagem de erro consistente nos pontos de entrada do serviço de
// sumarização.
type DBSummarizationStore struct{}

// NewDBStore cria um DBSummarizationStore pronto para uso.
func NewDBStore() *DBSummarizationStore { return &DBSummarizationStore{} }

func (s *DBSummarizationStore) GetMessages(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return database.GetMessagesWithContext(ctx, conversationID, nil)
}

func (s *DBSummarizationStore) GetConversationSummary(ctx context.Context, conversationID string) (string, string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", "", err
	}
	return database.GetConversationSummaryWithContext(ctx, conversationID)
}

func (s *DBSummarizationStore) IsSummarizingInProgress(ctx context.Context, conversationID string) (bool, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return false, err
	}
	return database.IsSummarizingInProgressWithContext(ctx, conversationID)
}

func (s *DBSummarizationStore) SetSummarizingInProgress(ctx context.Context, conversationID string, inProgress bool) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.SetSummarizingInProgressWithContext(ctx, conversationID, inProgress)
}

func (s *DBSummarizationStore) CommitConversationSummary(ctx context.Context, conversationID, summary, upToMessageID, previousSummary string, sourceIDs []string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.NewSummarizationRepository(database.DB()).CommitConversationSummary(ctx, conversationID, summary, upToMessageID, previousSummary, sourceIDs)
}
