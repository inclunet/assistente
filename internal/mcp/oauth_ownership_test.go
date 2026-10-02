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
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

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
	rt := m.buildPKCERoundTripperForServer(ctx, "owned", legacy)
	legacy.OAuth2ClientID = "late-client"
	rt.onConfigUpdate(legacy)
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
	cfg.OAuthManaged = false
	cfg.OAuth2TokenAuthMethod = "client_secret_post" // transient in legacy storage
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	original := m.servers["legacy"].Config // real cache projection: Env is nil
	if original.Env != nil {
		t.Fatal("fixture must cover nil environment")
	}
	write := m.legacyOAuthConfigWriter(original)
	updated := original
	updated.OAuth2CallbackPort = 12345
	if err := write(updated); err != nil {
		t.Fatal(err)
	}
	updated.OAuth2CallbackPort = 12346
	if err := write(updated); err != nil {
		t.Fatal(err)
	}
	edited := updated
	edited.Name = "User edit"
	if err := m.SaveConfig("legacy", edited); err != nil {
		t.Fatal(err)
	}
	updated.OAuth2CallbackPort = 12347
	if err := write(updated); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("overwrote user edit: %v", err)
	}
	fresh, err := repo.GetServer(ctx, "legacy")
	if err != nil || fresh.Name != edited.Name || fresh.OAuth2CallbackPort != edited.OAuth2CallbackPort {
		t.Fatal("edit lost")
	}
	write = m.legacyOAuthConfigWriter(*fresh)
	m.credMgr.Reset(make([]byte, 32), true)
	if err := write(*fresh); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("stale session wrote: %v", err)
	}
}

func TestLegacyRefreshRetryPersistenceFailureDoesNotSendAnotherRequest(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	var requests atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			requests.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	if err := m.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "old", RefreshURL: "refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	writes := 0
	if err := repo.db.Callback().Create().Before("gorm:create").Register("reject_second_save", func(tx *gorm.DB) {
		if tx.Statement.Table == "credential_entries" {
			writes++
			if writes == 2 {
				_ = tx.AddError(errors.New("disk full"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.db.Callback().Create().Remove("reject_second_save") })
	rt := &pkceRoundTripper{credMgr: m.credMgr, serverSlug: "legacy", cfg: ServerConfig{URL: tokenServer.URL}, authCtxProvider: func() context.Context { return ctx }, base: http.DefaultTransport}
	rt.oauthCfg = &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: tokenServer.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	rt.tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "old", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, tokenServer.URL, nil)
	response, err := rt.RoundTrip(req)
	if response != nil || !errors.Is(err, errOAuthPersistence) || requests.Load() != 1 {
		t.Fatalf("hidden persistence failure: requests=%d err=%v", requests.Load(), err)
	}
}

func TestLegacyTokenPersistenceFailureIsTerminalAndSanitized(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	const privateError = "database error containing private material"
	if err := repo.db.Callback().Create().Before("gorm:create").Register("reject_legacy_token", func(tx *gorm.DB) {
		if tx.Statement.Table == "credential_entries" {
			_ = tx.AddError(errors.New(privateError))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.db.Callback().Create().Remove("reject_legacy_token") })
	rt := &pkceRoundTripper{credMgr: m.credMgr, serverSlug: "legacy", authCtxProvider: func() context.Context { return ctx }}
	pts := &persistingTokenSource{inner: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "new-access", RefreshToken: "new-refresh"}), rt: rt}
	token, err := pts.Token()
	if token != nil || !errors.Is(err, errOAuthPersistence) || strings.Contains(err.Error(), privateError) {
		t.Fatalf("persistence failure reported as success or leaked: %v", err)
	}
	if pts.lastToken != "" || !terminalOAuthNetworkError(ctx, err) || !terminalDeviceGrantError(ctx, err) {
		t.Fatal("failed save could initiate another authorization")
	}
	if err := repo.db.Callback().Create().Remove("reject_legacy_token"); err != nil {
		t.Fatal(err)
	}
	if token, err = pts.Token(); err != nil || token.AccessToken != "new-access" {
		t.Fatalf("same returned token could not be persisted after storage recovery: %v", err)
	}
	stored := loadUserTokens(ctx, m.credMgr, "legacy")
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
