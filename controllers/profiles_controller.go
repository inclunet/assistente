package controllers

import (
	"assistente/internal/contextprovider"
	"assistente/internal/core/ports"
	"assistente/internal/logging"
	"assistente/internal/profiles"
	"context"
	"encoding/json"
	"fmt"
)

// ProfilesController é o adapter primário (Inbound) para operações de perfis.
// Expõe a API de perfis ao frontend sem referências ao megastruct App.
type ProfilesController struct {
	profileMgr       *profiles.Manager
	emitter          ports.Emitter
	contextProviders *contextprovider.Registry
	onProfileChanged func(slug string) // callback para reinicializar LLM/Speech/Hotkeys
	commitMutation   CommitProfileMutation
}

// CommitProfileMutation entrega uma mutação preparada ao coordenador da App.
// O coordenador deve executar a mutação e chamar publish somente depois de um
// commit bem-sucedido. O slug recebido por publish é o resultado efetivo da
// operação (em particular, o slug gerado para create/duplicate).
type CommitProfileMutation func(context.Context, *profiles.CommandMutation, func(string) error) (string, error)

// PreparedProfileMutation conserva o plano e a publicação do domínio juntos.
// O conteúdo é capturado na preparação; não é autoridade fornecida pela UI.
type PreparedProfileMutation struct {
	Mutation *profiles.CommandMutation
	Publish  func(string) error
}

func (c *ProfilesController) PrepareCommandMutation(operation, slug string, profile *profiles.Profile, fingerprint string) (*PreparedProfileMutation, error) {
	_, previousActive, currentFingerprint, err := c.profileMgr.ReadActiveCommandTarget()
	if err != nil {
		return nil, err
	}
	if fingerprint != currentFingerprint {
		return nil, profiles.ErrStaleCommandMutation
	}
	var frozen *profiles.Profile
	if profile != nil {
		data, err := json.Marshal(profile)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &frozen); err != nil {
			return nil, err
		}
	}
	mutation, err := c.profileMgr.PrepareCommandMutation(operation, slug, frozen, fingerprint)
	if err != nil {
		return nil, err
	}
	return &PreparedProfileMutation{Mutation: mutation, Publish: func(resultSlug string) error {
		if operation == profiles.CommandMutationDelete {
			c.emit("profile:deleted", map[string]interface{}{"slug": resultSlug})
			return nil
		}
		value, err := c.profileMgr.Get(resultSlug)
		if err != nil {
			return err
		}
		_, active, _, err := c.profileMgr.ReadActiveCommandTarget()
		if err != nil {
			return err
		}
		refresh := operation == profiles.CommandMutationActivate ||
			(operation == profiles.CommandMutationCreate && (frozen.Active || active != previousActive)) ||
			(operation == profiles.CommandMutationUpdate && (resultSlug == previousActive || active != previousActive))
		if refresh && c.onProfileChanged != nil {
			if active == "" {
				active = resultSlug
			}
			c.onProfileChanged(active)
		}
		event := "profile:created"
		if operation == profiles.CommandMutationUpdate {
			event = "profile:updated"
		}
		if operation == profiles.CommandMutationActivate {
			event = "profile:changed"
		}
		payload := map[string]interface{}{"slug": resultSlug}
		if operation != profiles.CommandMutationActivate && value != nil {
			payload["name"] = value.Name
		}
		c.emit(event, payload)
		return nil
	}}, nil
}

// ProfilesControllerConfig agrupa as dependências do ProfilesController.
type ProfilesControllerConfig struct {
	ProfileMgr            *profiles.Manager
	Emitter               ports.Emitter
	ContextProviders      *contextprovider.Registry
	OnProfileChanged      func(slug string)
	CommitProfileMutation CommitProfileMutation
}

