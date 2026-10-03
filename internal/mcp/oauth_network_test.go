package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

func TestConnectOAuthConsentPreservesUserAndStopsAfterDenial(t *testing.T) {
	var forbiddenCalls, prompts atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forbiddenCalls.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	var origin string
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource/mcp" || r.URL.Path == "/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{target.URL}})
			return
		}
		if r.URL.Path == "/authorize" || r.URL.Path == "/token" {
			forbiddenCalls.Add(1)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer source.Close()
	origin = source.URL
	m, _, userCtx := managedFixture(t)
	defer m.CloseAll()
	capturedUser, _ := database.RequireUserID(userCtx)
	m.SetOAuthNetworkAuthorizer(func(ctx context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		user, err := database.RequireUserID(ctx)
		if err != nil || user != capturedUser {
			t.Errorf("lost connection identity: %q %v", user, err)
		}
		return nil, false, nil
	})
	m.servers["srv"] = &ServerStatus{Slug: "srv", Config: ServerConfig{Enabled: true, Transport: TransportStreamable, URL: origin + "/mcp", AuthType: AuthOAuth2PKCE, OAuth2ClientID: "manual", OAuth2AuthURL: origin + "/authorize", OAuth2TokenURL: origin + "/token"}}
	seedManagedRuntime(t, m, userCtx, "srv", "", "", time.Time{})
	err := m.Connect("srv")
	if err == nil || prompts.Load() != 1 || forbiddenCalls.Load() != 0 {
		t.Fatalf("denial ignored or identity path missed: %v prompts=%d forbidden=%d", err, prompts.Load(), forbiddenCalls.Load())
	}
}
func TestDeviceVerificationPrivateDestinationRequiresConsent(t *testing.T) {
	var forbiddenCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forbiddenCalls.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "code", "user_code": "user", "verification_uri": target.URL + "/verify", "expires_in": 300})
	}))
	defer source.Close()
	rt := &oauthProtocol{cfg: ServerConfig{URL: source.URL, OAuth2DeviceAuthURL: source.URL + "/device", OAuth2TokenURL: source.URL + "/token"}, resolvedClientID: "client", networkAuthorizer: func(ctx context.Context, _ oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		if user, err := database.RequireUserID(ctx); err != nil || user != "device-user" {
			t.Errorf("device context lost: %q %v", user, err)
		}
		return nil, false, nil
	}}
	err := rt.authorizeDeviceFlow(database.WithUserID(context.Background(), "device-user"))
	if !errors.Is(err, oauthflow.ErrNetworkAuthorization) || forbiddenCalls.Load() != 0 {
		t.Fatalf("unapproved verification accessed: %v calls=%d", err, forbiddenCalls.Load())
	}
}

func TestPersistedExpiredTokenUsesNetworkGuardFromConstruction(t *testing.T) {
	var prompts, posts atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	m, _, ctx := managedFixture(t)
	cfg := managedConfig("https://192.0.2.1/mcp")
	cfg.OAuth2TokenURL, cfg.OAuth2TokenAuthMethod = target.URL, "none"
	if err := m.SaveConfig("srv", cfg); err != nil {
		t.Fatal(err)
	}
	seedManagedRuntime(t, m, ctx, "srv", "expired", "refresh", time.Now().Add(-time.Hour))
	cfg, _, _ = loadManaged(t, m, ctx, "srv")
	m.SetOAuthNetworkAuthorizer(func(context.Context, oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		return nil, false, nil
	})
	err := clientGrantGet(m.managedHTTPClient(ctx, cfg), cfg.URL)
	if !errors.Is(err, oauthflow.ErrNetworkAuthorization) || prompts.Load() != 1 || posts.Load() != 0 {
		t.Fatalf("stored refresh bypassed consent: %v prompts=%d posts=%d", err, prompts.Load(), posts.Load())
	}
}

