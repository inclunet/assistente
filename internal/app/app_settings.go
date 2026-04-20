package app

import (
	"fmt"
	"runtime"

	"assistente/controllers"
	"assistente/internal/config"
	"assistente/internal/skills"
)

// GetNativeTTSProviders retorna os IDs de provedores TTS nativos
// disponíveis na plataforma atual (ex.: webspeech sempre, sapi5 apenas no Windows).
func (a *App) GetNativeTTSProviders() []string {
	providers := []string{"webspeech"}
	if runtime.GOOS == "windows" {
		providers = append(providers, "sapi5")
	}
	return providers
}

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

// ============================================================================
// Settings API — delegação para SettingsController
// ============================================================================

func (a *App) SendMessageSync(messages []Message, params ChatParams) (string, error) {
	if a.settingsCtrl == nil {
		return "", fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}
	return a.settingsCtrl.SendMessageSync(a.ctx, messages, params)
}

func (a *App) SetChatModel(model string) error {
	return a.settingsCtrl.SetChatModel(model)
}

func (a *App) GetConfig() (*config.Config, error) { return a.settingsCtrl.GetConfig() }
func (a *App) SaveSettings(input controllers.SettingsInput) error {
	return a.settingsCtrl.SaveSettings(input)
}
func (a *App) SetDefaultModel(model string) error { return a.settingsCtrl.SetDefaultModel(model) }
func (a *App) TestConnection() (bool, error)      { return a.settingsCtrl.TestConnection() }
func (a *App) TestConnectionWithModels() ([]string, error) {
	return a.settingsCtrl.TestConnectionWithModels()
}
func (a *App) ResetConfig() error         { return a.settingsCtrl.ResetConfig() }
func (a *App) ClearAllCredentials() error { return a.settingsCtrl.ClearAllCredentials() }
func (a *App) ClearAllProfiles() error    { return a.settingsCtrl.ClearAllProfiles() }
func (a *App) ClearAllSkills() error      { return a.settingsCtrl.ClearAllSkills() }
func (a *App) ClearAllChannels() error    { return a.settingsCtrl.ClearAllChannels() }

// parseSlashCommand é um shim para manter compatibilidade com testes e código existente.
func parseSlashCommand(content string) (slug string, args string, ok bool) {
	return skills.ParseSlashCommand(content)
}
