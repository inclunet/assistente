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

// Runtime tests migrate the persisted historical fixture explicitly, then exercise
// only the composed store. The legacy fixture remains useful to conversion tests.
func composedClientFixture(t *testing.T, endpoint string) (*Manager, *Manager, context.Context, ServerConfig) {
	t.Helper()
	a, b, ctx, cfg := legacyClientFixture(t, endpoint)
	a.snapshotRoot = t.TempDir()
	info, err := a.CreateOAuthSnapshot(ctx, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ConvertOAuthClientSnapshot(ctx, info.ID, "client_secret_post"); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ = loadManaged(t, a, ctx, cfg.Slug)
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

func TestComposedClientGrantCoordinatesProcessesAndMutations(t *testing.T) {
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
	a, b, ctx, cfg := composedClientFixture(t, server.URL)
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
	waitingCtx, cancelWaiting := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelWaiting()
	waitingRequest, _ := http.NewRequestWithContext(waitingCtx, http.MethodGet, cfg.URL, nil)
	if _, err := other.Do(waitingRequest); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("parallel grant allowed", err)
	}
	_, store, active := loadManaged(t, a, ctx, cfg.Slug)
	if !active.RefreshActive() || grants.Load() != 1 {
		t.Fatal("grant lease or exclusion missing")
	}
	if err := b.SaveConfigWithOAuthSecret(cfg.Slug, cfg, "changed"); err == nil {
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
	finished, err := store.Load(ctx, cfg.OAuthAuthorizationID)
	if err != nil || finished.RefreshPending || finished.RefreshActive() || finished.Tokens.Access != "issued" {
		t.Fatal("finished grant not committed/released", err)
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

func TestComposedClientGrantRelatesCacheToCurrentClientAndOwner(t *testing.T) {
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
	a, b, ctx, cfg := composedClientFixture(t, server.URL)
	client := a.buildAuthHTTPClient(ctx, "legacy", cfg)
	if err := clientGrantGet(client, cfg.URL); err != nil {
		t.Fatal(err)
	}
	if err := b.SaveConfigWithOAuthSecret(cfg.Slug, cfg, "replacement"); err != nil {
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

func TestComposedClientGrantSessionEndDoesNotPublishToken(t *testing.T) {
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
	a, _, ctx, cfg := composedClientFixture(t, server.URL)
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

func TestComposedClientGrantRechecksConsumerAfterIssuance(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var grants, resources atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			grants.Add(1)
			close(started)
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
	}))
	defer server.Close()
	defer unblock()
	a, b, ctx, cfg := composedClientFixture(t, server.URL)
	client := a.buildAuthHTTPClient(ctx, cfg.Slug, cfg)
	done := make(chan error, 1)
	go func() { done <- clientGrantGet(client, cfg.URL) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("grant not started")
	}
	if err := b.repository().(*DBRepository).db.Model(&database.MCPServer{}).Where("id = ?", cfg.ID).Update("oauth_authorization_id", "replacement").Error; err != nil {
		t.Fatal(err)
	}
	unblock()
	if err := <-done; !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("token escaped after consumer changed during issuance", err)
	}
	if _, ok := a.resolveNativeAuthToken(ctx, nativeMCPCandidate{slug: cfg.Slug, managedConfig: cfg}); ok {
		t.Fatal("stale authorization escaped through native resolution")
	}
	if grants.Load() != 1 || resources.Load() != 0 {
		t.Fatal("changed consumer caused another grant or resource call")
	}
}

func TestComposedClientGrantFailedIssuanceCanRetryWithoutLeakingBody(t *testing.T) {
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
	a, _, ctx, cfg := composedClientFixture(t, server.URL)
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

func TestComposedClientGrantReusesConsentBeforeLeaseAcrossOrigins(t *testing.T) {
	var grants, prompts atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grants.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"issued","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer resource.Close()
	a, _, ctx, cfg := composedClientFixture(t, resource.URL)
	cfg.OAuth2TokenURL = tokenServer.URL + "/token"
	if err := a.SaveConfigWithOAuthSecret("legacy", cfg, "secret"); err != nil {
		t.Fatal(err)
	}
	a.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		_, _, active := loadManaged(t, a, ctx, cfg.Slug)
		if active.RefreshActive() {
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

func TestComposedClientGrantChecksBindingAfterNetworkConsent(t *testing.T) {
	var grants, resources, prompts atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grants.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"must-not-issue","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { resources.Add(1) }))
	defer resource.Close()
	a, b, ctx, cfg := composedClientFixture(t, resource.URL)
	cfg.OAuth2TokenURL = tokenServer.URL + "/token"
	if err := a.SaveConfigWithOAuthSecret(cfg.Slug, cfg, "secret"); err != nil {
		t.Fatal(err)
	}
	_, store, before := loadManaged(t, a, ctx, cfg.Slug)
	a.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		// Simulate another process replacing the consumer while human consent
		// is pending, after the resolver's initial read but before its lease CAS.
		if err := b.repository().(*DBRepository).db.Model(&database.MCPServer{}).Where("id = ?", cfg.ID).Update("oauth_authorization_id", "replacement").Error; err != nil {
			t.Error(err)
		}
		return d.IPs, true, nil
	})
	if err := clientGrantGet(a.buildAuthHTTPClient(ctx, cfg.Slug, cfg), cfg.URL); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("changed binding allowed issuance after consent", err)
	}
	after, err := store.Load(ctx, cfg.OAuthAuthorizationID)
	if err != nil || after.Revision != before.Revision || after.RefreshPending || after.Tokens.Access != "" {
		t.Fatal("rejected lease changed authorization", err)
	}
	if prompts.Load() != 1 || grants.Load() != 0 || resources.Load() != 0 {
		t.Fatalf("stale consumer reached network: prompts=%d grants=%d resources=%d", prompts.Load(), grants.Load(), resources.Load())
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

func TestLegacyClientCredentialsHTTPRequiresMigration(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	m, _, ctx, cfg := legacyClientFixture(t, server.URL)
	for _, persisted := range []bool{true, false} {
		candidate := cfg
		if !persisted {
			candidate.ID, candidate.UserID = "", ""
		}
		client := m.buildAuthHTTPClient(ctx, cfg.Slug, candidate)
		if err := clientGrantGet(client, cfg.URL); !errors.Is(err, errOAuthMigrationRequired) {
			t.Fatalf("legacy HTTP helper did not require migration (persisted=%v): %v", persisted, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("legacy HTTP helper contacted provider")
	}
	stored, err := m.credMgr.GetByPatternWithContext(ctx, clientCredPattern(cfg.Slug))
	if err != nil || stored == nil || stored.ClientID != "client" || stored.ClientSecret != "secret" {
		t.Fatal("migration refusal changed recoverable credentials", err)
	}
}
