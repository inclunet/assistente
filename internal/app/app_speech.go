package app

import (
	"context"

	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/speech"
)

// ==================== Adapter para speech.ProfileProvider ====================

// profileProviderAdapter adapta profileManager + providerSvc para speech.ProfileProvider.
type profileProviderAdapter struct{ app *App }

func (a profileProviderAdapter) GetActive() (*profiles.Profile, error) {
	return a.app.profileManager.GetActive()
}

func (a profileProviderAdapter) ResolveDefaults(ctx context.Context, p *profiles.Profile) *profiles.Profile {
	return a.app.providerSvc.ResolveProfileDefaults(ctx, p)
}

// ==================== Projeção para Wails bindings ====================

// SynthesisResultInfo projeta SynthesisResult sem AudioData (bytes) para o frontend.
type SynthesisResultInfo struct {
	AudioBase64 string `json:"audioBase64"`
	Format      string `json:"format"`
	Provider    string `json:"provider"`
}

func synthesisResultInfo(r *speech.SynthesisResult) *SynthesisResultInfo {
	return &SynthesisResultInfo{
		AudioBase64: r.AudioBase64,
		Format:      r.Format,
		Provider:    r.Provider,
	}
}

// ============================================================================
// OpenAI Speech API Methods (Whisper STT + OpenAI TTS)
// ============================================================================

func (a *App) InitSpeechManagerFromProfile() error {
	return a.speechSvc.InitFromProfile(a.authenticatedContext())
}

func (a *App) TranscribeWhisper(audioBase64, filename string) (*speech.TranscriptionResult, error) {
	return a.speechSvc.Transcribe(a.authenticatedContext(), audioBase64, filename)
}

func (a *App) SynthesizeOpenAI(text string) (*SynthesisResultInfo, error) {
	r, err := a.speechSvc.Synthesize(a.authenticatedContext(), text)
	if err != nil {
		return nil, err
	}
	return synthesisResultInfo(r), nil
}

func (a *App) SynthesizeOpenAIWithVoice(text, voice string) (*SynthesisResultInfo, error) {
	r, err := a.speechSvc.SynthesizeWithVoice(a.authenticatedContext(), text, voice)
	if err != nil {
		return nil, err
	}
	return synthesisResultInfo(r), nil
}

func (a *App) SynthesizeOpenAIStream(text, voice, sessionID string) error {
	return a.speechSvc.SynthesizeStream(a.authenticatedContext(), text, voice, sessionID)
}

func (a *App) GetOpenAITTSVoices() []speech.TTSVoiceInfo {
	return a.speechSvc.GetAvailableVoices()
}

func (a *App) SetOpenAITTSVoice(voice string) { a.speechSvc.SetTTSVoice(voice) }
func (a *App) SetOpenAITTSSpeed(rate int)     { a.speechSvc.SetTTSSpeed(float64(rate)) }

// ============================================================================
// Message Audio API (TTS storage)
// ============================================================================

func (a *App) GetMessageAudio(messageID string) (*speech.AudioResult, error) {
	return a.speechSvc.GetMessageAudio(a.authenticatedContext(), messageID)
}

func (a *App) SaveMessageAudio(messageID string, audioBase64, mimeType string) error {
	return a.speechSvc.SaveMessageAudio(a.authenticatedContext(), messageID, audioBase64, mimeType)
}

func (a *App) GenerateAndSaveMessageAudio(messageID string, text string) (*speech.AudioResult, error) {
	return a.speechSvc.GenerateAndSaveMessageAudio(a.authenticatedContext(), messageID, text)
}

func (a *App) SpeakMessage(messageID string, providerID, model, voiceID string, rate float64) (*speech.AudioResult, error) {
	return a.speechSvc.SpeakMessage(a.authenticatedContext(), messageID, providerID, model, voiceID, rate)
}

// ============================================================================
// Voice Profile TTS/STT API
// ============================================================================

func (a *App) GetSpeechProviders() []*llm.ProviderConfig {
	return a.speechSvc.GetSpeechProviders()
}

func (a *App) GetTTSModels(providerID string) []speech.TTSModelInfo {
	return a.speechSvc.GetTTSModels(a.authenticatedContext(), providerID)
}

func (a *App) GetTTSVoices(providerID, modelID string) []speech.TTSVoiceInfo {
	return a.speechSvc.GetTTSVoices(a.authenticatedContext(), providerID, modelID)
}

func (a *App) GetSTTModels(providerID string) []speech.SpeechModelInfo {
	return a.speechSvc.GetSTTModels(a.authenticatedContext(), providerID)
}

func (a *App) SpeakPreview(providerID, model, voiceID string, rate, volume float64, language, text, sessionID string) error {
	return a.speechSvc.SpeakPreview(a.authenticatedContext(), speech.SpeakPreviewParams{
		ProviderID: providerID,
		Model:      model,
		VoiceID:    voiceID,
		Language:   language,
		Rate:       rate,
		Volume:     volume,
		Text:       text,
		SessionID:  sessionID,
	})
}
