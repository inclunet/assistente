package providers

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/logging"
	"assistente/internal/oauthflow"

	"github.com/google/uuid"
	"github.com/pkg/browser"
	"gorm.io/gorm"
)

func (s *Service) oauthStore(ctx context.Context) (oauthflow.Store, error) {
	mgr, ok := s.credMgr.(*credentials.Manager)
	if !ok {
		return nil, errors.New("oauth_vault_unavailable")
	}
	return mgr.OAuthStore(ctx)
}
func (s *Service) CreateChatGPTConnection(ctx context.Context, name string) (oauthflow.Summary, error) {
	generation := s.registry.Generation()
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return oauthflow.Summary{}, errors.New("oauth_label_required")
	}
	store, err := s.oauthStore(ctx)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	id := uuid.NewString()
	r, err := s.oauth.Pending(id, user, "chatgpt")
	if err != nil {
		return oauthflow.Summary{}, err
	}
	p := &llm.ProviderConfig{ID: id, Name: name, Type: llm.ProviderChatGPT, APIFormat: llm.APIFormatOpenAIResponses, BaseURL: r.Resource, CredentialPattern: "oauth:" + id, AuthMode: llm.AuthModeRequired, Timeout: 180}
	if err = persistChatGPTAuthorization(ctx, store, r, p, nil); err != nil {
		return oauthflow.Summary{}, err
	}
	if err = s.publishChatGPT(ctx, store, p, generation); err != nil {
		return oauthflow.Summary{}, err
	}
	return r.Summary(), nil
}
func (s *Service) chatGPTProvider(ctx context.Context, id string) (*llm.ProviderConfig, error) {
	provider, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if provider == nil || provider.Type != llm.ProviderChatGPT {
		return nil, oauthflow.ErrNotFound
	}
	return provider, nil
}
func (s *Service) ChatGPTConnection(ctx context.Context, id string) (oauthflow.Summary, error) {
	provider, err := s.chatGPTProvider(ctx, id)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	store, err := s.oauthStore(ctx)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	if !strings.HasPrefix(provider.CredentialPattern, "oauth:") {
		return oauthflow.Summary{ID: id, Integration: "chatgpt", State: "disconnected"}, nil
	}
	r, err := store.Load(ctx, credentials.OAuthCredentialID(provider.CredentialPattern))
	if errors.Is(err, oauthflow.ErrNotFound) {
		return oauthflow.Summary{ID: id, Integration: "chatgpt", State: "disconnected"}, nil
	}
	if err != nil {
		return oauthflow.Summary{}, err
	}
	if r.Integration != "chatgpt" {
		return oauthflow.Summary{}, oauthflow.ErrResource
	}
	return r.Summary(), nil
}

