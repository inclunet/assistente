package main

import (
	"fmt"
	"log"

	"assistente/internal/config"
	"assistente/internal/skills"
)

// ==================== Adapters para interfaces do SettingsService ====================

// profileCleanerAdapter adapta profiles.Manager para config.ProfileCleaner.
type profileCleanerAdapter struct{ app *App }

func (a profileCleanerAdapter) ListSlugs() ([]string, error) {
	profiles, err := a.app.profileManager.List()
	if err != nil {
		return nil, err
	}
	slugs := make([]string, len(profiles))
	for i, p := range profiles {
		slugs[i] = p.Slug
	}
	return slugs, nil
}

func (a profileCleanerAdapter) DeleteSlug(slug string) error {
	return a.app.profileManager.Delete(slug)
}

// skillCleanerAdapter adapta skills.Manager para config.SkillCleaner.
type skillCleanerAdapter struct{ app *App }

func (a skillCleanerAdapter) ListSlugs() ([]string, error) {
	skills, err := a.app.skillMgr.List()
	if err != nil {
		return nil, err
	}
	slugs := make([]string, len(skills))
	for i, s := range skills {
		slugs[i] = s.Slug
	}
	return slugs, nil
}

func (a skillCleanerAdapter) DeleteSlug(slug string) error {
	return a.app.skillMgr.Delete(slug)
}

// ==================== Thin Wrappers (Wails bindings) ====================

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
	return a.settingsSvc.SetChatModel(model)
}

// SaveSettings salva as configurações
func (a *App) SaveSettings(input SettingsInput) error {
	return a.settingsSvc.SaveSettings(input)
}

// SetDefaultModel salva o modelo padrão
func (a *App) SetDefaultModel(model string) error {
	return a.settingsSvc.SetDefaultModel(model)
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
	return a.settingsSvc.ResetConfig()
}

// ClearAllCredentials apaga todas as credenciais armazenadas
func (a *App) ClearAllCredentials() error {
	return a.settingsSvc.ClearAllCredentials(a.ctx)
}

// ClearAllProfiles apaga todos os perfis, mantendo apenas o ativo padrão
func (a *App) ClearAllProfiles() error {
	return a.settingsSvc.ClearAllProfiles()
}

// ClearAllSkills apaga todos os skills
func (a *App) ClearAllSkills() error {
	return a.settingsSvc.ClearAllSkills()
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
