package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func legacyOperationFixture(t *testing.T) (*Manager, *Manager, *gorm.DB, context.Context, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "legacy.db") + "?_pragma=busy_timeout(100)&_pragma=journal_mode(WAL)"
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		pool.SetMaxOpenConns(4)
		t.Cleanup(func() { _ = pool.Close() })
		return db
	}
	db, other := open(), open()
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	consumer := database.MCPServer{UserID: "owner", Slug: "legacy", AuthType: "oauth2_pkce", Name: "Legacy", Transport: "streamable"}
	if err := db.Create(&consumer).Error; err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{5}, 32)
	a := NewManagerWithStore(key, &DBStore{db: db}, true)
	b := NewManagerWithStore(key, &DBStore{db: other}, true)
	ctx := database.WithUserID(context.Background(), "owner")
	if err := a.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &AuthConfig{Source: "static", Type: "oauth2", Token: "old", RefreshURL: "old-refresh"}); err != nil {
		t.Fatal(err)
	}
	return a, b, db, ctx, consumer.ID
}

type racingRefreshStore struct {
	*DBStore
	before func()
}

// Seed the encrypted on-disk format left by historical versions. This helper
// creates recovery data only; there is no legacy grant execution or writer.
func seedLegacyControlFixture(t *testing.T, m *Manager, ctx context.Context, slug, consumerID string, until time.Time, pending bool) string {
	t.Helper()
	body, err := json.Marshal(legacyOAuthControl{Version: 1, ConsumerID: consumerID, Attempt: "historical", Until: until, Pending: pending})
	if err != nil {
		t.Fatal(err)
	}
	enc, err := m.encrypt(string(body))
	if err != nil {
		t.Fatal(err)
	}
	user, err := database.RequireUserID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := m.store.(*DBStore).db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", user, "mcp-tokens:"+slug).Update("legacy_oauth_control_enc", enc)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("historical fixture: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	return enc
}

func readLegacyControlFixture(t *testing.T, m *Manager, ctx context.Context, slug string) legacyOAuthControl {
	t.Helper()
	user, err := database.RequireUserID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var row database.CredentialEntry
	if err := m.store.(*DBStore).db.Where("user_id = ? AND pattern = ?", user, "mcp-tokens:"+slug).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := m.decrypt(row.LegacyOAuthControlEnc)
	if err != nil {
		t.Fatal(err)
	}
	var control legacyOAuthControl
	if err := json.Unmarshal([]byte(plain), &control); err != nil {
		t.Fatal(err)
	}
	return control
}

func (s *racingRefreshStore) UpdateRefreshTokenEncByID(ctx context.Context, id, previous, value string) error {
	if s.before != nil {
		before := s.before
		s.before = nil
		before()
	}
	return s.DBStore.UpdateRefreshTokenEncByID(ctx, id, previous, value)
}

func TestLegacyRefreshMaintenanceRecoversConcurrentCAS(t *testing.T) {
	for _, mode := range []string{"reencrypted", "controlled", "rotated", "deleted", "unreadable"} {
		t.Run(mode, func(t *testing.T) {
			a, b, db, ctx, id := legacyOperationFixture(t)
			if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "mcp-tokens:legacy").Update("refresh_token_enc", "plain-refresh").Error; err != nil {
				t.Fatal(err)
			}
			store := &racingRefreshStore{DBStore: b.store.(*DBStore)}
			b.store = store
			store.before = func() {
				if n, err := a.reencryptLegacyPlaintextRefreshTokens(ctx); err != nil || n != 1 {
					t.Fatalf("winning maintenance: n=%d err=%v", n, err)
				}
				switch mode {
				case "controlled":
					seedLegacyControlFixture(t, a, ctx, "legacy", id, time.Now().Add(time.Minute), true)
				case "rotated":
					if err := a.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &AuthConfig{Source: "static", Type: "oauth2", Token: "new", RefreshURL: "new-refresh"}); err != nil {
						t.Fatal(err)
					}
				case "deleted":
					if err := db.Where("pattern = ?", "mcp-tokens:legacy").Delete(&database.CredentialEntry{}).Error; err != nil {
						t.Fatal(err)
					}
				case "unreadable":
					if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "mcp-tokens:legacy").Update("refresh_token_enc", "unreadable-replacement").Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			n, err := b.reencryptLegacyPlaintextRefreshTokens(ctx)
			b.store = store.DBStore
			if mode == "unreadable" {
				if !errors.Is(err, oauthflow.ErrConflict) {
					t.Fatalf("genuine failure hidden: %v", err)
				}
				return
			}
			if err != nil || n != 0 {
				t.Fatalf("lost CAS aborted bootstrap or reported own write: n=%d err=%v", n, err)
			}
			if mode == "rotated" {
				if err := b.LoadUserCredentials(ctx, "owner"); err != nil {
					t.Fatal(err)
				}
				auth, err := b.GetByPatternWithContext(ctx, "mcp-tokens:legacy")
				if err != nil || auth == nil || auth.RefreshURL != "new-refresh" {
					t.Fatal("rotation lost")
				}
			}
		})
	}
}

