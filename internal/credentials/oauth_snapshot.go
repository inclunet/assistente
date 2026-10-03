package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"assistente/internal/oauthsnapshot"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrSnapshot = errors.New("oauth_snapshot_unavailable")
var ErrSnapshotConflict = errors.New("oauth_snapshot_conflict")

// OAuthSnapshotInfo is the only projection allowed outside the backend.
type OAuthSnapshotInfo struct {
	ID          string    `json:"id"`
	ConsumerID  string    `json:"consumerId"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"createdAt"`
	RetainUntil time.Time `json:"retainUntil"`
	Location    string    `json:"location"`
	Expired     bool      `json:"expired"`
}
type snapshotCredential struct {
	Entry   database.CredentialEntry
	Control string
}
type legacySnapshot struct {
	Version                int
	Schema                 string
	Build                  string
	ID, UserID, Database   string
	CreatedAt, RetainUntil time.Time
	Consumer               database.MCPServer
	Credentials            []snapshotCredential // absences are preserved by the empty slice
	HostnameCredential     *snapshotCredential  `json:",omitempty"`
}
type snapshotSession struct {
	store          *oauthStore
	codec          *Manager // captured key: reuse vault cipher without its global lock
	db             *gorm.DB
	identity, path string
}

func (m *Manager) snapshotSession(ctx context.Context, directory string) (*snapshotSession, error) {
	captured, err := m.OAuthStore(ctx)
	if err != nil {
		return nil, ErrSnapshot
	}
	s := captured.(*oauthStore)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := s.check(ctx); err != nil {
		return nil, ErrSnapshot
	}
	store, ok := m.store.(*DBStore)
	if !ok {
		return nil, ErrSnapshot
	}
	// Bind recovery to the local database, not merely to a matching slug/DEK.
	var databases []struct {
		Name string
		File string
	}
	if err := store.db.WithContext(ctx).Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		return nil, ErrSnapshot
	}
	path := ""
	for _, db := range databases {
		if db.Name == "main" {
			path = db.File
		}
	}
	if path == "" {
		return nil, ErrSnapshot
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, ErrSnapshot
	}
	hash := sha256.Sum256([]byte(filepath.Clean(abs) + "\x00" + s.userID))
	identity := hex.EncodeToString(hash[:])
	return &snapshotSession{store: s, codec: &Manager{encKey: append([]byte(nil), m.encKey...)}, db: store.db, identity: identity, path: filepath.Join(directory, identity)}, nil
}
func (s *snapshotSession) close() { clear(s.codec.encKey) }
func (s *snapshotSession) check(ctx context.Context) error {
	return s.store.WithSession(ctx, func() error { return nil })
}
func (s *snapshotSession) info(p *legacySnapshot) OAuthSnapshotInfo {
	if p.Schema == "legacy-hostname-v1" {
		return OAuthSnapshotInfo{ID: p.ID, ConsumerID: "credential:" + p.HostnameCredential.Entry.Pattern, Name: p.HostnameCredential.Entry.Pattern, CreatedAt: p.CreatedAt, RetainUntil: p.RetainUntil, Location: filepath.Join(s.path, p.ID+".oauth"), Expired: !time.Now().Before(p.RetainUntil)}
	}
	return OAuthSnapshotInfo{ID: p.ID, ConsumerID: p.Consumer.ID, Name: p.Consumer.Name, CreatedAt: p.CreatedAt, RetainUntil: p.RetainUntil, Location: filepath.Join(s.path, p.ID+".oauth"), Expired: !time.Now().Before(p.RetainUntil)}
}
func (s *snapshotSession) open() (*oauthsnapshot.Files, error) { return oauthsnapshot.Open(s.path) }
func (s *snapshotSession) read(ctx context.Context, files *oauthsnapshot.Files, id string) (*legacySnapshot, error) {
	if err := s.check(ctx); err != nil {
		return nil, ErrSnapshot
	}
	data, err := files.Read(id)
	if err != nil {
		return nil, ErrSnapshot
	}
	plain, err := s.codec.decrypt(string(data))
	if err != nil {
		return nil, ErrSnapshot
	}
	var p legacySnapshot
	if json.Unmarshal([]byte(plain), &p) != nil || p.Version != 1 || !validLegacySnapshotSchema(p) || p.ID != id || p.UserID != s.store.userID || p.Database != s.identity {
		return nil, ErrSnapshot
	}
	if p.Schema != "legacy-hostname-v1" && (p.Consumer.UserID != s.store.userID || p.Consumer.ID == "" || p.Consumer.Slug == "" || p.Consumer.OAuthManaged || p.Consumer.OAuthAuthorizationID != "") {
		return nil, ErrSnapshot
	}
	if err := s.check(ctx); err != nil {
		return nil, ErrSnapshot
	}
	return &p, nil
}

