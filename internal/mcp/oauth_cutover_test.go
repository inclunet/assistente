package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Runtime regressions seed the current composed format directly. Migration
// itself is covered by snapshot/conversion tests and never happens implicitly.
func seedManagedRuntime(t *testing.T, m *Manager, ctx context.Context, slug, access, refresh string, expiry time.Time) {
	t.Helper()
	current, err := m.GetConfig(slug)
	if err != nil {
		t.Fatal(err)
	}
	cfg := *current
	cfg.OAuthManaged = true
	if cfg.OAuth2TokenAuthMethod == "" {
		cfg.OAuth2TokenAuthMethod = "none"
	}
	if err := m.SaveConfig(slug, cfg); err != nil {
		t.Fatal(err)
	}
	if access == "" {
		return
	}
	_, store, r := loadManaged(t, m, ctx, slug)
	r.State = "connected"
	r.Tokens = oauthflow.Tokens{Access: access, Refresh: refresh, Type: "Bearer", ExpiresAt: expiry}
	r.Revision++
	if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyOAuthRuntimeRequiresExplicitMigration(t *testing.T) {
	for _, authType := range []AuthType{AuthOAuth2PKCE, AuthOAuth2ClientCredentials} {
		t.Run(string(authType), func(t *testing.T) {
			var requests, browsers atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusUnauthorized) }))
			defer srv.Close()
			m, _, ctx, cfg := legacyWALManagers(t, srv.URL)
			m.snapshotRoot = t.TempDir()
			cfg.AuthType, cfg.AutoConnect = authType, true
			if err := m.SaveConfig("legacy", cfg); err != nil {
				t.Fatal(err)
			}
			before, err := m.GetConfig("legacy")
			if err != nil {
				t.Fatal(err)
			}
			db := m.repository().(*DBRepository).db
			var rowsBefore, rowsAfter []database.CredentialEntry
			if err := db.Order("id").Find(&rowsBefore).Error; err != nil {
				t.Fatal(err)
			}
			oldBrowser := browserOpen
			browserOpen = func(string) error { browsers.Add(1); return errors.New("unexpected browser") }
			defer func() { browserOpen = oldBrowser }()
			for _, operation := range []func() error{
				func() error { return m.Connect("legacy") },
				func() error { return m.connectWithContext(ctx, "legacy") },
				func() error { return m.Reconnect("legacy") },
				func() error { return m.RecoverServerBestEffort(ctx, "legacy").Err },
			} {
				if err := operation(); !errors.Is(err, errOAuthMigrationRequired) {
					t.Fatalf("migration not required: %v", err)
				}
			}
			if authType == AuthOAuth2PKCE {
				if err := m.ReauthorizeServer(ctx, "legacy"); !errors.Is(err, errOAuthMigrationRequired) {
					t.Fatal(err)
				}
			}
			m.reconnectWithRetry("legacy")
			m.AutoConnectAll(ctx)
			m.mu.Lock()
			m.servers["legacy"].Status = StatusConnected
			m.servers["legacy"].Tools = []MCPToolInfo{{Name: "tool", FullName: "mcp_legacy__tool"}}
			m.mu.Unlock()
			if len(m.GetEligibleNativeMCPServers()) != 0 {
				t.Fatal("legacy authorization exposed to native provider")
			}
			if requests.Load() != 0 || browsers.Load() != 0 {
				t.Fatal("legacy runtime contacted OAuth or resource")
			}
			after, err := m.GetConfig("legacy")
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("runtime changed recovery configuration: %v", err)
			}
			if err := db.Order("id").Find(&rowsAfter).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rowsBefore, rowsAfter) {
				t.Fatal("runtime changed legacy credentials")
			}
			if _, err := m.CreateOAuthSnapshot(ctx, cfg.ID); err != nil {
				t.Fatalf("legacy recovery unavailable: %v", err)
			}
		})
	}
}