// Explicit reconnect repairs an imported provider without importing tokens or
// reusing another host's registration. Merely listing it never creates grants.
func (s *Service) ensureChatGPTAuthorization(ctx context.Context, store oauthflow.Store, provider *llm.ProviderConfig, generation uint64) (string, error) {
	if strings.HasPrefix(provider.CredentialPattern, "oauth:") {
		id := credentials.OAuthCredentialID(provider.CredentialPattern)
		r, err := store.Load(ctx, id)
		if err == nil {
			if r.Integration != "chatgpt" {
				return "", oauthflow.ErrResource
			}
			return id, nil
		}
		if !errors.Is(err, oauthflow.ErrNotFound) {
			return "", err
		}
	}
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	r, err := s.oauth.Pending(id, user, "chatgpt")
	if err != nil {
		return "", err
	}
	updated := *provider
	updated.CredentialPattern = "oauth:" + id
	updated.BaseURL = r.Resource
	updated.APIFormat = llm.APIFormatOpenAIResponses
	updated.AuthMode = llm.AuthModeRequired
	if err = persistChatGPTAuthorization(ctx, store, r, &updated, &provider.CredentialPattern); err != nil {
		return "", err
	}
	if err = s.publishChatGPT(ctx, store, &updated, generation); err != nil {
		return "", err
	}
	return id, nil
}
func (s *Service) AuthorizeChatGPT(ctx context.Context, id, completionText string) (oauthflow.Summary, error) {
	generation := s.registry.Generation()
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	key := user + ":" + id
	ctx, cancel := context.WithCancel(ctx)
	s.oauthMu.Lock()
	if _, active := s.oauthAttempts[key]; active {
		s.oauthMu.Unlock()
		cancel()
		return oauthflow.Summary{}, errors.New("oauth_already_pending")
	}
	s.oauthAttempts[key] = cancel
	s.oauthMu.Unlock()
	defer func() { cancel(); s.oauthMu.Lock(); delete(s.oauthAttempts, key); s.oauthMu.Unlock() }()
	store, err := s.oauthStore(ctx)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	if _, err = s.ChatGPTConnection(ctx, id); err != nil {
		return oauthflow.Summary{}, err
	}
	if err = ctx.Err(); err != nil {
		return oauthflow.Summary{}, err
	}
	provider, err := s.chatGPTProvider(ctx, id)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	authorizationID, err := s.ensureChatGPTAuthorization(ctx, store, provider, generation)
	if err != nil {
		return oauthflow.Summary{}, err
	}
	host, err := oauthflow.HostID()
	if err != nil {
		return oauthflow.Summary{}, err
	}
	summary, err := s.oauth.Authorize(ctx, store, authorizationID, host, browser.OpenURL, completionText)
	if err != nil || summary.State != "connected" {
		return summary, err
	}
	// The account's first listed model is its server-defined default. A catalog
	// outage does not undo successful consent; the profile picker can retry.
	provider = s.registry.Get(id)
	if provider != nil && provider.DefaultModel == "" {
		cp, err := s.GetChatProvider(ctx, id)
		if err == nil {
			models, listErr := cp.GetModels(ctx)
			if listErr == nil && len(models) > 0 {
				s.setChatGPTDefaultModel(ctx, store, authorizationID, provider, models[0], generation)
			}
		}
	}
	return summary, nil
}
func (s *Service) CancelChatGPT(ctx context.Context, id string) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if cancel := s.oauthAttempts[user+":"+id]; cancel != nil {
		cancel()
	}
	return nil
}
func (s *Service) DisconnectChatGPT(ctx context.Context, id string) (bool, error) {
	if err := s.CancelChatGPT(ctx, id); err != nil {
		return false, err
	}
	store, err := s.oauthStore(ctx)
	if err != nil {
		return false, err
	}
	provider, err := s.chatGPTProvider(ctx, id)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(provider.CredentialPattern, "oauth:") {
		return true, nil
	}
	authorizationID := credentials.OAuthCredentialID(provider.CredentialPattern)
	if _, err = store.Load(ctx, authorizationID); errors.Is(err, oauthflow.ErrNotFound) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	return s.oauth.Disconnect(ctx, store, authorizationID)
}

func persistChatGPTAuthorization(ctx context.Context, store oauthflow.Store, r oauthflow.Record, p *llm.ProviderConfig, expectedPattern *string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	transaction, ok := store.(interface {
		CreateWithConsumer(context.Context, oauthflow.Record, func(*gorm.DB) error) error
	})
	if !ok {
		return errors.New("oauth_store_not_supported")
	}
	return transaction.CreateWithConsumer(ctx, r, func(tx *gorm.DB) error {
		repository := database.NewProviderRepository(tx)
		if expectedPattern == nil {
			count, err := repository.CountLLMProviders(ctx)
			if err != nil {
				return err
			}
			p.IsDefault = count == 0
		} else {
			current, err := repository.GetLLMProvider(ctx, p.ID)
			if err != nil {
				return err
			}
			if current.Type != string(llm.ProviderChatGPT) || current.CredentialPattern != *expectedPattern {
				return oauthflow.ErrConflict
			}
			fields := map[string]any{"credential_pattern": p.CredentialPattern, "base_url": p.BaseURL, "api_format": string(p.APIFormat), "auth_mode": string(p.AuthMode)}
			result := database.ScopeByUser(ctx, tx.Model(&database.LLMProvider{}), "user_id").Where("id = ? AND credential_pattern = ?", p.ID, *expectedPattern).Updates(fields)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return oauthflow.ErrConflict
			}
			current.CredentialPattern, current.BaseURL = p.CredentialPattern, p.BaseURL
			current.APIFormat, current.AuthMode = string(p.APIFormat), string(p.AuthMode)
			updated, err := fromDBModel(current)
			if err != nil {
				return err
			}
			*p = *updated
			return nil
		}
		return repository.SaveLLMProvider(ctx, toDBModel(p))
	})
}

