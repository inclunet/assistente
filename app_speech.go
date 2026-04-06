package main

import (
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/speech"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
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
	log.Printf("Speech Manager inicializado (STT: whisper, TTS: openai)")
	return nil
}

// InitSpeechManagerFromProfile inicializa o gerenciador de speech usando providers do perfil ativo
func (a *App) InitSpeechManagerFromProfile() error {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		return fmt.Errorf("perfil ativo não encontrado: %w", err)
	}

	sm := a.createSpeechManagerForProfile(activeProfile)
	if sm == nil {
		return fmt.Errorf("falha ao criar speech manager para perfil ativo")
	}
	a.speechManager = sm
	return nil
}

// createSpeechManagerForProfile cria um SpeechManager configurado a partir de um perfil.
// Resolve defaults e credenciais. Retorna nil se o perfil for nil.
func (a *App) createSpeechManagerForProfile(p *profiles.Profile) *speech.SpeechManager {
	if p == nil {
		return nil
	}

	// Resolve defaults ($default → provider ID real)
	p = a.resolveProfileDefaults(p)

	// Resolve credenciais para TTS
	var ttsAPIKey, ttsBaseURL, ttsCredPattern string
	if p.Voice.LLMProviderID != "" {
		cfg := a.llmRegistry.Get(p.Voice.LLMProviderID)
		if cfg != nil {
			ttsBaseURL = cfg.BaseURL
			ttsCredPattern = cfg.CredentialPattern
			if cfg.CredentialPattern != "" {
				if auth, err := a.credMgr.GetByPattern(cfg.CredentialPattern); err == nil && auth != nil {
					ttsAPIKey = auth.Token
				}
			}
		} else {
			log.Printf("[Speech] TTS Provider '%s' não encontrado no registry", p.Voice.LLMProviderID)
		}
	}

	// Build role config from flat VoiceConfig (same config for all roles)
	assistantCfg := speech.RoleVoiceConfig{
		Provider:          p.Voice.Provider,
		APIKey:            ttsAPIKey,
		BaseURL:           ttsBaseURL,
		CredentialPattern: ttsCredPattern,
		Voice:             p.Voice.VoiceID,
		Model:             "tts-1",
		Rate:              p.Voice.Rate,
		Volume:            p.Voice.Volume,
	}

	// Log detalhado para diagnóstico de TTS
	if p.Voice.Provider == "openai" && ttsAPIKey == "" {
		log.Printf("[Speech] AVISO: Provider é 'openai' mas API key está vazia. "+
			"LLMProviderID='%s'", p.Voice.LLMProviderID)
	}

	// Resolve credenciais STT
	var sttURL, sttCredPattern string
	if p.Interaction.LLMProviderID != "" {
		cfg := a.llmRegistry.Get(p.Interaction.LLMProviderID)
		if cfg != nil {
			sttURL = cfg.BaseURL
			sttCredPattern = cfg.CredentialPattern
		}
	}

	speechCfg := speech.SpeechConfig{
		// STT
		STTProvider:          speech.STTProvider(p.Interaction.STTProvider),
		STTAPIBaseURL:        sttURL,
		STTCredentialPattern: sttCredPattern,
		WhisperModel:         "whisper-1",
		WhisperLanguage:      p.Interaction.Language,

		// TTS — uses same config for all roles (flat profile VoiceConfig)
		Assistant: assistantCfg,
		User:      assistantCfg,
		System:    assistantCfg,
	}

	sm := speech.NewSpeechManager(speechCfg, a.credMgr)

	log.Printf("[Speech] Manager inicializado | TTS: %s | STT: %s",
		p.Voice.Provider, p.Interaction.STTProvider)
	return sm
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
		a.speechManager.SetTTSSpeed(float64(rate))
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

// ============================================================================
// Voice Profile TTS/STT API (feat/voice-profile-settings)
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

// GetTTSModels retorna modelos TTS disponíveis para um provedor.
// Busca dinamicamente via /v1/models, com fallback para lista estática.
func (a *App) GetTTSModels(providerID string) []speech.SpeechModelInfo {
	if providerID == "" {
		return []speech.SpeechModelInfo{}
	}
	client := a.createTTSClientForProvider(providerID, "")
	if client == nil {
		return speech.StaticTTSModels()
	}
	return client.FetchTTSModels()
}

// GetSTTModels retorna modelos STT disponíveis para um provedor.
// Busca dinamicamente via /v1/models, com fallback para lista estática.
func (a *App) GetSTTModels(providerID string) []speech.SpeechModelInfo {
	if providerID == "" {
		return []speech.SpeechModelInfo{}
	}
	client := a.createTTSClientForProvider(providerID, "")
	if client == nil {
		return speech.StaticSTTModels()
	}
	return client.FetchSTTModels()
}

// SpeakPreview faz preview de voz para configurações de perfil.
// Suporta webspeech, sapi5 e provedores OpenAI-like (com streaming).
func (a *App) SpeakPreview(providerId string, voiceId string, model string, rate float64, volume float64, text string, sessionId string) error {
	if text == "" {
		text = "Este é um teste das configurações de voz"
	}

	log.Printf("[SpeakPreview] provider=%s, voice=%s, model=%s, rate=%.2f, volume=%.2f", providerId, voiceId, model, rate, volume)

	switch providerId {
	case "webspeech":
		// WebSpeech é handled no frontend — este caso não deveria chegar aqui
		return fmt.Errorf("webspeech preview deve ser feito no frontend")

	case "sapi5":
		// SAPI5 usa COM do Windows — delega ao manager
		manager := speech.GetSAPI5Manager()
		sapiRate := int((rate - 1.0) * 10) // 0.5→-5, 1.0→0, 2.0→10
		sapiVolume := int(volume * 100)
		if err := manager.SetRate(sapiRate); err != nil {
			log.Printf("[SpeakPreview] SetRate error: %v", err)
		}
		if err := manager.SetVolume(sapiVolume); err != nil {
			log.Printf("[SpeakPreview] SetVolume error: %v", err)
		}
		return manager.Speak(text, voiceId)

	default:
		// LLM provider (OpenAI-like) — cria TTSClient com provider específico
		client := a.createTTSClientForProvider(providerId, model)
		if client == nil {
			// Fallback para ad-hoc
			client = a.getOrCreateAdHocTTSClient()
		}
		if client == nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionId,
				Error:     "nenhum provedor OpenAI com credenciais encontrado",
			})
			return fmt.Errorf("no TTS provider available for %s", providerId)
		}

		go func() {
			runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
				SessionID: sessionId,
				Format:    "mp3",
			})

			callbacks := speech.TTSStreamCallbacks{
				OnChunk: func(chunk []byte) {
					chunkBase64 := base64.StdEncoding.EncodeToString(chunk)
					runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
						SessionID:   sessionId,
						ChunkBase64: chunkBase64,
						Format:      "mp3",
					})
				},
				OnDone: func() {
					runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
						SessionID: sessionId,
						Done:      true,
					})
				},
				OnError: func(err error) {
					log.Printf("[SpeakPreview] Stream error: %v", err)
					runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
						SessionID: sessionId,
						Error:     err.Error(),
					})
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			voice := speech.TTSVoice(voiceId)

			// Para provedores dinâmicos (LocalAI/Piper), a "voz" selecionada é na
			// verdade um modelo TTS (ex: "voice-pt_BR-cadu-medium"). Nesse caso,
			// precisamos usar como model (não como voice) na chamada à API.
			if speech.IsDynamicTTSModel(voiceId) {
				client.SetModel(speech.TTSModel(voiceId))
				voice = speech.TTSVoice(voiceId)
			}

			if err := client.SynthesizeStreamWithVoice(ctx, text, voice, callbacks); err != nil {
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionId,
					Error:     err.Error(),
				})
			}
		}()

		return nil
	}
}

