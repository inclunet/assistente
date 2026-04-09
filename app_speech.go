package main

import (
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

func (a profileProviderAdapter) ResolveDefaults(p *profiles.Profile) *profiles.Profile {
	return a.app.resolveProfileDefaults(p)
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
// SAPI5 Voice Methods (Windows only)
// ============================================================================

func (a *App) GetSAPI5Voices() ([]speech.Voice, error) {
	return a.speechSvc.GetSAPI5Voices(), nil
}
func (a *App) SpeakSAPI5(text, voiceName string) error   { return a.speechSvc.SpeakSAPI5(text, voiceName) }
func (a *App) StopSAPI5() error                          { return a.speechSvc.StopSAPI5() }
func (a *App) SetSAPI5Volume(volume int) error           { return a.speechSvc.SetSAPI5Volume(volume) }
func (a *App) SetSAPI5Rate(rate int) error               { return a.speechSvc.SetSAPI5Rate(rate) }
func (a *App) IsSAPI5Speaking() bool                     { return a.speechSvc.IsSAPI5Speaking() }

// ============================================================================
// OpenAI Speech API Methods (Whisper STT + OpenAI TTS)
// ============================================================================

func (a *App) InitSpeechManagerFromProfile() error { return a.speechSvc.InitFromProfile() }

func (a *App) TranscribeWhisper(audioBase64, filename string) (*speech.TranscriptionResult, error) {
	return a.speechSvc.Transcribe(audioBase64, filename)
}

func (a *App) SynthesizeOpenAI(text string) (*SynthesisResultInfo, error) {
	r, err := a.speechSvc.Synthesize(text)
	if err != nil {
		return nil, err
	}
	return synthesisResultInfo(r), nil
}

func (a *App) SynthesizeOpenAIWithVoice(text, voice string) (*SynthesisResultInfo, error) {
	r, err := a.speechSvc.SynthesizeWithVoice(text, voice)
	if err != nil {
		return nil, err
	}
	return synthesisResultInfo(r), nil
}

func (a *App) SynthesizeOpenAIStream(text, voice, sessionID string) error {
	return a.speechSvc.SynthesizeStream(text, voice, sessionID)
}

func (a *App) GetOpenAITTSVoices() []speech.TTSVoiceInfo {
	return a.speechSvc.GetAvailableVoices()
}

func (a *App) SetOpenAITTSVoice(voice string)  { a.speechSvc.SetTTSVoice(voice) }
func (a *App) SetOpenAITTSSpeed(rate int)       { a.speechSvc.SetTTSSpeed(float64(rate)) }

// ============================================================================
// Message Audio API (TTS storage)
// ============================================================================

func (a *App) GetMessageAudio(messageID uint) (*speech.AudioResult, error) {
	return a.speechSvc.GetMessageAudio(messageID)
}

func (a *App) SaveMessageAudio(messageID uint, audioBase64, mimeType string) error {
	return a.speechSvc.SaveMessageAudio(messageID, audioBase64, mimeType)
}

func (a *App) GenerateAndSaveMessageAudio(messageID uint, text string) (*speech.AudioResult, error) {
	return a.speechSvc.GenerateAndSaveMessageAudio(messageID, text)
}

func (a *App) SpeakMessage(messageID uint, providerID, voiceID, model string, rate float64) (*speech.AudioResult, error) {
	return a.speechSvc.SpeakMessage(messageID, providerID, voiceID, model, rate)
}

// ============================================================================
// Voice Profile TTS/STT API
// ============================================================================

func (a *App) GetSpeechProviders() []*llm.ProviderConfig {
	return a.speechSvc.GetSpeechProviders()
}

func (a *App) GetTTSVoices(profileID, providerID string) []speech.TTSVoiceEntry {
	return a.speechSvc.GetTTSVoices(providerID)
}

func (a *App) GetTTSModels(providerID string) []speech.SpeechModelInfo {
	return a.speechSvc.GetTTSModels(providerID)
}

func (a *App) GetSTTModels(providerID string) []speech.SpeechModelInfo {
	return a.speechSvc.GetSTTModels(providerID)
}

func (a *App) SpeakPreview(providerId, voiceId, model string, rate, volume float64, text, sessionId string) error {
	return a.speechSvc.SpeakPreview(providerId, voiceId, model, rate, volume, text, sessionId)
}

// ============================================================================
// TTS Client Helpers (usados por outros arquivos root)
// ============================================================================

func (a *App) createTTSClientForProvider(providerID, model string) *speech.TTSClient {
	return a.speechSvc.CreateTTSClient(providerID, model)
}

func (a *App) findOpenAILikeProvider() *llm.ProviderConfig {
	return a.speechSvc.FindOpenAILikeProvider()
}
