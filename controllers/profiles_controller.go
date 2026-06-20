package controllers

import (
	"fmt"
	"log"

	"assistente/internal/core/ports"
	"assistente/internal/profiles"
)

// ProfilesController é o adapter primário (Inbound) para operações de perfis.
// Expõe a API de perfis ao frontend sem referências ao megastruct App.
type ProfilesController struct {
	profileMgr       *profiles.Manager
	emitter          ports.Emitter
	onProfileChanged func(slug string) // callback para reinicializar LLM/Speech/Hotkeys
}

// ProfilesControllerConfig agrupa as dependências do ProfilesController.
type ProfilesControllerConfig struct {
	ProfileMgr       *profiles.Manager
	Emitter          ports.Emitter
	OnProfileChanged func(slug string)
}

// NewProfilesController cria um ProfilesController com suas dependências.
func NewProfilesController(cfg ProfilesControllerConfig) *ProfilesController {
	return &ProfilesController{
		profileMgr:       cfg.ProfileMgr,
		emitter:          cfg.Emitter,
		onProfileChanged: cfg.OnProfileChanged,
	}
}

func (c *ProfilesController) GetProfiles() ([]profiles.ProfileInfo, error) {
	return c.profileMgr.List()
}

func (c *ProfilesController) GetProfile(slug string) (*profiles.Profile, error) {
	return c.profileMgr.Get(slug)
}

func (c *ProfilesController) GetActiveProfile() (*profiles.Profile, error) {
	return c.profileMgr.GetActive()
}

func (c *ProfilesController) GetActiveProfileSlug() string {
	return c.profileMgr.GetActiveSlug()
}

// GetActiveProfileAndSlug resolve perfil ativo e slug numa única passada. Usado
// por operações de escrita (ex.: CLI fixando o modelo no perfil ativo).
func (c *ProfilesController) GetActiveProfileAndSlug() (*profiles.Profile, string, error) {
	return c.profileMgr.GetActiveAndSlug()
}

func (c *ProfilesController) SetActiveProfile(slug string) error {
	if err := c.profileMgr.SetActive(slug); err != nil {
		return err
	}
	if c.onProfileChanged != nil {
		c.onProfileChanged(slug)
	}
	c.emitter.Emit("profile:changed", map[string]interface{}{"slug": slug})
	return nil
}

func (c *ProfilesController) CreateProfile(profile profiles.Profile) (string, error) {
	slug, err := c.profileMgr.Create(&profile)
	if err != nil {
		return "", err
	}
	c.emitter.Emit("profile:created", map[string]interface{}{"slug": slug, "name": profile.Name})
	return slug, nil
}

func (c *ProfilesController) DuplicateProfile(slug string) (string, error) {
	newSlug, err := c.profileMgr.Duplicate(slug)
	if err != nil {
		return "", err
	}
	if profile, err := c.profileMgr.Get(newSlug); err == nil && profile != nil {
		c.emitter.Emit("profile:created", map[string]interface{}{"slug": newSlug, "name": profile.Name})
	}
	return newSlug, nil
}

func (c *ProfilesController) UpdateProfile(slug string, profile profiles.Profile) error {
	if err := c.profileMgr.Update(slug, &profile); err != nil {
		return err
	}
	if slug == c.profileMgr.GetActiveSlug() && c.onProfileChanged != nil {
		log.Printf("[Profile] Perfil ativo atualizado, disparando onProfileChanged")
		c.onProfileChanged(slug)
	}
	c.emitter.Emit("profile:updated", map[string]interface{}{"slug": slug, "name": profile.Name})
	return nil
}

func (c *ProfilesController) DeleteProfile(slug string) error {
	if slug == c.profileMgr.GetActiveSlug() {
		return fmt.Errorf("não é possível deletar o perfil ativo")
	}
	if err := c.profileMgr.Delete(slug); err != nil {
		return err
	}
	c.emitter.Emit("profile:deleted", map[string]interface{}{"slug": slug})
	return nil
}

func (c *ProfilesController) GetProfileSearchPaths() []string {
	return c.profileMgr.GetSearchPaths()
}

// UpdateProfileMediaSupport atualiza o MediaSupport de um perfil e salva.
// Chamado quando detectamos que um modelo não suporta determinado tipo de mídia.
func (c *ProfilesController) UpdateProfileMediaSupport(mediaType string, supported bool) {
	profile, err := c.profileMgr.GetActive()
	if err != nil || profile == nil {
		return
	}

	if profile.MediaSupport == nil {
		profile.MediaSupport = &profiles.MediaSupport{}
	}

	switch mediaType {
	case "audio":
		profile.MediaSupport.Audio = &supported
	case "image":
		profile.MediaSupport.Image = &supported
	case "document":
		profile.MediaSupport.Document = &supported
	case "video":
		profile.MediaSupport.Video = &supported
	}

	slug := c.profileMgr.GetActiveSlug()
	if slug == "" {
		return
	}
	if err := c.profileMgr.Update(slug, profile); err != nil {
		log.Printf("[MediaSupport] Erro ao salvar perfil: %v", err)
	} else {
		log.Printf("[MediaSupport] Perfil atualizado: %s=%v", mediaType, supported)
	}
}
