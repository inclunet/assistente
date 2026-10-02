package mcp

import (
	"bytes"
	"context"
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