func TestManagedOAuthMissingReferenceNeverFallsBack(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(401) }))
	defer srv.Close()
	m, _, ctx, _ := legacyWALManagers(t, srv.URL)
	// Simulate a damaged reference loaded from storage, with old token rows
	// still present. Operational code must not use those rows as a fallback.
	m.mu.Lock()
	m.servers["legacy"].Config.OAuthManaged = true
	m.mu.Unlock()
	for _, call := range []func() error{
		func() error { return m.Connect("legacy") },
		func() error { return m.connectWithContext(ctx, "legacy") },
		func() error { return m.ReauthorizeServer(ctx, "legacy") },
		func() error { return m.RecoverServerBestEffort(ctx, "legacy").Err },
	} {
		if err := call(); !errors.Is(err, oauthflow.ErrResource) {
			t.Fatalf("damaged authorization bypassed: %v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("used legacy OAuth with missing composed reference")
	}
}

func TestRecoveryStdioIgnoresResidualLegacyOAuth(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(401) }))
	defer srv.Close()
	m, _, ctx, cfg := legacyWALManagers(t, srv.URL)
	cfg.Transport, cfg.Command = TransportStdio, "in-memory"
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	result := m.RecoverServerBestEffort(ctx, "legacy")
	if result.Err != nil || !result.Reconnected || result.Refreshed || requests.Load() != 0 {
		t.Fatalf("STDIO attempted residual OAuth: %+v requests=%d", result, requests.Load())
	}
	stored, err := m.credMgr.ReadLegacyOAuthToken(ctx, "legacy", cfg.ID)
	if err != nil || stored.Token != "old" {
		t.Fatalf("STDIO changed preserved OAuth token: %v", err)
	}
	m.CloseAll()
}

func TestHistoricalURLOnlyAuthenticationRequiresExplicitChoice(t *testing.T) {
	for _, hasGrant := range []bool{false, true} {
		t.Run(map[bool]string{false: "public", true: "discovery_grant"}[hasGrant], func(t *testing.T) {
			m, repo, ctx := managedFixture(t)
			var requests atomic.Int32
			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "public", Version: "1"}, nil)
			handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Header.Get("Authorization") != "" {
					t.Error("public connection sent credentials")
				}
				handler.ServeHTTP(w, r)
			}))
			defer srv.Close()
			defer m.CloseAll()
			cfg := ServerConfig{Slug: "historical", URL: srv.URL, Enabled: true, AutoConnect: true, DisableSSE: true}
			if err := repo.SaveServer(ctx, &cfg); err != nil {
				t.Fatal(err)
			}
			// Historical URL-only storage, before LoadConfigs inferred PKCE.
			if err := repo.db.Model(&database.MCPServer{}).Where("id = ?", cfg.ID).Update("auth_type", "").Error; err != nil {
				t.Fatal(err)
			}
			if hasGrant {
				if err := m.credMgr.RegisterPatternWithContext(ctx, userTokensPattern(cfg.Slug), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "old", RefreshURL: "refresh"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := m.LoadConfigs(); err != nil {
				t.Fatal(err)
			}
			before, err := m.GetConfig(cfg.Slug)
			if err != nil || before.AuthType != AuthOAuth2PKCE {
				t.Fatalf("historical inference fixture: %v", err)
			}
			var rowsBefore, rowsAfter []database.CredentialEntry
			if err := repo.db.Order("id").Find(&rowsBefore).Error; err != nil {
				t.Fatal(err)
			}
			if err := m.Connect(cfg.Slug); !errors.Is(err, errOAuthAuthenticationSelection) {
				t.Fatalf("ambiguous record offered impossible migration: %v", err)
			}
			m.AutoConnectAll(ctx)
			after, err := m.GetConfig(cfg.Slug)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("authentication inferred destructively")
			}
			if err := repo.db.Order("id").Find(&rowsAfter).Error; err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 0 || !reflect.DeepEqual(rowsBefore, rowsAfter) {
				t.Fatal("ambiguous configuration used or changed credentials")
			}
			if hasGrant {
				return
			}
			// The existing editor's explicit None choice enables a public server
			// without requiring a snapshot or an OAuth provider that does not exist.
			after.AuthType = AuthNone
			if err := m.SaveConfig(cfg.Slug, *after); err != nil {
				t.Fatal(err)
			}
			if err := m.LoadConfigs(); err != nil {
				t.Fatal(err)
			}
			if err := m.Connect(cfg.Slug); err != nil {
				t.Fatal(err)
			}
			if requests.Load() == 0 {
				t.Fatal("public server never connected")
			}
			persisted, err := repo.GetServer(ctx, cfg.Slug)
			if err != nil || persisted.AuthType != AuthNone || persisted.OAuthAuthorizationID != "" {
				t.Fatal("public choice did not survive reload")
			}
		})
	}
}
