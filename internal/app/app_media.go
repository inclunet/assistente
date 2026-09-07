package app

import (
	"assistente/internal/chat"
)

// extractAudioFromMedia é um wrapper de chat.ExtractAudio para uso interno no pacote app.
func extractAudioFromMedia(mediaJSON string) (string, string) {
	return chat.ExtractAudio(mediaJSON)
}
