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

// ReconnectLegacyOAuth stages a new grant in the encrypted legacy control. Only
// the shared OAuth service runs the protocol; its final CAS performs the cutover.
// All callbacks except authorize run under the vault lock and must not perform I/O.
func (m *Manager) ReconnectLegacyOAuth(ctx context.Context, directory, snapshotID, authMethod string,
	prepare func(database.MCPServer, *AuthConfig, string) (oauthflow.Record, error),
	authorize func(context.Context, oauthflow.Store, oauthflow.Record, database.MCPServer) error,
	project func(database.MCPServer, oauthflow.Record) (database.MCPServer, error),
	publish func(database.MCPServer)) error {
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
	if p.Schema != "legacy-pkce-v1" || !time.Now().Before(p.RetainUntil) || prepare == nil || authorize == nil || project == nil {
		return ErrSnapshot
	}
	// Bind the original user choice to the receipt, not to the resulting client:
	// a DCR fallback may legitimately issue a public client during authorization.
	r := &reconnectStore{s: s, p: p, id: uuid.NewSHA1(uuid.NameSpaceOID, []byte("reconnect:"+p.Database+":"+p.ID+":"+authMethod)).String(), attempt: uuid.NewString(), until: time.Now().Add(10 * time.Minute), project: project, publish: publish}
	var initial oauthflow.Record
	var consumer database.MCPServer
	err = r.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", p.Consumer.ID, p.UserID).First(&consumer).Error; err != nil {
			return ErrSnapshotConflict
		}
		if consumer.OAuthManaged || consumer.OAuthAuthorizationID != "" {
			if !consumer.OAuthManaged || consumer.OAuthAuthorizationID != r.id {
				return ErrSnapshotConflict
			}
			var entry database.CredentialEntry
			if err := tx.Where("id = ? AND user_id = ? AND source = ?", r.id, p.UserID, "oauth").First(&entry).Error; err != nil {
				return ErrSnapshotConflict
			}
			plain, err := m.decrypt(entry.OAuthEnc)
			if err != nil || json.Unmarshal([]byte(plain), &initial) != nil || !r.bound(initial) {
				return ErrSnapshotConflict
			}
			r.committed = true
			return nil
		}
		rows, err := r.validate(tx, false)
		if err != nil {
			return err
		}
		client := &AuthConfig{}
		for _, row := range rows {
			if row.Pattern == "mcp-client:"+p.Consumer.Slug {
				client.ClientGrantType = row.ClientGrantType
				if row.ClientIDEnc != "" {
					client.ClientID, err = m.decrypt(row.ClientIDEnc)
					if err != nil {
						return ErrSnapshot
					}
				}
				if row.ClientSecretEnc != "" {
					client.ClientSecret, err = m.decrypt(row.ClientSecretEnc)
					if err != nil {
						return ErrSnapshot
					}
				}
			}
			if row.Pattern == "mcp-tokens:"+p.Consumer.Slug {
				r.rowID = row.ID
				r.original = row.LegacyOAuthControlEnc
			}
		}
		initial, err = prepare(consumer, client, r.id)
		if err != nil {
			return err
		}
		if !r.bound(initial) || initial.Client.AuthMethod != authMethod || initial.Revision != 1 || initial.State != "pending" || initial.Tokens != (oauthflow.Tokens{}) {
			return ErrSnapshotConflict
		}
		if r.rowID == "" {
			row := database.CredentialEntry{UserID: p.UserID, Pattern: "mcp-tokens:" + p.Consumer.Slug, Source: "static", AuthType: "oauth2"}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			r.rowID, r.inserted = row.ID, true
		}
		return r.write(tx, initial)
	}, func() {
		if r.committed && publish != nil {
			publish(consumer)
		}
	})
	if err != nil {
		return err
	}
	if r.committed {
		return nil
	}
	defer r.close()
	ctx, cancelSession := s.store.SessionContext(ctx)
	defer cancelSession()
	ctx, cancel := context.WithDeadline(ctx, r.until)
	defer cancel()
	if err := authorize(ctx, r, initial, consumer); err != nil {
		return err
	}
	if !r.committed {
		return oauthflow.ErrConflict
	}
	return nil
}

type reconnectStore struct {
	s                                     *snapshotSession
	p                                     *legacySnapshot
	id, attempt, rowID, original, control string
	until                                 time.Time
	inserted, committed                   bool
	project                               func(database.MCPServer, oauthflow.Record) (database.MCPServer, error)
	publish                               func(database.MCPServer)
}

