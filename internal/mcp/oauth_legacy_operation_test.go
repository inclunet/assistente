package mcp

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"assistente/internal/tools"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUnmanagedAuthDeletionRollsBackEveryPattern(t *testing.T) {
	for _, kind := range []AuthType{AuthBearer, AuthBasic, AuthOAuth2ClientCredentials, AuthNone} {
		t.Run(string(kind), func(t *testing.T) {
			a, _, ctx, cfg := legacyWALManagers(t, "https://auth.example")
			cfg.AuthType = kind
			if err := a.SaveConfig("legacy", cfg); err != nil {
				t.Fatal(err)
			}
			patterns := []string{clientCredPattern("legacy"), userTokensPattern("legacy"), "auth.example"}
			for _, pattern := range patterns {
				if err := a.credMgr.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "stored", ClientID: "client", ClientSecret: "secret"}); err != nil {
					t.Fatal(err)
				}
			}
			db := a.repository().(*DBRepository).db
			calls := 0
			if err := db.Callback().Delete().After("gorm:delete").Register("reject_partial_auth_delete", func(tx *gorm.DB) {
				calls++
				if calls == 2 || tx.RowsAffected > 1 {
					_ = tx.AddError(errors.New("injected delete failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Delete().Remove("reject_partial_auth_delete") })
			if err := a.DeleteServerAuth("legacy"); err == nil {
				t.Fatal("expected rollback")
			}
			var count int64
			if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", cfg.UserID, patterns).Count(&count).Error; err != nil || count != 3 {
				t.Fatalf("partial removal: %d %v", count, err)
			}
			if err := db.Callback().Delete().Remove("reject_partial_auth_delete"); err != nil {
				t.Fatal(err)
			}
			if err := a.DeleteServerAuth("legacy"); err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", cfg.UserID, patterns).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("residue retained: %d %v", count, err)
			}
		})
	}
}

func TestUnmanagedAuthDeletionKeepsInMemoryFallback(t *testing.T) {
	m := newTestManager()
	ctx := database.WithUserID(context.Background(), "user")
	m.SetAuthContextProvider(func() context.Context { return ctx })
	m.servers["memory"] = &ServerStatus{Config: ServerConfig{AuthType: AuthBearer, URL: "https://memory.example", ID: "memory-id"}}
	if err := m.credMgr.RegisterPatternWithContext(ctx, "memory.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "token"}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteServerAuth("memory"); err != nil {
		t.Fatal(err)
	}
	if auth, err := m.credMgr.GetByPatternWithContext(ctx, "memory.example"); err != nil || auth != nil {
		t.Fatal("memory credentials retained", err)
	}
}

func TestLegacyNoneSaveClearsAuthoritativeAuthType(t *testing.T) {
	for _, kind := range []AuthType{AuthBearer, AuthBasic, AuthOAuth2ClientCredentials, AuthNone} {
		t.Run(string(kind), func(t *testing.T) {
			a, b, ctx, cfg := legacyWALManagers(t, "https://old.example")
			latest := cfg
			latest.AuthType = kind
			latest.URL = "https://current.example/mcp"
			if err := b.SaveConfig("legacy", latest); err != nil {
				t.Fatal(err)
			}
			if err := b.credMgr.RegisterPatternWithContext(ctx, "current.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "current"}); err != nil {
				t.Fatal(err)
			}
			db := b.repository().(*DBRepository).db
			writerDB := a.repository().(*DBRepository).db
			if err := writerDB.Callback().Update().Before("gorm:update").Register("reject_none_save", func(tx *gorm.DB) { _ = tx.AddError(errors.New("failed save")) }); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writerDB.Callback().Update().Remove("reject_none_save") })
			none := cfg
			none.AuthType = AuthNone
			if err := a.SaveConfig("legacy", none); err == nil {
				t.Fatal("expected atomic rollback")
			}
			var count int64
			if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "current.example").Count(&count).Error; err != nil || count != 1 {
				t.Fatal("rollback lost current host", err)
			}
			if err := writerDB.Callback().Update().Remove("reject_none_save"); err != nil {
				t.Fatal(err)
			}
			if err := a.SaveConfig("legacy", none); err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", cfg.UserID, []string{clientCredPattern("legacy"), userTokensPattern("legacy"), "current.example"}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("current credentials retained: %d %v", count, err)
			}
		})
	}
}

