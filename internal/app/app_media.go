package app

import (
	"assistente/internal/chat"
	"assistente/internal/profiles"
)

// extractAudioFromMedia é um wrapper de chat.ExtractAudio para compatibilidade com testes do pacote main.
func extractAudioFromMedia(mediaJSON string) (string, string) {
	return chat.ExtractAudio(mediaJSON)
}

// preprocessMediaMessages converte formatos de áudio e aplica fallbacks de mídia para o modelo.
func (a *App) preprocessMediaMessages(messages []Message, profile *profiles.Profile) []Message {
	var audioSupported, docSupported *bool
	if profile != nil && profile.MediaSupport != nil {
		audioSupported = profile.MediaSupport.Audio
		docSupported = profile.MediaSupport.Document
	}
	return chat.PreprocessMessages(messages, a.whisperTranscribeFunc(), audioSupported, docSupported)
}

// UpdateProfileMediaSupport atualiza o MediaSupport de um perfil e salva.
func (a *App) UpdateProfileMediaSupport(mediaType string, supported bool) {
	a.profilesCtrl.UpdateProfileMediaSupport(mediaType, supported)
}
