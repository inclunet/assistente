package speech

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"assistente/internal/credentials"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// WhisperConfig configuração para o Whisper
type WhisperConfig struct {
	BaseURL           string // URL base (vazio = default OpenAI)
	CredentialPattern string // padrão para credential transport (ex: "api.openai.com")
	Model             string // "whisper-1"
	Language          string // "pt" para português
}

// WhisperClient cliente para transcrição de áudio via SDK OpenAI.
// Usa CredentialTransport para injeção automática de credenciais,
// seguindo o mesmo padrão do TTSClient.
type WhisperClient struct {
	client *openai.Client
	config WhisperConfig
}

// NewWhisperClient cria um novo cliente Whisper usando o SDK openai-go.
// Credenciais são injetadas automaticamente via CredentialTransport.
func NewWhisperClient(config WhisperConfig, credMgr *credentials.Manager) *WhisperClient {
	if config.Model == "" {
		config.Model = "whisper-1"
	}
	if config.Language == "" {
		config.Language = "pt"
	}
	// Whisper exige ISO-639-1 (ex: "pt"), não locale (ex: "pt-BR")
	if idx := strings.IndexAny(config.Language, "-_"); idx > 0 {
		config.Language = config.Language[:idx]
	}
	config.Language = strings.ToLower(config.Language)

	httpClient := credentials.NewHTTPClient(credMgr, config.CredentialPattern, 60*time.Second)

	opts := []option.RequestOption{
		option.WithHTTPClient(httpClient),
		option.WithAPIKey("managed-by-credential-transport"),
	}

	if config.BaseURL != "" {
		baseURL := strings.TrimSuffix(config.BaseURL, "/")
		if !strings.HasSuffix(baseURL, "/v1") {
			baseURL += "/v1"
		}
		opts = append(opts, option.WithBaseURL(baseURL+"/"))
	}

	client := openai.NewClient(opts...)

	return &WhisperClient{
		client: &client,
		config: config,
	}
}

// Transcribe transcreve áudio para texto usando o SDK openai-go.
// audioData: bytes do arquivo de áudio
// filename: nome do arquivo com extensão (ex: "audio.webm", "audio.wav")
func (c *WhisperClient) Transcribe(audioData []byte, filename string) (string, error) {
	return c.TranscribeWithContext(context.Background(), audioData, filename)
}

// TranscribeWithContext transcreve áudio com context cancelável (suporta timeout/barge-in).
func (c *WhisperClient) TranscribeWithContext(ctx context.Context, audioData []byte, filename string) (string, error) {
	if len(audioData) == 0 {
		return "", fmt.Errorf("audio data is empty")
	}

	params := openai.AudioTranscriptionNewParams{
		File:           openai.File(bytes.NewReader(audioData), filename, "audio/wav"),
		Model:          openai.AudioModel(c.config.Model),
		ResponseFormat: openai.AudioResponseFormatJSON,
	}

	if c.config.Language != "" {
		params.Language = param.NewOpt(c.config.Language)
	}

	result, err := c.client.Audio.Transcriptions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("whisper transcription failed: %w", err)
	}

	return result.Text, nil
}

