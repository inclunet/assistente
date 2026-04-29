package speech

// AudioRepository abstrai a persistência de áudio de mensagens.
// Implementado por DBAudioStore; pode ser mockado em testes.
type AudioRepository interface {
	// GetMessageAudio retorna o áudio (base64) e o MIME type de uma mensagem.
	GetMessageAudio(messageID string) (audioBase64 string, mimeType string, err error)

	// SaveMessageAudio persiste o áudio de uma mensagem.
	SaveMessageAudio(messageID string, audioBase64, mimeType string) error

	// GetMessageContent retorna o conteúdo textual de uma mensagem.
	GetMessageContent(messageID string) (string, error)
}
