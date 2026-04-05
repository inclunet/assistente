package speech

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"

	"assistente/internal/credentials"
)

// STTProvider tipos de provedores de STT
type STTProvider string

const (
	STTProviderWebSpeech STTProvider = "webspeech"
	STTProviderWhisper   STTProvider = "whisper"
)

// TTSProvider tipos de provedores de TTS
type TTSProvider string

const (
	TTSProviderWebSpeech TTSProvider = "webspeech"
	TTSProviderSAPI5     TTSProvider = "sapi5"
	TTSProviderOpenAI    TTSProvider = "openai"
)

// SpeechConfig configuração de speech derivada do perfil ativo
type SpeechConfig struct {
	// STT
	STTProvider          STTProvider `json:"sttProvider"`
	STTAPIBaseURL        string      `json:"-"` // Base URL para Whisper API
	STTCredentialPattern string      `json:"-"` // padrão para credential transport
	WhisperModel         string      `json:"whisperModel"`
	WhisperLanguage      string      `json:"whisperLanguage"`

	// TTS — configuração por role
	Assistant RoleVoiceConfig `json:"assistant"`
	User      RoleVoiceConfig `json:"user"`
	System    RoleVoiceConfig `json:"system"`
}

// RoleVoiceConfig configuração de voz para um role (assistant, user, system)
type RoleVoiceConfig struct {
	Provider          string  `json:"provider"` // "disabled", "webspeech", "sapi5", "openai"
	APIKey            string  `json:"-"`
	BaseURL           string  `json:"-"`
	CredentialPattern string  `json:"-"` // para resolução lazy de credenciais
	Voice             string  `json:"voice"`
	Model             string  `json:"model"`   // "tts-1", "tts-1-hd"
	Rate              float64 `json:"rate"`     // 0.25–4.0 (speed para OpenAI)
	Volume            float64 `json:"volume"`   // 0.0–1.0
}

// SpeechManager gerenciador central de speech
type SpeechManager struct {
	config        SpeechConfig
	whisperClient *WhisperClient
	ttsClients    map[string]*TTSClient // key: "assistant", "user", "system"
	credMgr       *credentials.Manager
	mu            sync.RWMutex
}

// TranscriptionResult resultado da transcrição
type TranscriptionResult struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Provider string  `json:"provider"`
}

// SynthesisResult resultado da síntese
type SynthesisResult struct {
	AudioData   []byte `json:"audioData,omitempty"`
	AudioBase64 string `json:"audioBase64,omitempty"`
	Format      string `json:"format"`
	Provider    string `json:"provider"`
}

// NewSpeechManager cria um novo gerenciador de speech
func NewSpeechManager(config SpeechConfig, credMgr *credentials.Manager) *SpeechManager {
	sm := &SpeechManager{
		config:  config,
		credMgr: credMgr,
	}

	sm.reinitClients()
	return sm
}

// reinitClients inicializa os clientes com base na config atual
func (sm *SpeechManager) reinitClients() {
	// Whisper STT client — usa SDK openai-go com CredentialTransport
	if sm.config.STTCredentialPattern != "" {
		log.Printf("[Speech] reinitClients: criando WhisperClient (baseURL=%q, credPattern=%q, model=%q, lang=%q)",
			sm.config.STTAPIBaseURL, sm.config.STTCredentialPattern,
			sm.config.WhisperModel, sm.config.WhisperLanguage)
		sm.whisperClient = NewWhisperClient(WhisperConfig{
			BaseURL:           sm.config.STTAPIBaseURL,
			CredentialPattern: sm.config.STTCredentialPattern,
			Model:             sm.config.WhisperModel,
			Language:          sm.config.WhisperLanguage,
		}, sm.credMgr)
	} else {
		sm.whisperClient = nil
	}

	// TTS clients por role — cria para cada role com provider OpenAI
	sm.ttsClients = make(map[string]*TTSClient, 3)
	for _, entry := range []struct {
		name string
		role RoleVoiceConfig
	}{
		{"assistant", sm.config.Assistant},
		{"user", sm.config.User},
		{"system", sm.config.System},
	} {
		if entry.role.Provider == string(TTSProviderOpenAI) && entry.role.CredentialPattern != "" {
			log.Printf("[Speech] reinitClients: criando TTSClient[%s] (baseURL=%q, credPattern=%q, voice=%q, model=%q)",
				entry.name, entry.role.BaseURL, entry.role.CredentialPattern,
				entry.role.Voice, entry.role.Model)
			speed := entry.role.Rate
			if speed < 0.25 {
				speed = 1.0
			}
			sm.ttsClients[entry.name] = NewTTSClient(TTSConfig{
				BaseURL:           entry.role.BaseURL,
				CredentialPattern: entry.role.CredentialPattern,
				Model:             TTSModel(entry.role.Model),
				Voice:             TTSVoice(entry.role.Voice),
				Speed:             speed,
			}, sm.credMgr)
		}
	}

	if len(sm.ttsClients) == 0 {
		log.Printf("[Speech] reinitClients: nenhum TTSClient criado (nenhum role com provider OpenAI)")
	}
}

