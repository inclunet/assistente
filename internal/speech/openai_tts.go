package speech

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TTSVoice representa uma voz disponível no OpenAI TTS
type TTSVoice string

const (
	VoiceAlloy   TTSVoice = "alloy"   // Neutra, balanceada
	VoiceEcho    TTSVoice = "echo"    // Masculina, profunda
	VoiceFable   TTSVoice = "fable"   // Expressiva, narrativa
	VoiceOnyx    TTSVoice = "onyx"    // Masculina, autoritária
	VoiceNova    TTSVoice = "nova"    // Feminina, jovem
	VoiceShimmer TTSVoice = "shimmer" // Feminina, clara
)

// TTSModel representa o modelo de TTS
type TTSModel string

const (
	ModelTTS1   TTSModel = "tts-1"    // Rápido, menor qualidade
	ModelTTS1HD TTSModel = "tts-1-hd" // Mais lento, melhor qualidade
)

// TTSFormat representa o formato de saída de áudio
type TTSFormat string

const (
	FormatMP3  TTSFormat = "mp3"
	FormatOpus TTSFormat = "opus"
	FormatAAC  TTSFormat = "aac"
	FormatFLAC TTSFormat = "flac"
	FormatWAV  TTSFormat = "wav"
	FormatPCM  TTSFormat = "pcm"
)

// TTSConfig configuração para o OpenAI TTS
type TTSConfig struct {
	APIKey     string
	APIBaseURL string
	Model      TTSModel  // "tts-1" ou "tts-1-hd"
	Voice      TTSVoice  // alloy, echo, fable, onyx, nova, shimmer
	Format     TTSFormat // mp3, opus, aac, flac, wav, pcm
	Speed      float64   // 0.25 a 4.0 (1.0 = normal)
}

// TTSClient cliente para síntese de voz via OpenAI TTS
type TTSClient struct {
	config     TTSConfig
	httpClient *http.Client
}

// TTSRequest requisição para a API de TTS
type TTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

// TTSVoiceInfo informações sobre uma voz
type TTSVoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Gender      string `json:"gender"`
	Provider    string `json:"provider"`
}

// NewTTSClient cria um novo cliente TTS
func NewTTSClient(config TTSConfig) *TTSClient {
	if config.APIBaseURL == "" {
		config.APIBaseURL = "https://api.openai.com/v1"
	}
	if config.Model == "" {
		config.Model = ModelTTS1
	}
	if config.Voice == "" {
		config.Voice = VoiceNova
	}
	if config.Format == "" {
		config.Format = FormatMP3
	}
	if config.Speed == 0 {
		config.Speed = 1.0
	}

	return &TTSClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
	}
}

// Synthesize converte texto em áudio
// Retorna os bytes do áudio no formato configurado
func (c *TTSClient) Synthesize(text string) ([]byte, error) {
	if c.config.APIKey == "" {
		return nil, fmt.Errorf("API key not configured")
	}

	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	// Limita o texto a 4096 caracteres (limite da API)
	if len(text) > 4096 {
		text = text[:4096]
	}

	// Cria a requisição
	reqBody := TTSRequest{
		Model:          string(c.config.Model),
		Input:          text,
		Voice:          string(c.config.Voice),
		ResponseFormat: string(c.config.Format),
		Speed:          c.config.Speed,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Cria a requisição HTTP
	url := fmt.Sprintf("%s/audio/speech", c.config.APIBaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	// Envia a requisição
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Lê a resposta
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// SynthesizeWithVoice converte texto em áudio usando uma voz específica
func (c *TTSClient) SynthesizeWithVoice(text string, voice TTSVoice) ([]byte, error) {
	originalVoice := c.config.Voice
	c.config.Voice = voice
	defer func() { c.config.Voice = originalVoice }()

	return c.Synthesize(text)
}

// SetVoice altera a voz padrão
func (c *TTSClient) SetVoice(voice TTSVoice) {
	c.config.Voice = voice
}

// SetSpeed altera a velocidade de fala
func (c *TTSClient) SetSpeed(speed float64) {
	if speed < 0.25 {
		speed = 0.25
	}
	if speed > 4.0 {
		speed = 4.0
	}
	c.config.Speed = speed
}

// SetModel altera o modelo de TTS
func (c *TTSClient) SetModel(model TTSModel) {
	c.config.Model = model
}

// SetFormat altera o formato de saída
func (c *TTSClient) SetFormat(format TTSFormat) {
	c.config.Format = format
}

// GetAvailableVoices retorna a lista de vozes disponíveis
func GetAvailableVoices() []TTSVoiceInfo {
	return []TTSVoiceInfo{
		{
			ID:          "alloy",
			Name:        "Alloy",
			Description: "Voz neutra e balanceada",
			Gender:      "neutral",
			Provider:    "openai",
		},
		{
			ID:          "echo",
			Name:        "Echo",
			Description: "Voz masculina e profunda",
			Gender:      "male",
			Provider:    "openai",
		},
		{
			ID:          "fable",
			Name:        "Fable",
			Description: "Voz expressiva, ideal para narrativas",
			Gender:      "neutral",
			Provider:    "openai",
		},
		{
			ID:          "onyx",
			Name:        "Onyx",
			Description: "Voz masculina e autoritária",
			Gender:      "male",
			Provider:    "openai",
		},
		{
			ID:          "nova",
			Name:        "Nova",
			Description: "Voz feminina jovem e energética",
			Gender:      "female",
			Provider:    "openai",
		},
		{
			ID:          "shimmer",
			Name:        "Shimmer",
			Description: "Voz feminina clara e expressiva",
			Gender:      "female",
			Provider:    "openai",
		},
	}
}