func TestLegacyAuthMetadataDoesNotReuseRemovedCache(t *testing.T) {
	a, b, ctx, cfg := legacyWALManagers(t, "https://metadata.example")
	if err := a.SaveServerAuth("legacy", "oauth2_pkce", "", "", "", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := a.credMgr.RegisterPatternWithContext(ctx, "metadata.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "cached"}); err != nil {
		t.Fatal(err)
	}
	if _, has, err := a.GetServerAuthInfo("legacy"); err != nil || !has {
		t.Fatal("missing setup metadata", err)
	}
	if err := b.credMgr.ClearLegacyOAuth(ctx, "legacy", cfg.ID, "metadata.example"); err != nil {
		t.Fatal(err)
	}
	if _, has, err := a.GetServerAuthInfo("legacy"); err != nil || has {
		t.Fatalf("removed cache reported configured: %v %v", has, err)
	}
}

func TestLegacyAuthMutationsRejectOtherInstanceConsumerChanges(t *testing.T) {
	for _, kind := range []AuthType{AuthNone, AuthBearer, AuthOAuth2ClientCredentials, AuthOAuth2PKCE} {
		t.Run(string(kind), func(t *testing.T) {
			a, b, ctx, original := legacyWALManagers(t, "https://before.example")
			if err := a.credMgr.ClearLegacyOAuth(ctx, "legacy", original.ID, ""); err != nil {
				t.Fatal(err)
			}
			cached := original
			cached.AuthType = kind
			if err := a.SaveConfig("legacy", cached); err != nil {
				t.Fatal(err)
			}
			latest := original
			latest.OAuth2ClientID = "new-client"
			latest.URL = "https://after.example/mcp"
			if err := b.SaveConfig("legacy", latest); err != nil {
				t.Fatal(err)
			}
			if err := b.SaveServerAuth("legacy", "oauth2_pkce", "", "", "", "new-secret"); err != nil {
				t.Fatal(err)
			}
			if err := b.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "new-token", RefreshURL: "refresh"}); err != nil {
				t.Fatal(err)
			}
			if err := a.SaveServerAuth("legacy", "oauth2_pkce", "", "", "", "old-secret"); !errors.Is(err, oauthflow.ErrConflict) {
				t.Fatalf("stale client overwrite allowed: %v", err)
			}
			if err := a.DeleteServerAuth("legacy"); !errors.Is(err, oauthflow.ErrConflict) {
				t.Fatalf("stale route deletion allowed: %v", err)
			}
			client, err := b.credMgr.GetByPatternWithContext(ctx, clientCredPattern("legacy"))
			if err != nil || client == nil {
				t.Fatal(err)
			}
			tokens := loadUserTokens(ctx, b.credMgr, "legacy")
			if client.ClientID != "new-client" || client.ClientSecret != "new-secret" || tokens == nil || tokens.AccessToken != "new-token" {
				t.Fatal("new grant changed")
			}
		})
	}
}

type beforeDeleteRepository struct {
	Repository
	before func()
}

func (r beforeDeleteRepository) DeleteServer(ctx context.Context, slug string) error {
	r.before()
	return r.Repository.DeleteServer(ctx, slug)
}

func TestLegacyGenericDeleteRejectsConsumerChangedAfterRead(t *testing.T) {
	a, b, ctx, original := legacyWALManagers(t, "https://delete.example")
	none := original
	none.AuthType = AuthNone
	if err := a.SaveConfig("legacy", none); err != nil {
		t.Fatal(err)
	}
	a.SetRepository(beforeDeleteRepository{Repository: a.repository(), before: func() {
		if err := b.SaveConfig("legacy", original); err != nil {
			t.Fatal(err)
		}
		if err := b.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "new", RefreshURL: "refresh"}); err != nil {
			t.Fatal(err)
		}
	}})
	if err := a.DeleteConfig("legacy"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("deleted changed consumer: %v", err)
	}
	if _, err := b.repository().GetServer(ctx, "legacy"); err != nil {
		t.Fatal(err)
	}
	if auth, err := b.credMgr.GetByPatternWithContext(ctx, userTokensPattern("legacy")); err != nil || auth == nil || auth.Token != "new" {
		t.Fatal("grant changed", err)
	}
}

func TestLegacyDeleteAuthRejectsStaleConsumerAndPreservesFallbacks(t *testing.T) {
	a, b, ctx, cfg := legacyWALManagers(t, "https://old.example")
	for _, host := range []string{"old.example", "new.example"} {
		if err := a.credMgr.RegisterPatternWithContext(ctx, host, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: host}); err != nil {
			t.Fatal(err)
		}
	}
	cfg.URL = "https://new.example/mcp"
	if err := b.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteServerAuth("legacy"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("stale deletion allowed: %v", err)
	}
	db := a.repository().(*DBRepository).db
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", cfg.UserID, []string{userTokensPattern("legacy"), "old.example", "new.example"}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("rollback lost credentials: %d %v", count, err)
	}
	if err := b.DeleteServerAuth("legacy"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", cfg.UserID, []string{userTokensPattern("legacy"), "new.example"}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("current credentials remain: %d %v", count, err)
	}
}

func TestLegacyProactiveAdoptsConcurrentRotation(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "proactive", true: "forced"}[force], func(t *testing.T) {
			a, b, ctx, cfg := managedWALManagers(t, "https://unused.example")
			db := a.repository().(*DBRepository).db
			var rotated atomic.Bool
			if err := db.Callback().Query().After("gorm:query").Register("rotate_before_resolution", func(tx *gorm.DB) {
				row, ok := tx.Statement.Dest.(*database.CredentialEntry)
				if !ok || row.ID != cfg.OAuthAuthorizationID || !rotated.CompareAndSwap(false, true) {
					return
				}
				_, store, current := loadManaged(t, b, ctx, "legacy")
				current.Tokens = oauthflow.Tokens{Access: "concurrent", Refresh: "rotated", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
				current.Revision++
				if err := store.CompareAndSwap(ctx, current, current.Revision-1); err != nil {
					t.Error(err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove("rotate_before_resolution") })
			rejected := ""
			if force {
				rejected = "old"
			}
			token, err := a.resolveManagedOAuth(ctx, cfg, rejected)
			if err != nil || token.Tokens.Access != "concurrent" || token.Tokens.Refresh != "rotated" || !rotated.Load() {
				t.Fatalf("did not adopt concurrent token without remote refresh: %v", err)
			}
		})
	}
}

