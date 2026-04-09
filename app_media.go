package main

import (
	"log"

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
// Chamado quando detectamos que um modelo não suporta determinado tipo de mídia.
func (a *App) UpdateProfileMediaSupport(mediaType string, supported bool) {
	if a.profileManager == nil {
		return
	}

	profile, err := a.profileManager.GetActive()
	if err != nil || profile == nil {
		return
	}

	if profile.MediaSupport == nil {
		profile.MediaSupport = &profiles.MediaSupport{}
	}

	switch mediaType {
	case "audio":
		profile.MediaSupport.Audio = &supported
	case "image":
		profile.MediaSupport.Image = &supported
	case "document":
		profile.MediaSupport.Document = &supported
	case "video":
		profile.MediaSupport.Video = &supported
	}

	slug := a.profileManager.GetActiveSlug()
	if slug == "" {
		return
	}
	if err := a.profileManager.Update(slug, profile); err != nil {
		log.Printf("[MediaSupport] Erro ao salvar perfil: %v", err)
	} else {
		log.Printf("[MediaSupport] Perfil atualizado: %s=%v", mediaType, supported)
	}
}


