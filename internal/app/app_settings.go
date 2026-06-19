package app

import (
	"fmt"
	"runtime"

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

// ============================================================================
// Settings API — delegação para SettingsController
// ============================================================================

func (a *App) SendMessageSync(messages []Message, params ChatParams) (string, error) {
	if a.settingsCtrl == nil {
		return "", fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return "", err
	}
	return a.settingsCtrl.SendMessageSync(ctx, messages, params)
}

func (a *App) TestConnection() (bool, error) { return a.settingsCtrl.TestConnection() }
func (a *App) TestConnectionWithModels() ([]string, error) {
	return a.settingsCtrl.TestConnectionWithModels()
}
func (a *App) ResetConfig() error {
	if _, err := a.requireAdminContext(); err != nil {
		return err
	}
	return a.settingsCtrl.ResetConfig()
}
func (a *App) ClearAllCredentials() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.settingsCtrl.ClearAllCredentials(ctx)
}
func (a *App) ClearAllProfiles() error {
	if _, err := a.requireAdminContext(); err != nil {
		return err
	}
	return a.settingsCtrl.ClearAllProfiles()
}
func (a *App) ClearAllSkills() error {
	if _, err := a.requireAdminContext(); err != nil {
		return err
	}
	return a.settingsCtrl.ClearAllSkills()
}
func (a *App) ClearAllChannels() error {
	if _, err := a.requireAdminContext(); err != nil {
		return err
	}
	return a.settingsCtrl.ClearAllChannels()
}

// parseSlashCommand é um shim para manter compatibilidade com testes e código existente.
func parseSlashCommand(content string) (slug string, args string, ok bool) {
	return skills.ParseSlashCommand(content)
}
