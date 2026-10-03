package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func TestLegacyPreflightDeadlineStopsBeforeAnonymousRequest(t *testing.T) {
	m, ctx, cfg := managedTokenTestFixture(t, "https://example.com/mcp", "old", "refresh", time.Now().Add(-time.Hour))
	store, _, _, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := m.repository().(*DBRepository).db
	if err := db.Callback().Query().Before("gorm:query").Register("deadline_preflight", func(tx *gorm.DB) {
		if tx.Statement.Table == "credential_entries" {
			_ = tx.AddError(errors.Join(errors.New("preflight"), context.DeadlineExceeded))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("deadline_preflight") })
	rt := &managedOAuthTransport{manager: m, cfg: cfg, ctx: ctx, store: store, base: managedTestRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("preflight timeout must not send an anonymous request or start interactive fallback")
		return nil, nil
	})}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := rt.RoundTrip(req)
	if response != nil || !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("unexpected deadline propagation: %v", err)
	}
}

func TestLegacyStoreFailureStopsBeforeAnonymousRequest(t *testing.T) {
	m, ctx, cfg := managedTokenTestFixture(t, "https://resource.example", "old", "refresh", time.Now().Add(-time.Hour))
	store, _, _, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := &managedOAuthTransport{manager: m, cfg: cfg, ctx: ctx, store: store}
	injected := errors.New("injected store failure")
	db := m.repository().(*DBRepository).db
	if err := db.Callback().Query().Before("gorm:query").Register("fail_legacy_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "credential_entries" {
			_ = tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("fail_legacy_read") })
	rt.base = managedTestRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("store failure must not send an anonymous request")
		return nil, nil
	})
	previousBrowser := browserOpen
	browserOpen = func(string) error {
		t.Fatal("store failure must not start interactive fallback")
		return nil
	}
	t.Cleanup(func() { browserOpen = previousBrowser })
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := rt.RoundTrip(req)
	if response != nil || !errors.Is(err, injected) {
		t.Fatalf("store failure was not preserved as terminal: %v", err)
	}
}

func TestLegacyMissingGrantRequiresMigrationBeforeInitialProbe(t *testing.T) {
	m, _, ctx, cfg := legacyWALManagers(t, "https://resource.example")
	if err := m.DeleteServerAuth("legacy"); err != nil {
		t.Fatal(err)
	}
	client := m.buildAuthHTTPClient(ctx, "legacy", cfg)
	oldBrowser := browserOpen
	browserOpen = func(string) error { t.Error("missing historical grant opened browser"); return nil }
	defer func() { browserOpen = oldBrowser }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if response != nil || !errors.Is(err, errOAuthMigrationRequired) {
		t.Fatalf("missing historical grant allowed initial probe: %v", err)
	}
}

func TestOAuthCreationPreservesExplicitConnectionFlags(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, autoConnect := range []bool{false, true} {
				m, repo, ctx := managedFixture(t)
				cfg := managedConfig("https://resource.example")
				cfg.OAuthManaged, cfg.Enabled, cfg.AutoConnect = managed, enabled, autoConnect
				if err := m.SaveConfig("flags", cfg); err != nil {
					t.Fatal(err)
				}
				stored, err := repo.GetServer(ctx, "flags")
				if err != nil {
					t.Fatal(err)
				}
				m.mu.RLock()
				cached := m.servers["flags"].Config
				m.mu.RUnlock()
				if stored.Enabled != enabled || stored.AutoConnect != autoConnect || cached.Enabled != enabled || cached.AutoConnect != autoConnect {
					t.Fatalf("changed explicit flags: managed=%v enabled=%v autoConnect=%v database=%v/%v cache=%v/%v", managed, enabled, autoConnect, stored.Enabled, stored.AutoConnect, cached.Enabled, cached.AutoConnect)
				}
				if !enabled {
					if err := m.Connect("flags"); err == nil {
						t.Fatal("disabled server accepted connection")
					}
				}
			}
		}
	}
}

func TestManagedOAuthRejectsLateLegacyWriters(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	cfg := managedConfig("https://resource.example")
	if err := m.SaveConfigWithOAuthSecret("owned", cfg, "secret"); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetServer(ctx, "owned")
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{clientCredPattern("owned"), userTokensPattern("owned")} {
		if err := m.credMgr.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "late"}); !errors.Is(err, oauthflow.ErrConflict) {
			t.Fatalf("late writer: %v", err)
		}
		if value, err := m.credMgr.GetByPatternWithContext(ctx, pattern); err != nil || value != nil {
			t.Fatalf("late write published to cache: %v", err)
		}
	}
	stale := *stored
	stale.OAuthManaged, stale.OAuthAuthorizationID = false, ""
	stale.OAuth2ClientID = "old-client"
	if err := repo.SaveServer(ctx, &stale); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("stale config reclaimed ownership: %v", err)
	}
	fresh, err := repo.GetServer(ctx, "owned")
	if err != nil || fresh.OAuthAuthorizationID != stored.OAuthAuthorizationID || !fresh.OAuthManaged {
		t.Fatal("managed binding changed")
	}
	var count int64
	if err := repo.db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy pair recreated: count=%d err=%v", count, err)
	}
}

