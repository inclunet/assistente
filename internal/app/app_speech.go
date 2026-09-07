package app

import (
	"context"

	"assistente/internal/profiles"
)

// Speech / TTS/STT — superfície Wails do domínio está em wailsapi.Speech (AEP-0088).
// Helpers lowercase (dispatchSpeechEvent, resolveSpeechProfile, buildChatSpeakEvent)
// e o adapter profileProviderAdapter permanecem neste pacote.
//
// SpeechController é montado no startup e associado via wireSpeech / AttachSpeech.

// profileProviderAdapter adapta profileManager + providerSvc para speech.ProfileProvider.
type profileProviderAdapter struct{ app *App }

func (a profileProviderAdapter) GetActive() (*profiles.Profile, error) {
	return a.app.profileManager.GetActive()
}

func (a profileProviderAdapter) ResolveDefaults(ctx context.Context, p *profiles.Profile) *profiles.Profile {
	return a.app.providerSvc.ResolveProfileDefaults(ctx, p)
}
