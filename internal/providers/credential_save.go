package providers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/logging"
	"assistente/internal/oauthflow"
	"context"
	"errors"
	"gorm.io/gorm"
	"reflect"
)

// CredentialSpec is a vault draft, never a field in ProviderConfig.
type CredentialSpec struct {
	Pattern string
	Auth    *credentials.AuthConfig
}

func validateCredentialMode(p *llm.ProviderConfig, draft *CredentialSpec) error {
	switch p.AuthMode {
	case "", llm.AuthModeRequired, llm.AuthModeOptional, llm.AuthModeNone:
	default:
		return credentials.ErrCredentialResolution
	}
	if p.GetAPIFormat() == llm.APIFormatGoogle && p.EffectiveAuthMode() != llm.AuthModeRequired {
		return credentials.ErrCredentialResolution
	}
	if draft == nil {
		return nil
	}
	if p.IsACP() || p.Type == llm.ProviderChatGPT || p.EffectiveAuthMode() == llm.AuthModeNone || draft.Auth == nil || credentials.IsManagedPattern(draft.Pattern) || draft.Auth.Source == "oauth" {
		return credentials.ErrCredentialResolution
	}
	switch draft.Auth.Type {
	case "bearer", "basic", "custom":
	default:
		return credentials.ErrCredentialResolution
	}
	if p.GetAPIFormat() == llm.APIFormatGoogle && draft.Auth.Type != "bearer" {
		return credentials.ErrCredentialResolution
	}
	return credentials.ValidateSource(draft.Auth)
}

// Resolve metadata only. An explicit existing reference wins on the same origin;
// new destinations use the manager's effective pattern, including wildcards.
func (s *Service) prepareCredential(ctx context.Context, p, existing *llm.ProviderConfig, draft *CredentialSpec, explicitKey bool) error {
	if err := validateCredentialMode(p, draft); err != nil {
		return err
	}
	if p.IsACP() {
		return nil
	}
	if p.CredentialPattern == "" {
		hostname, err := ExtractHostname(p.BaseURL)
		if err != nil {
			return err
		}
		p.CredentialPattern = hostname
	}
	if existing != nil && sameCredentialOrigin(p.BaseURL, existing.BaseURL) && existing.CredentialPattern != "" {
		p.CredentialPattern = existing.CredentialPattern
	} else if mgr, ok := s.credMgr.(*credentials.Manager); ok && !explicitKey {
		pattern, err := mgr.PatternForURLWithContext(ctx, p.BaseURL)
		if err != nil {
			return err
		}
		if pattern != "" {
			p.CredentialPattern = pattern
		}
	}
	if draft != nil && (draft.Pattern != p.CredentialPattern || credentials.IsManagedPattern(p.CredentialPattern) || (existing != nil && credentials.IsManagedPattern(existing.CredentialPattern))) {
		return credentials.ErrCredentialResolution
	}
	if draft == nil && !explicitKey && p.GetAPIFormat() == llm.APIFormatGoogle {
		auth, err := s.credentialConfig(ctx, p.CredentialPattern)
		if err != nil {
			return err
		}
		if auth != nil && auth.Type != "bearer" {
			return credentials.ErrCredentialResolution
		}
	}
	return nil
}

// Reuses the vault's atomic consumer transaction. No secret resolution, network
// request or shell execution is performed while saving configuration.
type credentialConsumerStore interface {
	SaveWithConsumer(context.Context, string, *credentials.AuthConfig, *oauthflow.Record, func(*gorm.DB) error, func()) error
}
type credentialSaveScope struct {
	store      credentialConsumerStore
	generation uint64
}