func TestLegacyConfigurationCallbackCannotEditManagedAuthorization(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	if err := m.SaveConfigWithOAuthSecret("owned", managedConfig("https://resource.example"), "secret"); err != nil {
		t.Fatal(err)
	}
	cfg, store, before := loadManaged(t, m, ctx, "owned")
	legacy := cfg
	legacy.OAuthManaged, legacy.OAuthAuthorizationID = false, ""
	legacy.OAuth2ClientID = "late-client"
	if err := repo.SaveServer(ctx, &legacy); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("legacy configuration reclaimed shared authorization: %v", err)
	}
	after, err := store.Load(ctx, before.ID)
	if err != nil || after.Revision != before.Revision || after.Client.ID != before.Client.ID {
		t.Fatal("protocol callback changed shared authorization")
	}
	stored, err := repo.GetServer(ctx, "owned")
	if err != nil || !stored.OAuthManaged || stored.OAuthAuthorizationID != before.ID {
		t.Fatal("protocol callback removed shared ownership")
	}
}

func TestLegacyConfigurationWriterPreservesEditsAndSession(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	cfg := managedConfig("https://resource.example")
	cfg.OAuth2TokenAuthMethod = "none"
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	original := m.servers["legacy"].Config // real cache projection: Env is nil
	if original.Env != nil {
		t.Fatal("fixture must cover nil environment")
	}
	_, store, updated := loadManaged(t, m, ctx, "legacy")
	updated.Callback.Port, updated.Callback.PortPolicy = 12345, "fixed"
	updated.Revision++
	if err := store.CompareAndSwap(ctx, updated, updated.Revision-1); err != nil {
		t.Fatal(err)
	}
	_, _, updated = loadManaged(t, m, ctx, "legacy")
	updated.Callback.Port = 12346
	updated.Revision++
	if err := store.CompareAndSwap(ctx, updated, updated.Revision-1); err != nil {
		t.Fatal(err)
	}
	_, _, updated = loadManaged(t, m, ctx, "legacy")
	edited := projectOAuthConfiguration(original, updated)
	edited.Name = "User edit"
	if err := m.SaveConfig("legacy", edited); err != nil {
		t.Fatal(err)
	}
	updated.Callback.Port = 12347
	updated.Revision++
	if err := store.CompareAndSwap(ctx, updated, updated.Revision-1); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("overwrote user edit: %v", err)
	}
	fresh, err := repo.GetServer(ctx, "legacy")
	_, _, saved := loadManaged(t, m, ctx, "legacy")
	if err != nil || fresh.Name != edited.Name || saved.Callback.Port != edited.OAuth2CallbackPort {
		t.Fatal("edit lost")
	}
	m.credMgr.Reset(make([]byte, 32), true)
	saved.Revision++
	if err := store.CompareAndSwap(ctx, saved, saved.Revision-1); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("stale session wrote: %v", err)
	}
}

func TestManagedRuntimeRevisionConflictKeepsPublicClassification(t *testing.T) {
	m, ctx, cfg := managedTokenTestFixture(t, "https://example.com/mcp", "old", "refresh", time.Now().Add(-time.Hour))
	_, store, original := loadManaged(t, m, ctx, cfg.Slug)
	newer := original
	newer.Revision++
	newer.Tokens.Access = "newer-token"
	if err := store.CompareAndSwap(ctx, newer, original.Revision); err != nil {
		t.Fatal(err)
	}
	bound := mcpRuntimeOAuthStore{mcpAtomicOAuthStore: store.(mcpAtomicOAuthStore), cfg: cfg}
	stale := original
	stale.Revision++
	err := bound.CompareAndSwap(ctx, stale, original.Revision)
	if !errors.Is(err, oauthflow.ErrConflict) || err.Error() != oauthflow.ErrConflict.Error() || errors.Is(err, errOAuthPersistence) {
		t.Fatalf("revision conflict lost its safe public code: %v", err)
	}
	saved, err := store.Load(ctx, original.ID)
	if err != nil || saved.Revision != newer.Revision || saved.Tokens.Access != newer.Tokens.Access {
		t.Fatal("stale publication changed current grant", err)
	}
}

