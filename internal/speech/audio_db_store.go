package speech

import (
	"context"
	"errors"

	"assistente/internal/core/ports"

	"assistente/internal/database"
)

// DBAudioStore implementa AudioRepository usando o banco de dados SQLite via GORM.
type DBAudioStore struct {
	mutateTimeline func(context.Context, string, []string, func() ([]string, error)) error
}

// NewDBAudioStore cria um DBAudioStore pronto para uso.
func NewDBAudioStore(mutateTimeline func(context.Context, string, []string, func() ([]string, error)) error) *DBAudioStore {
	return &DBAudioStore{mutateTimeline: mutateTimeline}
}

func (s *DBAudioStore) GetMessageAudio(ctx context.Context, messageID string) (string, string, error) {
	return database.GetMessageAudioWithContext(ctx, messageID)
}

func (s *DBAudioStore) SaveMessageAudio(ctx context.Context, messageID string, audioBase64, mimeType string) error {
	if s.mutateTimeline == nil {
		return errors.New("timeline coordinator não inicializado")
	}
	// Autoriza e resolve apenas IDs; não carrega o áudio anterior para coordenar.
	message, err := database.GetMessageIdentityWithContext(ctx, messageID)
	if err != nil {
		return err
	}
	ids := []string{ports.MessageItemID(message.ID)}
	if message.TurnID != nil {
		if responseID := ports.ResponseItemID(*message.TurnID); responseID != "" {
			ids = append(ids, responseID)
		}
	}
	return s.mutateTimeline(ctx, message.ConversationID, ids, func() ([]string, error) {
		return nil, database.SaveMessageAudioWithContext(ctx, messageID, audioBase64, mimeType)
	})
}

func (s *DBAudioStore) GetMessageContent(ctx context.Context, messageID string) (string, string, error) {
	return database.GetMessageContentAndRoleWithContext(ctx, messageID)
}
