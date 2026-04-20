package app

import (
	"assistente/internal/chat"
)

// extractAudioFromMedia é um wrapper de chat.ExtractAudio para uso interno no pacote app.
func extractAudioFromMedia(mediaJSON string) (string, string) {
	return chat.ExtractAudio(mediaJSON)
}

// UpdateProfileMediaSupport atualiza o MediaSupport de um perfil e salva.
func (a *App) UpdateProfileMediaSupport(mediaType string, supported bool) {
	a.profilesCtrl.UpdateProfileMediaSupport(mediaType, supported)
}