func (r *reconnectStore) bound(v oauthflow.Record) bool {
	return v.Version == 1 && v.ID == r.id && v.UserID == r.p.UserID && v.ConsumerID == r.p.Consumer.ID && v.Integration == "mcp" && v.GrantType == "authorization_code" && v.Resource == r.p.Consumer.URL
}
func (r *reconnectStore) transaction(ctx context.Context, fn func(*gorm.DB) error, after ...func()) error {
	m := r.s.store.manager
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := r.s.store.check(ctx); err != nil {
		return err
	}
	previous := r.control
	err := database.WithSQLiteImmediateTransactionOnce(ctx, r.until, r.s.db, "oauth.legacy.reconnect", fn)
	if err != nil {
		r.control = previous
	}
	if err == nil {
		for _, publish := range after {
			publish()
		}
	}
	return err
}

// Exact snapshot comparison also fences other processes, config edits and secret
// changes. Only this attempt's control and connection diagnostics may differ.
func (r *reconnectStore) validate(tx *gorm.DB, owned bool) ([]database.CredentialEntry, error) {
	m, p := r.s.store.manager, r.p
	var current database.MCPServer
	if err := tx.Where("id = ? AND user_id = ?", p.Consumer.ID, p.UserID).First(&current).Error; err != nil {
		return nil, ErrSnapshotConflict
	}
	expected := p.Consumer
	if !current.CreatedAt.Equal(expected.CreatedAt) {
		return nil, ErrSnapshotConflict
	}
	current.CreatedAt = expected.CreatedAt
	current.LastConnectedAt, current.LastDiscoveredAt, current.LastError = expected.LastConnectedAt, expected.LastDiscoveredAt, expected.LastError
	current.UpdatedAt = expected.UpdatedAt
	if !reflect.DeepEqual(current, expected) {
		return nil, ErrSnapshotConflict
	}
	var rows []database.CredentialEntry
	if err := tx.Where("user_id = ? AND pattern IN ?", p.UserID, []string{"mcp-client:" + current.Slug, "mcp-tokens:" + current.Slug}).Find(&rows).Error; err != nil {
		return nil, err
	}
	previousRows := make([]database.CredentialEntry, 0, len(rows))
	for _, row := range rows {
		if owned && row.ID == r.rowID {
			if row.LegacyOAuthControlEnc != r.control || !time.Now().Before(r.until) {
				return nil, oauthflow.ErrConflict
			}
			row.LegacyOAuthControlEnc = r.original
			if r.inserted {
				if !emptyReconnectTokenRow(row) {
					return nil, ErrSnapshotConflict
				}
				continue
			}
		} else {
			var omit bool
			var err error
			row, omit, err = m.snapshotLegacyCredential(row, current)
			if err != nil {
				return nil, err
			}
			if omit {
				if owned {
					return nil, ErrSnapshotConflict
				}
				r.inserted, r.rowID, r.original = true, row.ID, ""
				continue
			}
		}
		previousRows = append(previousRows, row)
	}
	if len(previousRows) != len(p.Credentials) {
		return nil, ErrSnapshotConflict
	}
	for _, row := range previousRows {
		found := false
		for _, saved := range p.Credentials {
			e := saved.Entry
			e.LegacyOAuthControlEnc = saved.Control
			a := row
			if !a.CreatedAt.Equal(e.CreatedAt) {
				continue
			}
			a.CreatedAt = e.CreatedAt
			a.UpdatedAt = e.UpdatedAt
			if reflect.DeepEqual(a, e) {
				found = true
				break
			}
		}
		if !found || (row.Source != "" && row.Source != "static") || row.AuthType != "oauth2" || row.OAuthEnc != "" || row.SourceConfigEnc != "" || row.PasswordEnc != "" || row.HeadersEnc != "" || row.Username != "" {
			return nil, ErrSnapshotConflict
		}
		for _, enc := range []string{row.TokenEnc, row.RefreshTokenEnc, row.ClientIDEnc, row.ClientSecretEnc} {
			if enc != "" {
				if _, err := m.decrypt(enc); err != nil {
					return nil, ErrSnapshot
				}
			}
		}
	}
	return previousRows, nil
}

