package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

func legacyClientFixture(t *testing.T, endpoint string) (*Manager, *Manager, context.Context, ServerConfig) {
	t.Helper()
	a, b, ctx, cfg := legacyWALManagers(t, endpoint)
	cfg.AuthType = AuthOAuth2ClientCredentials
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.credMgr.DeletePattern(ctx, userTokensPattern("legacy")); err != nil {
		t.Fatal(err)
	}
	if err := a.credMgr.RegisterPatternWithContext(ctx, clientCredPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []*Manager{a, b} {
		m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
			return d.IPs, true, nil
		})
	}
	return a, b, ctx, cfg
}

func clientGrantGet(client *http.Client, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	response, err := client.Do(req)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func TestLegacyClientGrantCoordinatesProcessesAndMutations(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var grants, resources atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			if grants.Add(1) == 1 {
				close(started)
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"issued","token_type":"Bearer","expires_in":3600}`)
			return
		}
		resources.Add(1)
		if r.Header.Get("Authorization") != "Bearer issued" {
			t.Error("missing issued bearer")
		}
	}))
	defer server.Close()
	defer unblock()
	a, b, ctx, cfg := legacyClientFixture(t, server.URL)
	a.snapshotRoot = t.TempDir()
	client := a.buildAuthHTTPClient(ctx, "legacy", cfg)
	other := b.buildAuthHTTPClient(ctx, "legacy", cfg)
	done := make(chan error, 1)
	go func() { done <- clientGrantGet(client, cfg.URL) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("grant not started")
	}
	if err := clientGrantGet(other, cfg.URL); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatal("parallel grant allowed", err)
	}
	if _, err := a.CreateOAuthSnapshot(ctx, cfg.ID); err == nil {
		t.Fatal("active grant captured")
	}
	if err := b.credMgr.RegisterPatternWithContext(ctx, clientCredPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "changed"}); err == nil {
		t.Fatal("client edited during grant")
	}
	if err := a.DeleteServerAuth("legacy"); err == nil {
		t.Fatal("grant deleted during request")
	}
	edit := cfg
	edit.Name = "Changed"
	if err := a.SaveConfig("legacy", edit); err == nil {
		t.Fatal("consumer edited during grant")
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := clientGrantGet(client, cfg.URL); err != nil {
		t.Fatal(err)
	}
	if grants.Load() != 1 || resources.Load() != 2 {
		t.Fatal("cache or coordination failed", grants.Load(), resources.Load())
	}
	if _, err := a.CreateOAuthSnapshot(ctx, cfg.ID); err != nil {
		t.Fatal("finished lease not released", err)
	}
	if err := a.DeleteServerAuth("legacy"); err != nil {
		t.Fatal(err)
	}
	if err := clientGrantGet(client, cfg.URL); err == nil {
		t.Fatal("deleted client reused cached token")
	}
	if grants.Load() != 1 || resources.Load() != 2 {
		t.Fatal("request escaped after deletion")
	}
}

func TestLegacyClientGrantRelatesCacheToCurrentClientAndOwner(t *testing.T) {
	var grants, resources atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			grants.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"issued","token_type":"Bearer","expires_in":3600}`)
			return
		}
		resources.Add(1)
	}))
	defer server.Close()
	a, b, ctx, cfg := legacyClientFixture(t, server.URL)
	client := a.buildAuthHTTPClient(ctx, "legacy", cfg)
	if err := clientGrantGet(client, cfg.URL); err != nil {
		t.Fatal(err)
	}
	if err := b.credMgr.RegisterPatternWithContext(ctx, clientCredPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "replacement"}); err != nil {
		t.Fatal(err)
	}
	if err := clientGrantGet(client, cfg.URL); err != nil {
		t.Fatal(err)
	}
	if grants.Load() != 2 {
		t.Fatal("cached token ignored changed client")
	}
	// Simulate an atomic ownership change from another upgraded process.
	db := a.repository().(*DBRepository).db
	if err := db.Model(&database.MCPServer{}).Where("id = ?", cfg.ID).Updates(map[string]any{"oauth_managed": true, "oauth_authorization_id": "converted"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := clientGrantGet(client, cfg.URL); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("stale owner used token", err)
	}
	if resources.Load() != 2 {
		t.Fatal("stale resource request escaped")
	}
}