// UpdateConfig atualiza a configuração
func (sm *SpeechManager) UpdateConfig(config SpeechConfig) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config = config
	sm.reinitClients()
}

// Transcribe transcreve áudio para texto
// audioBase64: áudio codificado em base64
// filename: nome do arquivo com extensão
func (sm *SpeechManager) Transcribe(audioBase64 string, filename string) (*TranscriptionResult, error) {
	return sm.TranscribeWithContext(context.Background(), audioBase64, filename)
}

// TranscribeWithContext transcreve áudio com context cancelável (suporta timeout/barge-in).
func (sm *SpeechManager) TranscribeWithContext(ctx context.Context, audioBase64 string, filename string) (*TranscriptionResult, error) {
	sm.mu.RLock()
	provider := sm.config.STTProvider
	sm.mu.RUnlock()

	switch provider {
	case STTProviderWhisper, "whisper_api":
		return sm.transcribeWhisperCtx(ctx, audioBase64, filename)
	case STTProviderWebSpeech:
		return nil, fmt.Errorf("WebSpeech STT is handled in the frontend")
	default:
		return nil, fmt.Errorf("unknown STT provider: %s", provider)
	}
}

// transcribeWhisper transcreve usando Whisper
func (sm *SpeechManager) transcribeWhisper(audioBase64 string, filename string) (*TranscriptionResult, error) {
	return sm.transcribeWhisperCtx(context.Background(), audioBase64, filename)
}

// transcribeWhisperCtx transcreve usando Whisper com context cancelável.
func (sm *SpeechManager) transcribeWhisperCtx(ctx context.Context, audioBase64 string, filename string) (*TranscriptionResult, error) {
	sm.mu.RLock()
	client := sm.whisperClient
	sm.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("Whisper client not configured")
	}

	// Decodifica o base64
	audioData, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio: %w", err)
	}

	// Transcreve com context cancelável
	text, err := client.TranscribeWithContext(ctx, audioData, filename)
	if err != nil {
		return nil, err
	}

	return &TranscriptionResult{
		Text:     text,
		Provider: "whisper",
	}, nil
}

// getTTSClientForRole retorna o TTSClient para um role específico.
// Roles válidos: "assistant", "user", "system". Deve ser chamado com sm.mu held.
func (sm *SpeechManager) getTTSClientForRole(role string) *TTSClient {
	if sm.ttsClients == nil {
		return nil
	}
	return sm.ttsClients[role]
}

// getRoleConfig retorna o RoleVoiceConfig para um role. Deve ser chamado com sm.mu held.
func (sm *SpeechManager) getRoleConfig(role string) RoleVoiceConfig {
	switch role {
	case "user":
		return sm.config.User
	case "system":
		return sm.config.System
	default:
		return sm.config.Assistant
	}
}

// Synthesize converte texto em áudio usando o role assistant.
func (sm *SpeechManager) Synthesize(text string) (*SynthesisResult, error) {
	return sm.SynthesizeForRole("assistant", text)
}

