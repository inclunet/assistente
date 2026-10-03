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

	"assistente/internal/database"
	"assistente/internal/oauthflow"
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