func TestLegacyClientGrantSessionEndDoesNotPublishToken(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var resources atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"late-secret","token_type":"Bearer","expires_in":3600}`)
			return
		}
		resources.Add(1)
	}))
	defer server.Close()
	defer close(release)
	a, _, ctx, cfg := legacyClientFixture(t, server.URL)
	client := a.buildAuthHTTPClient(ctx, "legacy", cfg)
	done := make(chan error, 1)
	go func() { done <- clientGrantGet(client, cfg.URL) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("grant not started")
	}
	a.credMgr.Reset(bytes.Repeat([]byte{8}, 32), true)
	if err := <-done; err == nil {
		t.Fatal("ended session accepted token")
	}
	if resources.Load() != 0 {
		t.Fatal("resource called after logout")
	}
}

func TestLegacyClientGrantFailedIssuanceCanRetryWithoutLeakingBody(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			if failing.Load() {
				http.Error(w, "provider-secret-must-not-escape", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"issued","token_type":"Bearer","expires_in":3600}`)
		}
	}))
	defer server.Close()
	a, _, ctx, cfg := legacyClientFixture(t, server.URL)
	client := a.buildAuthHTTPClient(ctx, "legacy", cfg)
	err := clientGrantGet(client, cfg.URL)
	if !errors.Is(err, oauthflow.ErrTransient) || strings.Contains(err.Error(), "provider-secret") {
		t.Fatal("grant error leaked or lost classification", err)
	}
	failing.Store(false)
	if err := clientGrantGet(client, cfg.URL); err != nil {
		t.Fatal("non rotating grant required reauthorization", err)
	}
}

func TestLegacyClientGrantReusesConsentBeforeLeaseAcrossOrigins(t *testing.T) {
	var grants, prompts atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grants.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"issued","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer resource.Close()
	a, _, ctx, cfg := legacyClientFixture(t, resource.URL)
	cfg.OAuth2TokenURL = tokenServer.URL + "/token"
	if err := a.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	a.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		var active int64
		if err := a.repository().(*DBRepository).db.Model(&database.CredentialEntry{}).Where("pattern = ? AND legacy_oauth_control_enc <> ''", userTokensPattern("legacy")).Count(&active).Error; err != nil {
			t.Error(err)
		}
		if active != 0 {
			t.Error("consent requested inside durable lease")
		}
		return d.IPs, true, nil
	})
	client := a.buildAuthHTTPClient(ctx, "legacy", cfg)
	if err := clientGrantGet(client, cfg.URL); err != nil {
		t.Fatal(err)
	}
	if prompts.Load() != 1 || grants.Load() != 1 {
		t.Fatal("approval scope was not reused", prompts.Load(), grants.Load())
	}
	if err := clientGrantGet(client, cfg.URL); err != nil {
		t.Fatal(err)
	}
	if prompts.Load() != 1 || grants.Load() != 1 {
		t.Fatal("cached grant prompted or issued again")
	}
}

func TestLegacyClientGrantNativeNeverUsesCachedHostnameAfterCutover(t *testing.T) {
	a, b, ctx, cfg := legacyClientFixture(t, "https://shared.example")
	if err := a.credMgr.RegisterPatternWithContext(ctx, "shared.example", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "stale-native-token"}); err != nil {
		t.Fatal(err)
	}
	c := nativeMCPCandidate{slug: cfg.Slug, name: cfg.Name, url: cfg.URL, authType: cfg.AuthType, managedConfig: cfg}
	if token, ok := a.resolveNativeAuthToken(ctx, c); ok || token != "" {
		t.Fatal("legacy CC escaped bridge through hostname")
	}
	if err := b.repository().(*DBRepository).db.Model(&database.MCPServer{}).Where("id = ?", cfg.ID).Updates(map[string]any{"oauth_managed": true, "oauth_authorization_id": "converted"}).Error; err != nil {
		t.Fatal(err)
	}
	if token, ok := a.resolveNativeAuthToken(ctx, c); ok || token != "" {
		t.Fatal("cached native token survived ownership change")
	}
}
