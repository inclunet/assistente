package speech

import (
	"encoding/base64"
	"fmt"
	"sync"
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

// SpeechConfig configuração global de speech
type SpeechConfig struct {
	// STT
	STTProvider STTProvider `json:"sttProvider"`
	
	// TTS
	TTSProvider TTSProvider `json:"ttsProvider"`
	TTSVoice    string      `json:"ttsVoice"`
	TTSVolume   int         `json:"ttsVolume"` // 0-100
	TTSRate     int         `json:"ttsRate"`   // -10 a 10 (SAPI5) ou 0.25-4.0 mapeado (OpenAI)
	
	// OpenAI API
	OpenAIAPIKey     string `json:"openaiApiKey"`
	OpenAIAPIBaseURL string `json:"openaiApiBaseUrl"`
	WhisperModel     string `json:"whisperModel"`
	WhisperLanguage  string `json:"whisperLanguage"`
	TTSModel         string `json:"ttsModel"` // tts-1 ou tts-1-hd
	
	// Comportamento
	AutoSpeak bool `json:"autoSpeak"`
}

// SpeechManager gerenciador central de speech
type SpeechManager struct {
	config        SpeechConfig
	whisperClient *WhisperClient
	ttsClient     *TTSClient
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
func NewSpeechManager(config SpeechConfig) *SpeechManager {
	sm := &SpeechManager{
		config: config,
	}

	// Inicializa clientes se API key disponível
	if config.OpenAIAPIKey != "" {
		sm.whisperClient = NewWhisperClient(WhisperConfig{
			APIKey:     config.OpenAIAPIKey,
			APIBaseURL: config.OpenAIAPIBaseURL,
			Model:      config.WhisperModel,
			Language:   config.WhisperLanguage,
		})

		sm.ttsClient = NewTTSClient(TTSConfig{
			APIKey:     config.OpenAIAPIKey,
			APIBaseURL: config.OpenAIAPIBaseURL,
			Model:      TTSModel(config.TTSModel),
			Voice:      TTSVoice(config.TTSVoice),
			Speed:      sm.rateToSpeed(config.TTSRate),
		})
	}

	return sm
}

// UpdateConfig atualiza a configuração
func (sm *SpeechManager) UpdateConfig(config SpeechConfig) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config = config

	// Recria clientes se necessário
	if config.OpenAIAPIKey != "" {
		sm.whisperClient = NewWhisperClient(WhisperConfig{
			APIKey:     config.OpenAIAPIKey,
			APIBaseURL: config.OpenAIAPIBaseURL,
			Model:      config.WhisperModel,
			Language:   config.WhisperLanguage,
		})

		sm.ttsClient = NewTTSClient(TTSConfig{
			APIKey:     config.OpenAIAPIKey,
			APIBaseURL: config.OpenAIAPIBaseURL,
			Model:      TTSModel(config.TTSModel),
			Voice:      TTSVoice(config.TTSVoice),
			Speed:      sm.rateToSpeed(config.TTSRate),
		})
	}
}

// Transcribe transcreve áudio para texto
// audioBase64: áudio codificado em base64
// filename: nome do arquivo com extensão
func (sm *SpeechManager) Transcribe(audioBase64 string, filename string) (*TranscriptionResult, error) {
	sm.mu.RLock()
	provider := sm.config.STTProvider
	sm.mu.RUnlock()

	switch provider {
	case STTProviderWhisper:
		return sm.transcribeWhisper(audioBase64, filename)
	case STTProviderWebSpeech:
		return nil, fmt.Errorf("WebSpeech STT is handled in the frontend")
	default:
		return nil, fmt.Errorf("unknown STT provider: %s", provider)
	}
}

// transcribeWhisper transcreve usando Whisper
func (sm *SpeechManager) transcribeWhisper(audioBase64 string, filename string) (*TranscriptionResult, error) {
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

	// Transcreve
	text, err := client.Transcribe(audioData, filename)
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
	provider := sm.config.TTSProvider
	sm.mu.RUnlock()

	switch provider {
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

// GetAvailableSTTProviders retorna os provedores de STT disponíveis
func (sm *SpeechManager) GetAvailableSTTProviders() []string {
	providers := []string{"webspeech"}

	sm.mu.RLock()
	hasOpenAI := sm.config.OpenAIAPIKey != ""
	sm.mu.RUnlock()

	if hasOpenAI {
		providers = append(providers, "whisper")
	}

	return providers
}

// GetAvailableTTSProviders retorna os provedores de TTS disponíveis
func (sm *SpeechManager) GetAvailableTTSProviders() []string {
	providers := []string{"webspeech", "sapi5"}

	sm.mu.RLock()
	hasOpenAI := sm.config.OpenAIAPIKey != ""
	sm.mu.RUnlock()

	if hasOpenAI {
		providers = append(providers, "openai")
	}

	return providers
}

// GetOpenAIVoices retorna as vozes disponíveis do OpenAI TTS
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

// SetTTSVoice altera a voz do TTS
func (sm *SpeechManager) SetTTSVoice(voice string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.TTSVoice = voice
	if sm.ttsClient != nil {
		sm.ttsClient.SetVoice(TTSVoice(voice))
	}
}

// SetTTSSpeed altera a velocidade do TTS
func (sm *SpeechManager) SetTTSSpeed(rate int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config.TTSRate = rate
	if sm.ttsClient != nil {
		sm.ttsClient.SetSpeed(sm.rateToSpeed(rate))
	}
}

