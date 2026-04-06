package main

import (
	"assistente/internal/profiles"
	"fmt"
	"log"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ============================================================================
// Unified Profile API (arquivo JSON via configdir)
// ============================================================================

// GetProfiles retorna todos os perfis disponíveis
func (a *App) GetProfiles() ([]profiles.ProfileInfo, error) {
	return a.profileManager.List()
}

// GetProfile retorna um perfil pelo slug
func (a *App) GetProfile(slug string) (*profiles.Profile, error) {
	return a.profileManager.Get(slug)
}

// GetActiveProfile retorna o perfil ativo global
func (a *App) GetActiveProfile() (*profiles.Profile, error) {
	return a.profileManager.GetActive()
}

// GetActiveProfileSlug retorna o slug do perfil ativo
func (a *App) GetActiveProfileSlug() string {
	return a.profileManager.GetActiveSlug()
}

// SetActiveProfile define o perfil ativo e re-registra hotkeys
func (a *App) SetActiveProfile(slug string) error {
	if err := a.profileManager.SetActive(slug); err != nil {
		return err
	}

	// Recarrega o cliente LLM para usar o provedor do novo perfil ativo
	a.initLLMClient()

	// Recarrega o speech manager com os providers do novo perfil (TTS/STT podem ser independentes do LLM)
	if err := a.InitSpeechManagerFromProfile(); err != nil {
		log.Printf("[Profile] Erro ao inicializar speech manager para perfil %s: %v", slug, err)
	}

	// Re-registra hotkeys do novo perfil
	a.registerActiveProfileHotkeys()

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "profile:changed", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// CreateProfile cria um novo perfil
func (a *App) CreateProfile(profile profiles.Profile) (string, error) {
	slug, err := a.profileManager.Create(&profile)
	if err != nil {
		return "", err
	}

	runtime.EventsEmit(a.ctx, "profile:created", map[string]interface{}{
		"slug": slug,
		"name": profile.Name,
	})

	return slug, nil
}

// DuplicateProfile cria uma copia de um perfil existente.
func (a *App) DuplicateProfile(slug string) (string, error) {
	newSlug, err := a.profileManager.Duplicate(slug)
	if err != nil {
		return "", err
	}

	profile, err := a.profileManager.Get(newSlug)
	if err == nil && profile != nil {
		runtime.EventsEmit(a.ctx, "profile:created", map[string]interface{}{
			"slug": newSlug,
			"name": profile.Name,
		})
	}

	return newSlug, nil
}

// UpdateProfile atualiza um perfil existente
func (a *App) UpdateProfile(slug string, profile profiles.Profile) error {
	if err := a.profileManager.Update(slug, &profile); err != nil {
		return err
	}

	// Se for o perfil ativo, re-registra hotkeys
	if slug == a.profileManager.GetActiveSlug() {
		a.registerActiveProfileHotkeys()
	}

	runtime.EventsEmit(a.ctx, "profile:updated", map[string]interface{}{
		"slug": slug,
		"name": profile.Name,
	})

	return nil
}

// DeleteProfile deleta um perfil
func (a *App) DeleteProfile(slug string) error {
	// Não permite deletar o perfil ativo
	if slug == a.profileManager.GetActiveSlug() {
		return fmt.Errorf("não é possível deletar o perfil ativo")
	}

	if err := a.profileManager.Delete(slug); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "profile:deleted", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// GetProfileSearchPaths retorna os caminhos de busca dos perfis
func (a *App) GetProfileSearchPaths() []string {
	return a.profileManager.GetSearchPaths()
}
