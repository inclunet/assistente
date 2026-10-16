package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

// Transitional coordination only: this is not an authorization or a converted grant.
type legacyOAuthControl struct {
	// Reconnection stages the shared service record without replacing the legacy grant.
	Migration         *oauthflow.Record `json:",omitempty"`
	OriginalControl   string            `json:",omitempty"`
	MigrationInserted bool              `json:",omitempty"`
	Version           int
	ConsumerID        string
	Attempt           string
	Until             time.Time
	Pending           bool
}

// ClearLegacyOAuth is explicit local disconnection. It atomically removes the
// pair only after the live attempt ends; an uncertain inactive grant may be discarded.
func (m *Manager) ClearLegacyOAuth(ctx context.Context, slug, consumerID, hostname string) error {
	return m.ClearLegacyOAuthWithConsumer(ctx, slug, consumerID, hostname, nil, nil)
}

// ClearLegacyOAuthWithConsumer atomically detaches unmanaged MCP credentials and
// updates its consumer. Publish runs only after commit, under the vault lock.
func (m *Manager) ClearLegacyOAuthWithConsumer(ctx context.Context, slug, consumerID, hostname string, update func(*gorm.DB) error, publish func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	store, ok := m.store.(*DBStore)
	if !ok || !m.persist {
		return oauthflow.ErrResource
	}
	patterns := []string{"mcp-client:" + slug, "mcp-tokens:" + slug}
	if hostname != "" {
		patterns = append(patterns, hostname)
	}
	err = database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, store.db, "credentials.legacy_oauth.clear", func(tx *gorm.DB) error {
		var consumer database.MCPServer
		if err := tx.Where("id = ? AND user_id = ? AND slug = ?", consumerID, user, slug).First(&consumer).Error; err != nil {
			return oauthflow.ErrConflict
		}
		if consumer.OAuthManaged || consumer.OAuthAuthorizationID != "" {
			return oauthflow.ErrConflict
		}
		var row database.CredentialEntry
		readErr := tx.Where("user_id = ? AND pattern = ?", user, "mcp-tokens:"+slug).First(&row).Error
		if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
			return readErr
		}
		if row.LegacyOAuthControlEnc != "" {
			data, err := m.decrypt(row.LegacyOAuthControlEnc)
			if err != nil {
				return oauthflow.ErrConflict
			}
			var control legacyOAuthControl
			if json.Unmarshal([]byte(data), &control) != nil || control.Version != 1 || control.ConsumerID != consumerID {
				return oauthflow.ErrConflict
			}
			if time.Now().Before(control.Until) {
				return oauthflow.ErrTransient
			}
		}
		if err := tx.Where("user_id = ? AND pattern IN ?", user, patterns).Delete(&database.CredentialEntry{}).Error; err != nil {
			return err
		}
		if update != nil {
			return update(tx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	kept := make([]*DomainCredential, 0, len(m.credentials))
	for _, entry := range m.credentials {
		remove := false
		for _, pattern := range patterns {
			if entry.UserID == user && entry.Pattern == pattern {
				remove = true
			}
		}
		if !remove {
			kept = append(kept, entry)
		} else {
			for _, cancel := range m.oauthRequests[entry.ID] {
				cancel()
			}
			entry.invalidateCommandCache()
		}
	}
	m.credentials = kept
	if publish != nil {
		publish()
	}
	return nil
}

// CheckLegacyOAuthMutation runs inside the caller's write transaction. Unknown
// encrypted control is fail-closed; ordinary saves must never erase it.
type mcpConsumerCheckKey struct{}

// WithMCPConsumerCheck fences UI mutations by the consumer snapshot they edited.
// The check must be read-only and runs inside each credential write transaction.
func WithMCPConsumerCheck(ctx context.Context, check func(*gorm.DB) error) context.Context {
	return context.WithValue(ctx, mcpConsumerCheckKey{}, check)
}

func checkMCPConsumer(ctx context.Context, tx *gorm.DB) error {
	if check, ok := ctx.Value(mcpConsumerCheckKey{}).(func(*gorm.DB) error); ok && check != nil {
		return check(tx)
	}
	return nil
}

func CheckLegacyOAuthMutation(ctx context.Context, tx *gorm.DB, user, slug string) error {
	if err := checkMCPConsumer(ctx, tx); err != nil {
		return err
	}
	if !tx.Migrator().HasTable(&database.CredentialEntry{}) {
		return nil
	}
	var row database.CredentialEntry
	err := tx.Where("user_id = ? AND pattern = ?", user, "mcp-tokens:"+slug).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.LegacyOAuthControlEnc == "" {
		return nil
	}
	// Historical coordination is recovery data, never permission for a generic
	// writer. Only the explicit snapshot/reconnection lifecycle may handle it.
	return oauthflow.ErrConflict
}

// InspectMCPAuthPresence reads metadata only; absence must never fall back to cache.
func (m *Manager) InspectMCPAuthPresence(ctx context.Context, slug, hostname string) (bool, error) {
	base, err := m.OAuthStore(ctx)
	if err != nil {
		return false, err
	}
	s := base.(*oauthStore)
	var count int64
	err = s.WithSession(ctx, func() error {
		store, ok := m.store.(*DBStore)
		if !ok {
			return oauthflow.ErrResource
		}
		return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := checkMCPConsumer(ctx, tx); err != nil {
				return err
			}
			patterns := []string{"mcp-client:" + slug, "mcp-tokens:" + slug}
			if hostname != "" {
				patterns = append(patterns, hostname)
			}
			return tx.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", s.userID, patterns).Count(&count).Error
		})
	})
	return count > 0, err
}