func validLegacySnapshotSchema(p legacySnapshot) bool {
	if p.Schema == "legacy-hostname-v1" {
		return p.HostnameCredential != nil && p.HostnameCredential.Entry.UserID == p.UserID && validSnapshotHostname(p.HostnameCredential.Entry) && p.Consumer.ID == "" && len(p.Credentials) == 0
	}
	if p.HostnameCredential != nil {
		return false
	}
	return p.Schema == "legacy-pkce-v1" && p.Consumer.AuthType == "oauth2_pkce" ||
		p.Schema == "legacy-client-credentials-v1" && p.Consumer.AuthType == "oauth2_client_credentials"
}

// Hostname recovery never treats managed namespaces or URL-shaped patterns as
// shared host credentials. Preserve exact spelling, including wildcard patterns.
func validSnapshotHostname(entry database.CredentialEntry) bool {
	if entry.ID == "" || entry.Pattern == "" || IsManagedPattern(entry.Pattern) || entry.OAuthEnc != "" || entry.LegacyOAuthControlEnc != "" || entry.SourceConfigEnc != "" || (entry.Source != "" && entry.Source != "static") || (entry.AuthType != "oauth2" && entry.AuthType != "bearer") {
		return false
	}
	if net.ParseIP(entry.Pattern) != nil {
		return true
	}
	u, err := url.Parse("https://" + entry.Pattern)
	return err == nil && u.Host == entry.Pattern && u.Hostname() == entry.Pattern && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && !strings.ContainsAny(entry.Pattern, " \\?#@")
}

