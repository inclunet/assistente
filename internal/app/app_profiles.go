package app

import (
	"assistente/internal/contextprovider"
	"assistente/internal/profiles"
)

// ============================================================================
// Unified Profile API — delegação para ProfilesController
// Os métodos abaixo existem apenas para manter compatibilidade com o Wails Bind
// enquanto a migração para controllers/ está em andamento (Strangler Fig).
// ============================================================================

func (a *App) GetProfiles() ([]profiles.ProfileInfo, error) { return a.profilesCtrl.GetProfiles() }
func (a *App) GetProfile(slug string) (*profiles.Profile, error) {
	return a.profilesCtrl.GetProfile(slug)
}
func (a *App) GetActiveProfile() (*profiles.Profile, error) {
	return a.profilesCtrl.GetActiveProfile()
}
func (a *App) GetActiveProfileSlug() string { return a.profilesCtrl.GetActiveProfileSlug() }
func (a *App) GetActiveProfileAndSlug() (*profiles.Profile, string, error) {
	return a.profilesCtrl.GetActiveProfileAndSlug()
}
func (a *App) SetActiveProfile(slug string) error { return a.profilesCtrl.SetActiveProfile(slug) }
func (a *App) CreateProfile(p profiles.Profile) (string, error) {
	return a.profilesCtrl.CreateProfile(p)
}
func (a *App) DuplicateProfile(slug string) (string, error) {
	return a.profilesCtrl.DuplicateProfile(slug)
}
func (a *App) UpdateProfile(slug string, p profiles.Profile) error {
	return a.profilesCtrl.UpdateProfile(slug, p)
}
func (a *App) DeleteProfile(slug string) error { return a.profilesCtrl.DeleteProfile(slug) }
func (a *App) GetProfileSearchPaths() []string { return a.profilesCtrl.GetProfileSearchPaths() }
func (a *App) GetContextProviders() []contextprovider.ProviderMetadata {
	if a.contextProviders == nil {
		return []contextprovider.ProviderMetadata{}
	}
	return a.contextProviders.Metadata()
}