func TestLegacyRefreshRetryPersistenceFailureDoesNotSendAnotherRequest(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	var requests, tokenCalls atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			requests.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	cfg := managedConfig(tokenServer.URL)
	cfg.OAuth2TokenAuthMethod = "none"
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	seedManagedRuntime(t, m, ctx, "legacy", "old", "refresh", time.Now().Add(time.Hour))
	cfg, _, _ = loadManaged(t, m, ctx, "legacy")
	if err := repo.db.Callback().Update().Before("gorm:update").Register("reject_second_save", func(tx *gorm.DB) {
		if fields, ok := tx.Statement.Dest.(map[string]any); ok && fields["oauth_enc"] != nil && tokenCalls.Load() > 0 {
			_ = tx.AddError(errors.New("disk full"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.db.Callback().Update().Remove("reject_second_save") })
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, tokenServer.URL, nil)
	response, err := m.managedHTTPClient(ctx, cfg).Do(req)
	if response != nil || !errors.Is(err, errOAuthPersistence) || requests.Load() != 1 || tokenCalls.Load() != 1 {
		t.Fatalf("hidden persistence failure: requests=%d err=%v", requests.Load(), err)
	}
}

func TestLegacyTokenPersistenceFailureIsTerminalAndSanitized(t *testing.T) {
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()
	m, ctx, cfg := managedTokenTestFixture(t, srv.URL, "old", "refresh", time.Now().Add(-time.Hour))
	repo := m.repository().(*DBRepository)
	const privateError = "database error containing private material"
	var failed atomic.Bool
	if err := repo.db.Callback().Update().Before("gorm:update").Register("reject_legacy_token", func(tx *gorm.DB) {
		if fields, ok := tx.Statement.Dest.(map[string]any); ok && fields["oauth_enc"] != nil && tokenCalls.Load() > 0 && failed.CompareAndSwap(false, true) {
			_ = tx.AddError(errors.New(privateError))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.db.Callback().Update().Remove("reject_legacy_token") })
	token, err := m.resolveManagedOAuth(ctx, cfg, "")
	if token.Tokens.Access != "" || !errors.Is(err, errOAuthPersistence) || strings.Contains(err.Error(), privateError) {
		t.Fatalf("persistence failure reported as success or leaked: %v", err)
	}
	_, _, pending := loadManaged(t, m, ctx, "srv")
	if !pending.RefreshPending || pending.Tokens.Access != "old" || !terminalOAuthNetworkError(ctx, err) || !terminalDeviceGrantError(ctx, err) {
		t.Fatal("failed save could initiate another authorization")
	}
	if err := repo.db.Callback().Update().Remove("reject_legacy_token"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.resolveManagedOAuth(ctx, cfg, ""); !errors.Is(err, oauthflow.ErrReauthorize) || tokenCalls.Load() != 1 {
		t.Fatalf("ambiguous rotation retried after storage recovery: %v", err)
	}
	// Explicit reconnection may publish the replacement grant after storage recovers.
	store, _, service, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AuthorizeUsing(ctx, store, cfg.OAuthAuthorizationID, func(_ context.Context, r oauthflow.Record) (oauthflow.Record, error) {
		r.Tokens = oauthflow.Tokens{Access: "new-access", Refresh: "new-refresh", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
		return r, nil
	})
	if err != nil {
		t.Fatalf("explicit recovery could not persist replacement: %v", err)
	}
	stored := loadManagedTestToken(t, m, ctx, "srv")
	if stored == nil || stored.RefreshToken != "new-refresh" {
		t.Fatal("rotated token was not preserved")
	}
}

func TestManagedOAuthLegacyFencePreservesOtherConsumersAndResidues(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	legacy := &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "original"}
	if err := m.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("owned"), legacy); err != nil {
		t.Fatal(err)
	}
	var residue database.CredentialEntry
	if err := repo.db.Where("pattern = ?", userTokensPattern("owned")).First(&residue).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.SaveConfig("owned", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	if err := m.credMgr.RegisterStoredCredentialWithContext(ctx, credentials.StoredCredential{ID: residue.ID, Pattern: residue.Pattern, Auth: &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "late"}}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("ID update bypassed fence: %v", err)
	}
	value, err := m.credMgr.GetByPatternWithContext(ctx, residue.Pattern)
	if err != nil || value.Token != "original" {
		t.Fatal("residue overwritten")
	}
	otherUser := database.WithUserID(t.Context(), "other")
	if err := m.credMgr.RegisterPatternWithContext(otherUser, residue.Pattern, legacy); err != nil {
		t.Fatal(err)
	}
	if err := m.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("different"), legacy); err != nil {
		t.Fatal(err)
	}
}
