package controllers

import (
	"assistente/internal/llm"
	"assistente/internal/speech"
)

// SpeechControllerConfig agrupa as dependências do SpeechController.
type SpeechControllerConfig struct {
	SpeechSvc *speech.Service
}

// SpeechController é o Inbound Adapter para operações de voz (TTS/STT).
type SpeechController struct {
	speechSvc *speech.Service
}

// NewSpeechController cria um SpeechController com as dependências injetadas.
func NewSpeechController(cfg SpeechControllerConfig) *SpeechController {
	return &SpeechController{speechSvc: cfg.SpeechSvc}
}

// ============================================================================
// OpenAI Speech API Methods (Whisper STT + OpenAI TTS)
// ============================================================================

func (c *SpeechController) InitFromProfile() error {
	return c.speechSvc.InitFromProfile()
}

func (c *SpeechController) Transcribe(audioBase64, filename string) (*speech.TranscriptionResult, error) {
	return c.speechSvc.Transcribe(audioBase64, filename)
}

func (c *SpeechController) Synthesize(text string) (*speech.SynthesisResult, error) {
	return c.speechSvc.Synthesize(text)
}

func (c *SpeechController) SynthesizeWithVoice(text, voice string) (*speech.SynthesisResult, error) {
	return c.speechSvc.SynthesizeWithVoice(text, voice)
}

func (c *SpeechController) SynthesizeStream(text, voice, sessionID string) error {
	return c.speechSvc.SynthesizeStream(text, voice, sessionID)
}

func (c *SpeechController) GetAvailableVoices() []speech.TTSVoiceInfo {
	return c.speechSvc.GetAvailableVoices()
}

func (c *SpeechController) SetTTSVoice(voice string) {
	c.speechSvc.SetTTSVoice(voice)
}

func (c *SpeechController) SetTTSSpeed(rate int) {
	c.speechSvc.SetTTSSpeed(float64(rate))
}

// ============================================================================
// Message Audio API (TTS storage)
// ============================================================================

func (c *SpeechController) GetMessageAudio(messageID uint) (*speech.AudioResult, error) {
	return c.speechSvc.GetMessageAudio(messageID)
}

func (c *SpeechController) SaveMessageAudio(messageID uint, audioBase64, mimeType string) error {
	return c.speechSvc.SaveMessageAudio(messageID, audioBase64, mimeType)
}

func (c *SpeechController) GenerateAndSaveMessageAudio(messageID uint, text string) (*speech.AudioResult, error) {
	return c.speechSvc.GenerateAndSaveMessageAudio(messageID, text)
}

func (c *SpeechController) SpeakMessage(messageID uint, providerID, voiceID, model string, rate float64) (*speech.AudioResult, error) {
	return c.speechSvc.SpeakMessage(messageID, providerID, voiceID, model, rate)
}

// ============================================================================
// Voice Profile TTS/STT API
// ============================================================================

func (c *SpeechController) GetSpeechProviders() []*llm.ProviderConfig {
	return c.speechSvc.GetSpeechProviders()
}

func (c *SpeechController) GetTTSVoices(profileID, providerID string) []speech.TTSVoiceEntry {
	return c.speechSvc.GetTTSVoices(providerID)
}

func (c *SpeechController) GetTTSModels(providerID string) []speech.SpeechModelInfo {
	return c.speechSvc.GetTTSModels(providerID)
}

func (c *SpeechController) GetSTTModels(providerID string) []speech.SpeechModelInfo {
	return c.speechSvc.GetSTTModels(providerID)
}

func (c *SpeechController) SpeakPreview(providerID, voiceID, model string, rate, volume float64, text, sessionID string) error {
	return c.speechSvc.SpeakPreview(providerID, voiceID, model, rate, volume, text, sessionID)
}