func TestLegacyOAuthOperationSerializesManagersAndPreservesRotation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		_ = req.ParseForm()
		if req.Form.Get("refresh_token") != "old-refresh" {
			t.Error("wrong rotating grant")
		}
		select {
		case <-release:
		case <-req.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
	}))
	defer func() { unblock(); server.Close() }()
	f := newSharedOperationFixture(t, server.URL, "authorization_code")
	first, second := independentOAuthService(f.r), independentOAuthService(f.r)
	done := make(chan error, 1)
	go func() { _, err := first.Resolve(f.ctx, f.first, f.r.ID, f.r.Resource, ""); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh not started")
	}
	if _, err := second.Resolve(f.ctx, f.second, f.r.ID, f.r.Resource, ""); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("parallel refresh: %v", err)
	}
	if err := f.b.RegisterPatternWithContext(f.ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "replacement"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("parallel client edit: %v", err)
	}
	if err := f.b.DeleteOAuthAuthorization(f.ctx, f.r.ID, nil); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("parallel delete: %v", err)
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	current, err := second.Resolve(f.ctx, f.second, f.r.ID, f.r.Resource, "")
	if err != nil || current.Tokens.Access != "fresh" || current.Tokens.Refresh != "rotated" || current.RefreshPending || calls.Load() != 1 {
		t.Fatalf("authoritative rotation: calls=%d err=%v", calls.Load(), err)
	}
	var row database.CredentialEntry
	if err := f.db.First(&row, "id = ?", f.r.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.OAuthEnc == "" || strings.Contains(row.OAuthEnc, "fresh") || strings.Contains(row.OAuthEnc, "rotated") || row.TokenEnc != "" || row.RefreshTokenEnc != "" || row.LegacyOAuthControlEnc != "" {
		t.Fatal("plaintext, split storage or legacy control")
	}
}

func TestLegacyOAuthRejectsLateMaintenanceAndRenamedID(t *testing.T) {
	a, b, db, ctx, id := legacyOperationFixture(t)
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client"}); err != nil {
		t.Fatal(err)
	}
	var client database.CredentialEntry
	if err := db.Where("pattern = ?", "mcp-client:legacy").First(&client).Error; err != nil {
		t.Fatal(err)
	}
	var old database.CredentialEntry
	if err := db.Where("pattern = ?", "mcp-tokens:legacy").First(&old).Error; err != nil {
		t.Fatal(err)
	}
	seedLegacyControlFixture(t, a, ctx, "legacy", id, time.Now().Add(time.Minute), true)
	store := b.store.(*DBStore)
	for _, ids := range [][]string{{old.ID}, {client.ID}, {old.ID, client.ID}} {
		if removed, err := store.DeleteCredentialsByID(ctx, ids); removed != 0 || !errors.Is(err, oauthflow.ErrConflict) {
			t.Fatalf("purge bypassed operation: removed=%d err=%v", removed, err)
		}
	}
	if err := store.UpdateRefreshTokenEncByID(ctx, old.ID, old.RefreshTokenEnc, "late"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("maintenance bypassed lease: %v", err)
	}
	if err := store.SaveCredential(ctx, StoredCredential{ID: old.ID, UserID: "owner", Pattern: "renamed", Auth: &AuthConfig{Type: "bearer", Token: "replacement"}}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("renaming bypassed lease: %v", err)
	}
	// Advance the historical fixture to a rotation persisted by the old release;
	// maintenance must still compare the old ciphertext after its lease is gone.
	rotated, err := a.encrypt("rotated")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := a.encrypt("fresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.CredentialEntry{}).Where("id = ?", old.ID).Updates(map[string]any{"token_enc": fresh, "refresh_token_enc": rotated, "legacy_oauth_control_enc": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateRefreshTokenEncByID(ctx, old.ID, old.RefreshTokenEnc, "late"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("late maintenance overwrote rotation: %v", err)
	}
	if err := b.LoadUserCredentials(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	current, err := b.GetByPatternWithContext(ctx, "mcp-tokens:legacy")
	if err != nil || current == nil || current.RefreshURL != "rotated" {
		t.Fatalf("rotation lost: %v", err)
	}
}