func (r *reconnectStore) write(tx *gorm.DB, record oauthflow.Record) error {
	m := r.s.store.manager
	c := legacyOAuthControl{Version: 1, ConsumerID: r.p.Consumer.ID, Attempt: r.attempt, Until: r.until, Migration: &record, OriginalControl: r.original, MigrationInserted: r.inserted}
	if r.original != "" {
		plain, err := m.decrypt(r.original)
		var previous legacyOAuthControl
		if err != nil || json.Unmarshal([]byte(plain), &previous) != nil {
			return ErrSnapshot
		}
		c.Pending = previous.Pending
	}
	body, err := json.Marshal(c)
	if err != nil {
		return ErrSnapshot
	}
	enc, err := m.encrypt(string(body))
	if err != nil {
		return ErrSnapshot
	}
	result := tx.Model(&database.CredentialEntry{}).Where("id = ? AND user_id = ?", r.rowID, r.p.UserID).Update("legacy_oauth_control_enc", enc)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return oauthflow.ErrConflict
	}
	r.control = enc
	return nil
}
func (r *reconnectStore) Load(ctx context.Context, id string) (oauthflow.Record, error) {
	var record oauthflow.Record
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		if id != r.id || r.committed {
			return oauthflow.ErrConflict
		}
		if _, err := r.validate(tx, true); err != nil {
			return err
		}
		plain, err := r.s.store.manager.decrypt(r.control)
		if err != nil {
			return ErrSnapshot
		}
		var c legacyOAuthControl
		if json.Unmarshal([]byte(plain), &c) != nil || c.Migration == nil {
			return ErrSnapshot
		}
		record = *c.Migration
		return nil
	})
	return record, err
}
func (r *reconnectStore) Create(context.Context, oauthflow.Record) error {
	return oauthflow.ErrConflict
}
func (r *reconnectStore) SessionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return r.s.store.SessionContext(ctx)
}
func (r *reconnectStore) CompareAndSwap(ctx context.Context, record oauthflow.Record, revision uint64) error {
	m := r.s.store.manager
	var converted database.MCPServer
	var encrypted string
	// Publication must be ordered with session changes, so keep the lock through
	// commit and cache publication just like oauthStore.
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := r.s.store.check(ctx); err != nil {
		return err
	}
	final := record.State == "connected" && record.AuthorizationAttempt == "" && record.Tokens.Access != ""
	previous := r.control
	err := database.WithSQLiteImmediateTransactionOnce(ctx, r.until, r.s.db, "oauth.legacy.reconnect.cas", func(tx *gorm.DB) error {
		if r.committed || !r.bound(record) || record.Revision != revision+1 {
			return oauthflow.ErrConflict
		}
		if _, err := r.validate(tx, true); err != nil {
			return err
		}
		plain, err := m.decrypt(r.control)
		if err != nil {
			return ErrSnapshot
		}
		var c legacyOAuthControl
		if json.Unmarshal([]byte(plain), &c) != nil || c.Migration == nil || c.Migration.Revision != revision {
			return oauthflow.ErrConflict
		}
		if !final {
			return r.write(tx, record)
		}
		converted, err = r.project(r.p.Consumer, record)
		if err != nil {
			return err
		}
		if converted.ID != r.p.Consumer.ID || converted.UserID != r.p.UserID || converted.Slug != r.p.Consumer.Slug || !converted.OAuthManaged || converted.OAuthAuthorizationID != r.id {
			return ErrSnapshotConflict
		}
		body, err := json.Marshal(record)
		if err != nil {
			return ErrSnapshot
		}
		encrypted, err = m.encrypt(string(body))
		if err != nil {
			return ErrSnapshot
		}
		entry := database.CredentialEntry{UUIDModel: database.UUIDModel{ID: r.id}, UserID: r.p.UserID, Pattern: "oauth:" + r.id, Source: "oauth", AuthType: "bearer", OAuthEnc: encrypted}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.MCPServer{}).Where("id = ? AND user_id = ?", converted.ID, converted.UserID).Select("*").Omit("id", "created_at", "User", "Tools").Updates(&converted).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND pattern IN ?", r.p.UserID, []string{"mcp-client:" + converted.Slug, "mcp-tokens:" + converted.Slug}).Delete(&database.CredentialEntry{}).Error
	})
	if err != nil {
		r.control = previous
		return err
	}
	if final {
		r.committed = true
		kept := m.credentials[:0]
		for _, entry := range m.credentials {
			if entry.UserID == r.p.UserID && (entry.Pattern == "mcp-client:"+converted.Slug || entry.Pattern == "mcp-tokens:"+converted.Slug || entry.ID == r.id) {
				continue
			}
			kept = append(kept, entry)
		}
		m.credentials = append(kept, &DomainCredential{ID: r.id, UserID: r.p.UserID, Pattern: "oauth:" + r.id, Auth: &AuthConfig{Source: "oauth", Type: "bearer", OAuthEnc: encrypted}})
		if r.publish != nil {
			r.publish(converted)
		}
	}
	return nil
}
func (r *reconnectStore) close() {
	if r.committed {
		return
	}
	ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), r.p.UserID), 2*time.Second)
	defer cancel()
	m := r.s.store.manager
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.s.store.check(ctx) != nil {
		return
	}
	query := r.s.db.WithContext(ctx).Where("id = ? AND user_id = ? AND legacy_oauth_control_enc = ?", r.rowID, r.p.UserID, r.control)
	if r.inserted {
		_ = query.Delete(&database.CredentialEntry{}).Error
	} else {
		_ = query.Model(&database.CredentialEntry{}).Update("legacy_oauth_control_enc", r.original).Error
	}
}
