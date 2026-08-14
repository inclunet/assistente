package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/contextprovider"
	"assistente/internal/profiles"
	"context"
	"sync"
)

// Profiles é o bind Wails do domínio profiles (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Profiles struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.ProfilesController
}

// NewProfiles cria o bind vazio; AttachProfiles preenche session + controller no startup.
func NewProfiles() *Profiles {
	return &Profiles{}
}

// AttachProfiles associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachProfiles(p *Profiles, session Session, ctrl *controllers.ProfilesController) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.session = session
	p.ctrl = ctrl
}

func (p *Profiles) deps() (Session, *controllers.ProfilesController, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.session == nil || p.ctrl == nil {
		return nil, nil, ErrProfilesNotWired
	}
	return p.session, p.ctrl, nil
}

// GetProfiles retorna a lista de perfis disponíveis.
func (p *Profiles) GetProfiles() ([]profiles.ProfileInfo, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]profiles.ProfileInfo, error) {
		return ctrl.GetProfiles()
	})
}

// GetProfile retorna um perfil pelo slug.
func (p *Profiles) GetProfile(slug string) (*profiles.Profile, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*profiles.Profile, error) {
		return ctrl.GetProfile(slug)
	})
}

// GetActiveProfile retorna o perfil ativo.
func (p *Profiles) GetActiveProfile() (*profiles.Profile, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*profiles.Profile, error) {
		return ctrl.GetActiveProfile()
	})
}

// GetActiveProfileSlug retorna o slug do perfil ativo.
func (p *Profiles) GetActiveProfileSlug() (string, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.GetActiveProfileSlug(), nil
	})
}

// GetActiveProfileAndSlug resolve perfil ativo e slug numa única passada.
func (p *Profiles) GetActiveProfileAndSlug() (*profiles.ActiveProfile, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*profiles.ActiveProfile, error) {
		return ctrl.GetActiveProfileAndSlug()
	})
}

// SetActiveProfile ativa o perfil pelo slug.
func (p *Profiles) SetActiveProfile(slug string) error {
	session, ctrl, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetActiveProfile(slug)
	})
	return err
}

// CreateProfile cria um novo perfil.
func (p *Profiles) CreateProfile(profile profiles.Profile) (string, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.CreateProfile(profile)
	})
}

// DuplicateProfile duplica um perfil existente.
func (p *Profiles) DuplicateProfile(slug string) (string, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.DuplicateProfile(slug)
	})
}

// UpdateProfile atualiza um perfil existente.
func (p *Profiles) UpdateProfile(slug string, profile profiles.Profile) error {
	session, ctrl, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateProfile(slug, profile)
	})
	return err
}

// DeleteProfile exclui um perfil.
func (p *Profiles) DeleteProfile(slug string) error {
	session, ctrl, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteProfile(slug)
	})
	return err
}

// GetProfileSearchPaths retorna os caminhos de busca de perfis.
func (p *Profiles) GetProfileSearchPaths() ([]string, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		return ctrl.GetProfileSearchPaths(), nil
	})
}

// GetContextProviders retorna os metadados dos context providers registrados.
func (p *Profiles) GetContextProviders() ([]contextprovider.ProviderMetadata, error) {
	session, ctrl, err := p.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]contextprovider.ProviderMetadata, error) {
		return ctrl.GetContextProviders(), nil
	})
}

// knownProfileMediaTypes são os únicos tipos aceitos por UpdateProfileMediaSupport
// (espelha o switch do ProfilesController). Tipos desconhecidos são no-op após auth.
var knownProfileMediaTypes = map[string]struct{}{
	"audio":    {},
	"image":    {},
	"document": {},
	"video":    {},
}

// UpdateProfileMediaSupport atualiza o MediaSupport do perfil ativo e salva.
// O controller não retorna error (falhas são logadas); a borda só propaga
// ErrProfilesNotWired / falha de auth via WithUser.
// mediaType desconhecido: WithUser ainda roda (auth), mas não chama o controller.
func (p *Profiles) UpdateProfileMediaSupport(mediaType string, supported bool) error {
	session, ctrl, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		if _, ok := knownProfileMediaTypes[mediaType]; !ok {
			return struct{}{}, nil
		}
		ctrl.UpdateProfileMediaSupport(mediaType, supported)
		return struct{}{}, nil
	})
	return err
}
