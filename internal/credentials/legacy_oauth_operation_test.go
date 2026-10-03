package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type credentialRevisionCallback func(context.Context, string) error

func (callback credentialRevisionCallback) RefreshCredentialPatternRevisions(ctx context.Context, pattern string) error {
	return callback(ctx, pattern)
}

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

func TestLegacyHostnameRevalidatesAfterSourceResolution(t *testing.T) {
	for _, change := range []string{"unchanged", "grant", "consumer", "hostname_changed", "hostname_deleted"} {
		t.Run(change, func(t *testing.T) {
			a, b, db, ctx, id := legacyOperationFixture(t)
			if err := db.Where("pattern = ?", "mcp-tokens:legacy").Delete(&database.CredentialEntry{}).Error; err != nil {
				t.Fatal(err)
			}
			if err := a.RegisterPatternWithContext(ctx, "fallback.example", &AuthConfig{Source: "static", Type: "bearer", Token: "fallback"}); err != nil {
				t.Fatal(err)
			}
			validate := func(tx *gorm.DB) error {
				var count int64
				if err := tx.Model(&database.MCPServer{}).Where("id = ? AND auth_type = ?", id, "oauth2_pkce").Count(&count).Error; err != nil {
					return err
				}
				if count != 1 {
					return oauthflow.ErrConflict
				}
				return nil
			}
			called := false
			auth, err := a.readLegacyHostnameToken(ctx, "legacy", "fallback.example", validate, func(_ context.Context, auth *AuthConfig) (*AuthConfig, error) {
				called = true
				other := b.store.(*DBStore).db
				var err error
				switch change {
				case "grant":
					err = b.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &AuthConfig{Source: "static", Type: "oauth2", Token: "new-grant"})
				case "consumer":
					err = other.Model(&database.MCPServer{}).Where("id = ?", id).Update("auth_type", "none").Error
				case "hostname_changed":
					err = b.RegisterPatternWithContext(ctx, "fallback.example", &AuthConfig{Source: "static", Type: "bearer", Token: "replacement"})
				case "hostname_deleted":
					err = b.DeletePattern(ctx, "fallback.example")
				}
				if err != nil {
					t.Fatal(err)
				}
				return auth, nil
			})
			if !called {
				t.Fatal("source was not resolved")
			}
			if change == "unchanged" {
				if err != nil || auth == nil || auth.Token != "fallback" {
					t.Fatalf("unchanged fallback: %v", err)
				}
			} else if !errors.Is(err, oauthflow.ErrConflict) || auth != nil {
				t.Fatalf("stale source accepted: %v", err)
			}
		})
	}
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
				case "controlled", "rotated":
					op, _, err := a.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(op.End)
					if mode == "rotated" {
						if err := op.Commit(ctx, &AuthConfig{Token: "new", RefreshURL: "new-refresh"}); err != nil {
							t.Fatal(err)
						}
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
				auth, err := b.ReadLegacyOAuthToken(ctx, "legacy", id)
				if err != nil || auth.RefreshURL != "new-refresh" {
					t.Fatal("rotation lost")
				}
			}
		})
	}
}