// Optional catalog persistence must not turn successful consent into a failure.
func (s *Service) setChatGPTDefaultModel(ctx context.Context, store oauthflow.Store, authorizationID string, provider *llm.ProviderConfig, model string, generation uint64) {
	if _, err := store.Load(ctx, authorizationID); err != nil {
		logging.Warnf(ctx, "providers.service", "chatgpt_default_model_authorization_unavailable")
		return
	}
	transaction, ok := store.(interface {
		WithAuthorization(context.Context, string, func(*gorm.DB) error) error
	})
	if !ok {
		return
	}
	var updated *llm.ProviderConfig
	err := transaction.WithAuthorization(ctx, authorizationID, func(tx *gorm.DB) error {
		current, err := database.NewProviderRepository(tx).GetLLMProvider(ctx, provider.ID)
		if err != nil {
			return err
		}
		if current.Type != string(llm.ProviderChatGPT) || current.CredentialPattern != "oauth:"+authorizationID {
			return oauthflow.ErrConflict
		}
		if current.DefaultModel == "" {
			result := database.ScopeByUser(ctx, tx.Model(&database.LLMProvider{}), "user_id").Where("id = ? AND credential_pattern = ? AND default_model = ?", provider.ID, current.CredentialPattern, "").Update("default_model", model)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return oauthflow.ErrConflict
			}
			current.DefaultModel = model
		}
		updated, err = fromDBModel(current)
		return err
	})
	if err != nil {
		logging.Warnf(ctx, "providers.service", "chatgpt_default_model_save_failed")
		return
	}
	if err := s.publishChatGPT(ctx, store, updated, generation); err != nil {
		logging.Warnf(ctx, "providers.service", "chatgpt_default_model_registry_failed")
	}
}

func (s *Service) publishChatGPT(ctx context.Context, store oauthflow.Store, provider *llm.ProviderConfig, generation uint64) error {
	session, ok := store.(interface {
		WithSession(context.Context, func() error) error
	})
	if !ok {
		return errors.New("oauth_store_not_supported")
	}
	s.publicationMu.Lock()
	defer s.publicationMu.Unlock()
	return session.WithSession(ctx, func() error {
		// The consumer may have changed after the authorization transaction.
		// Publish current persisted preferences, never the caller's old snapshot.
		current, err := s.chatGPTProvider(ctx, provider.ID)
		if err != nil {
			return err
		}
		if current.CredentialPattern != provider.CredentialPattern {
			return oauthflow.ErrConflict
		}
		return s.registry.RegisterGeneration(current, generation)
	})
}

// updateChatGPTPreferences edits only consumer preferences. Authorization, client
// registration and tokens remain owned by the shared OAuth lifecycle.
func (s *Service) updateChatGPTPreferences(ctx context.Context, id string, req UpdateRequest) (*UpdateResult, error) {
	if req.Credential != nil || req.AuthMode != "" || req.Type != "" || req.APIFormat != "" || req.BaseURL != "" || req.APIKey != "" ||
		req.ReasoningContentMode != "" || req.ACPCommand != "" || req.ACPArgs != nil ||
		req.ACPEnv != nil || req.ACPCredentialEnv != nil || req.ACPAgentID != nil {
		return nil, errors.New("chatgpt_use_oauth_connection")
	}
	if (req.Name != "" && (strings.TrimSpace(req.Name) == "" || utf8.RuneCountInString(strings.TrimSpace(req.Name)) > 100)) ||
		(req.DefaultModel != "" && (strings.TrimSpace(req.DefaultModel) != req.DefaultModel || len(req.DefaultModel) > 256)) {
		return nil, errors.New("chatgpt_invalid_preferences")
	}
	generation := s.registry.Generation()
	provider, err := s.chatGPTProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	store, err := s.oauthStore(ctx)
	if err != nil {
		return nil, err
	}
	authorizationID := credentials.OAuthCredentialID(provider.CredentialPattern)
	transaction, ok := store.(interface {
		WithAuthorization(context.Context, string, func(*gorm.DB) error) error
	})
	if !ok {
		return nil, errors.New("oauth_store_not_supported")
	}
	var updated *llm.ProviderConfig
	err = transaction.WithAuthorization(ctx, authorizationID, func(tx *gorm.DB) error {
		current, err := database.NewProviderRepository(tx).GetLLMProvider(ctx, id)
		if err != nil {
			return err
		}
		if current.Type != string(llm.ProviderChatGPT) || current.CredentialPattern != provider.CredentialPattern {
			return oauthflow.ErrConflict
		}
		fields := map[string]any{}
		if req.Name != "" {
			current.Name = strings.TrimSpace(req.Name)
			fields["name"] = current.Name
		}
		if req.DefaultModel != "" {
			current.DefaultModel = req.DefaultModel
			fields["default_model"] = current.DefaultModel
		}
		if len(fields) > 0 {
			result := database.ScopeByUser(ctx, tx.Model(&database.LLMProvider{}), "user_id").Where("id = ? AND credential_pattern = ?", id, provider.CredentialPattern).Updates(fields)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return oauthflow.ErrConflict
			}
		}
		updated, err = fromDBModel(current)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = s.publishChatGPT(ctx, store, updated, generation); err != nil {
		return nil, err
	}
	return &UpdateResult{Provider: updated, CredentialConfigured: true}, nil
}
