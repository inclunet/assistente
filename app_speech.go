package main

import (
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/speech"
	"fmt"
	"log"
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

// ============================================================================
// Global Hotkey API
// ============================================================================

// HotkeyInfo informações sobre um hotkey
type HotkeyInfo struct {
	ID          int    `json:"id"`
	Modifiers   string `json:"modifiers"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados
func (a *App) IsGlobalHotkeySupported() bool {
	return hotkey.IsSupported()
}

// ============================================================================
// SAPI5 Voice Methods (Windows only)
// ============================================================================

// SAPI5VoiceInfo representa informações de uma voz SAPI5 para o frontend
type SAPI5VoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Age         string `json:"age"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// GetSAPI5Voices retorna a lista de vozes SAPI5 instaladas
func (a *App) GetSAPI5Voices() ([]SAPI5VoiceInfo, error) {
	manager := speech.GetSAPI5Manager()

	if err := manager.Initialize(); err != nil {
		log.Printf("SAPI5 Initialize error (may be expected on non-Windows): %v", err)
		return []SAPI5VoiceInfo{}, nil
	}

	voices := manager.GetVoices()
	result := make([]SAPI5VoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = SAPI5VoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Language:    v.Language,
			Gender:      v.Gender,
			Age:         v.Age,
			Vendor:      v.Vendor,
			Description: v.Description,
			Source:      v.Source,
		}
	}

	return result, nil
}

// SpeakSAPI5 sintetiza texto usando uma voz SAPI5
func (a *App) SpeakSAPI5(text string, voiceName string) error {
	manager := speech.GetSAPI5Manager()
	return manager.Speak(text, voiceName)
}

// StopSAPI5 para a síntese SAPI5 atual
func (a *App) StopSAPI5() error {
	manager := speech.GetSAPI5Manager()
	return manager.Stop()
}

// SetSAPI5Volume define o volume (0-100)
func (a *App) SetSAPI5Volume(volume int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetVolume(volume)
}

// SetSAPI5Rate define a velocidade (-10 a 10, 0 é normal)
func (a *App) SetSAPI5Rate(rate int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetRate(rate)
}

// IsSAPI5Speaking verifica se está falando
func (a *App) IsSAPI5Speaking() bool {
	manager := speech.GetSAPI5Manager()
	return manager.IsSpeaking()
}

// ============================================================================
// OpenAI Speech API Methods (Whisper STT + OpenAI TTS)
// ============================================================================

// OpenAITTSVoiceInfo representa uma voz OpenAI TTS para o frontend
type OpenAITTSVoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Gender      string `json:"gender"`
	Provider    string `json:"provider"`
}

// TranscriptionResultInfo resultado da transcrição para o frontend
type TranscriptionResultInfo struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Provider string  `json:"provider"`
}

// SynthesisResultInfo resultado da síntese para o frontend
type SynthesisResultInfo struct {
	AudioBase64 string `json:"audioBase64"`
	Format      string `json:"format"`
	Provider    string `json:"provider"`
}

// InitSpeechManager inicializa o gerenciador de speech com as configurações
// DEPRECATED: Use InitSpeechManagerFromProfile() que usa providers do perfil
func (a *App) InitSpeechManager(apiKey, apiBaseURL, whisperLanguage, ttsVoice, ttsModel string) error {
	cfg := speech.SpeechConfig{
		STTProvider:     speech.STTProviderWhisper,
		WhisperModel:    "whisper-1",
		WhisperLanguage: whisperLanguage,
		Assistant: speech.RoleVoiceConfig{
			Provider: string(speech.TTSProviderOpenAI),
			APIKey:   apiKey,
			BaseURL:  apiBaseURL,
			Voice:    ttsVoice,
			Model:    ttsModel,
			Rate:     1.0,
			Volume:   1.0,
		},
	}

	a.speechManager = speech.NewSpeechManager(cfg, a.credMgr)
	a.speechSvc.SetSpeechManager(a.speechManager)
	log.Printf("Speech Manager inicializado (STT: whisper, TTS: openai)")
	return nil
}

// InitSpeechManagerFromProfile inicializa o gerenciador de speech usando providers do perfil ativo
func (a *App) InitSpeechManagerFromProfile() error {
	if err := a.speechSvc.InitFromProfile(); err != nil {
		return err
	}
	a.speechManager = a.speechSvc.GetSpeechManager()
	return nil
}

// ensureSpeechManager garante que o speechManager está inicializado.
func (a *App) ensureSpeechManager() bool {
	if a.speechManager != nil {
		return true
	}
	if !a.speechSvc.EnsureSpeechManager() {
		return false
	}
	a.speechManager = a.speechSvc.GetSpeechManager()
	return a.speechManager != nil
}

// TranscribeWhisper transcreve áudio usando OpenAI Whisper
func (a *App) TranscribeWhisper(audioBase64 string, filename string) (*TranscriptionResultInfo, error) {
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}

	result, err := a.speechManager.Transcribe(audioBase64, filename)
	if err != nil {
		return nil, err
	}

	return &TranscriptionResultInfo{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
		Provider: result.Provider,
	}, nil
}

