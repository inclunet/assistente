package credentials

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ConvertLegacyClientCredentials consumes a verified local snapshot, never a
// frontend secret. The callback only projects MCP configuration and performs no
// I/O. Snapshot, consumer and both legacy rows must still describe the same state.
func (m *Manager) ConvertLegacyClientCredentials(ctx context.Context, directory, snapshotID string,
	prepare func(database.MCPServer, *AuthConfig, string) (oauthflow.Record, database.MCPServer, error),
	publish func(database.MCPServer, bool)) error {
	s, err := m.snapshotSession(ctx, directory)
	if err != nil {
		return err
	}
	defer s.close()
	files, err := s.open()
	if err != nil {
		return ErrSnapshot
	}
	defer files.Close()
	p, err := s.read(ctx, files, snapshotID)
	if err != nil {
		return err
	}
	if p.Schema != "legacy-client-credentials-v1" || !time.Now().Before(p.RetainUntil) || prepare == nil {
		return ErrSnapshot
	}
	// Stable per snapshot: a repeated request after a lost response is harmless.
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(p.Database+":"+p.ID)).String()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.store.check(ctx); err != nil {
		return err
	}
	var converted database.MCPServer
	changed := false
	var encrypted string
	err = database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, s.db, "oauth.legacy.convert", func(tx *gorm.DB) error {
		var current database.MCPServer
		if err := tx.Where("id = ? AND user_id = ?", p.Consumer.ID, p.UserID).First(&current).Error; err != nil {
			return ErrSnapshotConflict
		}
		if current.OAuthManaged || current.OAuthAuthorizationID != "" {
			if !current.OAuthManaged || current.OAuthAuthorizationID != id {
				return ErrSnapshotConflict
			}
			var entry database.CredentialEntry
			if err := tx.Where("id = ? AND user_id = ? AND source = ?", id, p.UserID, "oauth").First(&entry).Error; err != nil {
				return ErrSnapshotConflict
			}
			plain, err := m.decrypt(entry.OAuthEnc)
			var record oauthflow.Record
			if err != nil || json.Unmarshal([]byte(plain), &record) != nil || record.ID != id || record.UserID != p.UserID || record.ConsumerID != current.ID || record.Integration != "mcp" || record.GrantType != "client_credentials" {
				return ErrSnapshotConflict
			}
			converted, encrypted = current, entry.OAuthEnc
			return nil
		}
		actual, expected := current, p.Consumer
		// Connection diagnostics are not configuration edits.
		actual.LastConnectedAt, actual.LastDiscoveredAt, actual.LastError = expected.LastConnectedAt, expected.LastDiscoveredAt, expected.LastError
		actual.CreatedAt, actual.UpdatedAt = expected.CreatedAt, expected.UpdatedAt
		if !current.CreatedAt.Equal(p.Consumer.CreatedAt) || !reflect.DeepEqual(actual, expected) {
			return ErrSnapshotConflict
		}
		patterns := []string{"mcp-client:" + current.Slug, "mcp-tokens:" + current.Slug}
		var rows []database.CredentialEntry
		if err := tx.Where("user_id = ? AND pattern IN ?", p.UserID, patterns).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(p.Credentials) {
			return ErrSnapshotConflict
		}
		var client *AuthConfig
		for _, row := range rows {
			found := false
			for _, saved := range p.Credentials {
				expected := saved.Entry
				expected.LegacyOAuthControlEnc = saved.Control
				actual := row
				actual.CreatedAt, actual.UpdatedAt = expected.CreatedAt, expected.UpdatedAt
				if row.ID == expected.ID && row.CreatedAt.Equal(expected.CreatedAt) && reflect.DeepEqual(actual, expected) {
					found = true
					break
				}
			}
			if !found {
				return ErrSnapshotConflict
			}
			if err := m.snapshotInactive(row.LegacyOAuthControlEnc, current.ID); err != nil {
				return err
			}
			if (row.Source != "" && row.Source != "static") || row.AuthType != "oauth2" || row.OAuthEnc != "" || row.SourceConfigEnc != "" || row.TokenEnc != "" || row.RefreshTokenEnc != "" || row.PasswordEnc != "" || row.HeadersEnc != "" || row.Username != "" {
				return ErrSnapshotConflict
			}
			if row.Pattern == patterns[0] {
				client = &AuthConfig{}
				if row.ClientIDEnc != "" {
					client.ClientID, err = m.decrypt(row.ClientIDEnc)
					if err != nil {
						return ErrSnapshot
					}
				}
				client.ClientSecret, err = m.decrypt(row.ClientSecretEnc)
				if err != nil || client.ClientSecret == "" {
					return ErrSnapshot
				}
			} else if row.ClientIDEnc != "" || row.ClientSecretEnc != "" {
				return ErrSnapshotConflict
			}
		}
		if client == nil {
			return ErrSnapshotConflict
		}
		record, candidate, err := prepare(current, client, id)
		if err != nil {
			return err
		}
		if record.ID != id || record.UserID != p.UserID || record.ConsumerID != current.ID || record.Version != 1 || record.Revision != 1 || record.Integration != "mcp" || record.GrantType != "client_credentials" || candidate.ID != current.ID || candidate.UserID != current.UserID || candidate.Slug != current.Slug || !candidate.OAuthManaged || candidate.OAuthAuthorizationID != id {
			return ErrSnapshotConflict
		}
		data, err := json.Marshal(record)
		if err != nil {
			return ErrSnapshot
		}
		encrypted, err = m.encrypt(string(data))
		if err != nil {
			return ErrSnapshot
		}
		entry := database.CredentialEntry{UUIDModel: database.UUIDModel{ID: id}, UserID: p.UserID, Pattern: "oauth:" + id, Source: "oauth", AuthType: "bearer", OAuthEnc: encrypted}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.MCPServer{}).Where("id = ? AND user_id = ?", current.ID, p.UserID).Select("*").Omit("id", "created_at", "User", "Tools").Updates(&candidate).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND pattern IN ?", p.UserID, patterns).Delete(&database.CredentialEntry{}).Error; err != nil {
			return err
		}
		converted = candidate
		changed = true
		return nil
	})
	if err != nil {
		return err
	}
	kept := m.credentials[:0]
	for _, entry := range m.credentials {
		if entry.UserID == p.UserID && (entry.ID == id || entry.Pattern == "mcp-client:"+p.Consumer.Slug || entry.Pattern == "mcp-tokens:"+p.Consumer.Slug) {
			continue
		}
		kept = append(kept, entry)
	}
	m.credentials = append(kept, &DomainCredential{ID: id, UserID: p.UserID, Pattern: "oauth:" + id, Auth: &AuthConfig{Source: "oauth", Type: "bearer", OAuthEnc: encrypted}})
	if publish != nil {
		publish(converted, changed)
	}
	return nil
}