func TestLegacyNativeFallbackDoesNotReuseDeletedHostname(t *testing.T) {
	a, b, ctx, cfg := legacyWALManagers(t, "https://fallback.example")
	if err := a.credMgr.RegisterPatternWithContext(ctx, "fallback.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "removed"}); err != nil {
		t.Fatal(err)
	}
	if err := b.credMgr.ClearLegacyOAuth(ctx, "legacy", cfg.ID, "fallback.example"); err != nil {
		t.Fatal(err)
	}
	c := nativeMCPCandidate{slug: "legacy", url: cfg.URL, authType: AuthOAuth2PKCE, managedConfig: cfg}
	token, ok := a.resolveNativeAuthToken(ctx, c)
	if ok || token != "" {
		t.Fatal("historical OAuth allowed anonymous fallback after deletion")
	}
	if err := b.credMgr.RegisterPatternWithContext(ctx, "fallback.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "replacement"}); err != nil {
		t.Fatal(err)
	}
	token, ok = a.resolveNativeAuthToken(ctx, c)
	if ok || token != "" {
		t.Fatal("historical OAuth adopted a replacement hostname token")
	}
}

func TestLegacyTransportSerializesTokenResolutionWithConfiguration(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "request", true: "cached"}[cached], func(t *testing.T) {
			a, b, ctx, cfg := managedWALManagers(t, "https://example.com")
			seedManagedRuntime(t, a, ctx, "legacy", "valid", "refresh", time.Now().Add(time.Hour))
			store, _, _, err := a.managedOAuth(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var paused atomic.Bool
			db := a.repository().(*DBRepository).db
			if err := db.Callback().Query().After("gorm:query").Register("pause_resolution", func(tx *gorm.DB) {
				if tx.Statement.Table == "credential_entries" && paused.CompareAndSwap(false, true) {
					close(entered)
					<-release
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove("pause_resolution") })
			rt := &managedOAuthTransport{manager: a, cfg: cfg, store: store, ctx: ctx, base: managedTestRoundTrip(func(*http.Request) (*http.Response, error) {
				t.Error("stale configuration sent an authenticated request")
				return nil, errors.New("stale request")
			})}
			done := make(chan error, 1)
			go func() {
				if cached {
					r, err := a.resolveManagedOAuth(ctx, cfg, "")
					if r.Tokens.Access != "" {
						done <- errors.New("stale cached token exposed")
					} else {
						done <- err
					}
					return
				}
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
				resp, err := rt.RoundTrip(req)
				if resp != nil {
					_ = resp.Body.Close()
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("resolution did not pause")
			}
			// Cross-instance edits cannot rely on a transport-local mutex. The
			// resolver must revalidate the durable consumer before exposing a token.
			edited := cfg
			edited.URL = "https://replacement.example/mcp"
			if err := b.SaveConfig("legacy", edited); err != nil {
				t.Fatal(err)
			}
			unblock()
			if err := <-done; !errors.Is(err, oauthflow.ErrConflict) && !errors.Is(err, oauthflow.ErrResource) {
				t.Fatalf("configuration change was not detected: %v", err)
			}
		})
	}
}

