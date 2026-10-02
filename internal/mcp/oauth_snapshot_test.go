package mcp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

func TestOAuthSnapshotRestoreReauthorizeThenEnable(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/token" {
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
	if err := m.ReauthorizeServer(ctx, "legacy"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	status := m.servers["legacy"].Status
	m.mu.RUnlock()
	if status != StatusDisconnected {
		t.Fatal("reauthorization connected disabled consumer")
	}
	tokens, err := m.credMgr.ReadLegacyOAuthToken(ctx, "legacy", cfg.ID, nil)
	if err != nil || tokens.Token != "new-access" {
		t.Fatalf("reauthorization did not clear pending marker: %v", err)
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