// SynthesizeForRole converte texto em áudio para um role específico.
func (sm *SpeechManager) SynthesizeForRole(role string, text string) (*SynthesisResult, error) {
	sm.mu.RLock()
	rc := sm.getRoleConfig(role)
	client := sm.getTTSClientForRole(role)
	sm.mu.RUnlock()

	switch TTSProvider(rc.Provider) {
	case TTSProviderOpenAI:
		if client == nil {
			return nil, fmt.Errorf("TTS client not configured for role %s", role)
		}
		audioData, err := client.Synthesize(text)
		if err != nil {
			return nil, err
		}
		audioBase64 := base64.StdEncoding.EncodeToString(audioData)
		return &SynthesisResult{
			AudioData:   audioData,
			AudioBase64: audioBase64,
			Format:      "mp3",
			Provider:    "openai",
		}, nil
	case TTSProviderSAPI5:
		return nil, fmt.Errorf("SAPI5 TTS is handled directly via SpeakSAPI5")
	case TTSProviderWebSpeech:
		return nil, fmt.Errorf("WebSpeech TTS is handled in the frontend")
	default:
		return nil, fmt.Errorf("unknown TTS provider: %s", rc.Provider)
	}
}

// SynthesizeWithVoice sintetiza com uma voz específica (role assistant).
func (sm *SpeechManager) SynthesizeWithVoice(text string, voice string) (*SynthesisResult, error) {
	return sm.SynthesizeWithVoiceForRole("assistant", text, voice)
}

// SynthesizeWithVoiceForRole sintetiza com voz específica para um role.
func (sm *SpeechManager) SynthesizeWithVoiceForRole(role string, text string, voice string) (*SynthesisResult, error) {
	sm.mu.RLock()
	client := sm.getTTSClientForRole(role)
	sm.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("TTS client not configured for role %s", role)
	}

	audioData, err := client.SynthesizeWithVoice(text, TTSVoice(voice))
	if err != nil {
		return nil, err
	}

	audioBase64 := base64.StdEncoding.EncodeToString(audioData)
	return &SynthesisResult{
		AudioData:   audioData,
		AudioBase64: audioBase64,
		Format:      "mp3",
		Provider:    "openai",
	}, nil
}

// StreamCallbacks callbacks padronizados para streaming de TTS
// Interface unificada para todos os provedores
type StreamCallbacks struct {
	OnChunk func(chunkBase64 string) // Chunk de áudio em base64
	OnDone  func()                   // Streaming concluído
	OnError func(err error)          // Erro no streaming
}

// SynthesizeStream sintetiza com streaming (role assistant).
func (sm *SpeechManager) SynthesizeStream(ctx context.Context, text string, voice string, callbacks StreamCallbacks) error {
	return sm.SynthesizeStreamForRole(ctx, "assistant", text, voice, callbacks)
}

// SynthesizeStreamForRole sintetiza com streaming para um role específico.
func (sm *SpeechManager) SynthesizeStreamForRole(ctx context.Context, role string, text string, voice string, callbacks StreamCallbacks) error {
	sm.mu.RLock()
	rc := sm.getRoleConfig(role)
	client := sm.getTTSClientForRole(role)
	sm.mu.RUnlock()

	switch TTSProvider(rc.Provider) {
	case TTSProviderOpenAI:
		return sm.synthesizeStreamOpenAI(ctx, text, voice, client, callbacks)
	default:
		return fmt.Errorf("streaming not supported for provider: %s", rc.Provider)
	}
}

// synthesizeStreamOpenAI implementa streaming para OpenAI
func (sm *SpeechManager) synthesizeStreamOpenAI(ctx context.Context, text string, voice string, client *TTSClient, callbacks StreamCallbacks) error {
	if client == nil {
		return fmt.Errorf("TTS client not configured")
	}

	// Callbacks internos que convertem bytes para base64
	internalCallbacks := TTSStreamCallbacks{
		OnChunk: func(chunk []byte) {
			if callbacks.OnChunk != nil {
				// Converte para base64 para enviar ao frontend
				chunkBase64 := base64.StdEncoding.EncodeToString(chunk)
				callbacks.OnChunk(chunkBase64)
			}
		},
		OnDone: func() {
			if callbacks.OnDone != nil {
				callbacks.OnDone()
			}
		},
		OnError: func(err error) {
			if callbacks.OnError != nil {
				callbacks.OnError(err)
			}
		},
	}

	// Usa a voz especificada ou a padrão
	if voice != "" {
		return client.SynthesizeStreamWithVoice(ctx, text, TTSVoice(voice), internalCallbacks)
	}
	return client.SynthesizeStream(ctx, text, internalCallbacks)
}