// ============================================================================
// TTS Client Helpers (feat/voice-profile-settings)
// ============================================================================

// getOrCreateAdHocTTSClient retorna um TTSClient funcional.
// Primeiro verifica se o speechManager já tem um; se não, cria um ad-hoc
// usando credenciais do LLM provider default.
// Isso permite preview de voz mesmo quando o perfil ainda não salvou TTS como "openai".
func (a *App) getOrCreateAdHocTTSClient() *speech.TTSClient {
	// 1. Tenta o speech manager existente
	if a.speechManager != nil && a.speechManager.HasTTSClient() {
		return a.speechManager.GetTTSClient()
	}

	// 2. Tenta reinicializar do perfil
	if err := a.InitSpeechManagerFromProfile(); err == nil && a.speechManager != nil && a.speechManager.HasTTSClient() {
		return a.speechManager.GetTTSClient()
	}

	// 3. Fallback: cria ad-hoc com provider default ou primeiro provider disponível
	provider := a.findOpenAILikeProvider()
	if provider == nil {
		return nil
	}

	// Lê o modelo TTS do perfil ativo (se existir)
	var model speech.TTSModel
	if profile, err := a.profileManager.GetActive(); err == nil && profile != nil {
		resolved := a.resolveProfileDefaults(profile)
		_ = resolved // Voice config is flat — no per-role model override yet
	}

	log.Printf("[TTS] Criando TTSClient ad-hoc com provider %s (baseURL=%s, model=%s)", provider.ID, provider.BaseURL, model)
	return speech.NewTTSClient(speech.TTSConfig{
		BaseURL:           provider.BaseURL,
		CredentialPattern: provider.CredentialPattern,
		Model:             model,
	}, a.credMgr)
}

