package mcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

func TestHostnameSnapshotManagerPreservesConsumerAndResolvesToken(t *testing.T) {
	m, _, ctx, cfg := legacyWALManagers(t, "https://shared.example")
	m.snapshotRoot = t.TempDir()
	if err := m.credMgr.RegisterPatternWithContext(ctx, "shared.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "copied-provider-token"}); err != nil {
		t.Fatal(err)
	}
	info, err := m.CreateOAuthSnapshot(ctx, "credential:shared.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.credMgr.DeletePattern(ctx, "shared.example"); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreOAuthSnapshot(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	after, err := m.GetConfig("legacy")
	if err != nil || !reflect.DeepEqual(cfg, *after) {
		t.Fatal("hostname recovery changed server", err)
	}
	for _, reload := range []bool{false, true} {
		if reload {
			if err := m.credMgr.LoadUserCredentials(ctx, "owner"); err != nil {
				t.Fatal(err)
			}
		}
		auth, err := m.credMgr.ResolveForURLWithContext(ctx, cfg.URL)
		if err != nil || auth == nil || auth.Token != "copied-provider-token" {
			t.Fatal("recovered hostname token not resolved", err)
		}
		private, err := m.credMgr.GetByPatternWithContext(ctx, userTokensPattern("legacy"))
		if err != nil || private == nil || private.Token != "old" {
			t.Fatal("hostname restore changed consumer grant", err)
		}
	}
}

func TestClientCredentialsSnapshotGetsNewTokenAfterEnable(t *testing.T) {
	t.Run("configured_id", func(t *testing.T) { testClientCredentialsSnapshotGetsNewToken(t, false) })
	t.Run("vault_only_id", func(t *testing.T) { testClientCredentialsSnapshotGetsNewToken(t, true) })
}

func testClientCredentialsSnapshotGetsNewToken(t *testing.T, vaultOnlyID bool) {
	t.Helper()
	var tokens, resources atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokens.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			client, secret, _ := r.BasicAuth()
			if client == "" {
				client, secret = r.Form.Get("client_id"), r.Form.Get("client_secret")
			}
			if r.Form.Get("grant_type") != "client_credentials" || client != "client" || secret != "restored-secret" {
				t.Error("wrong recovered client or grant")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"fresh-access","token_type":"Bearer","expires_in":3600}`)
			return
		}
		resources.Add(1)
		if r.Header.Get("Authorization") != "Bearer fresh-access" {
			t.Error("resource used old or missing token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	m, _, ctx, cfg := legacyWALManagers(t, server.URL)
	m.snapshotRoot = t.TempDir()
	m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		return d.IPs, true, nil
	})
	cfg.AuthType = AuthOAuth2ClientCredentials
	if vaultOnlyID {
		cfg.OAuth2ClientID = ""
	}
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.credMgr.RegisterPatternWithContext(ctx, clientCredPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "restored-secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := m.CreateOAuthSnapshot(ctx, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteServerAuth("legacy"); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreOAuthSnapshot(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if tokens.Load() != 0 || resources.Load() != 0 {
		t.Fatal("recovery contacted remote server")
	}
	m.mu.RLock()
	status := *m.servers["legacy"]
	m.mu.RUnlock()
	if status.Config.Enabled || status.Config.AutoConnect || status.NeedsReauth || status.Status != StatusDisconnected {
		t.Fatal("CC recovery enabled connection or requested interactive authorization")
	}
	conversion, err := m.CreateOAuthSnapshot(ctx, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ConvertOAuthClientSnapshot(ctx, conversion.ID, "client_secret_post"); err != nil {
		t.Fatal(err)
	}
	current, err := m.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	current.Enabled = true
	if err := m.SaveConfig("legacy", *current); err != nil {
		t.Fatal(err)
	}
	for _, reload := range []bool{false, true} {
		if reload {
			if err := m.credMgr.LoadUserCredentials(ctx, cfg.UserID); err != nil {
				t.Fatal(err)
			}
		}
		client := m.buildAuthHTTPClient(ctx, "legacy", *current)
		if client == nil {
			t.Fatal("restored registration missing in runtime")
		}
		resp, err := client.Get(cfg.URL)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if tokens.Load() != 1 || resources.Load() != 2 {
		t.Fatalf("unexpected token/resource requests: %d/%d", tokens.Load(), resources.Load())
	}
	stored, err := m.GetConfig("legacy")
	if err != nil || stored.OAuth2ClientID != "client" || stored.OAuthAuthorizationID == "" {
		t.Fatalf("token resolution changed stored configuration: %v", err)
	}
}

func TestOAuthSnapshotRestoreReauthorizeThenEnable(t *testing.T) {
	t.Run("disabled", func(t *testing.T) { testSnapshotReauthorization(t, false) })
	t.Run("enabled_after_authorization", func(t *testing.T) { testSnapshotReauthorization(t, true) })
}

func testSnapshotReauthorization(t *testing.T, enableAfterAuthorization bool) {
	t.Helper()
	var requests atomic.Int32
	var resourceRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/token" {
			resourceRequests.Add(1)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	m, _, ctx, cfg := legacyWALManagers(t, server.URL)
	m.snapshotRoot = t.TempDir()
	m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		return d.IPs, true, nil
	})
	cfg.OAuth2AuthURL = server.URL + "/authorize"
	cfg.OAuth2CallbackHost = "127.0.0.1"
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	info, err := m.CreateOAuthSnapshot(ctx, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	db := m.repository().(*DBRepository).db
	if err := db.AutoMigrate(&database.ToolCatalog{}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteConfig("legacy"); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreOAuthSnapshot(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := m.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Enabled || stored.AutoConnect {
		t.Fatal("restore connected automatically")
	}
	if requests.Load() != 0 {
		t.Fatal("snapshot contacted remote server")
	}
	oldBrowser := browserOpen
	t.Cleanup(func() { browserOpen = oldBrowser })
	browserOpen = func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		resp, err := http.Get(q.Get("redirect_uri") + "?code=ok&state=" + url.QueryEscape(q.Get("state")))
		if err != nil {
			return err
		}
		return resp.Body.Close()
	}
	if err := m.ReauthorizeServer(ctx, "legacy"); !errors.Is(err, errOAuthMigrationRequired) || requests.Load() != 0 {
		t.Fatalf("legacy reauthorization must require migration without remote calls: %v", err)
	}
	conversion, err := m.CreateOAuthSnapshot(ctx, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ReconnectOAuthSnapshot(ctx, conversion.ID, "none"); err != nil {
		t.Fatal(err)
	}
	m.signalNeedsReauth("legacy", cfg.Name, "test")
	if enableAfterAuthorization {
		m.emitEvent = func(event string, _ any) {
			if event == "mcp:server_reauthorized" {
				current, err := m.GetConfig("legacy")
				if err != nil {
					t.Fatal(err)
				}
				current.Enabled = true
				if err := m.SaveConfig("legacy", *current); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	err = m.ReauthorizeServer(ctx, "legacy")
	if enableAfterAuthorization {
		// The mock resource rejects the handshake; reaching it proves the latest
		// Enabled state was observed after OAuth, instead of the captured false.
		if err == nil || resourceRequests.Load() == 0 {
			t.Fatal("reauthorization did not reconnect newly enabled consumer")
		}
	} else if err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	status := m.servers["legacy"].Status
	m.mu.RUnlock()
	if !enableAfterAuthorization && status != StatusDisconnected {
		t.Fatal("reauthorization connected disabled consumer")
	}
	_, _, record := loadManaged(t, m, ctx, "legacy")
	if record.Tokens.Access != "new-access" || record.State != "connected" {
		t.Fatal("reauthorization did not connect composed authorization")
	}
	stored, err = m.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	stored.Enabled = true
	if err := m.SaveConfig("legacy", *stored); err != nil {
		t.Fatal(err)
	}
	if stored.AutoConnect {
		t.Fatal("reauthorization enabled automatic connection")
	}
}
