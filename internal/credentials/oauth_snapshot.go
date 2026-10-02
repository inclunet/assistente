package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"sort"
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
	if json.Unmarshal([]byte(plain), &p) != nil || p.Version != 1 || p.Schema != "legacy-pkce-v1" || p.ID != id || p.UserID != s.store.userID || p.Database != s.identity || p.Consumer.UserID != s.store.userID || p.Consumer.ID == "" || p.Consumer.Slug == "" || p.Consumer.OAuthManaged || p.Consumer.OAuthAuthorizationID != "" || p.Consumer.AuthType != "oauth2_pkce" {
		return nil, ErrSnapshot
	}
	if err := s.check(ctx); err != nil {
		return nil, ErrSnapshot
	}
	return &p, nil
}

// CreateLegacyOAuthSnapshot does not resolve credentials, invoke sources, or
// contact OAuth servers. Only persisted PKCE consumers are supported here.
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
			if err := tx.Where("user_id = ? AND id = ?", p.UserID, consumerID).First(&p.Consumer).Error; err != nil {
				return ErrSnapshot
			}
			if p.Consumer.AuthType != "oauth2_pkce" || p.Consumer.OAuthManaged || p.Consumer.OAuthAuthorizationID != "" {
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
				if err := m.snapshotInactive(row.LegacyOAuthControlEnc, p.Consumer.ID); err != nil {
					return err
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
			entry.LegacyOAuthControlEnc = ""
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
		}
		tokens := database.CredentialEntry{UserID: p.UserID, Pattern: "mcp-tokens:" + consumer.Slug, Source: "static", AuthType: "oauth2", LegacyOAuthControlEnc: control}
		if err := tx.Create(&tokens).Error; err != nil {
			return err
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
	if publish != nil {
		publish(consumer)
	}
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