func TestDisconnectCancelsPersistedRefreshConsent(t *testing.T) {
	var posts atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer source.Close()
	m, _, userCtx := managedFixture(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	m.SetOAuthNetworkAuthorizer(func(ctx context.Context, _ oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-release:
			return nil, false, nil
		}
	})
	m.servers["srv"] = &ServerStatus{Slug: "srv", Config: ServerConfig{Enabled: true, Transport: TransportStreamable, URL: source.URL, AuthType: AuthOAuth2PKCE, OAuth2ClientID: "client", OAuth2TokenURL: target.URL}}
	seedManagedRuntime(t, m, userCtx, "srv", "expired", "refresh", time.Now().Add(-time.Hour))
	done := make(chan error, 1)
	go func() { done <- m.Connect("srv") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh consent did not start")
	}
	disconnected := make(chan error, 1)
	go func() { disconnected <- m.Disconnect("srv") }()
	select {
	case err := <-disconnected:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Disconnect did not cancel refresh consent")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled connection succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Connect remained blocked")
	}
	if posts.Load() != 0 {
		t.Fatal("refresh sent despite cancellation")
	}
}

func TestConnectConsentOutlivesHandshakeBudget(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer source.Close()
	m, _, userCtx := managedFixture(t)
	defer m.CloseAll()
	m.connectTimeout = 100 * time.Millisecond
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	m.SetOAuthNetworkAuthorizer(func(ctx context.Context, _ oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		close(started)
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-release:
			return nil, false, nil
		}
	})
	m.servers["srv"] = &ServerStatus{Slug: "srv", Config: ServerConfig{Enabled: true, Transport: TransportStreamable, DisableSSE: true, URL: source.URL, AuthType: AuthOAuth2PKCE, OAuth2ClientID: "client", OAuth2TokenURL: "http://127.0.0.1:1/token"}}
	seedManagedRuntime(t, m, userCtx, "srv", "expired", "refresh", time.Now().Add(-time.Hour))
	done := make(chan error, 1)
	go func() { done <- m.Connect("srv") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("consent did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("handshake cancelled consent: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := m.Disconnect("srv"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled connection succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Disconnect did not stop consent")
	}
}

func TestConnectDevicePollingOutlivesHandshakeAndReusesApproval(t *testing.T) {
	for _, disableSSE := range []bool{false, true} {
		t.Run(map[bool]string{false: "probe-sse", true: "handshake"}[disableSSE], func(t *testing.T) {
			testConnectDevicePolling(t, disableSSE)
		})
	}
}
func testConnectDevicePolling(t *testing.T, disableSSE bool) {
	var polls, prompts, authenticated atomic.Int32
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if polls.Add(1) == 1 {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"device-access","token_type":"Bearer","expires_in":3600}`))
	}))
	defer token.Close()
	var origin string
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": origin, "token_endpoint": token.URL, "device_authorization_endpoint": origin + "/device"})
		case "/device":
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "device", "user_code": "user", "verification_uri": origin + "/verify", "expires_in": 60, "interval": 5})
		case "/verify":
			w.WriteHeader(200)
		case "/mcp":
			if r.Header.Get("Authorization") == "Bearer device-access" {
				authenticated.Add(1)
				w.WriteHeader(400)
			} else {
				w.WriteHeader(401)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer source.Close()
	origin = source.URL
	m, _, userCtx := managedFixture(t)
	defer m.CloseAll()
	m.connectTimeout = time.Second
	m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		return d.IPs, true, nil
	})
	m.servers["srv"] = &ServerStatus{Slug: "srv", Config: ServerConfig{Enabled: true, Transport: TransportStreamable, DisableSSE: disableSSE, URL: source.URL + "/mcp", AuthType: AuthOAuth2PKCE, OAuth2ClientID: "client", OAuth2DeviceAuthURL: source.URL + "/device", OAuth2TokenURL: token.URL}}
	seedManagedRuntime(t, m, userCtx, "srv", "", "", time.Time{})
	err := m.Connect("srv")
	// The fixture intentionally rejects MCP after OAuth: reaching it with the token
	// proves the real handshake survived two polling intervals and resumed.
	if errors.Is(err, context.DeadlineExceeded) || polls.Load() != 2 || prompts.Load() != 1 || authenticated.Load() == 0 {
		t.Fatalf("err=%v polls=%d prompts=%d authenticated=%d", err, polls.Load(), prompts.Load(), authenticated.Load())
	}
}

func TestBestEffortRefreshUsesNetworkConsentAndCredentialIdentity(t *testing.T) {
	for _, approve := range []bool{false, true} {
		t.Run(map[bool]string{false: "deny", true: "approve"}[approve], func(t *testing.T) {
			var prompts, posts atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"new","token_type":"Bearer","expires_in":3600}`))
			}))
			defer target.Close()
			m, _, userCtx := managedFixture(t)
			owner, _ := database.RequireUserID(userCtx)
			m.servers["srv"] = &ServerStatus{Slug: "srv", Config: ServerConfig{Transport: TransportStreamable, URL: "https://192.0.2.1/mcp", AuthType: AuthOAuth2PKCE, OAuth2ClientID: "client", OAuth2TokenURL: target.URL}}
			seedManagedRuntime(t, m, userCtx, "srv", "old", "refresh", time.Now().Add(-time.Hour))
			m.SetOAuthNetworkAuthorizer(func(ctx context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
				prompts.Add(1)
				if id, err := database.RequireUserID(ctx); err != nil || id != owner {
					t.Errorf("lost owner: %q %v", id, err)
				}
				if _, ok := ctx.Deadline(); ok {
					t.Error("HTTP timeout leaked into consent")
				}
				return d.IPs, approve, nil
			})
			changed, err := m.refreshOAuthTokenBestEffort(context.Background(), "srv", true)
			if changed != approve || prompts.Load() != 1 || (!approve && (!errors.Is(err, oauthflow.ErrNetworkAuthorization) || posts.Load() != 0)) || (approve && (err != nil || posts.Load() != 1)) {
				t.Fatalf("changed=%v err=%v prompts=%d posts=%d", changed, err, prompts.Load(), posts.Load())
			}
			if approve {
				_, _, record := loadManaged(t, m, userCtx, "srv")
				if record.Tokens.Access != "new" {
					t.Fatal("owner persistence failed")
				}
			}
		})
	}
}
func TestBestEffortRefreshConsentRetainsCallerCancellation(t *testing.T) {
	m, _, userCtx := managedFixture(t)
	m.servers["srv"] = &ServerStatus{Slug: "srv", Config: ServerConfig{Transport: TransportStreamable, URL: "https://192.0.2.1/mcp", AuthType: AuthOAuth2PKCE, OAuth2ClientID: "client", OAuth2TokenURL: "http://127.0.0.1:1/token"}}
	started := make(chan struct{})
	seedManagedRuntime(t, m, userCtx, "srv", "old", "refresh", time.Now().Add(-time.Hour))
	m.SetOAuthNetworkAuthorizer(func(ctx context.Context, _ oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		close(started)
		<-ctx.Done()
		return nil, false, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.refreshOAuthTokenBestEffort(ctx, "srv", true); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("consent did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("caller cancellation lost")
	}
}

func TestRecoveryStopsAfterRefreshNetworkRefusal(t *testing.T) {
	var prompts, requests atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(401) }))
	defer source.Close()
	m, _, ctx := managedFixture(t)
	m.servers["srv"] = &ServerStatus{Slug: "srv", Status: StatusDisconnected, Config: ServerConfig{Enabled: true, Transport: TransportStreamable, URL: source.URL + "/mcp", AuthType: AuthOAuth2PKCE, OAuth2ClientID: "client", OAuth2TokenURL: "http://127.0.0.1:1/token"}}
	seedManagedRuntime(t, m, ctx, "srv", "old", "refresh", time.Now().Add(-time.Hour))
	m.SetOAuthNetworkAuthorizer(func(context.Context, oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		return nil, false, nil
	})
	result := m.RecoverServerBestEffort(context.Background(), "srv")
	if !errors.Is(result.Err, oauthflow.ErrNetworkAuthorization) || !result.Attempted || result.Reconnected || result.Refreshed || prompts.Load() != 1 || requests.Load() != 0 {
		t.Fatalf("result=%+v prompts=%d requests=%d", result, prompts.Load(), requests.Load())
	}
}