// CreateLegacyOAuthSnapshot does not resolve credentials, invoke sources, or
// contact OAuth servers. Persisted PKCE and Client Credentials consumers use
// separate schemas so an older reader cannot misinterpret recovery semantics.
func (m *Manager) CreateLegacyOAuthSnapshot(ctx context.Context, directory, consumerID string) (OAuthSnapshotInfo, error) {
	s, err := m.snapshotSession(ctx, directory)
	if err != nil {
		return OAuthSnapshotInfo{}, err
	}
	defer s.close()
	if err := s.check(ctx); err != nil {
		return OAuthSnapshotInfo{}, ErrSnapshot
	}
	p := legacySnapshot{Version: 1, Schema: "legacy-pkce-v1", ID: uuid.NewString(), UserID: s.store.userID, Database: s.identity, CreatedAt: time.Now().UTC(), Credentials: []snapshotCredential{}}
	p.RetainUntil = p.CreatedAt.Add(30 * 24 * time.Hour)
	if info, ok := debug.ReadBuildInfo(); ok {
		p.Build = info.Main.Version
		for _, v := range info.Settings {
			if v.Key == "vcs.revision" {
				p.Build = v.Value
			}
		}
	}
	err = s.store.WithSession(ctx, func() error {
		return database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, s.db, "oauth.snapshot.capture", func(tx *gorm.DB) error {
			if strings.HasPrefix(consumerID, "credential:") {
				var entry database.CredentialEntry
				if err := tx.Where("user_id = ? AND pattern = ?", p.UserID, strings.TrimPrefix(consumerID, "credential:")).First(&entry).Error; err != nil {
					return ErrSnapshot
				}
				if !validSnapshotHostname(entry) {
					return ErrSnapshot
				}
				p.Schema = "legacy-hostname-v1"
				p.HostnameCredential = &snapshotCredential{Entry: entry}
				return nil
			}
			if err := tx.Where("user_id = ? AND id = ?", p.UserID, consumerID).First(&p.Consumer).Error; err != nil {
				return ErrSnapshot
			}
			if p.Consumer.AuthType == "oauth2_client_credentials" {
				p.Schema = "legacy-client-credentials-v1"
			}
			if !validLegacySnapshotSchema(p) || p.Consumer.OAuthManaged || p.Consumer.OAuthAuthorizationID != "" {
				return ErrSnapshot
			}
			var rows []database.CredentialEntry
			if err := tx.Where("user_id = ? AND pattern IN ?", p.UserID, []string{"mcp-client:" + p.Consumer.Slug, "mcp-tokens:" + p.Consumer.Slug}).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if row.OAuthEnc != "" || (row.Source != "" && row.Source != "static") {
					return ErrSnapshot
				}
				row, omit, err := m.snapshotLegacyCredential(row, p.Consumer)
				if err != nil {
					return err
				}
				if omit {
					continue
				}
				p.Credentials = append(p.Credentials, snapshotCredential{Entry: row, Control: row.LegacyOAuthControlEnc})
			}
			return nil
		})
	})
	if err != nil {
		return OAuthSnapshotInfo{}, err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return OAuthSnapshotInfo{}, ErrSnapshot
	}
	enc, err := s.codec.encrypt(string(data))
	if err != nil {
		return OAuthSnapshotInfo{}, ErrSnapshot
	}
	files, err := s.open()
	if err != nil {
		return OAuthSnapshotInfo{}, ErrSnapshot
	}
	defer files.Close()
	if err := s.check(ctx); err != nil {
		return OAuthSnapshotInfo{}, ErrSnapshot
	}
	if err := files.WriteGuarded(p.ID, []byte(enc), func(publish func() error) error { return s.store.WithSession(ctx, publish) }); err != nil {
		return OAuthSnapshotInfo{}, ErrSnapshot
	}
	if err := s.check(ctx); err != nil {
		return OAuthSnapshotInfo{}, ErrSnapshot
	}
	return s.info(&p), nil
}

func (m *Manager) snapshotInactive(enc, consumerID string) error {
	if enc == "" {
		return nil
	}
	plain, err := m.decrypt(enc)
	var control legacyOAuthControl
	if err != nil || json.Unmarshal([]byte(plain), &control) != nil || control.Version != 1 || control.ConsumerID != consumerID {
		return ErrSnapshot
	}
	if time.Now().Before(control.Until) {
		return oauthflow.ErrTransient
	}
	return nil
}

// An expired reconnection envelope is coordination metadata, not part of the
// previous grant. A new snapshot captures that grant and its original refresh
// barrier, independent of which snapshot started the abandoned attempt.
func (m *Manager) snapshotLegacyCredential(row database.CredentialEntry, consumer database.MCPServer) (database.CredentialEntry, bool, error) {
	if err := m.snapshotInactive(row.LegacyOAuthControlEnc, consumer.ID); err != nil {
		return row, false, err
	}
	if row.LegacyOAuthControlEnc == "" {
		return row, false, nil
	}
	plain, err := m.decrypt(row.LegacyOAuthControlEnc)
	var c legacyOAuthControl
	if err != nil || json.Unmarshal([]byte(plain), &c) != nil {
		return row, false, ErrSnapshot
	}
	if c.Migration == nil {
		return row, false, nil
	}
	r := c.Migration
	if row.UserID != consumer.UserID || row.Pattern != "mcp-tokens:"+consumer.Slug || consumer.AuthType != "oauth2_pkce" || r.Version != 1 || r.ID == "" || r.UserID != consumer.UserID || r.ConsumerID != consumer.ID || r.Integration != "mcp" || r.GrantType != "authorization_code" {
		return row, false, ErrSnapshotConflict
	}
	pending := false
	if c.OriginalControl != "" {
		if err := m.snapshotInactive(c.OriginalControl, consumer.ID); err != nil {
			return row, false, err
		}
		original, err := m.decrypt(c.OriginalControl)
		var previous legacyOAuthControl
		if err != nil || json.Unmarshal([]byte(original), &previous) != nil || previous.Migration != nil {
			return row, false, ErrSnapshot
		}
		pending = previous.Pending
	}
	if c.Pending != pending {
		return row, false, ErrSnapshotConflict
	}
	row.LegacyOAuthControlEnc = c.OriginalControl
	if c.MigrationInserted {
		if c.OriginalControl != "" || !emptyReconnectTokenRow(row) {
			return row, false, ErrSnapshotConflict
		}
		return row, true, nil
	}
	return row, false, nil
}

