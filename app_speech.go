package main

import (
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	"assistente/internal/speech"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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
	config := speech.SpeechConfig{
		STTProvider:      speech.STTProviderWhisper,
		TTSProvider:      speech.TTSProviderOpenAI,
		OpenAIAPIKey:     apiKey,
		OpenAIAPIBaseURL: apiBaseURL,
		WhisperModel:     "whisper-1",
		WhisperLanguage:  whisperLanguage,
		TTSModel:         ttsModel,
		TTSVoice:         ttsVoice,
	}

	a.speechManager = speech.NewSpeechManager(config, a.credMgr)
	log.Printf("Speech Manager inicializado (STT: whisper, TTS: openai)")
	return nil
}

// InitSpeechManagerFromProfile inicializa o gerenciador de speech usando providers do perfil ativo
// Permite TTS e STT usarem providers diferentes do LLM (ex: Claude para chat, OpenAI para vozes)
func (a *App) InitSpeechManagerFromProfile() error {
	// Carregar perfil ativo
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		return fmt.Errorf("perfil ativo não encontrado: %w", err)
	}

	// Provider para TTS (se habilitado e usar OpenAI)
	var ttsProviderConfig *llm.ProviderConfig
	if activeProfile.Voice.Provider == "openai" && activeProfile.Voice.LLMProviderID != "" {
		ttsProviderConfig = a.llmRegistry.Get(activeProfile.Voice.LLMProviderID)
		if ttsProviderConfig == nil {
			log.Printf("[Speech] TTS provider não encontrado: %s, usando fallback", activeProfile.Voice.LLMProviderID)
		}
	}

	// Provider para STT (se usar whisper_api)
	var sttProviderConfig *llm.ProviderConfig
	if activeProfile.Interaction.STTProvider == "whisper_api" && activeProfile.Interaction.LLMProviderID != "" {
		sttProviderConfig = a.llmRegistry.Get(activeProfile.Interaction.LLMProviderID)
		if sttProviderConfig == nil {
			log.Printf("[Speech] STT provider não encontrado: %s, usando fallback", activeProfile.Interaction.LLMProviderID)
		}
	}

	// Usar provider de TTS se disponível, senão fallback para config global (migração)
	apiKey := ""
	apiBaseURL := ""
	if ttsProviderConfig != nil {
		apiBaseURL = ttsProviderConfig.BaseURL
		// Credentials serão resolvidas automaticamente pelo httpclient via credMgr
	} else if sttProviderConfig != nil {
		apiBaseURL = sttProviderConfig.BaseURL
	} else {
		// Fallback: carregar da config global (compatibilidade)
		cfg, _ := config.Load()
		if cfg != nil {
			apiKey = cfg.APIKey
			apiBaseURL = cfg.APIBaseURL
		}
	}

	// Configurar speech manager
	config := speech.SpeechConfig{
		STTProvider:      speech.STTProvider(activeProfile.Interaction.STTProvider),
		TTSProvider:      speech.TTSProvider(activeProfile.Voice.Provider),
		OpenAIAPIKey:     apiKey, // Usado apenas em fallback legacy
		OpenAIAPIBaseURL: apiBaseURL,
		WhisperModel:     "whisper-1",
		WhisperLanguage:  activeProfile.Interaction.Language,
		TTSModel:         "tts-1",
		TTSVoice:         activeProfile.Voice.VoiceID,
	}

	a.speechManager = speech.NewSpeechManager(config, a.credMgr)

	ttsInfo := "disabled"
	if ttsProviderConfig != nil {
		ttsInfo = fmt.Sprintf("%s (%s)", activeProfile.Voice.Provider, ttsProviderConfig.Name)
	} else if activeProfile.Voice.Provider != "disabled" {
		ttsInfo = activeProfile.Voice.Provider
	}

	sttInfo := activeProfile.Interaction.STTProvider
	if sttProviderConfig != nil {
		sttInfo = fmt.Sprintf("%s (%s)", sttInfo, sttProviderConfig.Name)
	}

	log.Printf("[Speech] Manager inicializado | TTS: %s | STT: %s", ttsInfo, sttInfo)
	return nil
}

