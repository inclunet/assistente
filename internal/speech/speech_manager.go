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
	AssistantProvider          string `json:"assistantProvider"` // "disabled", "webspeech", "sapi5", "openai"
	AssistantAPIKey            string `json:"-"`
	AssistantBaseURL           string `json:"-"`
	AssistantCredentialPattern string `json:"-"` // para resolução lazy de credenciais
	AssistantVoice             string `json:"assistantVoice"`
	AssistantModel             string `json:"assistantModel"` // "tts-1", "tts-1-hd"
	AssistantRate              int    `json:"assistantRate"`
	AssistantVolume            int    `json:"assistantVolume"` // 0-100

	UserProvider string `json:"userProvider"`
	UserAPIKey   string `json:"-"`
	UserBaseURL  string `json:"-"`
	UserVoice    string `json:"userVoice"`
	UserModel    string `json:"userModel"`
	UserRate     int    `json:"userRate"`
	UserVolume   int    `json:"userVolume"`

	SystemProvider string `json:"systemProvider"`
	SystemAPIKey   string `json:"-"`
	SystemBaseURL  string `json:"-"`
	SystemVoice    string `json:"systemVoice"`
	SystemModel    string `json:"systemModel"`
	SystemRate     int    `json:"systemRate"`
	SystemVolume   int    `json:"systemVolume"`
}