// SynthesizeOpenAI sintetiza texto usando OpenAI TTS
func (a *App) SynthesizeOpenAI(text string) (*SynthesisResultInfo, error) {
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}

	result, err := a.speechManager.Synthesize(text)
	if err != nil {
		return nil, fmt.Errorf("synthesize: %w", err)
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// SynthesizeOpenAIWithVoice sintetiza texto usando OpenAI TTS com uma voz específica
func (a *App) SynthesizeOpenAIWithVoice(text string, voice string) (*SynthesisResultInfo, error) {
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}

	result, err := a.speechManager.SynthesizeWithVoice(text, voice)
	if err != nil {
		return nil, fmt.Errorf("synthesize with voice %q: %w", voice, err)
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// SynthesizeOpenAIStream sintetiza texto usando OpenAI TTS com streaming
func (a *App) SynthesizeOpenAIStream(text string, voice string, sessionID string) error {
	return a.speechSvc.SynthesizeStream(text, voice, sessionID)
}

// GetOpenAITTSVoices retorna as vozes disponíveis do OpenAI TTS
func (a *App) GetOpenAITTSVoices() []OpenAITTSVoiceInfo {
	voices := speech.GetAvailableVoices()
	result := make([]OpenAITTSVoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = OpenAITTSVoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Description: v.Description,
			Gender:      v.Gender,
			Provider:    v.Provider,
		}
	}

	return result
}

// SetOpenAITTSVoice altera a voz do OpenAI TTS
func (a *App) SetOpenAITTSVoice(voice string) {
	if a.speechManager != nil {
		a.speechManager.SetTTSVoice(voice)
	}
}

// SetOpenAITTSSpeed altera a velocidade do OpenAI TTS
func (a *App) SetOpenAITTSSpeed(rate int) {
	if a.speechManager != nil {
		a.speechManager.SetTTSSpeed(float64(rate))
	}
}

// ============================================================================
// Message Audio API (TTS storage)
// ============================================================================

// AudioResult é alias para Wails bindings.
type AudioResult = speech.AudioResult

// GetMessageAudio retorna o áudio base64 e MIME type de uma mensagem.
func (a *App) GetMessageAudio(messageID uint) (*speech.AudioResult, error) {
	audio, mime, err := a.audioSvc.GetMessageAudio(messageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar áudio: %w", err)
	}
	if audio == "" {
		return nil, nil
	}
	return &speech.AudioResult{Audio: audio, MimeType: mime, Cached: true}, nil
}

// SaveMessageAudio salva áudio (base64) numa mensagem existente.
func (a *App) SaveMessageAudio(messageID uint, audioBase64 string, mimeType string) error {
	return a.audioSvc.SaveMessageAudio(messageID, audioBase64, mimeType)
}

// GenerateAndSaveMessageAudio gera áudio TTS para uma mensagem e salva no DB.
func (a *App) GenerateAndSaveMessageAudio(messageID uint, text string) (*speech.AudioResult, error) {
	return a.speechSvc.GenerateAndSaveMessageAudio(messageID, text)
}

// SpeakMessage retorna o áudio de uma mensagem, usando cache do DB se disponível.
func (a *App) SpeakMessage(messageID uint, providerID string, voiceID string, model string, rate float64) (*speech.AudioResult, error) {
	return a.speechSvc.SpeakMessage(messageID, providerID, voiceID, model, rate)
}

// ============================================================================
// Voice Profile TTS/STT API
// ============================================================================

// GetSpeechProviders retorna todos os provedores LLM que suportam TTS ou STT.
func (a *App) GetSpeechProviders() []*llm.ProviderConfig {
	all := a.GetLLMProviders()
	result := make([]*llm.ProviderConfig, 0, len(all))
	for _, p := range all {
		if p.SupportsTTS() || p.SupportsSTT() {
			result = append(result, p)
		}
	}
	return result
}

// TTSVoiceEntry é alias para Wails bindings.
type TTSVoiceEntry = speech.TTSVoiceEntry

// GetTTSVoices retorna vozes TTS disponíveis para um provedor.
func (a *App) GetTTSVoices(profileID string, providerID string) []speech.TTSVoiceEntry {
	return a.speechSvc.GetTTSVoices(providerID)
}

// GetTTSModels retorna modelos TTS disponíveis para um provedor.
func (a *App) GetTTSModels(providerID string) []speech.SpeechModelInfo {
	return a.speechSvc.GetTTSModels(providerID)
}

// GetSTTModels retorna modelos STT disponíveis para um provedor.
func (a *App) GetSTTModels(providerID string) []speech.SpeechModelInfo {
	return a.speechSvc.GetSTTModels(providerID)
}

// SpeakPreview faz preview de voz para configurações de perfil.
func (a *App) SpeakPreview(providerId string, voiceId string, model string, rate float64, volume float64, text string, sessionId string) error {
	return a.speechSvc.SpeakPreview(providerId, voiceId, model, rate, volume, text, sessionId)
}

// TTSStreamEvent é alias para Wails bindings.
type TTSStreamEvent = speech.TTSStreamEvent

// ============================================================================
// TTS Client Helpers (usados por outros arquivos root)
// ============================================================================

// createTTSClientForProvider delega para speechSvc.
func (a *App) createTTSClientForProvider(providerID string, model string) *speech.TTSClient {
	return a.speechSvc.CreateTTSClient(providerID, model)
}

// findOpenAILikeProvider delega para speechSvc.
func (a *App) findOpenAILikeProvider() *llm.ProviderConfig {
	return a.speechSvc.FindOpenAILikeProvider()
}