// ensureSpeechManager garante que o speechManager está inicializado.
// Tenta inicializar a partir do perfil ativo.
// Retorna true se disponível, false caso contrário.
func (a *App) ensureSpeechManager() bool {
	if a.speechManager != nil {
		return true
	}

	// Tentar inicializar do perfil ativo
	if err := a.InitSpeechManagerFromProfile(); err != nil {
		log.Printf("[Speech] Erro ao inicializar speechManager do perfil: %v", err)
		return false
	}

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
		return nil, err
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
		return nil, err
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// TTSStreamEvent evento de streaming de TTS
type TTSStreamEvent struct {
	SessionID   string `json:"sessionId"`
	ChunkBase64 string `json:"chunkBase64"`
	Format      string `json:"format"`
	Done        bool   `json:"done"`
	Error       string `json:"error"`
}

// SynthesizeOpenAIStream sintetiza texto usando OpenAI TTS com streaming
func (a *App) SynthesizeOpenAIStream(text string, voice string, sessionID string) error {
	if !a.ensureSpeechManager() {
		runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
			SessionID: sessionID,
			Error:     "speech manager não disponível - configure um provedor no perfil",
		})
		return fmt.Errorf("speech manager não disponível")
	}

	if !a.speechManager.SupportsStreaming() {
		go func() {
			result, err := a.speechManager.SynthesizeWithVoice(text, voice)
			if err != nil {
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
				return
			}

			runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
				SessionID: sessionID,
				Format:    result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
				SessionID:   sessionID,
				ChunkBase64: result.AudioBase64,
				Format:      result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
				SessionID: sessionID,
				Done:      true,
			})
		}()
		return nil
	}

	go func() {
		runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
			SessionID: sessionID,
			Format:    "mp3",
		})

		callbacks := speech.StreamCallbacks{
			OnChunk: func(chunkBase64 string) {
				runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
					SessionID:   sessionID,
					ChunkBase64: chunkBase64,
					Format:      "mp3",
				})
			},
			OnDone: func() {
				runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
					SessionID: sessionID,
					Done:      true,
				})
			},
			OnError: func(err error) {
				log.Printf("[TTS] Stream error: %v", err)
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := a.speechManager.SynthesizeStream(ctx, text, voice, callbacks)
		if err != nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     err.Error(),
			})
		}
	}()

	return nil
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
		a.speechManager.SetTTSSpeed(rate)
	}
}

// ============================================================================
// Message Audio API (TTS storage)
// ============================================================================

// AudioResult é o resultado de busca/geração de áudio para o frontend.
type AudioResult struct {
	Audio    string `json:"audio"`
	MimeType string `json:"mimeType"`
}

// GetMessageAudio retorna o áudio base64 e MIME type de uma mensagem.
func (a *App) GetMessageAudio(messageID uint) (*AudioResult, error) {
	audio, mime, err := database.GetMessageAudio(messageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar áudio: %w", err)
	}
	if audio == "" {
		return nil, nil
	}
	return &AudioResult{Audio: audio, MimeType: mime}, nil
}

// SaveMessageAudio salva áudio (base64) numa mensagem existente.
// Usado pelo frontend para persistir áudio gerado via TTS OpenAI.
func (a *App) SaveMessageAudio(messageID uint, audioBase64 string, mimeType string) error {
	return database.SaveMessageAudio(messageID, audioBase64, mimeType)
}

// GenerateAndSaveMessageAudio gera áudio TTS para uma mensagem e salva no DB.
// Retorna o áudio base64 e MIME type. Usado pelo frontend para gerar+salvar+tocar.
func (a *App) GenerateAndSaveMessageAudio(messageID uint, text string) (*AudioResult, error) {
	if !a.ensureSpeechManager() {
		return nil, fmt.Errorf("speech manager indisponível")
	}

	result, err := a.speechManager.Synthesize(text)
	if err != nil {
		return nil, fmt.Errorf("erro ao sintetizar TTS: %w", err)
	}

	mimeType := "audio/mpeg"
	// Salva no DB
	if err := database.SaveMessageAudio(messageID, result.AudioBase64, mimeType); err != nil {
		log.Printf("[TTS] Erro ao salvar áudio no DB: %v", err)
		// Retorna o áudio mesmo se falhar ao salvar
	}

	return &AudioResult{Audio: result.AudioBase64, MimeType: mimeType}, nil
}