func emptyReconnectTokenRow(row database.CredentialEntry) bool {
	return row.TokenEnc == "" && row.RefreshTokenEnc == "" && row.ClientIDEnc == "" && row.ClientSecretEnc == "" && row.ClientGrantType == "" && row.ExpiresAt == 0 && row.Source == "static" && row.AuthType == "oauth2" && row.OAuthEnc == "" && row.SourceConfigEnc == "" && row.PasswordEnc == "" && row.HeadersEnc == "" && row.Username == ""
}

func (m *Manager) ListLegacyOAuthSnapshots(ctx context.Context, directory string) ([]OAuthSnapshotInfo, error) {
	s, err := m.snapshotSession(ctx, directory)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if err := s.check(ctx); err != nil {
		return nil, ErrSnapshot
	}
	files, err := s.open()
	if err != nil {
		return nil, ErrSnapshot
	}
	defer files.Close()
	ids, err := files.List()
	if err != nil {
		return nil, ErrSnapshot
	}
	result := []OAuthSnapshotInfo{}
	for _, id := range ids {
		p, err := s.read(ctx, files, id)
		if err != nil {
			return nil, err
		}
		result = append(result, s.info(p))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if err := s.check(ctx); err != nil {
		return nil, ErrSnapshot
	}
	return result, nil
}

// Recovery only repairs a missing pair under an unchanged consumer (or a
// completely removed consumer). It never overwrites a later grant or edit.
// Old tokens stay in the encrypted snapshot: a pending marker requires an
// explicit authorization and prevents replaying remotely rotated refresh tokens.
func (m *Manager) RestoreLegacyOAuthSnapshot(ctx context.Context, directory, id string, publish func(database.MCPServer), validate ...func(database.MCPServer) error) error {
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
	p, err := s.read(ctx, files, id)
	if err != nil {
		return err
	}
	if !time.Now().Before(p.RetainUntil) {
		return ErrSnapshotConflict
	}
	// Serialize the transaction and cache publication with other credential
	// mutations so a concurrent delete cannot leave a restored row uncached.
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if p.Schema == "legacy-hostname-v1" {
		return m.restoreHostnameSnapshot(ctx, s, p)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.store.check(ctx); err != nil {
		return ErrSnapshot
	}
	consumer := p.Consumer
	consumer.AutoConnect = false
	consumer.Enabled = false
	consumer.LastError = ""
	consumer.LastConnectedAt = nil
	consumer.LastDiscoveredAt = nil
	consumer.User = nil
	consumer.Tools = nil
	var recoveredClient *database.CredentialEntry
	for _, check := range validate {
		if err := check(consumer); err != nil {
			return ErrSnapshot
		}
	}
	marker, _ := json.Marshal(legacyOAuthControl{Version: 1, ConsumerID: consumer.ID, Pending: true})
	control, err := m.encrypt(string(marker))
	if err != nil {
		return ErrSnapshot
	}
	err = database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, s.db, "oauth.snapshot.restore", func(tx *gorm.DB) error {
		if err := s.store.check(ctx); err != nil {
			return ErrSnapshot
		}
		var existing database.MCPServer
		readErr := tx.Where("user_id = ? AND (id = ? OR slug = ?)", p.UserID, consumer.ID, consumer.Slug).First(&existing).Error
		if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
			return readErr
		}
		if readErr == nil {
			actual, expected := existing, p.Consumer
			if !actual.CreatedAt.Equal(expected.CreatedAt) {
				return ErrSnapshotConflict
			}
			actual.CreatedAt = expected.CreatedAt
			actual.UpdatedAt = expected.UpdatedAt
			actual.LastConnectedAt = expected.LastConnectedAt
			actual.LastDiscoveredAt = expected.LastDiscoveredAt
			actual.LastError = expected.LastError
			if !reflect.DeepEqual(actual, expected) {
				return ErrSnapshotConflict
			}
		}
		var count int64
		if err := tx.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", p.UserID, []string{"mcp-client:" + consumer.Slug, "mcp-tokens:" + consumer.Slug}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrSnapshotConflict
		}
		for _, row := range p.Credentials {
			entry := row.Entry
			if entry.Pattern != "mcp-client:"+consumer.Slug {
				continue
			}
			if entry.UserID != p.UserID || (entry.Source != "" && entry.Source != "static") || entry.AuthType != "oauth2" {
				return ErrSnapshot
			}
			for _, enc := range []string{entry.ClientIDEnc, entry.ClientSecretEnc} {
				if enc != "" {
					if _, err := m.decrypt(enc); err != nil {
						return ErrSnapshot
					}
				}
			}
			// Recover client registration only. Even malformed legacy client rows
			// must not reactivate access/refresh tokens or external source settings.
			entry = database.CredentialEntry{UUIDModel: entry.UUIDModel, UserID: p.UserID,
				Pattern: entry.Pattern, Source: "static", AuthType: "oauth2",
				ClientIDEnc: entry.ClientIDEnc, ClientSecretEnc: entry.ClientSecretEnc, ClientGrantType: entry.ClientGrantType}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			recoveredClient = &entry
		}
		if consumer.AuthType == "oauth2_pkce" {
			tokens := database.CredentialEntry{UserID: p.UserID, Pattern: "mcp-tokens:" + consumer.Slug, Source: "static", AuthType: "oauth2", LegacyOAuthControlEnc: control}
			if err := tx.Create(&tokens).Error; err != nil {
				return err
			}
		}
		if errors.Is(readErr, gorm.ErrRecordNotFound) {
			if err := tx.Omit("User", "Tools").Create(&consumer).Error; err != nil {
				return err
			}
			consumer.Enabled, consumer.AutoConnect = false, false
		}
		return tx.Model(&database.MCPServer{}).Where("id = ? AND user_id = ?", consumer.ID, p.UserID).Select("*").Omit("id", "created_at", "User", "Tools").Updates(&consumer).Error
	})
	if err != nil {
		return err
	}
	// Drop stale local cache entries; the coordinated reader reloads the DB.
	kept := m.credentials[:0]
	for _, entry := range m.credentials {
		if entry.UserID != p.UserID || (entry.Pattern != "mcp-client:"+consumer.Slug && entry.Pattern != "mcp-tokens:"+consumer.Slug) {
			kept = append(kept, entry)
		}
	}
	m.credentials = kept
	// Client Credentials reads registration from the vault cache. Publish only
	// after the transaction succeeds, under the captured session's vault lock.
	if consumer.AuthType == "oauth2_client_credentials" && recoveredClient != nil {
		entry := recoveredClient
		m.credentials = append(m.credentials, &DomainCredential{ID: entry.ID, UserID: entry.UserID, Pattern: entry.Pattern,
			Auth:  &AuthConfig{Source: "static", Type: "oauth2", ClientID: entry.ClientIDEnc, ClientSecret: entry.ClientSecretEnc, ClientGrantType: entry.ClientGrantType},
			regex: regexp.MustCompile(wildcardToRegex(entry.Pattern))})
	}
	if publish != nil {
		publish(consumer)
	}
	return nil
}