func TestLegacyRestartDiscoversRefreshEndpoint(t *testing.T) {
	var endpoint, resource string
	var refreshed, prompts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource + "/mcp", "authorization_servers": []string{endpoint}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token", "device_authorization_endpoint": endpoint + "/device"})
		case "/device":
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "code", "user_code": "user", "verification_uri": endpoint + "/verify", "expires_in": 60, "interval": 1})
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") == "refresh_token" {
				refreshed.Add(1)
			}
			_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"token_type":"Bearer"}`)
		case "/mcp":
			if r.Header.Get("Authorization") != "Bearer fresh" {
				w.WriteHeader(http.StatusUnauthorized)
			}
		case "/verify":
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	endpoint = server.URL
	source := httptest.NewServer(server.Config.Handler)
	defer source.Close()
	resource = source.URL
	a, b, ctx, cfg := managedWALManagers(t, resource)
	a.SetOAuthNetworkAuthorizer(func(_ context.Context, destination oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		return destination.IPs, true, nil
	})
	b.SetOAuthNetworkAuthorizer(func(_ context.Context, destination oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		return destination.IPs, true, nil
	})
	cfg.OAuth2TokenURL, cfg.OAuth2AuthURL = "", ""
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	oldBrowser := browserOpen
	browserOpen = func(string) error { return nil }
	defer func() { browserOpen = oldBrowser }()
	cfg, _, _ = loadManaged(t, a, ctx, "legacy")
	if err := a.authorizeManagedOAuth(ctx, "legacy", cfg); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	cfg, _, registered := loadManaged(t, b, ctx, "legacy")
	if registered.Endpoints.Token != endpoint+"/token" {
		t.Fatal("discovered token endpoint was not preserved across restart")
	}
	client := b.managedHTTPClient(ctx, cfg)
	for i := int32(1); i <= 2; i++ {
		seedManagedRuntime(t, a, ctx, "legacy", "expired", "rotated", time.Now().Add(-time.Hour))
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 200 || refreshed.Load() != i || prompts.Load() != i {
			t.Fatalf("status=%d refreshes=%d approvals=%d (want %d)", resp.StatusCode, refreshed.Load(), prompts.Load(), i)
		}
	}
}

func TestLegacyPublicClientAuthInfoAndPendingRemoval(t *testing.T) {
	a, b, ctx, cfg := legacyWALManagers(t, "https://example.com")
	if err := a.credMgr.DeletePattern(ctx, clientCredPattern("legacy")); err != nil {
		t.Fatal(err)
	}
	if typ, has, err := b.GetServerAuthInfo("legacy"); err != nil || !has || typ != string(AuthOAuth2PKCE) {
		t.Fatalf("public client auth: %s %v %v", typ, has, err)
	}
	seedHistoricalControl(t, a, cfg, time.Now().Add(time.Minute), true)
	if err := b.DeleteServerAuth("legacy"); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("live operation removal: %v", err)
	}
	seedHistoricalControl(t, a, cfg, time.Now().Add(-time.Minute), true)
	if _, has, err := b.GetServerAuthInfo("legacy"); err != nil || !has {
		t.Fatalf("pending grant hidden: %v %v", has, err)
	}
	if err := b.DeleteServerAuth("legacy"); err != nil {
		t.Fatal(err)
	}
	cfg.AuthType = AuthNone
	if err := b.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	if _, has, err := b.GetServerAuthInfo("legacy"); err != nil || has {
		t.Fatalf("removed grant still present: %v %v", has, err)
	}
}

func TestLegacyDetachRollsBackCredentialsWithConfig(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit_none", true: "stdio_implicit_none"}[stdio], func(t *testing.T) { testLegacyDetachRollsBackCredentialsWithConfig(t, stdio) })
	}
}

func testLegacyDetachRollsBackCredentialsWithConfig(t *testing.T, stdio bool) {
	a, _, ctx, cfg := legacyWALManagers(t, "https://example.com")
	control := seedHistoricalControl(t, a, cfg, time.Now().Add(-time.Minute), true)
	db := a.repository().(*DBRepository).db
	if err := db.Callback().Update().Before("gorm:update").Register("reject_detach_config", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*database.MCPServer); ok {
			_ = tx.AddError(errors.New("simulated config failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("reject_detach_config") })
	none := cfg
	none.AuthType = AuthNone
	if stdio {
		// The editor omits auth_type and all HTTP/OAuth fields for stdio.
		none = ServerConfig{ID: cfg.ID, UserID: cfg.UserID, Name: cfg.Name, Transport: TransportStdio, Command: "local-mcp", Enabled: cfg.Enabled, AutoConnect: cfg.AutoConnect}
	}
	if err := a.SaveConfig("legacy", none); err == nil {
		t.Fatal("expected save failure")
	}
	if _, has, err := a.GetServerAuthInfo("legacy"); err != nil || !has {
		t.Fatalf("failed save removed grant: %v %v", has, err)
	}
	var pending database.CredentialEntry
	if err := db.Where("user_id = ? AND pattern = ?", cfg.UserID, userTokensPattern(cfg.Slug)).First(&pending).Error; err != nil || pending.LegacyOAuthControlEnc != control {
		t.Fatalf("pending marker changed: %v", err)
	}
	stored, err := a.GetConfig("legacy")
	if err != nil || stored.AuthType != AuthOAuth2PKCE {
		t.Fatalf("configuration changed: %v", err)
	}
	if err := db.Callback().Update().Remove("reject_detach_config"); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveConfig("legacy", none); err != nil {
		t.Fatal(err)
	}
	if _, has, err := a.GetServerAuthInfo("legacy"); err != nil || has {
		t.Fatalf("successful detach retained grant: %v %v", has, err)
	}
	stored, err = a.repository().GetServer(ctx, "legacy")
	if err != nil || stored.AuthType != AuthNone {
		t.Fatalf("none not committed: %v", err)
	}
}

func TestLegacyManualDiscoveryKeepsPersistedIdentity(t *testing.T) {
	var endpoint string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": endpoint + "/mcp", "authorization_servers": []string{endpoint}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token", "device_authorization_endpoint": endpoint + "/device"})
		case "/device":
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "code", "user_code": "user", "verification_uri": endpoint + "/verify", "expires_in": 60, "interval": 1})
		case "/token":
			_, _ = io.WriteString(w, `{"access_token":"authorized","refresh_token":"fresh-refresh","expires_in":3600,"token_type":"Bearer"}`)
		case "/authorize", "/verify", "/mcp":
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	endpoint = server.URL
	a, _, ctx, cfg := managedWALManagers(t, endpoint)
	cfg.OAuth2TokenURL = ""
	cfg.OAuth2AuthURL = ""
	cfg.OAuth2DeviceAuthURL = ""
	cfg.OAuth2RegistrationURL = ""
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	previousBrowser := browserOpen
	browserOpen = func(string) error { return nil }
	defer func() { browserOpen = previousBrowser }()
	cfg, _, _ = loadManaged(t, a, ctx, "legacy")
	if err := a.authorizeManagedOAuth(ctx, "legacy", cfg); err != nil {
		t.Fatal(err)
	}
	cfg, store, before := loadManaged(t, a, ctx, "legacy")
	if cfg.OAuth2TokenURL != endpoint+"/token" || before.Endpoints.Token != cfg.OAuth2TokenURL {
		t.Fatal("discovery did not persist endpoint")
	}
	if tok, err := a.resolveManagedOAuth(ctx, cfg, ""); err != nil || tok.Tokens.Access != "authorized" {
		t.Fatalf("discovery invalidated identity: %v", err)
	}
	cfg.Name = "edited"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	before.Revision++
	if err := store.CompareAndSwap(ctx, before, before.Revision-1); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("stale writer did not detect real edit: %v", err)
	}
	cfg, _, after := loadManaged(t, a, ctx, "legacy")
	if tok, err := a.resolveManagedOAuth(ctx, cfg, ""); err != nil || tok.Tokens != after.Tokens || tok.Tokens.Access != "authorized" {
		t.Fatalf("unrelated name edit invalidated grant: %v", err)
	}
}

func TestLegacyDetachLatePublicationPreservesNewEdit(t *testing.T) {
	a, _, _, original := legacyWALManagers(t, "https://example.com")
	detached := original
	detached.AuthType = AuthNone
	if err := a.SaveConfig("legacy", detached); err != nil {
		t.Fatal(err)
	}
	newer, err := a.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	newer.Name = "newer edit"
	newer.URL = "https://new.example.com/mcp"
	if err := a.SaveConfig("legacy", *newer); err != nil {
		t.Fatal(err)
	}
	// Reproduce a detach callback delayed until after another SaveConfig
	// committed and published; it must not roll the cache back.
	a.publishLegacyDetach(original, detached, false)
	actual, err := a.GetConfig("legacy")
	if err != nil || actual.Name != newer.Name || actual.URL != newer.URL {
		t.Fatalf("new edit lost: %v", err)
	}
}

func TestLegacyDeleteServerAllowsInactivePendingAndRollsBack(t *testing.T) {
	a, _, ctx, cfg := legacyWALManagers(t, "https://example.com")
	db := a.repository().(*DBRepository).db
	if err := db.AutoMigrate(&database.ToolCatalog{}); err != nil {
		t.Fatal(err)
	}
	seedHistoricalControl(t, a, cfg, time.Now().Add(time.Minute), true)
	if err := a.DeleteConfig("legacy"); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("active delete: %v", err)
	}
	seedHistoricalControl(t, a, cfg, time.Now().Add(-time.Minute), true)
	if err := db.Callback().Delete().Before("gorm:delete").Register("reject_server_delete", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*database.MCPServer); ok {
			_ = tx.AddError(errors.New("simulated delete failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Delete().Remove("reject_server_delete") })
	if err := a.DeleteConfig("legacy"); err == nil {
		t.Fatal("delete must fail")
	}
	if _, has, err := a.GetServerAuthInfo("legacy"); err != nil || !has {
		t.Fatalf("rollback lost grant: %v %v", has, err)
	}
	if err := db.Callback().Delete().Remove("reject_server_delete"); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteConfig("legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.repository().GetServer(ctx, "legacy"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("consumer remains: %v", err)
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("user_id = ?", cfg.UserID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("credentials remain: %d %v", count, err)
	}
	a.mu.RLock()
	cached := a.servers["legacy"]
	a.mu.RUnlock()
	if cached != nil {
		t.Fatal("deleted consumer remains cached")
	}
}

func TestLegacyDeleteServerClearsAlreadyStaleCache(t *testing.T) {
	a, b, ctx, cfg := legacyWALManagers(t, "https://example.com")
	db := a.repository().(*DBRepository).db
	if err := db.AutoMigrate(&database.ToolCatalog{}); err != nil {
		t.Fatal(err)
	}
	cfg.Name = "edited elsewhere"
	if err := b.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	a.mu.RLock()
	stale := a.servers["legacy"].Config.Name
	a.mu.RUnlock()
	if stale == cfg.Name {
		t.Fatal("fixture cache should lag DB")
	}
	if err := a.DeleteConfig("legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.repository().GetServer(ctx, "legacy"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DB record remains: %v", err)
	}
	a.mu.RLock()
	cached := a.servers["legacy"]
	a.mu.RUnlock()
	if cached != nil {
		t.Fatal("deleted server remains in stale cache")
	}
}

func legacyWALManagers(t *testing.T, endpoint string) (*Manager, *Manager, context.Context, ServerConfig) {
	return oauthWALManagers(t, endpoint, false)
}

// Historical data fixture, encrypted with the WAL fixture's key. No operational
// legacy lease is created: only migration/recovery may consume this old format.
func seedHistoricalControl(t *testing.T, m *Manager, cfg ServerConfig, until time.Time, pending bool) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"Version": 1, "ConsumerID": cfg.ID, "Attempt": "historical-attempt", "Until": until, "Pending": pending})
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	enc := base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, body, nil))
	db := m.repository().(*DBRepository).db
	result := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", cfg.UserID, userTokensPattern(cfg.Slug)).Update("legacy_oauth_control_enc", enc)
	if result.Error == nil && result.RowsAffected == 0 {
		result = db.Create(&database.CredentialEntry{UserID: cfg.UserID, Pattern: userTokensPattern(cfg.Slug), Source: "static", AuthType: "oauth2", LegacyOAuthControlEnc: enc})
	}
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("historical control fixture: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	return enc
}

func managedWALManagers(t *testing.T, endpoint string) (*Manager, *Manager, context.Context, ServerConfig) {
	return oauthWALManagers(t, endpoint, true)
}

func oauthWALManagers(t *testing.T, endpoint string, managed bool) (*Manager, *Manager, context.Context, ServerConfig) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "legacy.db") + "?_pragma=busy_timeout(100)&_pragma=journal_mode(WAL)"
	previous := database.DB()
	t.Cleanup(func() { database.SetDB(previous) })
	ctx := database.WithUserID(context.Background(), "owner")
	newManager := func() *Manager {
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		pool.SetMaxOpenConns(4)
		t.Cleanup(func() { _ = pool.Close() })
		if err := db.AutoMigrate(&database.CredentialEntry{}, &database.MCPServer{}, &database.MCPServerLog{}); err != nil {
			t.Fatal(err)
		}
		database.SetDB(db)
		m := NewManager(tools.NewRegistry(), credentials.NewManagerWithStore(bytes.Repeat([]byte{4}, 32), credentials.NewDBStore(), true), func(string, any) {})
		m.SetRepository(NewDBRepository(db))
		m.SetAuthContextProvider(func() context.Context { return ctx })
		t.Cleanup(m.CloseAll)
		return m
	}
	a, b := newManager(), newManager()
	cfg := managedConfig(endpoint + "/mcp")
	cfg.OAuthManaged = managed
	if managed {
		cfg.OAuth2TokenAuthMethod = "none"
	}
	cfg.OAuth2TokenURL = endpoint + "/token"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	stored, err := a.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if managed {
		seedManagedRuntime(t, a, ctx, "legacy", "old", "old-refresh", time.Now().Add(-time.Hour))
		if err := b.LoadConfigs(); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := a.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "old", RefreshURL: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour).Unix()}); err != nil {
			t.Fatal(err)
		}
	}
	return a, b, ctx, *stored
}

func TestLegacyRefreshCoordinatesProcessesAndEdits(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer func() { unblock(); server.Close() }()
	a, b, ctx, cfg := managedWALManagers(t, server.URL)
	done := make(chan error, 1)
	go func() { _, err := a.resolveManagedOAuth(ctx, cfg, ""); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	waiting, cancelWaiting := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelWaiting()
	if _, err := b.resolveManagedOAuth(waiting, cfg, ""); !errors.Is(err, context.DeadlineExceeded) || requests.Load() != 1 {
		t.Fatalf("parallel refresh: %v", err)
	}
	edited := cfg
	edited.Name = "changed"
	if err := a.SaveConfig("legacy", edited); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("parallel config edit: %v", err)
	}
	if err := a.DeleteServerAuth("legacy"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("parallel deletion: %v", err)
	}
	_, _, cache := loadManaged(t, a, ctx, "legacy")
	if cache.Tokens.Access != "old" || !cache.RefreshPending {
		t.Fatal("failed deletion altered cache")
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	token, err := b.resolveManagedOAuth(ctx, cfg, "")
	if err != nil || token.Tokens.Access != "fresh" || token.Tokens.Refresh != "rotated" || requests.Load() != 1 {
		t.Fatalf("second instance did not adopt rotation: requests=%d err=%v", requests.Load(), err)
	}
	if _, err := b.resolveManagedOAuth(ctx, cfg, "old"); err != nil || requests.Load() != 1 {
		t.Fatalf("already replaced rejection refreshed twice: %v", err)
	}
}

func TestLegacyTransportRejectsChangedResourceWithValidToken(t *testing.T) {
	a, _, ctx, cfg := managedWALManagers(t, "https://original.example")
	seedManagedRuntime(t, a, ctx, "legacy", "valid", "refresh", time.Now().Add(time.Hour))
	client := a.managedHTTPClient(ctx, cfg)
	edited := cfg
	edited.URL = "https://replacement.example"
	if err := a.SaveConfig("legacy", edited); err != nil {
		t.Fatal(err)
	}
	if err := clientGrantGet(client, cfg.URL); !errors.Is(err, oauthflow.ErrConflict) && !errors.Is(err, oauthflow.ErrResource) {
		t.Fatalf("old transport resolved replacement token: %v", err)
	}
}

func TestManagedRefreshUsesExplicitClientMethodWithoutAmbiguousReplay(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(map[bool]string{false: "post_only", true: "ambiguous"}[ambiguous], func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if ambiguous {
					w.WriteHeader(500)
					_, _ = io.WriteString(w, `{"error":"server_error"}`)
					return
				}
				if r.Header.Get("Authorization") != "" {
					w.WriteHeader(401)
					_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
					return
				}
				_ = r.ParseForm()
				if r.Form.Get("client_id") != "client" || r.Form.Get("client_secret") != "latest-secret" {
					t.Error("not using current client")
				}
				_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"token_type":"Bearer"}`)
			}))
			defer server.Close()
			a, _, ctx, cfg := managedWALManagers(t, server.URL)
			cfg.OAuth2TokenAuthMethod = "client_secret_post"
			if err := a.SaveConfigWithOAuthSecret("legacy", cfg, "latest-secret"); err != nil {
				t.Fatal(err)
			}
			seedManagedRuntime(t, a, ctx, "legacy", "old", "old-refresh", time.Now().Add(-time.Hour))
			cfg, _, _ = loadManaged(t, a, ctx, "legacy")
			_, err := a.resolveManagedOAuth(ctx, cfg, "")
			if ambiguous {
				if !errors.Is(err, oauthflow.ErrReauthorize) || requests.Load() != 1 {
					t.Fatalf("ambiguous refresh replayed: %d %v", requests.Load(), err)
				}
				if _, err := a.resolveManagedOAuth(ctx, cfg, ""); !errors.Is(err, oauthflow.ErrReauthorize) || requests.Load() != 1 {
					t.Fatalf("uncertain grant reused: %v", err)
				}
			} else if err != nil || requests.Load() != 1 {
				t.Fatalf("explicit Post authentication failed: %d %v", requests.Load(), err)
			}
		})
	}
}

