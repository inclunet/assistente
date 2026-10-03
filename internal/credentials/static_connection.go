package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Static connections reuse the vault row, DEK and session lifecycle. Their
// components are secrets with distinct purposes, not OAuth grants or HTTP auth.
const StaticConnectionType = "static_components"

var ErrStaticConnection = errors.New("credential_connection_unavailable")

var errStaticConnectionMissing = errors.New("static connection missing")
var errStaticConnectionUnreadable = errors.New("static connection unreadable")

type SecretRole string

const (
	RoleBotToken SecretRole = "bot_token"
	RoleAppToken SecretRole = "app_token"
)

type staticConnectionRecord struct {
	Version     int                   `json:"version"`
	ID          string                `json:"id"`
	UserID      string                `json:"userId"`
	Integration string                `json:"integration"`
	ConsumerID  string                `json:"consumerId"`
	Components  map[SecretRole]string `json:"components"`
}

// StaticConnectionUpdate is backend-only. Nil Changes values preserve a role;
// an explicit empty value removes it. Legacy references are read only when a
// new record is created. The consumer must verify their exclusive ownership in
// Commit before the same transaction removes the replaced rows.
type StaticConnectionUpdate struct {
	ID          string
	Integration string
	ConsumerID  string
	Changes     map[SecretRole]*string
	// RecoveryRoles is the consumer's complete role contract. Reconstruction
	// requires an explicit replacement/removal of every role, never preservation.
	RecoveryRoles []SecretRole
	Legacy        map[SecretRole]string
	Commit        func(*gorm.DB, string, map[SecretRole]bool) error
}

func StaticConnectionPattern(id string) string { return "connection:" + id }

func guardLegacySlackWrite(tx *gorm.DB, userID, pattern string) error {
	if pattern != "channel:slack:bot_token" && pattern != "channel:slack:app_token" {
		return nil
	}
	if !tx.Migrator().HasColumn(&database.Channel{}, "CredentialID") {
		return nil
	}
	var count int64
	if err := tx.Model(&database.Channel{}).Where("user_id = ? AND slug = ? AND credential_id <> ''", userID, "slack").Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return ErrStaticConnection
	}
	return nil
}

// UpdateStaticConnection serializes the vault/session and atomically publishes
// one composed secret, its consumer references and removal of legacy entries.
// prepare/Commit only use the provided transaction; no network or reentrant
// calls to the credential manager are allowed while it holds the vault lock.
func (m *Manager) UpdateStaticConnection(ctx context.Context, prepare func(*gorm.DB) (StaticConnectionUpdate, error)) error {
	captured, err := m.OAuthStore(ctx)
	if err != nil {
		return ErrStaticConnection
	}
	s := captured.(*oauthStore)
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.check(ctx) != nil || prepare == nil {
		return ErrStaticConnection
	}
	store, ok := m.store.(*DBStore)
	if !ok {
		return ErrStaticConnection
	}
	if _, err := store.ensureDB(); err != nil {
		return ErrStaticConnection
	}
	var saved database.CredentialEntry
	var removed []string
	err = database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, store.db, "credentials.static_connection", func(tx *gorm.DB) error {
		u, err := prepare(tx)
		if err != nil {
			return err
		}
		if u.Integration == "" || u.ConsumerID == "" || u.Commit == nil {
			return ErrStaticConnection
		}
		r := staticConnectionRecord{Version: 1, ID: u.ID, UserID: s.userID, Integration: u.Integration, ConsumerID: u.ConsumerID, Components: map[SecretRole]string{}}
		createRecord := u.ID == ""
		if u.ID == "" {
			r.ID = uuid.NewString()
			for role, ref := range u.Legacy {
				if strings.TrimSpace(string(role)) == "" {
					return ErrStaticConnection
				}
				if ref == "" {
					continue
				}
				var legacy database.CredentialEntry
				if err := tx.Where("user_id = ? AND pattern = ?", s.userID, ref).First(&legacy).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) && u.Changes[role] != nil {
						continue
					}
					return ErrStaticConnection
				}
				if (legacy.Source != "" && legacy.Source != "static") || legacy.AuthType != "secret" || legacy.OAuthEnc != "" || legacy.LegacyOAuthControlEnc != "" || legacy.SourceConfigEnc != "" || legacy.PasswordEnc != "" || legacy.HeadersEnc != "" || legacy.ClientIDEnc != "" || legacy.ClientSecretEnc != "" || legacy.RefreshTokenEnc != "" || legacy.Username != "" {
					return ErrStaticConnection
				}
				if u.Changes[role] == nil {
					secret, err := m.decrypt(legacy.TokenEnc)
					if err != nil {
						return ErrStaticConnection
					}
					r.Components[role] = secret
				}
				removed = append(removed, legacy.ID)
			}
		} else {
			if len(u.Legacy) != 0 {
				return ErrStaticConnection
			}
			loaded, loadErr := m.loadStaticConnection(tx, s.userID, u.ID, u.Integration, u.ConsumerID)
			if loadErr == nil {
				r = loaded
			} else {
				if (!errors.Is(loadErr, errStaticConnectionMissing) && !errors.Is(loadErr, errStaticConnectionUnreadable)) || len(u.RecoveryRoles) == 0 {
					return ErrStaticConnection
				}
				for _, role := range u.RecoveryRoles {
					if role == "" || u.Changes[role] == nil {
						return ErrStaticConnection
					}
				}
				createRecord = errors.Is(loadErr, errStaticConnectionMissing)
			}
		}
		for role, value := range u.Changes {
			if strings.TrimSpace(string(role)) == "" {
				return ErrStaticConnection
			}
			if value == nil {
				continue
			}
			if *value == "" {
				delete(r.Components, role)
			} else {
				r.Components[role] = *value
			}
		}
		data, err := json.Marshal(r)
		if err != nil {
			return ErrStaticConnection
		}
		enc, err := m.encrypt(string(data))
		if err != nil {
			return ErrStaticConnection
		}
		saved = database.CredentialEntry{UUIDModel: database.UUIDModel{ID: r.ID}, UserID: s.userID, Pattern: StaticConnectionPattern(r.ID), Source: "static", AuthType: StaticConnectionType, TokenEnc: enc}
		if createRecord {
			err = tx.Create(&saved).Error
		} else {
			err = tx.Model(&database.CredentialEntry{}).Where("id = ? AND user_id = ?", r.ID, s.userID).Update("token_enc", enc).Error
		}
		if err != nil {
			return err
		}
		present := make(map[SecretRole]bool, len(r.Components))
		for role, secret := range r.Components {
			present[role] = secret != ""
		}
		if err := u.Commit(tx, r.ID, present); err != nil {
			return err
		}
		if len(removed) > 0 {
			return tx.Where("user_id = ? AND id IN ?", s.userID, removed).Delete(&database.CredentialEntry{}).Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	kept := m.credentials[:0]
	for _, entry := range m.credentials {
		drop := entry.UserID == s.userID && entry.ID == saved.ID
		for _, id := range removed {
			drop = drop || (entry.UserID == s.userID && entry.ID == id)
		}
		if !drop {
			kept = append(kept, entry)
		}
	}
	m.credentials = append(kept, &DomainCredential{ID: saved.ID, UserID: s.userID, Pattern: saved.Pattern, Auth: &AuthConfig{Source: "static", Type: StaticConnectionType, Token: saved.TokenEnc}})
	return nil
}

