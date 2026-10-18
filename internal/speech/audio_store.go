package speech

import "context"

// AudioRepository abstrai a persistência de áudio de mensagens.
// Implementado por DBAudioStore; pode ser mockado em testes.
type AudioRepository interface {
	// GetMessageAudio retorna o áudio (base64) e o MIME type de uma mensagem.
	GetMessageAudio(ctx context.Context, messageID string) (audioBase64 string, mimeType string, err error)

	// SaveMessageAudio persiste o áudio de uma mensagem.
	SaveMessageAudio(ctx context.Context, messageID string, audioBase64, mimeType string) error

	// GetMessageContent retorna o conteúdo e o papel da mensagem.
	GetMessageContent(ctx context.Context, messageID string) (content string, role string, err error)
}
