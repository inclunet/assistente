package config

import (
	"assistente/internal/configdir"
	"encoding/json"
	"sync"
)

// Mutex para serializar operações de config e evitar condições de corrida
var configMutex sync.Mutex

// Resolver para arquivos na raiz de .assistente/
var rootResolver = configdir.NewResolver("")

const configFilename = "config.json"

// ModelParams representa os parâmetros de um modelo LLM
type ModelParams struct {
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"` // 0.0 a 2.0
	MaxTokens   int     `json:"max_tokens,omitempty"`  // Limite de tokens na resposta
	TopP        float64 `json:"top_p,omitempty"`       // 0.0 a 1.0
}

// STTParams representa os parâmetros de transcrição
type STTParams struct {
	Provider      string `json:"provider,omitempty"`       // "webspeech" ou "whisper"
	RecordingMode string `json:"recording_mode,omitempty"` // "ptt", "toggle", "vad_silence", "vad_activity"
}

// Config representa a configuração da aplicação.
// NOTA: Configuração de canais de mensageria foi movida para .assistente/channels/<nome>/config.json
// (pacote internal/channels). Contatos autorizados em .assistente/contacts.json (pacote internal/contacts).
type Config struct {
	APIKey          string      `json:"api_key"`
	APIBaseURL      string      `json:"api_base_url"`
	DefaultModel    string      `json:"default_model,omitempty"`
	ResponseTimeout int         `json:"response_timeout,omitempty"` // Timeout em segundos para aguardar resposta da API (padrão: 180)
	ActiveProfile   string      `json:"active_profile,omitempty"`   // Nome (slug) do perfil de conversa ativo
	ChatParams      ModelParams `json:"chat_params,omitempty"`
	STTParams       STTParams   `json:"stt_params,omitempty"`
}

// DefaultConfig retorna a configuração padrão
func DefaultConfig() *Config {
	return &Config{
		APIKey:          "",
		APIBaseURL:      "https://api.openai.com/v1",
		DefaultModel:    "gpt-4o-mini",
		ResponseTimeout: 180,
		ActiveProfile:   "padrao",
		ChatParams: ModelParams{
			Model:       "gpt-4o-mini",
			Temperature: 0.7,
			MaxTokens:   4096,
			TopP:        1.0,
		},
		STTParams: STTParams{
			Provider:      "webspeech",
			RecordingMode: "ptt",
		},
	}
}

// GetConfigPath retorna o caminho do arquivo de configuração válido (resolvido nos 3 diretórios).
// Se não existir em nenhum, retorna o caminho no diretório home (~/.assistente/config.json).
func GetConfigPath() (string, error) {
	resolved, err := rootResolver.Resolve(configFilename)
	if err != nil {
		// Não existe em nenhum diretório — retorna o caminho no home
		if err := rootResolver.EnsureHomeDir(); err != nil {
			return "", err
		}
		homeDir := configdir.GetHomeDir()
		return homeDir + string('/') + configFilename, nil
	}
	return resolved.Path, nil
}

// Load carrega a configuração do arquivo válido (maior prioridade)
func Load() (*Config, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	return loadUnsafe()
}

// loadUnsafe carrega config sem lock (para uso interno quando já tem o mutex)
func loadUnsafe() (*Config, error) {
	data, _, err := rootResolver.Read(configFilename)
	if err != nil {
		// Arquivo não existe em nenhum diretório — retorna config padrão
		return DefaultConfig(), nil
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save salva a configuração no arquivo válido (ou cria no home se não existir)
func Save(config *Config) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	return saveUnsafe(config)
}

// saveUnsafe salva config sem lock (para uso interno quando já tem o mutex)
func saveUnsafe(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return rootResolver.Write(configFilename, data)
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

// GetResponseTimeout retorna o timeout de resposta em segundos (padrão: 180)
func (c *Config) GetResponseTimeout() int {
	if c.ResponseTimeout <= 0 {
		return 180 // Padrão de 3 minutos
	}
	return c.ResponseTimeout
}
