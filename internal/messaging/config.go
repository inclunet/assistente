package messaging

import (
	"assistente/internal/configdir"
	"encoding/json"
	"fmt"
	"sync"
)

const configFilename = "messaging.json"

// configResolver resolve o arquivo de configuração na raiz de .assistente/
var configResolver = configdir.NewResolver("")

// configMutex serializa operações de config de mensageria
var configMutex sync.Mutex

// ChannelConfig contém a configuração de um canal de mensageria.
type ChannelConfig struct {
	Enabled         bool     `json:"enabled"`
	BotToken        string   `json:"bot_token"`
	AllowedContacts []string `json:"allowed_contacts"` // IDs de contatos autorizados
	Profile         string   `json:"profile,omitempty"` // Perfil de chat a usar (vazio = ativo)
	MaxHistory      int      `json:"max_history,omitempty"` // Mensagens no contexto (0 = padrão)
}

// Config contém a configuração de todos os canais de mensageria.
type Config struct {
	Telegram *ChannelConfig `json:"telegram,omitempty"`
	// Signal   *ChannelConfig `json:"signal,omitempty"`   // futuro
	// WhatsApp *ChannelConfig `json:"whatsapp,omitempty"` // futuro
}

// DefaultConfig retorna a configuração padrão (tudo desabilitado).
func DefaultConfig() *Config {
	return &Config{}
}

// LoadConfig carrega a configuração de mensageria de ~/.assistente/messaging.json.
// Se o arquivo não existir, retorna a config padrão (tudo desabilitado).
func LoadConfig() (*Config, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	data, _, err := configResolver.Read(configFilename)
	if err != nil {
		// Arquivo não existe — retorna config padrão
		return DefaultConfig(), nil
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("erro ao parsear %s: %w", configFilename, err)
	}

	return cfg, nil
}

// SaveConfig salva a configuração de mensageria.
func SaveConfig(cfg *Config) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar config: %w", err)
	}

	return configResolver.Write(configFilename, data)
}

// IsContactAllowed verifica se um contato está na allowlist de um canal.
// Se allowlist está vazia, REJEITA tudo (segurança por padrão).
func IsContactAllowed(cfg *ChannelConfig, contactID string) bool {
	if cfg == nil || len(cfg.AllowedContacts) == 0 {
		return false
	}
	for _, allowed := range cfg.AllowedContacts {
		if allowed == contactID {
			return true
		}
	}
	return false
}