func (m *Manager) loadStaticConnection(tx *gorm.DB, userID, id, integration, consumerID string) (staticConnectionRecord, error) {
	var row database.CredentialEntry
	var r staticConnectionRecord
	if err := tx.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r, errStaticConnectionMissing
		}
		return r, ErrStaticConnection
	}
	if row.UserID != userID || row.Pattern != StaticConnectionPattern(id) || row.Source != "static" || row.AuthType != StaticConnectionType {
		return r, ErrStaticConnection
	}
	plain, err := m.decrypt(row.TokenEnc)
	if err != nil || json.Unmarshal([]byte(plain), &r) != nil {
		return staticConnectionRecord{}, errStaticConnectionUnreadable
	}
	if r.Version != 1 || r.ID != id || r.UserID != userID || r.Integration != integration || r.ConsumerID != consumerID || r.Components == nil {
		return staticConnectionRecord{}, ErrStaticConnection
	}
	return r, nil
}

// ResolveStaticComponent requires the consumer and purpose, never a hostname.
func (m *Manager) ResolveStaticComponent(ctx context.Context, id, integration, consumerID string, role SecretRole) (string, error) {
	components, err := m.ResolveStaticComponents(ctx, id, integration, consumerID, role)
	return components[role], err
}

// ResolveStaticComponents resolves the requested roles from one vault snapshot.
func (m *Manager) ResolveStaticComponents(ctx context.Context, id, integration, consumerID string, roles ...SecretRole) (map[SecretRole]string, error) {
	captured, err := m.OAuthStore(ctx)
	if err != nil {
		return nil, ErrStaticConnection
	}
	s := captured.(*oauthStore)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s.check(ctx) != nil {
		return nil, ErrStaticConnection
	}
	store, ok := m.store.(*DBStore)
	if !ok {
		return nil, ErrStaticConnection
	}
	if _, err := store.ensureDB(); err != nil {
		return nil, ErrStaticConnection
	}
	r, err := m.loadStaticConnection(store.db.WithContext(ctx), s.userID, id, integration, consumerID)
	if err != nil {
		return nil, ErrStaticConnection
	}
	result := make(map[SecretRole]string, len(roles))
	if len(roles) == 0 {
		for role, value := range r.Components {
			result[role] = value
		}
		return result, nil
	}
	for _, role := range roles {
		if r.Components[role] == "" {
			return nil, ErrStaticConnection
		}
		result[role] = r.Components[role]
	}
	return result, nil
}
