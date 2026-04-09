package controllers

import (
	"context"
	"fmt"
	"log"
	"os"

	"assistente/internal/config"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/profiles"
	"assistente/internal/skills"
)

// SettingsInput é o payload de SaveSettings (mantém compatibilidade com o frontend).
type SettingsInput struct {
	APIKey          string             `json:"apiKey"`
	APIBaseURL      string             `json:"apiBaseUrl"`
	ResponseTimeout int                `json:"responseTimeout"`
	ChatParams      config.ModelParams `json:"chatParams"`
	STTParams       config.STTParams   `json:"sttParams"`
}

// SettingsControllerConfig agrupa as dependências do SettingsController.
type SettingsControllerConfig struct {
	CredMgr    *credentials.Manager
	ProfileMgr *profiles.Manager
	SkillMgr   *skills.Manager
	Emitter    ports.Emitter
	// Callbacks cross-domain
	RestartChannel func(channelName string) error
	GetModels      func() ([]string, error)
}

// SettingsController é o adapter primário (Inbound) para operações de configurações globais e reset.
type SettingsController struct {
	credMgr        *credentials.Manager
	profileMgr     *profiles.Manager
	skillMgr       *skills.Manager
	emitter        ports.Emitter
	restartChannel func(string) error
	getModels      func() ([]string, error)
}

// NewSettingsController cria um SettingsController com suas dependências.
func NewSettingsController(cfg SettingsControllerConfig) *SettingsController {
	return &SettingsController{
		credMgr:        cfg.CredMgr,
		profileMgr:     cfg.ProfileMgr,
		skillMgr:       cfg.SkillMgr,
		emitter:        cfg.Emitter,
		restartChannel: cfg.RestartChannel,
		getModels:      cfg.GetModels,
	}
}

func (c *SettingsController) GetConfig() (*config.Config, error) {
	return config.Load()
}

func (c *SettingsController) SaveSettings(input SettingsInput) error {
	responseTimeout := input.ResponseTimeout
	if responseTimeout <= 0 {
		responseTimeout = 180
	}
	return config.Update(func(existing *config.Config) *config.Config {
		return &config.Config{
			APIKey:          input.APIKey,
			APIBaseURL:      input.APIBaseURL,
			DefaultModel:    input.ChatParams.Model,
			ResponseTimeout: responseTimeout,
			ChatParams: config.ModelParams{
				Model:       input.ChatParams.Model,
				Temperature: input.ChatParams.Temperature,
				MaxTokens:   input.ChatParams.MaxTokens,
				TopP:        input.ChatParams.TopP,
			},
			STTParams: config.STTParams{
				Provider:      input.STTParams.Provider,
				RecordingMode: input.STTParams.RecordingMode,
			},
		}
	})
}

func (c *SettingsController) SetDefaultModel(model string) error {
	return config.Update(func(cfg *config.Config) *config.Config {
		cfg.DefaultModel = model
		return cfg
	})
}

func (c *SettingsController) TestConnection() (bool, error) {
	if c.getModels == nil {
		return false, fmt.Errorf("getModels não configurado")
	}
	models, err := c.getModels()
	if err != nil {
		return false, err
	}
	if len(models) > 0 {
		return true, nil
	}
	return false, fmt.Errorf("nenhum modelo encontrado")
}

func (c *SettingsController) TestConnectionWithModels() ([]string, error) {
	if c.getModels == nil {
		return nil, fmt.Errorf("getModels não configurado")
	}
	models, err := c.getModels()
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar com a API: %v", err)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("nenhum modelo encontrado na API")
	}
	return models, nil
}

func (c *SettingsController) ResetConfig() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho da configuração: %v", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("erro ao remover arquivo de configuração: %v", err)
		}
	}
	return nil
}

func (c *SettingsController) ClearAllCredentials() error {
	if c.credMgr == nil {
		return fmt.Errorf("gerenciador de credenciais não disponível")
	}
	if err := c.credMgr.DeletePattern(context.Background(), ""); err != nil {
		return fmt.Errorf("erro ao limpar credenciais: %v", err)
	}
	log.Println("[ClearAllCredentials] Credenciais apagadas")
	c.emitter.Emit("credentials:cleared", nil)
	return nil
}

func (c *SettingsController) ClearAllProfiles() error {
	if c.profileMgr == nil {
		return fmt.Errorf("gerenciador de perfis não disponível")
	}
	list, err := c.profileMgr.List()
	if err != nil {
		return fmt.Errorf("erro ao listar perfis: %v", err)
	}
	for _, p := range list {
		if err := c.profileMgr.Delete(p.Slug); err != nil {
			log.Printf("[ClearAllProfiles] Erro ao deletar perfil %s: %v", p.Slug, err)
		}
	}
	log.Println("[ClearAllProfiles] Perfis apagados")
	c.emitter.Emit("profiles:cleared", nil)
	return nil
}

func (c *SettingsController) ClearAllSkills() error {
	if c.skillMgr == nil {
		return fmt.Errorf("gerenciador de skills não disponível")
	}
	list, err := c.skillMgr.List()
	if err != nil {
		return fmt.Errorf("erro ao listar skills: %v", err)
	}
	for _, s := range list {
		if err := c.skillMgr.Delete(s.Slug); err != nil {
			log.Printf("[ClearAllSkills] Erro ao deletar skill %s: %v", s.Slug, err)
		}
	}
	log.Println("[ClearAllSkills] Skills apagados")
	c.emitter.Emit("skills:cleared", nil)
	return nil
}

func (c *SettingsController) ClearAllChannels() error {
	if c.restartChannel == nil {
		return fmt.Errorf("restartChannel não configurado")
	}
	// sem acesso direto ao gateway — usa callback injetado
	c.emitter.Emit("channels:cleared", nil)
	log.Println("[ClearAllChannels] Evento channels:cleared emitido")
	return nil
}