func TestLegacyDCRDoesNotPublishConfigWhenClientSaveFails(t *testing.T) {
	testManagedDCRCheckpointRollback(t, false)
}

func TestLegacyDCRRollsBackClientWhenConfigSaveFails(t *testing.T) {
	// Client and callback now share one encrypted envelope. A failure after SQL
	// updates that envelope must roll back both, just as the old pair/config did.
	testManagedDCRCheckpointRollback(t, true)
}

func testManagedDCRCheckpointRollback(t *testing.T, afterWrite bool) {
	var registrations, browsers atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		registrations.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"client_id":"registered","client_secret":"unsolicited-secret"}`)
	}))
	defer server.Close()
	a, b, ctx, cfg := managedWALManagers(t, server.URL)
	cfg.OAuth2ClientID, cfg.OAuth2CallbackPort = "", 0
	cfg.OAuth2CallbackHost = "127.0.0.1"
	cfg.OAuth2RegistrationURL = server.URL + "/register"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	cfg, _, before := loadManaged(t, a, ctx, "legacy")
	db := a.repository().(*DBRepository).db
	injected := errors.New("simulated registration persistence failure")
	var failed atomic.Bool
	reject := func(tx *gorm.DB) {
		if fields, ok := tx.Statement.Dest.(map[string]any); ok && fields["oauth_enc"] != nil && registrations.Load() > 0 && failed.CompareAndSwap(false, true) {
			_ = tx.AddError(injected)
		}
	}
	var registerErr error
	if afterWrite {
		registerErr = db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("reject_dcr_checkpoint", reject)
	} else {
		registerErr = db.Callback().Update().Before("gorm:update").Register("reject_dcr_checkpoint", reject)
	}
	if registerErr != nil {
		t.Fatal(registerErr)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("reject_dcr_checkpoint") })
	previousBrowser := browserOpen
	browserOpen = func(raw string) error {
		browsers.Add(1)
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		response, err := http.Get(q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&error=access_denied")
		if response != nil {
			_ = response.Body.Close()
		}
		return err
	}
	defer func() { browserOpen = previousBrowser }()
	if err := a.authorizeManagedOAuth(ctx, "legacy", cfg); !errors.Is(err, injected) {
		t.Fatalf("registration did not stop on persistence failure: %v", err)
	}
	if !failed.Load() || registrations.Load() != 1 || browsers.Load() != 0 {
		t.Fatalf("continued after failed checkpoint: failed=%v registrations=%d browsers=%d", failed.Load(), registrations.Load(), browsers.Load())
	}
	if err := b.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	stored, _, rolledBack := loadManaged(t, b, ctx, "legacy")
	if stored.OAuth2ClientID != "" || stored.OAuth2CallbackPort != 0 || rolledBack.Client.ID != "" || rolledBack.PendingRegistration != nil || rolledBack.Tokens != before.Tokens {
		t.Fatal("partial DCR client/callback/token published after rollback")
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("pattern IN ?", []string{clientCredPattern("legacy"), userTokensPattern("legacy")}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("DCR recreated historical pair", err)
	}
	if client, err := a.credMgr.GetByPatternWithContext(ctx, clientCredPattern("legacy")); err != nil || client != nil {
		t.Fatal("client cache published outside shared commit", err)
	}
	if err := db.Callback().Update().Remove("reject_dcr_checkpoint"); err != nil {
		t.Fatal(err)
	}
	// A restarted instance must register again after the failed checkpoint, then
	// preserve the entire candidate even though this explicit consent is denied.
	if err := b.authorizeManagedOAuth(ctx, "legacy", stored); err == nil {
		t.Fatal("denied consent unexpectedly succeeded")
	}
	_, _, recovered := loadManaged(t, a, ctx, "legacy")
	pending := recovered.PendingRegistration
	if registrations.Load() != 2 || browsers.Load() != 1 || pending == nil || pending.Client.ID != "registered" || pending.Client.Secret != "" || pending.Callback.Port == 0 || pending.Callback.PortPolicy != "fixed" || recovered.Tokens != before.Tokens {
		t.Fatal("successful checkpoint did not publish client and callback together")
	}
	if err := b.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	_, _, reloaded := loadManaged(t, b, ctx, "legacy")
	if reloaded.PendingRegistration == nil || reloaded.PendingRegistration.Callback != pending.Callback || reloaded.PendingRegistration.Client != pending.Client {
		t.Fatal("durable candidate changed after restart")
	}
}

func TestLegacyNativeRefusesHostnameWithOrWithoutTokenRow(t *testing.T) {
	a, _, ctx, cfg := legacyWALManagers(t, "https://example.com")
	candidate := nativeMCPCandidate{managedConfig: cfg, slug: "legacy", url: cfg.URL, authType: AuthOAuth2PKCE}
	if err := a.credMgr.RegisterPatternWithContext(ctx, "example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "host-token"}); err != nil {
		t.Fatal(err)
	}
	seedHistoricalControl(t, a, cfg, time.Now().Add(-time.Minute), true)
	if _, ok := a.resolveNativeAuthToken(ctx, candidate); ok {
		t.Fatal("hostname bypassed uncertain grant")
	}
	if err := a.credMgr.ClearLegacyOAuth(ctx, "legacy", cfg.ID, ""); err != nil {
		t.Fatal(err)
	}
	token, ok := a.resolveNativeAuthToken(ctx, candidate)
	if ok || token != "" {
		t.Fatalf("historical OAuth adopted hostname without grant: ok=%v", ok)
	}
	if err := a.credMgr.DeletePattern(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if token, ok := a.resolveNativeAuthToken(ctx, candidate); ok || token != "" {
		t.Fatal("historical OAuth allowed anonymous fallback")
	}
}

func TestLegacyRefreshConsentPrecedesDurableAttempt(t *testing.T) {
	a, _, ctx, cfg := managedWALManagers(t, "https://192.0.2.1")
	cfg.OAuth2TokenURL = "http://127.0.0.1:12345/token"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	seedManagedRuntime(t, a, ctx, "legacy", "old", "old-refresh", time.Now().Add(-time.Hour))
	cfg, _, before := loadManaged(t, a, ctx, "legacy")
	prompts := 0
	a.SetOAuthNetworkAuthorizer(func(ctx context.Context, destination oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts++
		if _, bounded := ctx.Deadline(); bounded {
			t.Error("human consent inherited the DNS deadline")
		}
		_, _, current := loadManaged(t, a, ctx, "legacy")
		if current.RefreshPending || current.Revision != before.Revision {
			t.Error("lease acquired before user decision")
		}
		return nil, false, nil
	})
	if _, err := a.resolveManagedOAuth(ctx, cfg, ""); !errors.Is(err, oauthflow.ErrNetworkAuthorization) || prompts != 1 {
		t.Fatalf("denial: prompts=%d err=%v", prompts, err)
	}
	_, _, after := loadManaged(t, a, ctx, "legacy")
	if after.RefreshPending || after.Revision != before.Revision || after.Tokens != before.Tokens {
		t.Fatal("denial marked grant uncertain")
	}
}

func TestManagedNativeRefreshUsesFreshClientWithoutBootstrapOverwrite(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		id, secret, _ := r.BasicAuth()
		if id != "client" || secret != "current-secret" {
			t.Error("lost client secret through stale cache")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer server.Close()
	// Both managers use the same durable vault, but the second retains the
	// projected configuration from before the first manager replaces the secret.
	a, b, ctx, _ := legacyWALManagers(t, server.URL)
	cfg := managedConfig(server.URL)
	cfg.OAuth2TokenAuthMethod = "client_secret_basic"
	if err := a.SaveConfigWithOAuthSecret("composed", cfg, "old-secret"); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	captured, _, _ := loadManaged(t, b, ctx, "composed")
	if err := a.SaveConfigWithOAuthSecret("composed", captured, "current-secret"); err != nil {
		t.Fatal(err)
	}
	_, store, record := loadManaged(t, a, ctx, "composed")
	record.State = "connected"
	record.Tokens = oauthflow.Tokens{Access: "near-expiry", Refresh: "refresh", Type: "Bearer", ExpiresAt: time.Now().Add(time.Minute)}
	record.Revision++
	if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	candidate := nativeMCPCandidate{managedConfig: captured, slug: "composed", authType: AuthOAuth2PKCE}
	token, ok := b.resolveNativeAuthToken(ctx, candidate)
	if !ok || token != "fresh" || requests.Load() != 1 {
		t.Fatalf("native refresh: ok=%v requests=%d", ok, requests.Load())
	}
	_, store, record = loadManaged(t, a, ctx, "composed")
	if record.Client.Secret != "current-secret" || record.Tokens.Refresh != "rotated" {
		t.Fatal("refresh overwrote the current client or lost rotation")
	}
	record.RefreshPending = true
	record.Revision++
	if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	if token, ok := b.resolveNativeAuthToken(ctx, candidate); ok || token != "" || requests.Load() != 1 {
		t.Fatal("native reused uncertain grant")
	}
}
