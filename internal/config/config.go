package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Mutex para serializar operações de config e evitar condições de corrida
var configMutex sync.Mutex

// ModelParams representa os parâmetros de um modelo LLM
type ModelParams struct {
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"` // 0.0 a 2.0
	MaxTokens   int     `json:"max_tokens,omitempty"`  // Limite de tokens na resposta
	TopP        float64 `json:"top_p,omitempty"`       // 0.0 a 1.0
}

// EmbeddingsParams representa os parâmetros para embeddings
type EmbeddingsParams struct {
	Model      string `json:"model,omitempty"`
	Dimensions int    `json:"dimensions,omitempty"` // Dimensões do vetor (se suportado pelo modelo)
}

// VoiceParams representa os parâmetros de voz TTS
type VoiceParams struct {
	Voice     string `json:"voice,omitempty"`      // ID da voz (ou "disabled")
	AutoSpeak bool   `json:"auto_speak,omitempty"` // Falar respostas automaticamente
	Volume    int    `json:"volume,omitempty"`     // 0-100
	Rate      int    `json:"rate,omitempty"`       // -10 a 10
}

// STTParams representa os parâmetros de transcrição
type STTParams struct {
	Provider      string `json:"provider,omitempty"`       // "webspeech" ou "whisper"
	RecordingMode string `json:"recording_mode,omitempty"` // "ptt", "toggle", "vad_silence", "vad_activity"
}

// ChatDefaults representa as preferências padrão do chat
type ChatDefaults struct {
	UseTools             bool `json:"use_tools,omitempty"`              // Usar agentes/ferramentas
	ShowInternalMessages bool `json:"show_internal_messages,omitempty"` // Mostrar mensagens internas
}

// Config representa a configuração da aplicação
type Config struct {
	APIKey           string           `json:"api_key"`
	APIBaseURL       string           `json:"api_base_url"`
	DefaultModel     string           `json:"default_model,omitempty"`
	EmbeddingsModel  string           `json:"embeddings_model,omitempty"`
	ImageModel       string           `json:"image_model,omitempty"`
	ChatParams       ModelParams      `json:"chat_params,omitempty"`
	EmbeddingsParams EmbeddingsParams `json:"embeddings_params,omitempty"`
	VoiceParams      VoiceParams      `json:"voice_params,omitempty"`
	STTParams        STTParams        `json:"stt_params,omitempty"`
	ChatDefaults     ChatDefaults     `json:"chat_defaults,omitempty"`
}

// DefaultConfig retorna a configuração padrão
func DefaultConfig() *Config {
	return &Config{
		APIKey:       "",
		APIBaseURL:   "https://api.openai.com/v1",
		DefaultModel: "gpt-4o-mini", // Modelo padrão
		ChatParams: ModelParams{
			Model:       "gpt-4o-mini",
			Temperature: 0.7,
			MaxTokens:   4096,
			TopP:        1.0,
		},
		EmbeddingsParams: EmbeddingsParams{
			Model:      "text-embedding-3-small",
			Dimensions: 0, // Usa o padrão do modelo
		},
		VoiceParams: VoiceParams{
			Voice:     "disabled", // Desabilitado por padrão (usa leitor de telas)
			AutoSpeak: true,
			Volume:    100,
			Rate:      0,
		},
		STTParams: STTParams{
			Provider:      "webspeech",
			RecordingMode: "ptt",
		},
		ChatDefaults: ChatDefaults{
			UseTools:             true,
			ShowInternalMessages: false,
		},
	}
}

// GetConfigPath retorna o caminho do arquivo de configuração na pasta do usuário
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".assistente")

	// Cria o diretório se não existir
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(configDir, "config.json"), nil
}

// Load carrega a configuração do arquivo
func Load() (*Config, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	return loadUnsafe()
}

// loadUnsafe carrega config sem lock (para uso interno quando já tem o mutex)
func loadUnsafe() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// Se o arquivo não existe, retorna configuração padrão
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// Save salva a configuração no arquivo
func Save(config *Config) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	return saveUnsafe(config)
}

// saveUnsafe salva config sem lock (para uso interno quando já tem o mutex)
func saveUnsafe(config *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// Update atualiza a configuração de forma atômica
// updateFn recebe o config atual e deve retornar o config modificado
func Update(updateFn func(*Config) *Config) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	config, err := loadUnsafe()
	if err != nil {
		return err
	}

	updatedConfig := updateFn(config)
	return saveUnsafe(updatedConfig)
}
