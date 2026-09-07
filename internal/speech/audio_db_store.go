package speech

import (
	"context"

	"assistente/internal/database"
)

// DBAudioStore implementa AudioRepository usando o banco de dados SQLite via GORM.
type DBAudioStore struct{}

// NewDBAudioStore cria um DBAudioStore pronto para uso.
func NewDBAudioStore() *DBAudioStore { return &DBAudioStore{} }

func (s *DBAudioStore) GetMessageAudio(ctx context.Context, messageID string) (string, string, error) {
	return database.GetMessageAudioWithContext(ctx, messageID)
}

func (s *DBAudioStore) SaveMessageAudio(ctx context.Context, messageID string, audioBase64, mimeType string) error {
	return database.SaveMessageAudioWithContext(ctx, messageID, audioBase64, mimeType)
}

func (s *DBAudioStore) GetMessageContent(ctx context.Context, messageID string) (string, error) {
	return database.GetMessageContentWithContext(ctx, messageID)
}