func TestLegacyOAuthInterruptedRefreshRequiresExplicitRecovery(t *testing.T) {
	a, b, db, ctx, id := legacyOperationFixture(t)
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Updates(database.MCPServer{URL: "https://resource.example/mcp", OAuth2TokenURL: "https://resource.example/token"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	seedLegacyControlFixture(t, a, ctx, "legacy", id, time.Now().Add(-time.Minute), true)
	var before database.CredentialEntry
	if err := db.Where("pattern = ?", "mcp-tokens:legacy").First(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &AuthConfig{Source: "static", Type: "oauth2", Token: "implicit"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("implicit writer bypassed interrupted grant: %v", err)
	}
	dir := t.TempDir()
	snapshot, err := b.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	authorizations := 0
	authorize := func(flowCtx context.Context, store oauthflow.Store, r oauthflow.Record, consumer database.MCPServer) error {
		authorizations++
		var during database.CredentialEntry
		if err := db.First(&during, "id = ?", before.ID).Error; err != nil || during.TokenEnc != before.TokenEnc || during.RefreshTokenEnc != before.RefreshTokenEnc {
			t.Fatal("old tokens changed before explicit grant commit", err)
		}
		if err := a.store.(*DBStore).UpdateRefreshTokenEncByID(ctx, before.ID, before.RefreshTokenEnc, "late"); !errors.Is(err, oauthflow.ErrConflict) {
			t.Fatalf("late writer during reconnect: %v", err)
		}
		return reconnectTestAuthorize(flowCtx, store, r, consumer)
	}
	if err := b.ReconnectLegacyOAuth(ctx, dir, snapshot.ID, "client_secret_post", reconnectTestPrepare, authorize, reconnectTestProject, nil); err != nil {
		t.Fatal(err)
	}
	if authorizations != 1 {
		t.Fatal("explicit recovery did not execute exactly one grant")
	}
	var consumer database.MCPServer
	if err := db.First(&consumer, "id = ?", id).Error; err != nil || !consumer.OAuthManaged || consumer.OAuthAuthorizationID == "" {
		t.Fatal("recovery did not bind shared authorization", err)
	}
	store, err := a.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Load(ctx, consumer.OAuthAuthorizationID)
	if err != nil || current.Tokens.Access != "new" || current.Tokens.Refresh != "fresh" || current.RefreshPending {
		t.Fatalf("replacement grant was not committed: %v", err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &AuthConfig{Source: "static", Type: "oauth2", Token: "late"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("late writer after reconnect: %v", err)
	}
	if _, err := store.Load(database.WithUserID(context.Background(), "other"), current.ID); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("cross-user read: %v", err)
	}
	a.Reset(bytes.Repeat([]byte{5}, 32), true)
	if err := a.LoadUserCredentials(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if legacy, err := a.GetByPatternWithContext(ctx, "mcp-tokens:legacy"); err != nil || legacy != nil {
		t.Fatal(err)
	}
}

func TestLegacyOAuthExpiredCrashMarkerSurvivesRestart(t *testing.T) {
	a, b, db, ctx, id := legacyOperationFixture(t)
	seedLegacyControlFixture(t, a, ctx, "legacy", id, time.Now().Add(-time.Minute), true)
	var before database.CredentialEntry
	if err := db.Where("pattern = ?", "mcp-tokens:legacy").First(&before).Error; err != nil {
		t.Fatal(err)
	}
	control := readLegacyControlFixture(t, b, ctx, "legacy")
	if !control.Pending || control.ConsumerID != id || !control.Until.Before(time.Now()) {
		t.Fatal("expired crash barrier did not survive restart")
	}
	if err := b.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &AuthConfig{Source: "static", Type: "oauth2", Token: "implicit"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("expired marker allowed implicit writer: %v", err)
	}
	if err := b.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.store.(*DBStore).UpdateRefreshTokenEncByID(ctx, before.ID, before.RefreshTokenEnc, "late"); err == nil {
		t.Fatal("late maintenance resurrected deleted grant")
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("id = ?", before.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("deleted grant resurrected", err)
	}
}

func TestLegacyOAuthSessionAndFailedDeletionPreserveVault(t *testing.T) {
	a, _, _, ctx, id := legacyOperationFixture(t)
	seedLegacyControlFixture(t, a, ctx, "legacy", id, time.Now().Add(time.Minute), true)
	oldStore, err := a.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, cancel := oldStore.(*oauthStore).SessionContext(ctx)
	defer cancel()
	if err := a.DeletePattern(ctx, "mcp-tokens:legacy"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("delete accepted: %v", err)
	}
	if auth, err := a.GetByPatternWithContext(ctx, "mcp-tokens:legacy"); err != nil || auth == nil || auth.Token != "old" {
		t.Fatal("failed delete lost cache")
	}
	a.Reset(bytes.Repeat([]byte{5}, 32), true)
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("reset did not cancel old session")
	}
	candidate := oauthflow.Record{Version: 1, Revision: 1, ID: "late", UserID: "owner", ConsumerID: id, Integration: "mcp", GrantType: "authorization_code", State: "pending", Resource: "https://resource.example", Client: oauthflow.ClientRegistration{ID: "late", AuthMethod: "none"}}
	if err := oldStore.Create(ctx, candidate); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("old session saved client: %v", err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "late"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("ordinary writer bypassed historical recovery after reset: %v", err)
	}
	if control := readLegacyControlFixture(t, a, ctx, "legacy"); !control.Pending || control.ConsumerID != id {
		t.Fatal("session reset lost recovery barrier")
	}
}
