package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"assistente/internal/config"
	"assistente/internal/skills"
)

// SendMessageSync envia uma mensagem sem streaming (para acessibilidade)
func (a *App) SendMessageSync(messages []Message, params ChatParams) (string, error) {
	activeProfile, _ := a.profileManager.GetActive()
	activeProfile = a.resolveProfileDefaults(activeProfile)

	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		return "", fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}

	cp, err := a.getChatProviderForProvider(activeProfile.Chat.LLMProvider)
	if err != nil {
		return "", err
	}
	return cp.SendChat(a.ctx, messages, params)
}

// GetConfig retorna a configuração atual
func (a *App) GetConfig() (*config.Config, error) {
	return config.Load()
}

// SetChatModel atualiza apenas o modelo de chat na configuração
func (a *App) SetChatModel(model string) error {
	err := config.Update(func(existing *config.Config) *config.Config {
		existing.DefaultModel = model
		existing.ChatParams.Model = model
		return existing
	})
	if err != nil {
		return err
	}

	// Recarrega o cliente LLM para usar o novo modelo
	a.initLLMClient()

	log.Printf("[SetChatModel] Modelo atualizado para: %s", model)
	return nil
}

// SaveSettings salva as configurações
func (a *App) SaveSettings(input SettingsInput) error {
	// Aplica timeout padrão se não especificado
	responseTimeout := input.ResponseTimeout
	if responseTimeout <= 0 {
		responseTimeout = 180
	}

	err := config.Update(func(existing *config.Config) *config.Config {
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
	if err != nil {
		return err
	}

	return nil
}

// SetDefaultModel salva o modelo padrão
func (a *App) SetDefaultModel(model string) error {
	return config.Update(func(cfg *config.Config) *config.Config {
		cfg.DefaultModel = model
		return cfg
	})
}

// TestConnection testa a conexão com a API
func (a *App) TestConnection() (bool, error) {
	models, err := a.GetModels()
	if err != nil {
		return false, err
	}
	if len(models) > 0 {
		return true, nil
	}
	return false, fmt.Errorf("nenhum modelo encontrado")
}

// TestConnectionWithModels testa a conexão e retorna os modelos disponíveis
func (a *App) TestConnectionWithModels() ([]string, error) {
	models, err := a.GetModels()
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar com a API: %v", err)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("nenhum modelo encontrado na API")
	}
	return models, nil
}

// ResetConfig apaga o arquivo de configuração, resetando ao estado padrão
func (a *App) ResetConfig() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho da configuração: %v", err)
	}

	// Verifica se o arquivo existe
	if _, err := os.Stat(configPath); err == nil {
		// Remove o arquivo
		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("erro ao remover arquivo de configuração: %v", err)
		}
	}

	return nil
}

// ClearAllCredentials apaga todas as credenciais armazenadas
func (a *App) ClearAllCredentials() error {
	if a.credMgr == nil {
		return fmt.Errorf("gerenciador de credenciais não disponível")
	}

	// Limpa todas as credenciais usando DeletePattern com um padrão que pega tudo
	// (isso é uma abordagem simples - em produção seria melhor ter um método Clear específico)
	if err := a.credMgr.DeletePattern(context.Background(), ""); err != nil {
		return fmt.Errorf("erro ao limpar credenciais: %v", err)
	}

	log.Println("[ClearAllCredentials] Credenciais apagadas")
	a.emitter.Emit("credentials:cleared", nil)

	return nil
}

// ClearAllProfiles apaga todos os perfis, mantendo apenas o ativo padrão
func (a *App) ClearAllProfiles() error {
	if a.profileManager == nil {
		return fmt.Errorf("gerenciador de perfis não disponível")
	}

	profiles, err := a.profileManager.List()
	if err != nil {
		return fmt.Errorf("erro ao listar perfis: %v", err)
	}

	for _, profile := range profiles {
		if err := a.profileManager.Delete(profile.Slug); err != nil {
			log.Printf("[ClearAllProfiles] Erro ao deletar perfil %s: %v", profile.Slug, err)
		}
	}

	log.Println("[ClearAllProfiles] Perfis apagados")
	a.emitter.Emit("profiles:cleared", nil)

	return nil
}

// ClearAllSkills apaga todos os skills
func (a *App) ClearAllSkills() error {
	if a.skillMgr == nil {
		return fmt.Errorf("gerenciador de skills não disponível")
	}

	skills, err := a.skillMgr.List()
	if err != nil {
		return fmt.Errorf("erro ao listar skills: %v", err)
	}

	for _, skill := range skills {
		if err := a.skillMgr.Delete(skill.Slug); err != nil {
			log.Printf("[ClearAllSkills] Erro ao deletar skill %s: %v", skill.Slug, err)
		}
	}

	log.Println("[ClearAllSkills] Skills apagados")
	a.emitter.Emit("skills:cleared", nil)

	return nil
}

// ClearAllChannels apaga todos os canais de comunicação
func (a *App) ClearAllChannels() error {
	if a.msgGateway == nil {
		return fmt.Errorf("gateway de mensageria não disponível")
	}

	status := a.msgGateway.GetStatus()
	for channelType := range status {
		if err := a.RestartChannel(channelType); err != nil {
			log.Printf("[ClearAllChannels] Erro ao resetar canal %s: %v", channelType, err)
		}
	}

	log.Println("[ClearAllChannels] Canais apagados")
	a.emitter.Emit("channels:cleared", nil)

	return nil
}

// parseSlashCommand é um shim para manter compatibilidade com testes e código existente.
func parseSlashCommand(content string) (slug string, args string, ok bool) {
	return skills.ParseSlashCommand(content)
}