// SpeechManager gerenciador central de speech
type SpeechManager struct {
	config        SpeechConfig
	whisperClient *WhisperClient
	ttsClient     *TTSClient
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

	// TTS client — cria sempre que provider é OpenAI (credenciais via transport)
	if sm.config.AssistantProvider == string(TTSProviderOpenAI) {
		log.Printf("[Speech] reinitClients: criando TTSClient (provider=%q, baseURL=%q, credPattern=%q, voice=%q, model=%q)",
			sm.config.AssistantProvider, sm.config.AssistantBaseURL, sm.config.AssistantCredentialPattern,
			sm.config.AssistantVoice, sm.config.AssistantModel)
		sm.ttsClient = NewTTSClient(TTSConfig{
			BaseURL:           sm.config.AssistantBaseURL,
			CredentialPattern: sm.config.AssistantCredentialPattern,
			Model:             TTSModel(sm.config.AssistantModel),
			Voice:             TTSVoice(sm.config.AssistantVoice),
			Speed:             sm.rateToSpeed(sm.config.AssistantRate),
		}, sm.credMgr)
	} else {
		log.Printf("[Speech] reinitClients: TTSClient NÃO criado — AssistantProvider=%q (esperado %q)",
			sm.config.AssistantProvider, TTSProviderOpenAI)
		sm.ttsClient = nil
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

// Synthesize converte texto em áudio
func (sm *SpeechManager) Synthesize(text string) (*SynthesisResult, error) {
	sm.mu.RLock()
	provider := sm.config.AssistantProvider
	sm.mu.RUnlock()

	switch TTSProvider(provider) {
	case TTSProviderOpenAI:
		return sm.synthesizeOpenAI(text)
	case TTSProviderSAPI5:
		return nil, fmt.Errorf("SAPI5 TTS is handled directly via SpeakSAPI5")
	case TTSProviderWebSpeech:
		return nil, fmt.Errorf("WebSpeech TTS is handled in the frontend")
	default:
		return nil, fmt.Errorf("unknown TTS provider: %s", provider)
	}
}

// synthesizeOpenAI sintetiza usando OpenAI TTS
func (sm *SpeechManager) synthesizeOpenAI(text string) (*SynthesisResult, error) {
	sm.mu.RLock()
	client := sm.ttsClient
	sm.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("TTS client not configured")
	}

	// Sintetiza
	audioData, err := client.Synthesize(text)
	if err != nil {
		return nil, err
	}

	// Codifica em base64 para enviar ao frontend
	audioBase64 := base64.StdEncoding.EncodeToString(audioData)

	return &SynthesisResult{
		AudioData:   audioData,
		AudioBase64: audioBase64,
		Format:      "mp3",
		Provider:    "openai",
	}, nil
}

// SynthesizeWithVoice sintetiza com uma voz específica
func (sm *SpeechManager) SynthesizeWithVoice(text string, voice string) (*SynthesisResult, error) {
	sm.mu.RLock()
	client := sm.ttsClient
	sm.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("TTS client not configured")
	}

	// Sintetiza com voz específica
	audioData, err := client.SynthesizeWithVoice(text, TTSVoice(voice))
	if err != nil {
		return nil, err
	}

	// Codifica em base64
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

// SynthesizeStream sintetiza com streaming (OpenAI)
func (sm *SpeechManager) SynthesizeStream(ctx context.Context, text string, voice string, callbacks StreamCallbacks) error {
	sm.mu.RLock()
	provider := sm.config.AssistantProvider
	client := sm.ttsClient
	sm.mu.RUnlock()

	switch TTSProvider(provider) {
	case TTSProviderOpenAI:
		return sm.synthesizeStreamOpenAI(ctx, text, voice, client, callbacks)
	default:
		return fmt.Errorf("streaming not supported for provider: %s", provider)
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

// SupportsStreaming retorna true se o provedor atual suporta streaming
// e o client TTS está inicializado
func (sm *SpeechManager) SupportsStreaming() bool {
	sm.mu.RLock()
	provider := sm.config.AssistantProvider
	client := sm.ttsClient
	sm.mu.RUnlock()

	switch TTSProvider(provider) {
	case TTSProviderOpenAI:
		return client != nil
	default:
		return false
	}
}

// HasTTSClient retorna true se o client TTS está inicializado
func (sm *SpeechManager) HasTTSClient() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.ttsClient != nil
}

// GetTTSClient retorna o client TTS (pode ser nil)
func (sm *SpeechManager) GetTTSClient() *TTSClient {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.ttsClient
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
	hasOpenAI := sm.config.AssistantProvider == string(TTSProviderOpenAI) &&
		(sm.config.AssistantAPIKey != "" || sm.config.AssistantCredentialPattern != "")
	sm.mu.RUnlock()

	if hasOpenAI {
		providers = append(providers, "openai")
	}

	return providers
}

// GetAvailableTTSVoices retorna as vozes disponíveis no provedor dinâmico.
func (sm *SpeechManager) GetAvailableTTSVoices() ([]TTSVoiceInfo, error) {
	sm.mu.RLock()
	client := sm.ttsClient
	provider := sm.config.AssistantProvider
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

// rateToSpeed converte rate (-10 a 10) para speed (0.25 a 4.0)
func (sm *SpeechManager) rateToSpeed(rate int) float64 {
	// -10 -> 0.25
	// 0 -> 1.0
	// 10 -> 4.0
	if rate < -10 {
		rate = -10
	}
	if rate > 10 {
		rate = 10
	}

	if rate <= 0 {
		// -10 a 0 mapeia para 0.25 a 1.0
		return 1.0 - float64(-rate)*0.075
	} else {
		// 0 a 10 mapeia para 1.0 a 4.0
		return 1.0 + float64(rate)*0.3
	}
}

// SetTTSVoice altera a voz do TTS (assistant role)
func (sm *SpeechManager) SetTTSVoice(voice string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.AssistantVoice = voice
	if sm.ttsClient != nil {
		sm.ttsClient.SetVoice(TTSVoice(voice))
	}
}

// SetTTSSpeed altera a velocidade do TTS (assistant role)
func (sm *SpeechManager) SetTTSSpeed(rate int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.AssistantRate = rate
	if sm.ttsClient != nil {
		sm.ttsClient.SetSpeed(sm.rateToSpeed(rate))
	}
}

// SetTTSModel altera o modelo do TTS (assistant role)
func (sm *SpeechManager) SetTTSModel(model string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.AssistantModel = model
	if sm.ttsClient != nil {
		sm.ttsClient.SetModel(TTSModel(model))
	}
}
