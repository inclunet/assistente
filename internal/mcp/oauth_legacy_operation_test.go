package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type pausedLegacySource struct {
	inner   oauth2.TokenSource
	entered chan struct{}
	release chan struct{}
}

func TestLegacyProactiveAdoptsConcurrentRotation(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "proactive", true: "forced"}[force], func(t *testing.T) {
			a, b, ctx, _ := legacyWALManagers(t, "https://unused.example")
			db := a.repository().(*DBRepository).db
			var rotated atomic.Bool
			if err := db.Callback().Query().After("gorm:query").Register("rotate_before_resolution", func(tx *gorm.DB) {
				row, ok := tx.Statement.Dest.(*database.CredentialEntry)
				if !ok || row.Pattern != userTokensPattern("legacy") || !rotated.CompareAndSwap(false, true) {
					return
				}
				if err := b.credMgr.RegisterPatternWithContext(ctx, row.Pattern, &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "concurrent", RefreshURL: "rotated", ExpiresAt: time.Now().Add(time.Hour).Unix()}); err != nil {
					t.Error(err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove("rotate_before_resolution") })
			refreshed, err := a.refreshOAuthTokenBestEffort(ctx, "legacy", force)
			if err != nil || !refreshed || !rotated.Load() {
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
	if !ok || token != "" {
		t.Fatal("fallback reused deleted cache")
	}
	if err := b.credMgr.RegisterPatternWithContext(ctx, "fallback.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "replacement"}); err != nil {
		t.Fatal(err)
	}
	token, ok = a.resolveNativeAuthToken(ctx, c)
	if !ok || token != "replacement" {
		t.Fatal("fallback did not read replacement")
	}
}

func (s *pausedLegacySource) Token() (*oauth2.Token, error) {
	close(s.entered)
	<-s.release
	return s.inner.Token()
}

func TestLegacyTransportSerializesTokenResolutionWithConfiguration(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "request", true: "cached"}[cached], func(t *testing.T) {
			a, _, ctx, cfg := legacyWALManagers(t, "https://example.com")
			if err := a.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "valid", ExpiresAt: time.Now().Add(time.Hour).Unix()}); err != nil {
				t.Fatal(err)
			}
			rt := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
			paused := &pausedLegacySource{inner: rt.tokenSource, entered: make(chan struct{}), release: make(chan struct{})}
			rt.tokenSource = paused
			rt.base = managedTestRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Authorization") != "Bearer valid" {
					return nil, errors.New("missing token")
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			})
			done := make(chan error, 1)
			go func() {
				if cached {
					if rt.cachedAccessToken() != "valid" {
						done <- errors.New("missing cached token")
						return
					}
					done <- nil
					return
				}
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
				resp, err := rt.RoundTrip(req)
				if resp != nil {
					_ = resp.Body.Close()
				}
				done <- err
			}()
			<-paused.entered
			// Discovery/DCR uses this same lock to mutate the shared consumer.
			// The old implementation released it before calling Token.
			locked := !rt.mu.TryLock()
			if !locked {
				rt.mu.Unlock()
			}
			close(paused.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if !locked {
				t.Fatal("configuration could change while resolving the authoritative token")
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
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token"})
		case "/token":
			refreshed.Add(1)
			_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600,"token_type":"Bearer"}`)
		case "/mcp":
			if r.Header.Get("Authorization") != "Bearer fresh" {
				w.WriteHeader(http.StatusUnauthorized)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	endpoint = server.URL
	source := httptest.NewServer(server.Config.Handler)
	defer source.Close()
	resource = source.URL
	a, b, ctx, cfg := legacyWALManagers(t, resource)
	b.SetOAuthNetworkAuthorizer(func(_ context.Context, destination oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		return destination.IPs, true, nil
	})
	cfg.OAuth2TokenURL = ""
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	rt := b.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	for i := int32(1); i <= 2; i++ {
		if i > 1 {
			if err := a.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "expired", RefreshURL: "rotated", ExpiresAt: time.Now().Add(-time.Hour).Unix()}); err != nil {
				t.Fatal(err)
			}
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
		resp, err := rt.RoundTrip(req)
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
	op, _, err := a.credMgr.BeginLegacyOAuth(ctx, "legacy", cfg.ID, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteServerAuth("legacy"); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("live operation removal: %v", err)
	}
	op.End()
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
	op, _, err := a.credMgr.BeginLegacyOAuth(ctx, "legacy", cfg.ID, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	op.End()
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
	if _, err := a.credMgr.ReadLegacyOAuthToken(ctx, "legacy", cfg.ID, nil); !errors.Is(err, oauthflow.ErrReauthorize) {
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
	a, _, ctx, cfg := legacyWALManagers(t, endpoint)
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
	rt := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	rt.explicitAuthorization = true
	if err := rt.authorize(ctx); err != nil {
		t.Fatal(err)
	}
	if rt.cfg.OAuth2TokenURL == "" {
		t.Fatal("discovery did not enrich runtime")
	}
	if tok, _, err := rt.currentToken(); err != nil || tok.AccessToken != "authorized" {
		t.Fatalf("discovery invalidated identity: %v", err)
	}
	cfg.Name = "edited"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rt.currentToken(); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("real edit not detected: %v", err)
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
	op, _, err := a.credMgr.BeginLegacyOAuth(ctx, "legacy", cfg.ID, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteConfig("legacy"); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("active delete: %v", err)
	}
	op.End()
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
	cfg.OAuthManaged = false
	cfg.OAuth2TokenURL = endpoint + "/token"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	stored, err := a.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "old", RefreshURL: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour).Unix()}); err != nil {
		t.Fatal(err)
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
	a, b, ctx, cfg := legacyWALManagers(t, server.URL)
	rtA := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	rtB := b.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	done := make(chan error, 1)
	go func() { _, err := rtA.tokenSource.Token(); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	if _, err := rtB.tokenSource.Token(); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("parallel refresh: %v", err)
	}
	edited := cfg
	edited.Name = "changed"
	if err := a.SaveConfig("legacy", edited); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("parallel config edit: %v", err)
	}
	if err := a.DeleteServerAuth("legacy"); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatalf("parallel deletion: %v", err)
	}
	cache, err := a.credMgr.GetByPatternWithContext(ctx, userTokensPattern("legacy"))
	if err != nil || cache == nil || cache.Token != "old" {
		t.Fatal("failed deletion altered cache")
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	token, err := rtB.tokenSource.Token()
	if err != nil || token.AccessToken != "fresh" || token.RefreshToken != "rotated" || requests.Load() != 1 {
		t.Fatalf("second instance did not adopt rotation: requests=%d err=%v", requests.Load(), err)
	}
	if err := rtB.trySilentRefresh(ctx, "old"); err != nil || requests.Load() != 1 {
		t.Fatalf("already replaced rejection refreshed twice: %v", err)
	}
}

func TestLegacyTransportRejectsChangedResourceWithValidToken(t *testing.T) {
	a, _, ctx, cfg := legacyWALManagers(t, "https://original.example")
	if err := a.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "valid", RefreshURL: "refresh", ExpiresAt: time.Now().Add(time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	rt := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	edited := cfg
	edited.URL = "https://replacement.example"
	if err := a.SaveConfig("legacy", edited); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.tokenSource.Token(); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("old transport resolved replacement token: %v", err)
	}
}

func TestLegacyRefreshNegotiatesOnlyDefinitiveClientRejection(t *testing.T) {
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
				_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600}`)
			}))
			defer server.Close()
			a, _, ctx, cfg := legacyWALManagers(t, server.URL)
			rt := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
			if err := a.SaveServerAuth("legacy", "oauth2_pkce", "", "", "", "latest-secret"); err != nil {
				t.Fatal(err)
			}
			_, err := rt.tokenSource.Token()
			if ambiguous {
				if !errors.Is(err, oauthflow.ErrReauthorize) || requests.Load() != 1 {
					t.Fatalf("ambiguous refresh replayed: %d %v", requests.Load(), err)
				}
				if _, err := rt.tokenSource.Token(); !errors.Is(err, oauthflow.ErrReauthorize) || requests.Load() != 1 {
					t.Fatalf("uncertain grant reused: %v", err)
				}
			} else if err != nil || requests.Load() != 2 {
				t.Fatalf("Post negotiation failed: %d %v", requests.Load(), err)
			}
		})
	}
}

