package speech

import "assistente/internal/database"

// DBAudioStore implementa AudioRepository usando o banco de dados SQLite via GORM.
type DBAudioStore struct{}

// NewDBAudioStore cria um DBAudioStore pronto para uso.
func NewDBAudioStore() *DBAudioStore { return &DBAudioStore{} }

func (s *DBAudioStore) GetMessageAudio(messageID string) (string, string, error) {
	return database.GetMessageAudio(messageID)
}

func (s *DBAudioStore) SaveMessageAudio(messageID string, audioBase64, mimeType string) error {
	return database.SaveMessageAudio(messageID, audioBase64, mimeType)
}

func (s *DBAudioStore) GetMessageContent(messageID string) (string, error) {
	return database.GetMessageContent(messageID)
}