// Hostname recovery is an explicit restoration of copied secrets, not a grant
// conversion. Do not change consumers or infer ownership of a shared pattern.
func (m *Manager) restoreHostnameSnapshot(ctx context.Context, s *snapshotSession, p *legacySnapshot) error {
	pattern := p.HostnameCredential.Entry.Pattern
	if err := m.restoreHostnameSnapshotAndCache(ctx, s, p); err != nil {
		return err
	}
	return m.notifyCredentialPatternMutation(ctx, pattern)
}

func (m *Manager) restoreHostnameSnapshotAndCache(ctx context.Context, s *snapshotSession, p *legacySnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.store.check(ctx); err != nil {
		return ErrSnapshot
	}
	entry := p.HostnameCredential.Entry
	headers := map[string]string{}
	if entry.HeadersEnc != "" && json.Unmarshal([]byte(entry.HeadersEnc), &headers) != nil {
		return ErrSnapshot
	}
	values := []string{entry.TokenEnc, entry.RefreshTokenEnc, entry.ClientIDEnc, entry.ClientSecretEnc, entry.PasswordEnc}
	for _, value := range headers {
		values = append(values, value)
	}
	for _, value := range values {
		if value != "" {
			if _, err := m.decrypt(value); err != nil {
				return ErrSnapshot
			}
		}
	}
	// The user explicitly restores stored material; never resolve an external
	// reference or reuse the permissive legacy plaintext decryption fallback.
	entry.Source = "static"
	auth := &AuthConfig{Source: entry.Source, Type: entry.AuthType, Token: entry.TokenEnc,
		RefreshURL: entry.RefreshTokenEnc, ExpiresAt: entry.ExpiresAt, ClientID: entry.ClientIDEnc,
		ClientSecret: entry.ClientSecretEnc, ClientGrantType: entry.ClientGrantType,
		Username: entry.Username, Password: entry.PasswordEnc, Headers: headers}
	err := database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, s.db, "oauth.snapshot.restore_hostname", func(tx *gorm.DB) error {
		if err := s.store.check(ctx); err != nil {
			return ErrSnapshot
		}
		if !time.Now().Before(p.RetainUntil) {
			return ErrSnapshotConflict
		}
		var count int64
		if err := tx.Model(&database.CredentialEntry{}).Where("id = ?", entry.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrSnapshotConflict
		}
		var patterns []string
		if err := tx.Model(&database.CredentialEntry{}).Where("user_id = ?", p.UserID).Pluck("pattern", &patterns).Error; err != nil {
			return err
		}
		for _, pattern := range patterns {
			if strings.EqualFold(pattern, entry.Pattern) {
				return ErrSnapshotConflict
			}
		}
		return tx.Create(&entry).Error
	})
	if err != nil {
		return err
	}
	kept := m.credentials[:0]
	for _, cached := range m.credentials {
		if cached.UserID != entry.UserID || cached.Pattern != entry.Pattern {
			kept = append(kept, cached)
		}
	}
	m.credentials = append(kept, &DomainCredential{ID: entry.ID, UserID: entry.UserID, Pattern: entry.Pattern,
		Auth: auth, regex: regexp.MustCompile(wildcardToRegex(entry.Pattern))})
	return nil
}

// Discard always requires the caller's explicit confirmation of the rollback
// window. Expiration never silently removes the last recovery artifact.
func (m *Manager) DiscardLegacyOAuthSnapshot(ctx context.Context, directory, id string, confirmed bool) error {
	if !confirmed {
		return ErrSnapshotConflict
	}
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
	if _, err := s.read(ctx, files, id); err != nil {
		return err
	}
	return files.DeleteGuarded(id, func(remove func() error) error { return s.store.WithSession(ctx, remove) })
}
