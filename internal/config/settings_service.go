package config

import (
	"context"
	"fmt"
	"log"
	"os"

	"assistente/internal/events"
)

// SettingsService encapsula operações de configuração e reset de dados,
// desacoplado do framework Wails.
type SettingsService struct {
	emitter        events.Emitter
	credCleaner    CredentialCleaner
	profileCleaner ProfileCleaner
	skillCleaner   SkillCleaner
	reloadLLM      func() // callback para recarregar cliente LLM após mudança de modelo
}

// CredentialCleaner limpa credenciais armazenadas. O contrato é
// "iterar a lista visível e deletar pattern por pattern" — sem chamada
// "limpar tudo" sem nome. `ListVisible` já é filtrado por instance
// secrets e cross-user, então iterar é seguro por construção.
type CredentialCleaner interface {
	ListVisible(ctx context.Context) ([]VisibleCredential, error)
	DeletePattern(ctx context.Context, pattern string) error
}

// VisibleCredential é a forma mínima exposta para o cleaner — só
// pattern, sem segredos.
type VisibleCredential struct {
	Pattern string
}

// ProfileCleaner lista e deleta perfis.
type ProfileCleaner interface {
	ListSlugs() ([]string, error)
	DeleteSlug(slug string) error
}

// SkillCleaner lista e deleta skills.
type SkillCleaner interface {
	ListSlugs() ([]string, error)
	DeleteSlug(slug string) error
}

// SettingsServiceConfig contém dependências injetadas.
type SettingsServiceConfig struct {
	Emitter        events.Emitter
	CredCleaner    CredentialCleaner
	ProfileCleaner ProfileCleaner
	SkillCleaner   SkillCleaner
	ReloadLLM      func()
}

// NewSettingsService cria um SettingsService com as dependências fornecidas.
func NewSettingsService(cfg SettingsServiceConfig) *SettingsService {
	return &SettingsService{
		emitter:        cfg.Emitter,
		credCleaner:    cfg.CredCleaner,
		profileCleaner: cfg.ProfileCleaner,
		skillCleaner:   cfg.SkillCleaner,
		reloadLLM:      cfg.ReloadLLM,
	}
}

// GetConfig retorna a configuração atual.
func (s *SettingsService) GetConfig() (*Config, error) {
	return Load()
}

// SetChatModel atualiza o modelo de chat na configuração e recarrega o cliente LLM.
func (s *SettingsService) SetChatModel(model string) error {
	err := Update(func(existing *Config) *Config {
		existing.DefaultModel = model
		existing.ChatParams.Model = model
		return existing
	})
	if err != nil {
		return err
	}

	if s.reloadLLM != nil {
		s.reloadLLM()
	}

	log.Printf("[SetChatModel] Modelo atualizado para: %s", model)
	return nil
}

// SaveSettings salva as configurações a partir dos parâmetros de entrada.
func (s *SettingsService) SaveSettings(input SettingsInput) error {
	responseTimeout := input.ResponseTimeout
	if responseTimeout <= 0 {
		responseTimeout = 180
	}

	return Update(func(_ *Config) *Config {
		return &Config{
			APIKey:          input.APIKey,
			APIBaseURL:      input.APIBaseURL,
			DefaultModel:    input.ChatParams.Model,
			ResponseTimeout: responseTimeout,
			ChatParams: ModelParams{
				Model:       input.ChatParams.Model,
				Temperature: input.ChatParams.Temperature,
				MaxTokens:   input.ChatParams.MaxTokens,
				TopP:        input.ChatParams.TopP,
			},
			STTParams: STTParams{
				Provider:      input.STTParams.Provider,
				RecordingMode: input.STTParams.RecordingMode,
			},
		}
	})
}

// SetDefaultModel salva o modelo padrão na configuração.
func (s *SettingsService) SetDefaultModel(model string) error {
	return Update(func(cfg *Config) *Config {
		cfg.DefaultModel = model
		return cfg
	})
}

// ResetConfig apaga o arquivo de configuração, resetando ao estado padrão.
func (s *SettingsService) ResetConfig() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho da configuração: %w", err)
	}

	if _, err := os.Stat(configPath); err == nil {
		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("erro ao remover arquivo de configuração: %w", err)
		}
	}

	return nil
}

// ClearAllCredentials apaga todas as credenciais visíveis ao usuário
// do contexto, iterando pattern por pattern.
func (s *SettingsService) ClearAllCredentials(ctx context.Context) error {
	if s.credCleaner == nil {
		return fmt.Errorf("gerenciador de credenciais não disponível")
	}

	creds, err := s.credCleaner.ListVisible(ctx)
	if err != nil {
		return fmt.Errorf("erro ao listar credenciais: %w", err)
	}
	deleted := 0
	for _, cred := range creds {
		if cred.Pattern == "" {
			continue
		}
		if err := s.credCleaner.DeletePattern(ctx, cred.Pattern); err != nil {
			return fmt.Errorf("erro ao apagar credencial %q: %w", cred.Pattern, err)
		}
		deleted++
	}

	log.Printf("[ClearAllCredentials] %d credenciais apagadas", deleted)
	s.emitter.Emit("credentials:cleared", nil)

	return nil
}

// ClearAllProfiles apaga todos os perfis.
func (s *SettingsService) ClearAllProfiles() error {
	if s.profileCleaner == nil {
		return fmt.Errorf("gerenciador de perfis não disponível")
	}

	slugs, err := s.profileCleaner.ListSlugs()
	if err != nil {
		return fmt.Errorf("erro ao listar perfis: %w", err)
	}

	for _, slug := range slugs {
		if err := s.profileCleaner.DeleteSlug(slug); err != nil {
			log.Printf("[ClearAllProfiles] Erro ao deletar perfil %s: %v", slug, err)
		}
	}

	log.Println("[ClearAllProfiles] Perfis apagados")
	s.emitter.Emit("profiles:cleared", nil)

	return nil
}

// ClearAllSkills apaga todos os skills.
func (s *SettingsService) ClearAllSkills() error {
	if s.skillCleaner == nil {
		return fmt.Errorf("gerenciador de skills não disponível")
	}

	slugs, err := s.skillCleaner.ListSlugs()
	if err != nil {
		return fmt.Errorf("erro ao listar skills: %w", err)
	}

	for _, slug := range slugs {
		if err := s.skillCleaner.DeleteSlug(slug); err != nil {
			log.Printf("[ClearAllSkills] Erro ao deletar skill %s: %v", slug, err)
		}
	}

	log.Println("[ClearAllSkills] Skills apagados")
	s.emitter.Emit("skills:cleared", nil)

	return nil
}

// SettingsInput representa os parâmetros de entrada para salvar configurações.
type SettingsInput struct {
	APIKey          string            `json:"api_key"`
	APIBaseURL      string            `json:"api_base_url"`
	ResponseTimeout int               `json:"response_timeout,omitempty"`
	ChatParams      SettingsModelParams `json:"chat_params"`
	STTParams       STTParams         `json:"stt_params,omitempty"`
}

// SettingsModelParams são os parâmetros de modelo vindos do frontend.
// Diferente de ModelParams (config.go): sem omitempty nos campos obrigatórios,
// pois temperature=0 e max_tokens=0 são valores válidos que não devem ser omitidos.
type SettingsModelParams struct {
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p,omitempty"`
}
