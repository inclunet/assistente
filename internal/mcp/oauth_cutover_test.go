package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
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
			// The resolver itself must refuse the historical grant, including a
			// candidate without a persisted ID or an unavailable credential manager.
			for _, persisted := range []bool{true, false} {
				candidateConfig := *before
				if !persisted {
					candidateConfig.ID, candidateConfig.UserID = "", ""
				}
				candidate := nativeMCPCandidate{slug: "legacy", url: cfg.URL, authType: authType, managedConfig: candidateConfig}
				if token, ok := m.resolveNativeAuthToken(ctx, candidate); ok || token != "" {
					t.Fatal("native resolver reused historical grant")
				}
				withoutVault := &Manager{}
				if token, ok := withoutVault.resolveNativeAuthToken(ctx, candidate); ok || token != "" {
					t.Fatal("missing vault enabled anonymous OAuth fallback")
				}
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
	stored, err := m.credMgr.GetByPatternWithContext(ctx, userTokensPattern("legacy"))
	if err != nil || stored == nil || stored.Token != "old" {
		t.Fatalf("STDIO changed preserved OAuth token: %v", err)
	}
	m.CloseAll()
}

func TestHistoricalURLOnlyAuthenticationRequiresExplicitChoice(t *testing.T) {
	for _, hasGrant := range []bool{false, true} {
		t.Run(map[bool]string{false: "public", true: "discovery_grant"}[hasGrant], func(t *testing.T) {
			var requests atomic.Int32
			var endpoint string
			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "public", Version: "1"}, nil)
			handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if hasGrant {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/.well-known/oauth-protected-resource":
						_ = json.NewEncoder(w).Encode(map[string]any{"resource": endpoint, "authorization_servers": []string{endpoint}})
					case "/.well-known/oauth-authorization-server", "/.well-known/openid-configuration":
						_ = json.NewEncoder(w).Encode(map[string]any{"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token", "registration_endpoint": endpoint + "/register", "code_challenge_methods_supported": []string{"S256"}})
					case "/register":
						_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "discovered-client"})
					case "/token":
						_ = r.ParseForm()
						if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("client_id") != "discovered-client" {
							t.Error("wrong migration grant")
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "migrated", "refresh_token": "migrated-refresh", "token_type": "Bearer", "expires_in": 3600})
					default:
						w.WriteHeader(http.StatusUnauthorized)
					}
					return
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("public connection sent credentials")
				}
				handler.ServeHTTP(w, r)
			}))
			endpoint = srv.URL
			m, _, ctx, _ := legacyWALManagers(t, srv.URL)
			repo := m.repository().(*DBRepository)
			defer srv.Close()
			defer m.CloseAll()
			cfg := ServerConfig{Slug: "legacy", URL: srv.URL, Enabled: true, AutoConnect: true, DisableSSE: true}
			if !hasGrant {
				if err := m.credMgr.DeletePattern(ctx, userTokensPattern(cfg.Slug)); err != nil {
					t.Fatal(err)
				}
			}
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
				// Confirming and saving PKCE makes the persisted format eligible
				// for inventory/snapshots without changing the old grant.
				if err := m.SaveConfig(cfg.Slug, *after); err != nil {
					t.Fatal(err)
				}
				if err := repo.db.Order("id").Find(&rowsAfter).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(rowsBefore, rowsAfter) || requests.Load() != 0 {
					t.Fatal("confirmation modified the grant")
				}
				items, err := m.InspectOAuthInventory(ctx)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range items {
					if item.ID == cfg.ID {
						found = true
					}
				}
				if !found {
					t.Fatal("confirmed PKCE missing in diagnostic")
				}
				m.snapshotRoot = t.TempDir()
				snapshot, err := m.CreateOAuthSnapshot(ctx, cfg.ID)
				if err != nil {
					t.Fatal(err)
				}
				m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
					return d.IPs, true, nil
				})
				oldBrowser := browserOpen
				defer func() { browserOpen = oldBrowser }()
				browserOpen = func(raw string) error {
					u, err := url.Parse(raw)
					if err != nil {
						return err
					}
					q := u.Query()
					response, err := http.Get(q.Get("redirect_uri") + "?code=ok&state=" + url.QueryEscape(q.Get("state")))
					if err != nil {
						return err
					}
					return response.Body.Close()
				}
				if err := m.ReconnectOAuthSnapshot(ctx, snapshot.ID, "none"); err != nil {
					t.Fatal(err)
				}
				_, _, record := loadManaged(t, m, ctx, cfg.Slug)
				if record.Tokens.Access != "migrated" || record.Client.ID != "discovered-client" {
					t.Fatal("discovery-only grant did not migrate")
				}
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

// Moved from credentials: hostname resolution is no longer an OAuth vault API.
// All five historical race cases must now stop at the MCP boundary before a
// hostname credential can be read or its source executed, both before and after
// the competing instance changes the database.
func TestLegacyHostnameRevalidatesAfterSourceResolution(t *testing.T) {
	for _, change := range []string{"unchanged", "grant", "consumer", "hostname_changed", "hostname_deleted"} {
		t.Run(change, func(t *testing.T) {
			a, b, ctx, cfg := legacyWALManagers(t, "https://fallback.example")
			if err := a.credMgr.ClearLegacyOAuth(ctx, "legacy", cfg.ID, ""); err != nil {
				t.Fatal(err)
			}
			if err := a.credMgr.RegisterPatternWithContext(ctx, "fallback.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "fallback"}); err != nil {
				t.Fatal(err)
			}
			var reads atomic.Int32
			db := a.repository().(*DBRepository).db
			if err := db.Callback().Query().After("gorm:query").Register("observe_hostname_resolution", func(tx *gorm.DB) {
				if row, ok := tx.Statement.Dest.(*database.CredentialEntry); ok && row.Pattern == "fallback.example" {
					reads.Add(1)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove("observe_hostname_resolution") })
			candidate := nativeMCPCandidate{slug: "legacy", url: cfg.URL, authType: AuthOAuth2PKCE, managedConfig: cfg}
			if token, ok := a.resolveNativeAuthToken(ctx, candidate); ok || token != "" || reads.Load() != 0 {
				t.Fatal("historical OAuth resolved hostname before migration")
			}
			var err error
			switch change {
			case "grant":
				err = b.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "new-grant"})
			case "consumer":
				edited := cfg
				edited.AuthType = AuthNone
				err = b.SaveConfig("legacy", edited)
			case "hostname_changed":
				err = b.credMgr.RegisterPatternWithContext(ctx, "fallback.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "replacement"})
			case "hostname_deleted":
				err = b.credMgr.DeletePattern(ctx, "fallback.example")
			}
			if err != nil {
				t.Fatal(err)
			}
			if token, ok := a.resolveNativeAuthToken(ctx, candidate); ok || token != "" || reads.Load() != 0 {
				t.Fatalf("changed historical state bypassed cutover: hostname reads=%d", reads.Load())
			}
		})
	}
}
