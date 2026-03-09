package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

// WhisperConfig configuração para o Whisper
type WhisperConfig struct {
	APIKey     string
	APIBaseURL string
	Model      string // "whisper-1"
	Language   string // "pt" para português
}

// WhisperClient cliente para transcrição de áudio via OpenAI Whisper
type WhisperClient struct {
	config     WhisperConfig
	httpClient *httpclient.Client
}

// WhisperResponse resposta da API Whisper
type WhisperResponse struct {
	Text string `json:"text"`
}

// WhisperVerboseResponse resposta detalhada da API Whisper
type WhisperVerboseResponse struct {
	Task     string            `json:"task"`
	Language string            `json:"language"`
	Duration float64           `json:"duration"`
	Text     string            `json:"text"`
	Segments []WhisperSegment  `json:"segments,omitempty"`
}

// WhisperSegment segmento de transcrição
type WhisperSegment struct {
	ID               int     `json:"id"`
	Seek             int     `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	Tokens           []int   `json:"tokens"`
	Temperature      float64 `json:"temperature"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
}

// NewWhisperClient cria um novo cliente Whisper
func NewWhisperClient(config WhisperConfig, credMgr *credentials.Manager) *WhisperClient {
	if config.APIBaseURL == "" {
		config.APIBaseURL = "https://api.openai.com/v1"
	}
	if config.Model == "" {
		config.Model = "whisper-1"
	}
	if config.Language == "" {
		config.Language = "pt"
	}

	client := httpclient.New(&httpclient.Config{
		CredentialManager: credMgr,
		Timeout:           60 * time.Second,
	}, map[string]string{})

	return &WhisperClient{
		config:     config,
		httpClient: client,
	}
}

// Transcribe transcreve áudio para texto
// audioData: bytes do arquivo de áudio
// filename: nome do arquivo com extensão (ex: "audio.webm", "audio.wav")
func (c *WhisperClient) Transcribe(audioData []byte, filename string) (string, error) {
	if c.config.APIKey == "" {
		return "", fmt.Errorf("API key not configured")
	}

	// Cria o corpo multipart
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Adiciona o arquivo de áudio
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}

	// Adiciona o modelo
	if err := writer.WriteField("model", c.config.Model); err != nil {
		return "", fmt.Errorf("failed to write model field: %w", err)
	}

	// Adiciona o idioma (opcional, mas melhora precisão)
	if c.config.Language != "" {
		if err := writer.WriteField("language", c.config.Language); err != nil {
			return "", fmt.Errorf("failed to write language field: %w", err)
		}
	}

	// Formato de resposta
	if err := writer.WriteField("response_format", "json"); err != nil {
		return "", fmt.Errorf("failed to write response_format field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	// Cria a requisição
	url := fmt.Sprintf("%s/audio/transcriptions", c.config.APIBaseURL)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Envia a requisição
	resp, err := c.httpClient.Do(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Lê a resposta
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse da resposta
	var whisperResp WhisperResponse
	if err := json.Unmarshal(respBody, &whisperResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return whisperResp.Text, nil
}

// TranscribeVerbose transcreve áudio com informações detalhadas
func (c *WhisperClient) TranscribeVerbose(audioData []byte, filename string) (*WhisperVerboseResponse, error) {
	if c.config.APIKey == "" {
		return nil, fmt.Errorf("API key not configured")
	}

	// Cria o corpo multipart
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Adiciona o arquivo de áudio
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return nil, fmt.Errorf("failed to write audio data: %w", err)
	}

	// Adiciona o modelo
	if err := writer.WriteField("model", c.config.Model); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	// Adiciona o idioma
	if c.config.Language != "" {
		if err := writer.WriteField("language", c.config.Language); err != nil {
			return nil, fmt.Errorf("failed to write language field: %w", err)
		}
	}

	// Formato de resposta verbose
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		return nil, fmt.Errorf("failed to write response_format field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	// Cria a requisição
	url := fmt.Sprintf("%s/audio/transcriptions", c.config.APIBaseURL)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Envia a requisição
	resp, err := c.httpClient.Do(context.Background(), req)
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

	// Parse da resposta
	var whisperResp WhisperVerboseResponse
	if err := json.Unmarshal(respBody, &whisperResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &whisperResp, nil
}

