package mcp

import (
	"context"
	"reflect"
	"sync"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

type legacyOAuthWriterContextKey struct{}

type legacyOAuthWriter struct {
	update func(*ServerConfig) (ServerConfig, error)
}

func (w *legacyOAuthWriter) Write(cfg ServerConfig) error {
	_, err := w.update(&cfg)
	return err
}

func (w *legacyOAuthWriter) EnablePolling() (ServerConfig, error) {
	return w.update(nil)
}

// The legacy protocol callback is not a user edit: it may only update the
// original consumer while that consumer still belongs to the legacy runtime.
func (m *Manager) legacyOAuthConfigWriter(original ServerConfig) func(ServerConfig) error {
	return m.newLegacyOAuthWriter(original).Write
}

func (m *Manager) newLegacyOAuthWriter(original ServerConfig) *legacyOAuthWriter {
	original = persistedLegacyConfig(original)
	ctx := m.credentialContext()
	user, userErr := database.RequireUserID(ctx)
	if userErr != nil || original.UserID != user || original.ID == "" {
		return &legacyOAuthWriter{update: func(*ServerConfig) (ServerConfig, error) { return ServerConfig{}, oauthflow.ErrConflict }}
	}
	if m.credMgr == nil {
		return &legacyOAuthWriter{update: func(*ServerConfig) (ServerConfig, error) { return ServerConfig{}, oauthflow.ErrResource }}
	}
	store, captureErr := m.credMgr.OAuthStore(ctx)
	var mu sync.Mutex
	return &legacyOAuthWriter{update: func(candidate *ServerConfig) (ServerConfig, error) {
		mu.Lock()
		defer mu.Unlock()
		updated := original
		if candidate != nil {
			updated = *candidate
			updated.DisableSSE = original.DisableSSE
		} else {
			// Advance the transport preference from the latest callback snapshot.
			updated.DisableSSE = true
		}
		if captureErr != nil {
			return ServerConfig{}, captureErr
		}
		repo, ok := m.repository().(*DBRepository)
		if !ok {
			return ServerConfig{}, oauthflow.ErrResource
		}
		err := store.(interface {
			WithSession(context.Context, func() error) error
		}).WithSession(ctx, func() error {
			err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var current database.MCPServer
				if err := tx.Where("id = ? AND user_id = ? AND slug = ?", original.ID, original.UserID, original.Slug).First(&current).Error; err != nil {
					return oauthflow.ErrConflict
				}
				currentConfig, err := serverModelToConfig(current)
				if err != nil || current.OAuthManaged || current.OAuthAuthorizationID != "" || !reflect.DeepEqual(persistedLegacyConfig(currentConfig), original) {
					return oauthflow.ErrConflict
				}
				updated.ID, updated.Slug, updated.UserID = original.ID, original.Slug, original.UserID
				updated.OAuthManaged, updated.OAuthAuthorizationID = false, ""
				return NewDBRepository(tx).SaveServer(ctx, &updated)
			})
			if err != nil {
				return err
			}
			// A user edit can publish after this transaction commits. Do not
			// replace that newer in-memory configuration with this callback.
			m.mu.Lock()
			if current := m.servers[original.Slug]; current != nil && reflect.DeepEqual(persistedLegacyConfig(current.Config), original) {
				current.Config = updated
			}
			m.mu.Unlock()
			original = persistedLegacyConfig(updated)
			return nil
		})
		if err == nil {
			m.emit("mcp:config_changed", map[string]string{"slug": updated.Slug})
		}
		return updated, err
	}}
}

// Compare the repository projection, not transient editor/runtime fields or
// nil-versus-empty collection representations produced by JSON round-trips.
func persistedLegacyConfig(cfg ServerConfig) ServerConfig {
	row, _ := serverConfigToModel(cfg) // maps/slices contain only strings
	result, _ := serverModelToConfig(row)
	if len(result.Env) == 0 {
		result.Env = nil
	}
	if len(result.Args) == 0 {
		result.Args = nil
	}
	if len(result.OAuth2Scopes) == 0 {
		result.OAuth2Scopes = nil
	}
	return result
}
