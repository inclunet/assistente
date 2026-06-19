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
// DEPRECATED: Use ChatConfig em Profile ao invés
type ModelParams struct {
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

// STTParams representa os parâmetros de transcrição
// DEPRECATED: Use Voice + Interaction em Profile ao invés
type STTParams struct {
	Provider      string `json:"provider,omitempty"`
	RecordingMode string `json:"recording_mode,omitempty"`
}

// Defaults da política de manutenção do banco (AEP-0074).
const (
	DefaultJobRetentionHours          = 24                // dados de jobs são efêmeros
	DefaultRunsPerJobKeep             = 200               // teto de runs por job
	DefaultChatToolCallsRetentionDays = 0                 // 0 = vinculado à conversa (sem expiração)
	DefaultVacuumMinFreeBytes         = 16 * 1024 * 1024  // 16 MiB de freelist para VACUUM completo
)

// MaintenanceSettings controla a retenção e a compactação do banco (AEP-0074).
// É uma seção ATUAL (não-deprecada) do config.json e a única fonte dessas
// configurações — não há variáveis de ambiente.
//
// Distinção central:
//   - Dados de JOBS (runs/eventos + tool_invocations de jobs) são operacionais e
//     descartáveis: retenção curta (horas) + teto por contagem.
//   - Tool calls de CHAT fazem parte do histórico da conversa: por padrão NÃO
//     expiram por tempo (ChatToolCallsRetentionDays=0); só saem quando a conversa
//     é deletada. Um valor > 0 ativa um cap de idade opcional.
type MaintenanceSettings struct {
	JobRetentionHours          int   `json:"job_retention_hours"`
	RunsPerJobKeep             int   `json:"runs_per_job_keep"`
	ChatToolCallsRetentionDays int   `json:"chat_tool_calls_retention_days"`
	VacuumMinFreeBytes         int64 `json:"vacuum_min_free_bytes"`
}

// DefaultMaintenanceSettings retorna a política padrão.
func DefaultMaintenanceSettings() MaintenanceSettings {
	return MaintenanceSettings{
		JobRetentionHours:          DefaultJobRetentionHours,
		RunsPerJobKeep:             DefaultRunsPerJobKeep,
		ChatToolCallsRetentionDays: DefaultChatToolCallsRetentionDays,
		VacuumMinFreeBytes:         DefaultVacuumMinFreeBytes,
	}
}

// normalized aplica defaults a valores ausentes/ inválidos, preservando as
// escolhas explícitas do usuário (inclusive 0 para ChatToolCallsRetentionDays,
// que é semanticamente "sem expiração").
func (m MaintenanceSettings) normalized() MaintenanceSettings {
	out := m
	if out.JobRetentionHours <= 0 {
		out.JobRetentionHours = DefaultJobRetentionHours
	}
	if out.RunsPerJobKeep < 0 {
		out.RunsPerJobKeep = DefaultRunsPerJobKeep
	}
	if out.ChatToolCallsRetentionDays < 0 {
		out.ChatToolCallsRetentionDays = 0
	}
	if out.VacuumMinFreeBytes < 0 {
		out.VacuumMinFreeBytes = DefaultVacuumMinFreeBytes
	}
	return out
}

// Config representa a configuração da aplicação.
// NOTA: Configuração de canais de mensageria foi movida para .assistente/channels/<nome>/config.json
// (pacote internal/channels). Contatos autorizados em .assistente/contacts.json (pacote internal/contacts).
//
// ⚠️ DEPRECATED: config.json está sendo ELIMINADO (Fase 6 completa)
// Todos os campos abaixo foram movidos para o sistema de profiles + provider registry:
// - APIKey, APIBaseURL → Credentials Manager + Provider Registry
// - DefaultModel, ResponseTimeout → Profile.Chat
// - STTParams → Profile.Voice + Profile.Interaction
// - ActiveProfile → Profile.Active field
//
// A migração automática acontece no startup via App.migrateLegacyConfig():
// - APIKey é registrado no credentials.Manager (encrypted)
// - Providers já existentes são usados
// - Perfis controlam toda a configuração
//
// Este arquivo permanece apenas para compatibilidade temporária.
type Config struct {
	// ⚠️ DEPRECATED - Todos os campos abaixo não são mais usados
	APIKey          string      `json:"api_key,omitempty"`          // DEPRECATED: Migrado para credentials.Manager
	APIBaseURL      string      `json:"api_base_url,omitempty"`     // DEPRECATED: Migrado para Provider Registry
	DefaultModel    string      `json:"default_model,omitempty"`    // DEPRECATED: Migrado para Profile.Chat.Model
	ResponseTimeout int         `json:"response_timeout,omitempty"` // DEPRECATED: Migrado para Profile.Chat.ResponseTimeout
	ActiveProfile   string      `json:"active_profile,omitempty"`   // DEPRECATED: Migrado para Profile.Active
	ChatParams      ModelParams `json:"chat_params,omitempty"`      // DEPRECATED: Migrado para Profile.Chat
	STTParams       STTParams   `json:"stt_params,omitempty"`       // DEPRECATED: Migrado para Profile.Voice + Profile.Interaction

	// Maintenance é a seção ATUAL (não-deprecada) — política de retenção/
	// compactação do banco (AEP-0074).
	Maintenance MaintenanceSettings `json:"maintenance"`
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
		Maintenance: DefaultMaintenanceSettings(),
	}
}

// GetMaintenance carrega a política de manutenção do config.json, com defaults
// aplicados a valores ausentes/inválidos.
func GetMaintenance() (MaintenanceSettings, error) {
	cfg, err := Load()
	if err != nil {
		return DefaultMaintenanceSettings(), err
	}
	return cfg.Maintenance.normalized(), nil
}

// SaveMaintenance persiste a política de manutenção no config.json de forma
// atômica, preservando os demais campos.
func SaveMaintenance(settings MaintenanceSettings) error {
	normalized := settings.normalized()
	return Update(func(existing *Config) *Config {
		existing.Maintenance = normalized
		return existing
	})
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
// DEPRECATED: Use profile.Chat.ResponseTimeout ao invés
func (c *Config) GetResponseTimeout() int {
	if c.ResponseTimeout <= 0 {
		return 180
	}
	return c.ResponseTimeout
}
