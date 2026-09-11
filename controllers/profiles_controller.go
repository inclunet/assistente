package controllers

import (
	"assistente/internal/contextprovider"
	"assistente/internal/core/ports"
	"assistente/internal/logging"
	"assistente/internal/profiles"
	"context"
	"fmt"
)

// ProfilesController é o adapter primário (Inbound) para operações de perfis.
// Expõe a API de perfis ao frontend sem referências ao megastruct App.
type ProfilesController struct {
	profileMgr       *profiles.Manager
	emitter          ports.Emitter
	contextProviders *contextprovider.Registry
	onProfileChanged func(slug string) // callback para reinicializar LLM/Speech/Hotkeys
	deleteProfile    func(context.Context, string, func() error) error
	mutateProfiles   func(func() error) error
}

// ProfilesControllerConfig agrupa as dependências do ProfilesController.
type ProfilesControllerConfig struct {
	ProfileMgr       *profiles.Manager
	Emitter          ports.Emitter
	ContextProviders *contextprovider.Registry
	OnProfileChanged func(slug string)
	DeleteProfile    func(context.Context, string, func() error) error
	MutateProfiles   func(func() error) error
}

// NewProfilesController cria um ProfilesController com suas dependências.
func NewProfilesController(cfg ProfilesControllerConfig) *ProfilesController {
	return &ProfilesController{
		profileMgr:       cfg.ProfileMgr,
		emitter:          cfg.Emitter,
		contextProviders: cfg.ContextProviders,
		onProfileChanged: cfg.OnProfileChanged,
		deleteProfile:    cfg.DeleteProfile,
		mutateProfiles:   cfg.MutateProfiles,
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
func (c *ProfilesController) GetActiveProfileAndSlug() (*profiles.ActiveProfile, error) {
	return c.profileMgr.GetActiveAndSlug()
}

func (c *ProfilesController) SetActiveProfile(slug string) error {
	if err := c.mutateProfileFiles(func() error { return c.profileMgr.SetActive(slug) }); err != nil {
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
	if err := c.mutateProfileFiles(func() error { return c.profileMgr.Update(slug, &profile) }); err != nil {
		return err
	}
	if slug == c.profileMgr.GetActiveSlug() && c.onProfileChanged != nil {
		logging.Infof(context.Background(), "controllers.profiles-controller", "[Profile] Perfil ativo atualizado, disparando onProfileChanged")
		c.onProfileChanged(slug)
	}
	c.emitter.Emit("profile:updated", map[string]interface{}{"slug": slug, "name": profile.Name})
	return nil
}

func (c *ProfilesController) DeleteProfile(slug string) error {
	return c.DeleteProfileContext(context.Background(), slug)
}

func (c *ProfilesController) DeleteProfileContext(ctx context.Context, slug string) error {
	deleteFile := func() error {
		if slug == c.profileMgr.GetActiveSlug() {
			return fmt.Errorf("não é possível deletar o perfil ativo")
		}
		return c.profileMgr.Delete(slug)
	}
	var err error
	if c.deleteProfile != nil {
		err = c.deleteProfile(ctx, slug, deleteFile)
	} else {
		err = deleteFile()
	}
	if err != nil {
		return err
	}
	c.emitter.Emit("profile:deleted", map[string]interface{}{"slug": slug})
	return nil
}

func (c *ProfilesController) GetProfileSearchPaths() []string {
	return c.profileMgr.GetSearchPaths()
}

// GetContextProviders retorna os metadados dos context providers registrados.
func (c *ProfilesController) GetContextProviders() []contextprovider.ProviderMetadata {
	if c.contextProviders == nil {
		return []contextprovider.ProviderMetadata{}
	}
	return c.contextProviders.Metadata()
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
	if err := c.mutateProfileFiles(func() error { return c.profileMgr.Update(slug, profile) }); err != nil {
		logging.Errorf(context.Background(), "controllers.profiles-controller", "[MediaSupport] Erro ao salvar perfil: %v", err)
	} else {
		logging.Infof(context.Background(), "controllers.profiles-controller", "[MediaSupport] Perfil atualizado: %s=%v", mediaType, supported)
	}
}

func (c *ProfilesController) mutateProfileFiles(mutate func() error) error {
	if c.mutateProfiles != nil {
		return c.mutateProfiles(mutate)
	}
	return mutate()
}