// Capture session identity at operation entry, before any consumer or vault reads.
func (s *Service) captureCredentialSave(ctx context.Context, draft *CredentialSpec) (*credentialSaveScope, error) {
	if draft == nil {
		return nil, nil
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	mgr, ok := s.credMgr.(*credentials.Manager)
	if !ok {
		return nil, credentials.ErrStoreNotReady
	}
	if _, ok := s.store.(*DBStore); !ok {
		return nil, credentials.ErrStoreNotReady
	}
	generation := s.registry.Generation()
	captured, err := mgr.OAuthStore(ctx)
	if err != nil {
		return nil, err
	}
	atomicStore, ok := captured.(credentialConsumerStore)
	if !ok {
		return nil, credentials.ErrStoreNotReady
	}
	return &credentialSaveScope{store: atomicStore, generation: generation}, nil
}

func (s *Service) saveWithCredential(ctx context.Context, p, expected *llm.ProviderConfig, draft *CredentialSpec, scope *credentialSaveScope) error {
	if scope == nil {
		return credentials.ErrStoreNotReady
	}
	generation := scope.generation
	atomicStore := scope.store
	s.publicationMu.Lock()
	defer s.publicationMu.Unlock()
	// Capture the persisted consumer before entering the vault's lock/transaction.
	baseline, err := database.NewProviderRepository(database.DB()).GetLLMProvider(ctx, p.ID)
	if expected == nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return oauthflow.ErrConflict
		}
	} else if err != nil || baseline.Type != string(expected.Type) || baseline.BaseURL != expected.BaseURL || baseline.CredentialPattern != expected.CredentialPattern {
		return oauthflow.ErrConflict
	}
	if expected != nil {
		saved, err := fromDBModel(baseline)
		if err != nil {
			return err
		}
		a, b := toDBModel(saved), toDBModel(expected)
		a.IsDefault = b.IsDefault // default changes are independent of this partial edit
		if !reflect.DeepEqual(a, b) {
			return oauthflow.ErrConflict
		}
	}
	var publishErr error
	err = s.registry.WithGeneration(generation, func() error {
		return atomicStore.SaveWithConsumer(ctx, draft.Pattern, draft.Auth, nil, func(tx *gorm.DB) error {
			if generation != s.registry.Generation() {
				return oauthflow.ErrConflict
			}
			repo := database.NewProviderRepository(tx)
			current, loadErr := repo.GetLLMProvider(ctx, p.ID)
			if expected == nil {
				if !errors.Is(loadErr, gorm.ErrRecordNotFound) {
					return oauthflow.ErrConflict
				}
				count, err := repo.CountLLMProviders(ctx)
				if err != nil {
					return err
				}
				p.IsDefault = count == 0
			} else {
				if loadErr != nil || !current.UpdatedAt.Equal(baseline.UpdatedAt) || current.Type != baseline.Type || current.CredentialPattern != baseline.CredentialPattern {
					return oauthflow.ErrConflict
				}
				p.IsDefault = current.IsDefault
			}
			return repo.SaveLLMProvider(ctx, toDBModel(p))
		}, func() { publishErr = s.registry.RegisterGeneration(p, generation) })
	})
	if err != nil {
		return err
	}
	return publishErr
}

// Duplication accepts a scoped consumer ID, never an arbitrary credential alias.
func (s *Service) credentialSourceProvider(ctx context.Context, target *llm.ProviderConfig, sourceID string) (*llm.ProviderConfig, error) {
	if sourceID == "" {
		return nil, nil
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	source, err := s.store.Get(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if source == nil || source.IsACP() || target.IsACP() || source.Type == llm.ProviderChatGPT ||
		source.Type != target.Type || source.GetAPIFormat() != target.GetAPIFormat() ||
		!sameCredentialOrigin(source.BaseURL, target.BaseURL) || credentials.IsManagedPattern(source.CredentialPattern) {
		return nil, credentials.ErrCredentialResolution
	}
	if source.CredentialPattern != "" {
		auth, err := s.credentialConfig(ctx, source.CredentialPattern)
		if err != nil {
			return nil, err
		}
		if auth != nil && auth.Source == "oauth" {
			return nil, credentials.ErrCredentialResolution
		}
	}
	return source, nil
}

// Keep the same session reservation for existing references and legacy API keys.
// The APIKey compatibility path still uses separate vault/consumer writes.
func (s *Service) saveWithoutCredentialDraft(ctx context.Context, p *llm.ProviderConfig, generation uint64, apiKey string, makeDefault bool) error {
	return s.registry.WithGeneration(generation, func() error {
		if apiKey != "" {
			if credentials.IsManagedPattern(p.CredentialPattern) {
				return oauthflow.ErrConflict
			}
			if err := s.credMgr.RegisterPatternWithContext(ctx, p.CredentialPattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: apiKey}); err != nil {
				return err
			}
		}
		if err := s.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
			return err
		}
		if err := s.registry.RegisterGeneration(p, generation); err != nil {
			return err
		}
		if makeDefault {
			if err := s.store.SetDefault(ctx, p.ID); err != nil {
				logging.Warnf(ctx, "providers.service", "erro ao marcar provedor como default: %v", err)
			}
		}
		return nil
	})
}