func TestLegacyOAuthOperationSerializesManagersAndPreservesRotation(t *testing.T) {
	a, b, db, ctx, id := legacyOperationFixture(t)
	op, auth, err := a.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil)
	if err != nil || auth.RefreshURL != "old-refresh" {
		t.Fatalf("begin: %v", err)
	}
	defer op.End()
	if _, _, err := b.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("parallel refresh: %v", err)
	}
	if err := b.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "replacement"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("parallel client edit: %v", err)
	}
	if err := (&DBStore{db: db}).DeleteCredential(ctx, "mcp-tokens:legacy"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("parallel delete: %v", err)
	}
	if err := op.Commit(ctx, &AuthConfig{Source: "static", Type: "oauth2", Token: "fresh", RefreshURL: "rotated"}); err != nil {
		t.Fatal(err)
	}
	current, err := b.ReadLegacyOAuthToken(ctx, "legacy", id)
	if err != nil || current.Token != "fresh" || current.RefreshURL != "rotated" {
		t.Fatalf("authoritative read: %v", err)
	}
	var row database.CredentialEntry
	if err := db.Where("pattern = ?", "mcp-tokens:legacy").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.TokenEnc == "fresh" || row.RefreshTokenEnc == "rotated" || row.LegacyOAuthControlEnc != "" {
		t.Fatal("plaintext or uncleared control")
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
	op, _, err := a.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer op.End()
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
	if err := op.Commit(ctx, &AuthConfig{Token: "fresh", RefreshURL: "rotated"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateRefreshTokenEncByID(ctx, old.ID, old.RefreshTokenEnc, "late"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("late maintenance overwrote rotation: %v", err)
	}
	current, err := b.ReadLegacyOAuthToken(ctx, "legacy", id)
	if err != nil || current.RefreshURL != "rotated" {
		t.Fatalf("rotation lost: %v", err)
	}
}

func TestLegacyOAuthInterruptedRefreshRequiresExplicitRecovery(t *testing.T) {
	a, b, _, ctx, id := legacyOperationFixture(t)
	old, _, err := a.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	old.End() // remote result unknown, including crash after a response
	if _, err := b.ReadLegacyOAuthToken(ctx, "legacy", id); !errors.Is(err, oauthflow.ErrReauthorize) {
		t.Fatalf("pending token resolved: %v", err)
	}
	if _, _, err := b.BeginLegacyOAuth(ctx, "legacy", id, true, false, nil); !errors.Is(err, oauthflow.ErrReauthorize) {
		t.Fatalf("implicit consent bypass: %v", err)
	}
	current, _, err := b.BeginLegacyOAuth(ctx, "legacy", id, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer current.End()
	old.End()
	if err := old.Commit(ctx, &AuthConfig{Source: "static", Type: "oauth2", Token: "late"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("late writer: %v", err)
	}
	if err := current.Commit(ctx, &AuthConfig{Source: "static", Type: "oauth2", Token: "reconnected", RefreshURL: "new-grant"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReadLegacyOAuthToken(database.WithUserID(context.Background(), "other"), "legacy", id); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("cross-user read: %v", err)
	}
}

func TestLegacyOAuthExpiredCrashMarkerSurvivesRestart(t *testing.T) {
	a, b, db, ctx, id := legacyOperationFixture(t)
	op, _, err := a.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := a.decrypt(op.control)
	if err != nil {
		t.Fatal(err)
	}
	var control legacyOAuthControl
	if err := json.Unmarshal([]byte(data), &control); err != nil {
		t.Fatal(err)
	}
	control.Until = time.Now().Add(-time.Minute)
	body, _ := json.Marshal(control)
	enc, err := a.encrypt(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.CredentialEntry{}).Where("id = ?", op.rowID).Update("legacy_oauth_control_enc", enc).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.BeginLegacyOAuth(ctx, "legacy", id, false, false, nil); !errors.Is(err, oauthflow.ErrReauthorize) {
		t.Fatalf("expired crash marker allowed refresh: %v", err)
	}
	if err := b.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	if err := op.Commit(ctx, &AuthConfig{Source: "static", Type: "oauth2", Token: "late"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("deleted grant resurrected: %v", err)
	}
}

func TestClearLegacyOAuthRefreshesCredentialRevisionsAfterCommitAndUnlock(t *testing.T) {
	manager, _, db, ctx, id := legacyOperationFixture(t)
	if err := manager.RegisterPatternWithContext(ctx, "shared.example.test", &AuthConfig{Source: "static", Type: "bearer", Token: "shared"}); err != nil {
		t.Fatal(err)
	}
	var patterns []string
	manager.SetCredentialRevisionRefresher(credentialRevisionCallback(func(callbackCtx context.Context, pattern string) error {
		// This read lock would deadlock if ClearLegacyOAuthWithConsumer called
		// the refresher while still holding the manager's write lock.
		if !manager.CanPersist() {
			t.Error("manager deixou de persistir durante o refresh")
		}
		var count int64
		if err := db.WithContext(callbackCtx).Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", "owner", pattern).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("pattern %q ainda existia quando a revisão foi sincronizada", pattern)
		}
		patterns = append(patterns, pattern)
		return nil
	}))

	done := make(chan error, 1)
	go func() {
		done <- manager.ClearLegacyOAuthWithConsumer(ctx, "legacy", id, "shared.example.test", nil, nil)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("limpar credenciais: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refresh de revisões bloqueou enquanto o manager mantinha o lock")
	}
	if len(patterns) != 1 || patterns[0] != "shared.example.test" {
		t.Fatalf("esperava sincronização apenas do hostname compartilhado, recebeu %v", patterns)
	}
}

func TestLegacyOAuthSessionAndFailedDeletionPreserveVault(t *testing.T) {
	a, _, _, ctx, id := legacyOperationFixture(t)
	op, _, err := a.BeginLegacyOAuth(ctx, "legacy", id, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeletePattern(ctx, "mcp-tokens:legacy"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("delete accepted: %v", err)
	}
	if auth, err := a.GetByPatternWithContext(ctx, "mcp-tokens:legacy"); err != nil || auth == nil || auth.Token != "old" {
		t.Fatal("failed delete lost cache")
	}
	a.Reset(bytes.Repeat([]byte{5}, 32), true)
	if err := op.SaveClient(ctx, &AuthConfig{Source: "static", Type: "oauth2", ClientID: "late"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("old session saved client: %v", err)
	}
	if err := a.RegisterPatternWithContext(op.Context(ctx), "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "late"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("old receipt bypass: %v", err)
	}
}