// SupportsStreaming retorna true se o provedor do role assistant suporta streaming.
func (sm *SpeechManager) SupportsStreaming() bool {
	sm.mu.RLock()
	provider := sm.config.Assistant.Provider
	client := sm.getTTSClientForRole("assistant")
	sm.mu.RUnlock()

	switch TTSProvider(provider) {
	case TTSProviderOpenAI:
		return client != nil
	default:
		return false
	}
}

// HasTTSClient retorna true se o client TTS do assistant está inicializado.
func (sm *SpeechManager) HasTTSClient() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.getTTSClientForRole("assistant") != nil
}

// GetTTSClient retorna o client TTS do assistant (pode ser nil).
func (sm *SpeechManager) GetTTSClient() *TTSClient {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.getTTSClientForRole("assistant")
}

// GetAvailableSTTProviders retorna os provedores de STT disponíveis
func (sm *SpeechManager) GetAvailableSTTProviders() []string {
	providers := []string{"webspeech"}

	sm.mu.RLock()
	hasWhisper := sm.config.STTCredentialPattern != ""
	sm.mu.RUnlock()

	if hasWhisper {
		providers = append(providers, "whisper")
	}

	return providers
}

// GetAvailableTTSProviders retorna os provedores de TTS disponíveis
func (sm *SpeechManager) GetAvailableTTSProviders() []string {
	providers := []string{"webspeech", "sapi5"}

	sm.mu.RLock()
	hasOpenAI := sm.config.Assistant.Provider == string(TTSProviderOpenAI) &&
		(sm.config.Assistant.APIKey != "" || sm.config.Assistant.CredentialPattern != "")
	sm.mu.RUnlock()

	if hasOpenAI {
		providers = append(providers, "openai")
	}

	return providers
}

// GetAvailableTTSVoices retorna as vozes disponíveis no provedor dinâmico.
func (sm *SpeechManager) GetAvailableTTSVoices() ([]TTSVoiceInfo, error) {
	sm.mu.RLock()
	client := sm.getTTSClientForRole("assistant")
	provider := sm.config.Assistant.Provider
	sm.mu.RUnlock()

	// Se for OpenAI-like e temos cliente, tentamos buscar dinamicamente
	if TTSProvider(provider) == TTSProviderOpenAI && client != nil {
		return client.FetchVoices()
	}

	// Fallback para vozes estáticas para outros provedores (WebSpeech, SAPI5 expostos no frontend)
	return GetAvailableVoices(), nil
}

// GetOpenAIVoices retorna as vozes disponíveis do OpenAI TTS (Legacy)
func (sm *SpeechManager) GetOpenAIVoices() []TTSVoiceInfo {
	return GetAvailableVoices()
}

// clampSpeed garante que o speed está dentro dos limites da API OpenAI (0.25–4.0).
func clampSpeed(speed float64) float64 {
	if speed < 0.25 {
		return 0.25
	}
	if speed > 4.0 {
		return 4.0
	}
	return speed
}

// SetTTSVoice altera a voz do TTS (assistant role)
func (sm *SpeechManager) SetTTSVoice(voice string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.Assistant.Voice = voice
	if client := sm.ttsClients["assistant"]; client != nil {
		client.SetVoice(TTSVoice(voice))
	}
}

// SetTTSSpeed altera a velocidade do TTS (assistant role).
// speed: 0.25–4.0 (valores do OpenAI TTS).
func (sm *SpeechManager) SetTTSSpeed(speed float64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.Assistant.Rate = speed
	if client := sm.ttsClients["assistant"]; client != nil {
		client.SetSpeed(clampSpeed(speed))
	}
}

// SetTTSModel altera o modelo do TTS (assistant role)
func (sm *SpeechManager) SetTTSModel(model string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.Assistant.Model = model
	if client := sm.ttsClients["assistant"]; client != nil {
		client.SetModel(TTSModel(model))
	}
}
