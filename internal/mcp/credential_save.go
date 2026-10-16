package mcp

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"context"
	"errors"
	"gorm.io/gorm"
	"strings"
)

// SaveConfigWithCredential saves the consumer and vault entry atomically.
func (m *Manager) SaveConfigWithCredential(ctx context.Context, slug string, cfg ServerConfig, pattern string, auth *credentials.AuthConfig) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if m.credMgr == nil {
		return credentials.ErrStoreNotReady
	}
	captured, err := m.credMgr.OAuthStore(ctx)
	if err != nil {
		return err
	}
	atomicStore, ok := captured.(interface {
		SaveWithConsumer(context.Context, string, *credentials.AuthConfig, *oauthflow.Record, func(*gorm.DB) error, func()) error
	})
	if !ok {
		return credentials.ErrStoreNotReady
	}
	owner, _ := database.UserIDFromContext(m.credentialContext())
	if owner != user {
		return oauthflow.ErrConflict
	}
	slug = strings.TrimSpace(slug)
	if err := validateServerSlug(slug); err != nil {
		return err
	}
	if auth == nil || auth.Type != string(cfg.AuthType) || (cfg.AuthType != AuthBearer && cfg.AuthType != AuthBasic) || (cfg.Transport != TransportSSE && cfg.Transport != TransportStreamable) {
		return credentials.ErrCredentialResolution
	}
	repo, ok := m.repository().(*DBRepository)
	if !ok {
		return credentials.ErrStoreNotReady
	}
	existing, err := repo.GetServer(ctx, slug)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var retire *oauthflow.Record
	if existing != nil && existing.OAuthAuthorizationID != "" {
		_, record, _, err := m.managedOAuth(ctx, *existing)
		if err != nil {
			return err
		}
		retire = &record
	}
	effective, err := m.credMgr.PatternForURLWithContext(ctx, cfg.URL)
	if err != nil {
		return err
	}
	if effective == "" {
		effective = hostnameFromURL(cfg.URL)
	}
	if effective == "" || effective != pattern {
		return credentials.ErrCredentialResolution
	}
	cfg.Slug = slug
	cfg.UserID = user
	cfg.OAuthAuthorizationID = ""
	cfg.OAuthManaged = false
	clearOAuthConfiguration(&cfg)
	cfg.applyDefaults(slug)
	err = atomicStore.SaveWithConsumer(ctx, pattern, auth, retire, func(tx *gorm.DB) error {
		current, loadErr := NewDBRepository(tx).GetServer(ctx, slug)
		if existing == nil {
			if !errors.Is(loadErr, gorm.ErrRecordNotFound) {
				return oauthflow.ErrConflict
			}
		} else if loadErr != nil || current.ID != existing.ID || current.OAuthAuthorizationID != existing.OAuthAuthorizationID {
			return oauthflow.ErrConflict
		}
		return NewDBRepository(tx).saveServer(ctx, &cfg, retire != nil)
	}, func() { m.publishManagedOAuth(cfg) })
	if err != nil {
		return err
	}
	if retire != nil {
		_ = m.Disconnect(slug)
	}
	m.emit("mcp:config_changed", map[string]string{"slug": slug})
	return nil
}
