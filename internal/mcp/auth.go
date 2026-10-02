package mcp

import (
	"context"
	"fmt"
	"reflect"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func (m *Manager) detachLegacyOAuth(ctx context.Context, original, cfg ServerConfig, remove bool) error {
	if m.credMgr == nil {
		return oauthflow.ErrResource
	}
	cfg.ID, cfg.UserID, cfg.Slug = original.ID, original.UserID, original.Slug
	clearOAuthConfiguration(&cfg)
	// The cache may already lag another process. Compare publication with the
	// cache captured here, while the transaction validates the DB snapshot.
	cached := original
	m.mu.RLock()
	if current := m.servers[original.Slug]; current != nil {
		cached = current.Config
	}
	m.mu.RUnlock()
	err := m.credMgr.ClearLegacyOAuthWithConsumer(ctx, original.Slug, original.ID, hostnameFromURL(original.URL), func(tx *gorm.DB) error {
		current, err := NewDBRepository(tx).GetServerByID(ctx, original.ID)
		if err != nil || !reflect.DeepEqual(persistedLegacyConfig(*current), persistedLegacyConfig(original)) {
			return oauthflow.ErrConflict
		}
		if remove {
			return NewDBRepository(tx).DeleteServer(ctx, original.Slug)
		}
		return NewDBRepository(tx).SaveServer(ctx, &cfg)
	}, func() {
		m.publishLegacyDetach(cached, cfg, remove)
	})
	if err != nil {
		return err
	}
	_ = m.Disconnect(original.Slug)
	m.emit("mcp:config_changed", map[string]string{"slug": original.Slug})
	return nil
}

func (m *Manager) publishLegacyDetach(original, cfg ServerConfig, remove bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current := m.servers[original.Slug]; current != nil && reflect.DeepEqual(persistedLegacyConfig(current.Config), persistedLegacyConfig(original)) {
		if remove {
			delete(m.servers, original.Slug)
		} else {
			current.Config = cfg
		}
	}
}

func (m *Manager) SaveServerAuth(slug, authType, token, username, password, clientSecret string) error {
	if m.credMgr == nil {
		return fmt.Errorf("credential manager nao inicializado")
	}

	cfg, err := m.GetConfig(slug)
	if err != nil {
		return err
	}

	ctx := m.credentialContext()
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}

	if cfg.OAuthAuthorizationID != "" {
		if authType != string(cfg.AuthType) {
			return fmt.Errorf("oauth_resource_not_authorized")
		}
		return m.saveManagedOAuth(slug, *cfg, &clientSecret)
	}
	switch authType {
	case "bearer":
		hostname := hostnameFromURL(cfg.URL)
		if hostname == "" {
			return fmt.Errorf("servidor MCP '%s' nao tem URL valida", slug)
		}
		return m.credMgr.RegisterPatternWithContext(ctx, hostname, &credentials.AuthConfig{Source: "static",
			Type:  "bearer",
			Token: token,
		})

	case "basic":
		hostname := hostnameFromURL(cfg.URL)
		if hostname == "" {
			return fmt.Errorf("servidor MCP '%s' nao tem URL valida", slug)
		}
		return m.credMgr.RegisterPatternWithContext(ctx, hostname, &credentials.AuthConfig{Source: "static",
			Type:     "basic",
			Username: username,
			Password: password,
		})

	case "oauth2_client_credentials", "oauth2_pkce":
		return m.credMgr.RegisterPatternWithContext(ctx, clientCredPattern(slug), &credentials.AuthConfig{Source: "static",
			Type:         "oauth2",
			ClientID:     cfg.OAuth2ClientID,
			ClientSecret: clientSecret,
		})

	default:
		return fmt.Errorf("tipo de autenticacao invalido: %s", authType)
	}
}

func (m *Manager) DeleteServerAuth(slug string) error {
	if m.credMgr == nil {
		return fmt.Errorf("credential manager nao inicializado")
	}

	ctx := m.credentialContext()
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}

	cfgManaged, err := m.GetConfig(slug)
	if err != nil {
		return err
	}
	if cfgManaged.OAuthAuthorizationID != "" {
		store, r, service, err := m.managedOAuth(ctx, *cfgManaged)
		if err != nil {
			return err
		}
		if err = service.InvalidateAndClearClientSecret(ctx, store, r.ID); err != nil {
			return err
		}
		_ = m.Disconnect(slug)
		return nil
	}
	if cfgManaged.AuthType == AuthOAuth2PKCE && cfgManaged.ID != "" {
		return m.credMgr.ClearLegacyOAuth(ctx, slug, cfgManaged.ID, hostnameFromURL(cfgManaged.URL))
	}
	// Limpar entradas OAuth (client + tokens)
	if err := m.credMgr.DeletePattern(ctx, clientCredPattern(slug)); err != nil {
		return err
	}
	if err := m.credMgr.DeletePattern(ctx, userTokensPattern(slug)); err != nil {
		return err
	}

	// Limpar entrada legacy por hostname (bearer/basic)
	cfg, err := m.GetConfig(slug)
	if err == nil {
		if hostname := hostnameFromURL(cfg.URL); hostname != "" {
			_ = m.credMgr.DeletePattern(ctx, hostname)
		}
	}

	return nil
}

func (m *Manager) GetServerAuthInfo(slug string) (string, bool, error) {
	if m.credMgr == nil {
		return "", false, fmt.Errorf("credential manager nao inicializado")
	}

	cfg, err := m.GetConfig(slug)
	if err != nil {
		return "", false, err
	}

	if cfg.OAuthAuthorizationID != "" {
		_, r, _, err := m.managedOAuth(m.credentialContext(), *cfg)
		return string(cfg.AuthType), err == nil && (r.Tokens.Access != "" || r.Client.Secret != ""), err
	}
	// Verifica entrada OAuth (mcp-client:{slug})
	ctx := m.credentialContext()
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", false, err
	}
	if cfg.AuthType == AuthOAuth2PKCE && cfg.ID != "" {
		entries, err := m.credMgr.InspectLegacyOAuth(ctx)
		if err != nil {
			return "", false, err
		}
		for _, entry := range entries {
			if entry.Pattern == userTokensPattern(slug) || entry.Pattern == clientCredPattern(slug) {
				// Even an interrupted/unreadable grant is configuration that the
				// user must be able to explicitly remove. Never resolve its source.
				return string(cfg.AuthType), true, nil
			}
		}
	}

	clientAuth, _ := m.credMgr.GetConfigByPatternWithContext(ctx, clientCredPattern(slug))
	if clientAuth != nil && clientAuth.Source != "" {
		if cfg.AuthType != "" && cfg.AuthType != AuthNone {
			return string(cfg.AuthType), true, nil
		}
		return string(AuthOAuth2PKCE), true, nil
	}

	// Verifica entrada legacy por hostname (bearer/basic)
	hostname := hostnameFromURL(cfg.URL)
	if hostname == "" {
		return "", false, nil
	}

	auth, err := m.credMgr.GetConfigByPatternWithContext(ctx, hostname)
	if err != nil || auth == nil || auth.Source == "" {
		return "", false, err
	}

	resolvedType := ""
	if cfg.AuthType != "" && cfg.AuthType != AuthNone {
		resolvedType = string(cfg.AuthType)
	} else {
		switch auth.Type {
		case "bearer":
			resolvedType = "bearer"
		case "basic":
			resolvedType = "basic"
		case "oauth2":
			resolvedType = string(AuthOAuth2PKCE)
		default:
			resolvedType = auth.Type
		}
	}

	return resolvedType, true, nil
}