// NewProfilesController cria um ProfilesController com suas dependências.
func NewProfilesController(cfg ProfilesControllerConfig) *ProfilesController {
	return &ProfilesController{
		profileMgr:       cfg.ProfileMgr,
		emitter:          cfg.Emitter,
		contextProviders: cfg.ContextProviders,
		onProfileChanged: cfg.OnProfileChanged,
		commitMutation:   cfg.CommitProfileMutation,
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

// Os callers nativos e o catálogo compartilham plano e publicação.
func (c *ProfilesController) prepareNativeMutation(operation, slug string, profile *profiles.Profile) (*PreparedProfileMutation, error) {
	fingerprint, err := c.profileMgr.CommandMutationSnapshot(slug)
	if err != nil {
		return nil, err
	}
	return c.PrepareCommandMutation(operation, slug, profile, fingerprint)
}

func (c *ProfilesController) runNativeMutation(ctx context.Context, operation, slug string, profile *profiles.Profile) (string, error) {
	prepared, err := c.prepareNativeMutation(operation, slug, profile)
	if err != nil {
		return "", err
	}
	return c.commitPreparedMutation(ctx, prepared.Mutation, prepared.Publish)
}

func (c *ProfilesController) SetActiveProfileContext(ctx context.Context, slug string) error {
	_, err := c.runNativeMutation(ctx, profiles.CommandMutationActivate, slug, nil)
	return err
}

func (c *ProfilesController) CreateProfileContext(ctx context.Context, profile profiles.Profile) (string, error) {
	return c.runNativeMutation(ctx, profiles.CommandMutationCreate, "", &profile)
}

func (c *ProfilesController) DuplicateProfileContext(ctx context.Context, slug string) (string, error) {
	return c.runNativeMutation(ctx, profiles.CommandMutationDuplicate, slug, nil)
}

func (c *ProfilesController) UpdateProfileContext(ctx context.Context, slug string, profile profiles.Profile) error {
	_, err := c.runNativeMutation(ctx, profiles.CommandMutationUpdate, slug, &profile)
	return err
}

func (c *ProfilesController) DeleteProfileContext(ctx context.Context, slug string) error {
	_, err := c.runNativeMutation(ctx, profiles.CommandMutationDelete, slug, nil)
	return err
}

func (c *ProfilesController) commitPreparedMutation(ctx context.Context, mutation *profiles.CommandMutation, publish func(string) error) (string, error) {
	ctx = ctxOrBackground(ctx)
	if c.commitMutation == nil {
		return "", fmt.Errorf("coordenador de mutações de perfil não configurado")
	}
	return c.commitMutation(ctx, mutation, publish)
}

func (c *ProfilesController) emit(event string, payload any) {
	if c.emitter != nil {
		c.emitter.Emit(event, payload)
	}
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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
	if err := c.UpdateProfileMediaSupportContext(context.Background(), mediaType, supported); err != nil {
		logging.Errorf(context.Background(), "controllers.profiles-controller", "[MediaSupport] Erro ao salvar perfil: %v", err)
	}
}

// UpdateProfileMediaSupportContext atualiza MediaSupport pela mutação
// coordenada, sem disparar OnProfileChanged: a detecção automática não exige
// reinicialização do runtime.
func (c *ProfilesController) UpdateProfileMediaSupportContext(ctx context.Context, mediaType string, supported bool) error {
	profile, activeSlug, fingerprint, err := c.profileMgr.ReadActiveCommandTarget()
	if err != nil {
		return err
	}
	if profile == nil || activeSlug == "" {
		return fmt.Errorf("perfil ativo não encontrado")
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
	default:
		return nil
	}

	mutation, err := c.profileMgr.PrepareCommandMutation(profiles.CommandMutationUpdate, activeSlug, profile, fingerprint)
	if err != nil {
		return err
	}
	_, err = c.commitPreparedMutation(ctx, mutation, func(string) error {
		logging.Infof(ctxOrBackground(ctx), "controllers.profiles-controller", "[MediaSupport] Perfil atualizado: %s=%v", mediaType, supported)
		return nil
	})
	return err
}