// createTTSClientForProvider cria um TTSClient para um provider LLM específico.
// Usado quando o frontend sabe exatamente qual provider quer usar (ex: preview de voz).
func (a *App) createTTSClientForProvider(providerID string, model string) *speech.TTSClient {
	cfg := a.llmRegistry.Get(providerID)
	if cfg == nil {
		log.Printf("[TTS] Provider %s não encontrado", providerID)
		return nil
	}

	return speech.NewTTSClient(speech.TTSConfig{
		BaseURL:           cfg.BaseURL,
		CredentialPattern: cfg.CredentialPattern,
		Model:             speech.TTSModel(model),
	}, a.credMgr)
}

// findOpenAILikeProvider procura um provider LLM com API OpenAI-compatible que suporte TTS.
// Provedores Google e Anthropic não suportam a API /audio/speech.
// Prefere providers com a API oficial do OpenAI (api.openai.com) sobre proxies.
func (a *App) findOpenAILikeProvider() *llm.ProviderConfig {
	isOpenAILike := func(cfg *llm.ProviderConfig) bool {
		if cfg == nil {
			return false
		}
		format := cfg.GetAPIFormat()
		return format == llm.APIFormatOpenAI || format == llm.APIFormatOpenAIResponses
	}

	isOfficialOpenAI := func(cfg *llm.ProviderConfig) bool {
		return cfg.BaseURL == "" || strings.Contains(cfg.BaseURL, "api.openai.com")
	}

	// Tenta o provider do perfil ativo primeiro (voice → chat → default)
	if profile, err := a.profileManager.GetActive(); err == nil && profile != nil {
		resolved := a.resolveProfileDefaults(profile)
		if resolved.Voice.LLMProviderID != "" {
			if cfg := a.llmRegistry.Get(resolved.Voice.LLMProviderID); isOpenAILike(cfg) {
				return cfg
			}
		}
		if resolved.Chat.LLMProvider != "" {
			if cfg := a.llmRegistry.Get(resolved.Chat.LLMProvider); isOpenAILike(cfg) {
				return cfg
			}
		}
	}

	// Tenta o provider default do sistema
	if dp, err := database.GetDefaultProvider(); err == nil && dp != nil {
		if cfg := a.llmRegistry.Get(dp.ID); isOpenAILike(cfg) {
			return cfg
		}
	}

	// Último recurso: prefere providers com URL oficial do OpenAI
	var fallbackProxy *llm.ProviderConfig
	for _, cfg := range a.llmRegistry.List() {
		if isOpenAILike(cfg) {
			if isOfficialOpenAI(cfg) {
				return cfg
			}
			if fallbackProxy == nil {
				fallbackProxy = cfg
			}
		}
	}

	return fallbackProxy
}