var _ oauth2.TokenSource = (*legacyTokenSource)(nil)

func TestLegacyDCRDoesNotPublishConfigWhenClientSaveFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"client_id":"registered","client_secret":"secret"}`)
	}))
	defer server.Close()
	a, _, ctx, cfg := legacyWALManagers(t, server.URL)
	cfg.OAuth2ClientID = ""
	cfg.OAuth2CallbackPort = 0
	cfg.OAuth2RegistrationURL = server.URL + "/register"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	db := a.repository().(*DBRepository).db
	if err := db.Callback().Create().Before("gorm:create").Register("reject_dcr_client", func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*database.CredentialEntry); ok && row.Pattern == clientCredPattern("legacy") {
			_ = tx.AddError(errors.New("simulated secret persistence failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove("reject_dcr_client") })
	rt := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	op, _, err := a.credMgr.BeginLegacyOAuth(ctx, "legacy", cfg.ID, true, false, rt.validateLegacyConsumer)
	if err != nil {
		t.Fatal(err)
	}
	defer op.End()
	rt.legacyOperation = op
	defer rt.closeCallback()
	callbacks := 0
	write := rt.onConfigUpdate
	rt.onConfigUpdate = func(updated ServerConfig) { callbacks++; write(updated) }
	if err := rt.registerClient(ctx, true); !errors.Is(err, errOAuthPersistence) {
		t.Fatalf("registration did not stop: %v", err)
	}
	stored, err := a.GetConfig("legacy")
	if err != nil || stored.OAuth2ClientID != "" || stored.OAuth2CallbackPort != 0 || callbacks != 0 || rt.effectiveClientID() != "" {
		t.Fatal("partial DCR configuration published")
	}
	op.End()
	restarted := a.buildPKCERoundTripperForServer(ctx, "legacy", *stored)
	if restarted.effectiveClientID() != "" {
		t.Fatal("next transport would skip DCR after failed client save")
	}
}

func TestLegacyDCRRollsBackClientWhenConfigSaveFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"client_id":"registered","client_secret":"secret"}`)
	}))
	defer server.Close()
	a, b, ctx, cfg := legacyWALManagers(t, server.URL)
	cfg.OAuth2ClientID = ""
	cfg.OAuth2CallbackPort = 0
	cfg.OAuth2RegistrationURL = server.URL + "/register"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	db := a.repository().(*DBRepository).db
	if err := db.Callback().Update().Before("gorm:update").Register("reject_dcr_config", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*database.MCPServer); ok {
			_ = tx.AddError(errors.New("simulated config persistence failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("reject_dcr_config") })
	rt := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	op, _, err := a.credMgr.BeginLegacyOAuth(ctx, "legacy", cfg.ID, true, false, rt.validateLegacyConsumer)
	if err != nil {
		t.Fatal(err)
	}
	defer op.End()
	rt.legacyOperation = op
	defer rt.closeCallback()
	if err := rt.registerClient(ctx, true); !errors.Is(err, errOAuthPersistence) {
		t.Fatalf("registration did not stop: %v", err)
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", clientCredPattern("legacy")).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("client insert was not rolled back")
	}
	if auth, _ := a.credMgr.GetByPatternWithContext(ctx, clientCredPattern("legacy")); auth != nil {
		t.Fatal("client cache published before commit")
	}
	stored, err := b.repository().GetServer(ctx, "legacy")
	if err != nil || stored.OAuth2ClientID != "" || stored.OAuth2CallbackPort != 0 || rt.effectiveClientID() != "" {
		t.Fatal("failed registration changed consumer")
	}
	if err := db.Callback().Update().Remove("reject_dcr_config"); err != nil {
		t.Fatal(err)
	}
	if err := rt.registerClient(ctx, true); err != nil {
		t.Fatal(err)
	}
	stored, err = b.repository().GetServer(ctx, "legacy")
	if err != nil || stored.OAuth2ClientID != "registered" || stored.OAuth2CallbackPort == 0 {
		t.Fatal("successful registration did not publish callback")
	}
	client, err := a.credMgr.GetByPatternWithContext(ctx, clientCredPattern("legacy"))
	if err != nil || client == nil || client.ClientID != "registered" || client.ClientSecret != "secret" {
		t.Fatal("client and config not published together")
	}
}

func TestLegacyNativeHostnameFallbackRequiresAbsentTokenRow(t *testing.T) {
	a, _, ctx, cfg := legacyWALManagers(t, "https://example.com")
	candidate := nativeMCPCandidate{managedConfig: cfg, slug: "legacy", url: cfg.URL, authType: AuthOAuth2PKCE}
	if err := a.credMgr.RegisterPatternWithContext(ctx, "example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "host-token"}); err != nil {
		t.Fatal(err)
	}
	op, _, err := a.credMgr.BeginLegacyOAuth(ctx, "legacy", cfg.ID, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	op.End()
	if _, ok := a.resolveNativeAuthToken(ctx, candidate); ok {
		t.Fatal("hostname bypassed uncertain grant")
	}
	if err := a.credMgr.ClearLegacyOAuth(ctx, "legacy", cfg.ID, ""); err != nil {
		t.Fatal(err)
	}
	token, ok := a.resolveNativeAuthToken(ctx, candidate)
	if !ok || token != "host-token" {
		t.Fatalf("missing hostname fallback: ok=%v", ok)
	}
	if err := a.credMgr.DeletePattern(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if token, ok := a.resolveNativeAuthToken(ctx, candidate); !ok || token != "" {
		t.Fatal("missing anonymous fallback")
	}
}

func TestLegacyRefreshConsentPrecedesDurableAttempt(t *testing.T) {
	a, _, ctx, cfg := legacyWALManagers(t, "https://192.0.2.1")
	cfg.OAuth2TokenURL = "http://127.0.0.1:12345/token"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	rt := a.buildPKCERoundTripperForServer(ctx, "legacy", cfg)
	prompts := 0
	rt.networkAuthorizer = func(ctx context.Context, destination oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts++
		if _, bounded := ctx.Deadline(); bounded {
			t.Error("human consent inherited the DNS deadline")
		}
		var row database.CredentialEntry
		if err := database.DB().Where("pattern = ?", userTokensPattern("legacy")).First(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.LegacyOAuthControlEnc != "" {
			t.Error("lease acquired before user decision")
		}
		return nil, false, nil
	}
	if _, err := rt.tokenSource.Token(); !errors.Is(err, oauthflow.ErrNetworkAuthorization) || prompts != 1 {
		t.Fatalf("denial: prompts=%d err=%v", prompts, err)
	}
	if _, err := a.credMgr.ReadLegacyOAuthToken(ctx, "legacy", cfg.ID); err != nil {
		t.Fatalf("denial marked grant uncertain: %v", err)
	}
}

func TestLegacyNativeRefreshUsesFreshClientWithoutBootstrapOverwrite(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		id, secret, _ := r.BasicAuth()
		if id != "client" || secret != "current-secret" {
			t.Error("lost client secret through stale cache")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","expires_in":3600}`)
	}))
	defer server.Close()
	a, b, ctx, cfg := legacyWALManagers(t, server.URL)
	if err := a.SaveServerAuth("legacy", "oauth2_pkce", "", "", "", "current-secret"); err != nil {
		t.Fatal(err)
	}
	if err := a.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "near-expiry", RefreshURL: "refresh", ExpiresAt: time.Now().Add(time.Minute).Unix()}); err != nil {
		t.Fatal(err)
	}
	token, ok := b.resolveNativeAuthToken(ctx, nativeMCPCandidate{managedConfig: cfg, slug: "legacy", authType: AuthOAuth2PKCE})
	if !ok || token != "fresh" || requests.Load() != 1 {
		t.Fatalf("native refresh: ok=%v requests=%d", ok, requests.Load())
	}
	op, _, err := a.credMgr.BeginLegacyOAuth(ctx, "legacy", cfg.ID, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	op.End()
	if _, ok := b.resolveNativeAuthToken(ctx, nativeMCPCandidate{managedConfig: cfg, slug: "legacy", authType: AuthOAuth2PKCE}); ok || requests.Load() != 1 {
		t.Fatal("native reused uncertain grant")
	}
}
