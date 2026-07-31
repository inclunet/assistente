package controllers

import (
	"context"

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

func (c *SpeechController) InitFromProfile(ctx context.Context) error {
	return c.speechSvc.InitFromProfile(ctx)
}

func (c *SpeechController) Transcribe(ctx context.Context, audioBase64, filename string) (*speech.TranscriptionResult, error) {
	return c.speechSvc.Transcribe(ctx, audioBase64, filename)
}

func (c *SpeechController) Synthesize(ctx context.Context, text string) (*speech.SynthesisResult, error) {
	return c.speechSvc.Synthesize(ctx, text)
}

func (c *SpeechController) SynthesizeWithVoice(ctx context.Context, text, voice string) (*speech.SynthesisResult, error) {
	return c.speechSvc.SynthesizeWithVoice(ctx, text, voice)
}

func (c *SpeechController) SynthesizeStream(ctx context.Context, text, voice, sessionID string) error {
	return c.speechSvc.SynthesizeStream(ctx, text, voice, sessionID)
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

func (c *SpeechController) GetMessageAudio(ctx context.Context, messageID string) (*speech.AudioResult, error) {
	return c.speechSvc.GetMessageAudio(ctx, messageID)
}

func (c *SpeechController) SaveMessageAudio(ctx context.Context, messageID string, audioBase64, mimeType string) error {
	return c.speechSvc.SaveMessageAudio(ctx, messageID, audioBase64, mimeType)
}

func (c *SpeechController) GenerateAndSaveMessageAudio(ctx context.Context, messageID string, text string) (*speech.AudioResult, error) {
	return c.speechSvc.GenerateAndSaveMessageAudio(ctx, messageID, text)
}

func (c *SpeechController) SpeakMessage(ctx context.Context, messageID string, providerID, model, voiceID string, rate float64, language string) (*speech.AudioResult, error) {
	return c.speechSvc.SpeakMessage(ctx, messageID, providerID, model, voiceID, rate, language)
}

// ============================================================================
// Voice Profile TTS/STT API
// ============================================================================

func (c *SpeechController) GetSpeechProviders() []*llm.ProviderConfig {
	return c.speechSvc.GetSpeechProviders()
}

func (c *SpeechController) GetTTSModels(ctx context.Context, providerID string) []speech.TTSModelInfo {
	return c.speechSvc.GetTTSModels(ctx, providerID)
}

func (c *SpeechController) GetTTSVoices(ctx context.Context, providerID, modelID string) []speech.TTSVoiceInfo {
	return c.speechSvc.GetTTSVoices(ctx, providerID, modelID)
}

func (c *SpeechController) GetSTTModels(ctx context.Context, providerID string) []speech.SpeechModelInfo {
	return c.speechSvc.GetSTTModels(ctx, providerID)
}

func (c *SpeechController) SpeakPreview(ctx context.Context, providerID, model, voiceID string, rate, volume float64, language, text, sessionID string) error {
	return c.speechSvc.SpeakPreview(ctx, speech.SpeakPreviewParams{
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
