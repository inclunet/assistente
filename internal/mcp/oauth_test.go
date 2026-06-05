package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/credentials"

	"golang.org/x/oauth2"
)

func TestMain(m *testing.M) {
	browserOpen = func(url string) error { return nil }
	os.Exit(m.Run())
}

type oauth2Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

func toOAuth2Token(t *oauth2Token) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    "Bearer",
		Expiry:       t.Expiry,
	}
}

func newTestCredMgr() *credentials.Manager {
	return credentials.NewManager(nil)
}

func TestCallbackHostToListenIP(t *testing.T) {
	tests := []struct {
		name         string
		callbackHost string
		wantListenIP string
		wantHost     string // host used in redirectURL
	}{
		{
			name:         "empty defaults to localhost",
			callbackHost: "",
			wantListenIP: "127.0.0.1",
			wantHost:     "localhost",
		},
		{
			name:         "explicit localhost",
			callbackHost: "localhost",
			wantListenIP: "127.0.0.1",
			wantHost:     "localhost",
		},
		{
			name:         "explicit 127.0.0.1",
			callbackHost: "127.0.0.1",
			wantListenIP: "127.0.0.1",
			wantHost:     "127.0.0.1",
		},
		{
			name:         "IPv6 loopback",
			callbackHost: "[::1]",
			wantListenIP: "::1",
			wantHost:     "[::1]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotHost, gotListenIP := resolveCallbackHost(tc.callbackHost)

			if gotHost != tc.wantHost {
				t.Errorf("callbackHost: got %q, want %q", gotHost, tc.wantHost)
			}
			if gotListenIP != tc.wantListenIP {
				t.Errorf("listenIP: got %q, want %q", gotListenIP, tc.wantListenIP)
			}
		})
	}
}

func TestListenAddrFormat(t *testing.T) {
	tests := []struct {
		name     string
		listenIP string
		port     int
		wantAddr string
	}{
		{
			name:     "IPv4 random port",
			listenIP: "127.0.0.1",
			port:     0,
			wantAddr: "127.0.0.1:0",
		},
		{
			name:     "IPv4 fixed port",
			listenIP: "127.0.0.1",
			port:     3118,
			wantAddr: "127.0.0.1:3118",
		},
		{
			name:     "IPv6 random port",
			listenIP: "::1",
			port:     0,
			wantAddr: "::1:0",
		},
		{
			name:     "IPv6 fixed port",
			listenIP: "::1",
			port:     8080,
			wantAddr: "::1:8080",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if tc.port > 0 {
				got = fmt.Sprintf("%s:%d", tc.listenIP, tc.port)
			} else {
				got = tc.listenIP + ":0"
			}

			if got != tc.wantAddr {
				t.Errorf("listenAddr: got %q, want %q", got, tc.wantAddr)
			}
		})
	}
}

func TestRedirectURLFormat(t *testing.T) {
	tests := []struct {
		name         string
		callbackHost string
		port         int
		wantURL      string
	}{
		{
			name:         "localhost default with port 3118",
			callbackHost: "",
			port:         3118,
			wantURL:      "http://localhost:3118/callback",
		},
		{
			name:         "explicit localhost with port 8080",
			callbackHost: "localhost",
			port:         8080,
			wantURL:      "http://localhost:8080/callback",
		},
		{
			name:         "127.0.0.1 with port 3118",
			callbackHost: "127.0.0.1",
			port:         3118,
			wantURL:      "http://127.0.0.1:3118/callback",
		},
		{
			name:         "IPv6 loopback with port 9090",
			callbackHost: "[::1]",
			port:         9090,
			wantURL:      "http://[::1]:9090/callback",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, _ := resolveCallbackHost(tc.callbackHost)
			got := fmt.Sprintf("http://%s:%d/callback", host, tc.port)

			if got != tc.wantURL {
				t.Errorf("redirectURL: got %q, want %q", got, tc.wantURL)
			}
		})
	}
}

func TestDCRPersistsCallbackPort(t *testing.T) {
	var mu sync.Mutex
	var savedConfig *ServerConfig

	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2CallbackPort: 0,
		},
		serverSlug: "test",
		onConfigUpdate: func(cfg ServerConfig) {
			mu.Lock()
			defer mu.Unlock()
			c := cfg
			savedConfig = &c
		},
	}

	// Simula o que acontece após DCR: porta aleatória é alocada
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	// Simula a lógica de persistência de porta pós-DCR
	if rt.cfg.OAuth2CallbackPort == 0 {
		rt.cfg.OAuth2CallbackPort = port
		if rt.onConfigUpdate != nil {
			rt.onConfigUpdate(rt.cfg)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if savedConfig == nil {
		t.Fatal("onConfigUpdate não foi chamado")
	}
	if savedConfig.OAuth2CallbackPort != port {
		t.Errorf("porta persistida: got %d, want %d", savedConfig.OAuth2CallbackPort, port)
	}
}

func TestDCRDoesNotOverwriteFixedPort(t *testing.T) {
	called := false

	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2CallbackPort: 3118,
		},
		serverSlug: "test",
		onConfigUpdate: func(cfg ServerConfig) {
			called = true
		},
	}

	// Simula a lógica: porta fixa já existe, não deve sobrescrever
	if rt.cfg.OAuth2CallbackPort == 0 {
		rt.cfg.OAuth2CallbackPort = 9999
		if rt.onConfigUpdate != nil {
			rt.onConfigUpdate(rt.cfg)
		}
	}

	if called {
		t.Error("onConfigUpdate não deveria ter sido chamado quando porta fixa já existe")
	}
	if rt.cfg.OAuth2CallbackPort != 3118 {
		t.Errorf("porta não deveria mudar: got %d, want 3118", rt.cfg.OAuth2CallbackPort)
	}
}

func TestCallbackListenerBinds(t *testing.T) {
	tests := []struct {
		name         string
		callbackHost string
	}{
		{"localhost binds to 127.0.0.1", "localhost"},
		{"127.0.0.1 binds to 127.0.0.1", "127.0.0.1"},
		{"empty binds to 127.0.0.1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, listenIP := resolveCallbackHost(tc.callbackHost)
			addr := listenIP + ":0"

			listener, err := net.Listen("tcp", addr)
			if err != nil {
				t.Fatalf("net.Listen(%q) failed: %v", addr, err)
			}
			defer func() { _ = listener.Close() }()

			tcpAddr := listener.Addr().(*net.TCPAddr)
			if tcpAddr.Port == 0 {
				t.Error("expected non-zero port from listener")
			}
		})
	}
}

func TestHostnameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://api.example.com/path", "api.example.com"},
		{"http://localhost:8080/api", "localhost"},
		{"", ""},
		{"not-a-url", ""},
		{"https://my-server.com:443/v1", "my-server.com"},
	}
	for _, tc := range tests {
		got := hostnameFromURL(tc.url)
		if got != tc.want {
			t.Errorf("hostnameFromURL(%q): got %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestGenerateStateUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s := generateState()
		if s == "" {
			t.Fatal("generateState retornou string vazia")
		}
		if seen[s] {
			t.Fatalf("generateState gerou valor duplicado: %s", s)
		}
		seen[s] = true
	}
}

func TestClientCredPatternFormat(t *testing.T) {
	if got := clientCredPattern("my-server"); got != "mcp-client:my-server" {
		t.Errorf("clientCredPattern: got %q, want %q", got, "mcp-client:my-server")
	}
	if got := userTokensPattern("my-server"); got != "mcp-tokens:my-server" {
		t.Errorf("userTokensPattern: got %q, want %q", got, "mcp-tokens:my-server")
	}
}

func TestPersistAndLoadClientCreds(t *testing.T) {
	credMgr := newTestCredMgr()

	rt := &pkceRoundTripper{
		credMgr:    credMgr,
		serverSlug: "test-server",
	}

	ctx := context.Background()
	rt.persistClientCreds(ctx, "my-client-id", "my-client-secret")

	cid, csec := loadClientCreds(ctx, credMgr, "test-server")
	if cid != "my-client-id" {
		t.Errorf("ClientID: got %q, want %q", cid, "my-client-id")
	}
	if csec != "my-client-secret" {
		t.Errorf("ClientSecret: got %q, want %q", csec, "my-client-secret")
	}
}

func TestBuildPKCEHTTPClient_AutoImportsClientIDFromConfig(t *testing.T) {
	credMgr := newTestCredMgr()
	ctx := context.Background()

	cid, _ := loadClientCreds(ctx, credMgr, "slack")
	if cid != "" {
		t.Fatal("precondition: cred manager should be empty")
	}

	cfg := ServerConfig{
		URL:            "https://mcp.slack.com/mcp",
		OAuth2ClientID: "pre-registered-id",
		OAuth2AuthURL:  "https://slack.com/oauth/v2_user/authorize",
		OAuth2TokenURL: "https://slack.com/api/oauth.v2.user.access",
	}
	_ = buildPKCEHTTPClient(cfg, credMgr, nil, "slack", nil, nil)

	cid, _ = loadClientCreds(ctx, credMgr, "slack")
	if cid != "pre-registered-id" {
		t.Errorf("expected client_id to be auto-imported, got %q", cid)
	}
}

func TestBuildPKCEHTTPClient_DoesNotOverwriteExistingCreds(t *testing.T) {
	credMgr := newTestCredMgr()
	ctx := context.Background()

	rt := &pkceRoundTripper{credMgr: credMgr, serverSlug: "test"}
	rt.persistClientCreds(ctx, "existing-id", "existing-secret")

	cfg := ServerConfig{
		URL:            "https://example.com/mcp",
		OAuth2ClientID: "config-id-should-be-ignored",
		OAuth2AuthURL:  "https://example.com/auth",
		OAuth2TokenURL: "https://example.com/token",
	}
	_ = buildPKCEHTTPClient(cfg, credMgr, nil, "test", nil, nil)

	cid, csec := loadClientCreds(ctx, credMgr, "test")
	if cid != "existing-id" {
		t.Errorf("expected cred manager value to be preserved, got %q", cid)
	}
	if csec != "existing-secret" {
		t.Errorf("expected cred manager secret to be preserved, got %q", csec)
	}
}

func TestPersistAndLoadUserTokens(t *testing.T) {
	credMgr := newTestCredMgr()

	rt := &pkceRoundTripper{
		credMgr:    credMgr,
		serverSlug: "test-server",
	}

	token := &oauth2Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
	}
	ctx := context.Background()
	rt.persistTokens(ctx, toOAuth2Token(token))

	loaded := loadUserTokens(ctx, credMgr, "test-server")
	if loaded == nil {
		t.Fatal("loadUserTokens retornou nil")
	}
	if loaded.AccessToken != "access-123" {
		t.Errorf("AccessToken: got %q, want %q", loaded.AccessToken, "access-123")
	}
	if loaded.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken: got %q, want %q", loaded.RefreshToken, "refresh-456")
	}
}

func TestLoadClientCreds_NilManager(t *testing.T) {
	cid, csec := loadClientCreds(context.Background(), nil, "test")
	if cid != "" || csec != "" {
		t.Errorf("Esperado strings vazias para credMgr nil, got %q, %q", cid, csec)
	}
}

func TestLoadUserTokens_NilManager(t *testing.T) {
	token := loadUserTokens(context.Background(), nil, "test")
	if token != nil {
		t.Errorf("Esperado nil para credMgr nil, got %+v", token)
	}
}

func TestLoadUserTokens_NoEntry(t *testing.T) {
	credMgr := newTestCredMgr()
	token := loadUserTokens(context.Background(), credMgr, "nonexistent")
	if token != nil {
		t.Errorf("Esperado nil para servidor inexistente, got %+v", token)
	}
}

func TestEffectiveClientID(t *testing.T) {
	rt := &pkceRoundTripper{
		cfg:              ServerConfig{OAuth2ClientID: "config-id"},
		resolvedClientID: "resolved-id",
	}
	if got := rt.effectiveClientID(); got != "config-id" {
		t.Errorf("effectiveClientID com config: got %q, want 'config-id'", got)
	}

	rt.cfg.OAuth2ClientID = ""
	if got := rt.effectiveClientID(); got != "resolved-id" {
		t.Errorf("effectiveClientID sem config: got %q, want 'resolved-id'", got)
	}
}

func TestEffectiveClientSecret(t *testing.T) {
	rt := &pkceRoundTripper{
		resolvedClientSecret: "the-secret",
	}
	if got := rt.effectiveClientSecret(); got != "the-secret" {
		t.Errorf("effectiveClientSecret: got %q, want 'the-secret'", got)
	}
}

func TestIsMethodNotFound(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("connection refused"), false},
		{fmt.Errorf("timeout waiting for response"), false},
		{fmt.Errorf(`calling "ping": Method not found: ping`), true},
		{fmt.Errorf("method not found"), true},
		{fmt.Errorf("JSON-RPC error -32601: method not found"), true},
		{fmt.Errorf("code -32601"), true},
		{fmt.Errorf("METHOD NOT FOUND"), true},
	}
	for _, tc := range tests {
		got := isMethodNotFound(tc.err)
		if got != tc.want {
			t.Errorf("isMethodNotFound(%v): got %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestSessionExpiredError(t *testing.T) {
	err404 := &SessionExpiredError{StatusCode: 404}
	if err404.Error() != "mcp session expired (HTTP 404)" {
		t.Errorf("unexpected message: %s", err404.Error())
	}

	err410 := &SessionExpiredError{StatusCode: 410}
	if err410.Error() != "mcp session expired (HTTP 410)" {
		t.Errorf("unexpected message: %s", err410.Error())
	}

	var target *SessionExpiredError
	wrapped := fmt.Errorf("outer: %w", err404)
	if !errors.As(wrapped, &target) {
		t.Error("errors.As should unwrap SessionExpiredError from wrapped error")
	}
	if target.StatusCode != 404 {
		t.Errorf("expected StatusCode 404, got %d", target.StatusCode)
	}
}

func TestIsSessionExpiredStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{401, false},
		{403, false},
		{404, true},
		{410, true},
		{500, false},
	}
	for _, tc := range tests {
		got := isSessionExpiredStatus(tc.code)
		if got != tc.want {
			t.Errorf("isSessionExpiredStatus(%d): got %v, want %v", tc.code, got, tc.want)
		}
	}
}

// ============ Discovery Tests ============

func TestDiscoverOAuthEndpoints(t *testing.T) {
	asSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint":        "https://auth.example.com/authorize",
				"token_endpoint":                "https://auth.example.com/token",
				"registration_endpoint":         "https://auth.example.com/register",
				"device_authorization_endpoint": "https://auth.example.com/device/authorize",
				"grant_types_supported":         []string{"authorization_code", "urn:ietf:params:oauth:grant-type:device_code"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer asSrv.Close()

	var prmURL string
	prmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              prmURL + "/mcp",
				"authorization_servers": []string{asSrv.URL},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer prmSrv.Close()
	prmURL = prmSrv.URL

	disc, err := discoverOAuthEndpoints(prmSrv.URL + "/mcp")
	if err != nil {
		t.Fatalf("discoverOAuthEndpoints failed: %v", err)
	}

	if disc.AuthorizationEndpoint != "https://auth.example.com/authorize" {
		t.Errorf("AuthorizationEndpoint: got %q", disc.AuthorizationEndpoint)
	}
	if disc.TokenEndpoint != "https://auth.example.com/token" {
		t.Errorf("TokenEndpoint: got %q", disc.TokenEndpoint)
	}
	if disc.RegistrationEndpoint != "https://auth.example.com/register" {
		t.Errorf("RegistrationEndpoint: got %q", disc.RegistrationEndpoint)
	}
	if disc.DeviceAuthorizationEndpoint != "https://auth.example.com/device/authorize" {
		t.Errorf("DeviceAuthorizationEndpoint: got %q", disc.DeviceAuthorizationEndpoint)
	}
	if !strings.HasSuffix(disc.Resource, "/mcp") {
		t.Errorf("Resource: got %q, want suffix /mcp", disc.Resource)
	}
}

func TestDiscoverOAuthEndpoints_Fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := discoverOAuthEndpoints(srv.URL + "/mcp")
	if err == nil {
		t.Error("expected error when discovery returns 404")
	}

	// Config manual should still work (tested through authorize chain)
	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			URL:            srv.URL + "/mcp",
			OAuth2AuthURL:  "https://manual.example.com/authorize",
			OAuth2TokenURL: "https://manual.example.com/token",
			OAuth2ClientID: "manual-client",
		},
		serverSlug: "test",
	}

	rt.mergeDiscovery()

	if rt.cfg.OAuth2AuthURL != "https://manual.example.com/authorize" {
		t.Errorf("manual auth URL should not be overwritten: got %q", rt.cfg.OAuth2AuthURL)
	}
}

// ============ Discovery Fallback Tests ============

func TestBuildPRMCandidates(t *testing.T) {
	tests := []struct {
		name      string
		mcpURL    string
		wantCount int
		wantFirst string
		wantLast  string
	}{
		{
			name:      "URL with path tries resource first, then origin",
			mcpURL:    "https://example.com/mcp/default",
			wantCount: 2,
			wantFirst: "https://example.com/mcp/default/.well-known/oauth-protected-resource",
			wantLast:  "https://example.com/.well-known/oauth-protected-resource",
		},
		{
			name:      "URL at root only tries origin",
			mcpURL:    "https://example.com",
			wantCount: 1,
			wantFirst: "https://example.com/.well-known/oauth-protected-resource",
			wantLast:  "https://example.com/.well-known/oauth-protected-resource",
		},
		{
			name:      "URL with single path segment",
			mcpURL:    "https://example.com/mcp",
			wantCount: 2,
			wantFirst: "https://example.com/mcp/.well-known/oauth-protected-resource",
			wantLast:  "https://example.com/.well-known/oauth-protected-resource",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidates := buildPRMCandidates(tc.mcpURL)
			if len(candidates) != tc.wantCount {
				t.Fatalf("got %d candidates, want %d: %v", len(candidates), tc.wantCount, candidates)
			}
			if candidates[0] != tc.wantFirst {
				t.Errorf("first candidate: got %q, want %q", candidates[0], tc.wantFirst)
			}
			if candidates[len(candidates)-1] != tc.wantLast {
				t.Errorf("last candidate: got %q, want %q", candidates[len(candidates)-1], tc.wantLast)
			}
		})
	}
}

func TestBuildASMCandidates(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		wantCount int
		wantURLs  []string
	}{
		{
			name:      "auth server at origin has 2 candidates",
			base:      "https://example.com",
			wantCount: 2,
			wantURLs: []string{
				"https://example.com/.well-known/oauth-authorization-server",
				"https://example.com/.well-known/openid-configuration",
			},
		},
		{
			name:      "auth server with path generates 6 candidates",
			base:      "https://example.com/oauth",
			wantCount: 6,
			wantURLs: []string{
				"https://example.com/oauth/.well-known/oauth-authorization-server",
				"https://example.com/oauth/.well-known/openid-configuration",
				"https://example.com/.well-known/oauth-authorization-server/oauth",
				"https://example.com/.well-known/openid-configuration/oauth",
				"https://example.com/.well-known/oauth-authorization-server",
				"https://example.com/.well-known/openid-configuration",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidates := buildASMCandidates(tc.base)
			if len(candidates) != tc.wantCount {
				t.Fatalf("got %d candidates, want %d: %v", len(candidates), tc.wantCount, candidates)
			}
			for i, want := range tc.wantURLs {
				if candidates[i] != want {
					t.Errorf("candidate[%d]: got %q, want %q", i, candidates[i], want)
				}
			}
		})
	}
}

func TestDiscoverOAuth_AuthServerWithPath_FallbackToOriginRoot(t *testing.T) {
	// PRM aponta para auth server em /oauth,
	// mas ASM está publicado na raiz do origin.
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              "https://example.com",
				"resource_name":         "TestService",
				"authorization_servers": []string{srvURL + "/oauth"},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                           srvURL + "/oauth",
				"authorization_endpoint":           srvURL + "/oauth/authorize",
				"token_endpoint":                   srvURL + "/oauth/token",
				"registration_endpoint":            srvURL + "/oauth/register",
				"code_challenge_methods_supported": []string{"S256"},
				"grant_types_supported":            []string{"authorization_code", "refresh_token"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	result := DiscoverOAuth(srv.URL + "/mcp/default")
	if !result.Found {
		t.Fatalf("expected discovery to succeed, got error: %s", result.Error)
	}
	if result.AuthURL != srv.URL+"/oauth/authorize" {
		t.Errorf("AuthURL: got %q", result.AuthURL)
	}
	if result.TokenURL != srv.URL+"/oauth/token" {
		t.Errorf("TokenURL: got %q", result.TokenURL)
	}
	if result.RegistrationURL != srv.URL+"/oauth/register" {
		t.Errorf("RegistrationURL: got %q", result.RegistrationURL)
	}
	if result.ResourceName != "TestService" {
		t.Errorf("ResourceName: got %q", result.ResourceName)
	}
	if !result.SupportsPKCE {
		t.Error("expected SupportsPKCE=true")
	}
}

func TestDiscoverOAuth_PRMOnlyAtOrigin(t *testing.T) {
	// PRM não existe em /mcp/.well-known/..., só no origin root.
	asSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint":           "https://auth.test.com/authorize",
				"token_endpoint":                   "https://auth.test.com/token",
				"code_challenge_methods_supported": []string{"S256"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer asSrv.Close()

	var prmSrvURL string
	prmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              prmSrvURL + "/mcp",
				"authorization_servers": []string{asSrv.URL},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer prmSrv.Close()
	prmSrvURL = prmSrv.URL

	result := DiscoverOAuth(prmSrv.URL + "/mcp")
	if !result.Found {
		t.Fatalf("expected discovery to succeed, got error: %s", result.Error)
	}
	if result.AuthURL != "https://auth.test.com/authorize" {
		t.Errorf("AuthURL: got %q", result.AuthURL)
	}
	if result.TokenURL != "https://auth.test.com/token" {
		t.Errorf("TokenURL: got %q", result.TokenURL)
	}
}

func TestDiscoverOAuth_PRMAtResourcePath(t *testing.T) {
	// PRM existe no path do recurso (ex: /api/.well-known/oauth-protected-resource)
	asSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": "https://auth.path.com/authorize",
				"token_endpoint":         "https://auth.path.com/token",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer asSrv.Close()

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              srvURL + "/api",
				"authorization_servers": []string{asSrv.URL},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	srvURL = srv.URL

	disc, err := discoverOAuthEndpoints(srv.URL + "/api")
	if err != nil {
		t.Fatalf("expected discovery to succeed: %v", err)
	}
	if disc.AuthorizationEndpoint != "https://auth.path.com/authorize" {
		t.Errorf("AuthorizationEndpoint: got %q", disc.AuthorizationEndpoint)
	}
}

func TestDiscoverOAuth_ASMAtRFC8414PathLocation(t *testing.T) {
	// Auth server metadata publicado em {origin}/.well-known/oauth-authorization-server{path}
	// (RFC 8414 §3 correto para issuer com path)
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              srvURL,
				"authorization_servers": []string{srvURL + "/auth"},
			})
		case "/.well-known/oauth-authorization-server/auth":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srvURL + "/auth",
				"authorization_endpoint": srvURL + "/auth/authorize",
				"token_endpoint":         srvURL + "/auth/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	disc, err := discoverOAuthEndpoints(srv.URL + "/mcp")
	if err != nil {
		t.Fatalf("expected discovery to succeed: %v", err)
	}
	if disc.AuthorizationEndpoint != srv.URL+"/auth/authorize" {
		t.Errorf("AuthorizationEndpoint: got %q", disc.AuthorizationEndpoint)
	}
	if disc.TokenEndpoint != srv.URL+"/auth/token" {
		t.Errorf("TokenEndpoint: got %q", disc.TokenEndpoint)
	}
}

func TestDiscoverOAuth_TotalFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := DiscoverOAuth(srv.URL + "/mcp/default")
	if result.Found {
		t.Error("expected discovery to fail when all endpoints return 404")
	}
	if result.Error == "" {
		t.Error("expected diagnostic error message on total failure")
	}
}

func TestDiscoverOAuth_ASMDirectAtBase(t *testing.T) {
	// Auth server sem path — ASM no próprio base (comportamento original)
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              srvURL,
				"authorization_servers": []string{srvURL},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srvURL,
				"authorization_endpoint": srvURL + "/authorize",
				"token_endpoint":         srvURL + "/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	result := DiscoverOAuth(srv.URL + "/mcp")
	if !result.Found {
		t.Fatalf("expected discovery to succeed, got error: %s", result.Error)
	}
	if result.AuthURL != srv.URL+"/authorize" {
		t.Errorf("AuthURL: got %q", result.AuthURL)
	}
	if result.TokenURL != srv.URL+"/token" {
		t.Errorf("TokenURL: got %q", result.TokenURL)
	}
}

// ============ Device Flow Tests ============

func TestAuthorizeDeviceFlow_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device/authorize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "DEV-CODE-123",
				"user_code":                 "ABCD-1234",
				"verification_uri":          "https://auth.example.com/verify",
				"verification_uri_complete": "https://auth.example.com/verify?user_code=ABCD-1234",
				"expires_in":                300,
				"interval":                  1,
			})
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "device-access-token",
				"token_type":    "Bearer",
				"refresh_token": "device-refresh-token",
				"expires_in":    3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2DeviceAuthURL: srv.URL + "/device/authorize",
			OAuth2TokenURL:      srv.URL + "/token",
			OAuth2AuthURL:       "https://auth.example.com/authorize",
		},
		credMgr:          credentials.NewManager(nil),
		serverSlug:       "test-device",
		resolvedClientID: "test-client",
		resourceURL:      "https://mcp.example.com/mcp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := rt.authorizeDeviceFlow(ctx)
	if err != nil {
		t.Fatalf("authorizeDeviceFlow failed: %v", err)
	}

	if rt.tokenSource == nil {
		t.Error("tokenSource should be set after successful device flow")
	}
	if rt.oauthCfg == nil {
		t.Error("oauthCfg should be set after successful device flow")
	}
}

func TestAuthorizeDeviceFlow_SlowDown(t *testing.T) {
	pollCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device/authorize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "DEV-CODE",
				"user_code":        "SLOW-1234",
				"verification_uri": "https://example.com/verify",
				"expires_in":       300,
				"interval":         1,
			})
		case "/token":
			pollCount++
			if pollCount == 1 {
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "slow-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		}
	}))
	defer srv.Close()

	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2DeviceAuthURL: srv.URL + "/device/authorize",
			OAuth2TokenURL:      srv.URL + "/token",
			OAuth2AuthURL:       "https://example.com/authorize",
		},
		credMgr:          credentials.NewManager(nil),
		serverSlug:       "test-slow",
		resolvedClientID: "test-client",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := rt.authorizeDeviceFlow(ctx)
	if err != nil {
		t.Fatalf("authorizeDeviceFlow with slow_down failed: %v", err)
	}

	if pollCount < 2 {
		t.Errorf("expected at least 2 polls (got %d), first should be slow_down", pollCount)
	}
}

func TestAuthorizeDeviceFlow_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device/authorize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "DEV-TIMEOUT",
				"user_code":        "TIMEOUT-1",
				"verification_uri": "https://example.com/verify",
				"expires_in":       2,
				"interval":         1,
			})
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		}
	}))
	defer srv.Close()

	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2DeviceAuthURL: srv.URL + "/device/authorize",
			OAuth2TokenURL:      srv.URL + "/token",
			OAuth2AuthURL:       "https://example.com/authorize",
		},
		credMgr:          credentials.NewManager(nil),
		serverSlug:       "test-timeout",
		resolvedClientID: "test-client",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := rt.authorizeDeviceFlow(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// ============ DCR Tests ============

func TestDCRIncludesDeviceCodeGrant(t *testing.T) {
	var receivedGrants []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if grants, ok := body["grant_types"].([]any); ok {
			for _, g := range grants {
				receivedGrants = append(receivedGrants, g.(string))
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"client_id": "dcr-test-client",
		})
	}))
	defer srv.Close()

	cfg := ServerConfig{
		OAuth2RegistrationURL: srv.URL + "/register",
	}
	_, err := registerDynamicClient(cfg, "http://localhost:9999/callback", nil)
	if err != nil {
		t.Fatalf("registerDynamicClient failed: %v", err)
	}

	found := false
	for _, g := range receivedGrants {
		if g == "urn:ietf:params:oauth:grant-type:device_code" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DCR request should include device_code grant type. Got: %v", receivedGrants)
	}
}

// ============ Resource Param Tests ============

func TestResourceParamInAuthURL(t *testing.T) {
	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2AuthURL:  "https://auth.example.com/authorize",
			OAuth2TokenURL: "https://auth.example.com/token",
		},
		resourceURL: "https://mcp.example.com/mcp",
	}

	oauthCfg := &oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			AuthURL:  rt.cfg.OAuth2AuthURL,
			TokenURL: rt.cfg.OAuth2TokenURL,
		},
		RedirectURL: "http://localhost:9999/callback",
	}

	codeVerifier := oauth2.GenerateVerifier()
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(codeVerifier)}
	if rt.resourceURL != "" {
		opts = append(opts, oauth2.SetAuthURLParam("resource", rt.resourceURL))
	}
	authURL := oauthCfg.AuthCodeURL("test-state", opts...)

	if !strings.Contains(authURL, "resource=") {
		t.Errorf("auth URL should contain resource param: %s", authURL)
	}
	if !strings.Contains(authURL, "mcp.example.com") {
		t.Errorf("auth URL should contain the resource URL: %s", authURL)
	}
}

// ============ MergeDiscovery Tests ============

func TestMergeDiscovery_FillsEmpty(t *testing.T) {
	rt := &pkceRoundTripper{
		cfg: ServerConfig{},
		discovery: &OAuthDiscovery{
			Resource:                    "https://mcp.example.com/mcp",
			AuthorizationEndpoint:       "https://auth.example.com/authorize",
			TokenEndpoint:               "https://auth.example.com/token",
			RegistrationEndpoint:        "https://auth.example.com/register",
			DeviceAuthorizationEndpoint: "https://auth.example.com/device/authorize",
		},
	}

	rt.mergeDiscovery()

	if rt.resourceURL != "https://mcp.example.com/mcp" {
		t.Errorf("resourceURL: got %q", rt.resourceURL)
	}
	if rt.cfg.OAuth2AuthURL != "https://auth.example.com/authorize" {
		t.Errorf("OAuth2AuthURL: got %q", rt.cfg.OAuth2AuthURL)
	}
	if rt.cfg.OAuth2TokenURL != "https://auth.example.com/token" {
		t.Errorf("OAuth2TokenURL: got %q", rt.cfg.OAuth2TokenURL)
	}
	if rt.cfg.OAuth2RegistrationURL != "https://auth.example.com/register" {
		t.Errorf("OAuth2RegistrationURL: got %q", rt.cfg.OAuth2RegistrationURL)
	}
	if rt.cfg.OAuth2DeviceAuthURL != "https://auth.example.com/device/authorize" {
		t.Errorf("OAuth2DeviceAuthURL: got %q", rt.cfg.OAuth2DeviceAuthURL)
	}
}

func TestMergeDiscovery_ManualHasPriority(t *testing.T) {
	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2AuthURL:  "https://manual.example.com/authorize",
			OAuth2TokenURL: "https://manual.example.com/token",
		},
		resourceURL: "https://manual-resource.example.com/mcp",
		discovery: &OAuthDiscovery{
			Resource:              "https://discovered.example.com/mcp",
			AuthorizationEndpoint: "https://discovered.example.com/authorize",
			TokenEndpoint:         "https://discovered.example.com/token",
		},
	}

	rt.mergeDiscovery()

	if rt.resourceURL != "https://manual-resource.example.com/mcp" {
		t.Errorf("manual resourceURL should not be overwritten: got %q", rt.resourceURL)
	}
	if rt.cfg.OAuth2AuthURL != "https://manual.example.com/authorize" {
		t.Errorf("manual OAuth2AuthURL should not be overwritten: got %q", rt.cfg.OAuth2AuthURL)
	}
	if rt.cfg.OAuth2TokenURL != "https://manual.example.com/token" {
		t.Errorf("manual OAuth2TokenURL should not be overwritten: got %q", rt.cfg.OAuth2TokenURL)
	}
}

func TestMergeDiscovery_NilDiscovery(t *testing.T) {
	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2AuthURL: "https://existing.example.com/authorize",
		},
	}

	rt.mergeDiscovery()

	if rt.cfg.OAuth2AuthURL != "https://existing.example.com/authorize" {
		t.Errorf("config should be unchanged when discovery is nil: got %q", rt.cfg.OAuth2AuthURL)
	}
}

// ============ offline_access / scopes (#193) ============

func TestEffectiveScopes_AddsOfflineAccessWhenSupported(t *testing.T) {
	rt := &pkceRoundTripper{
		serverSlug: "atlassian",
		cfg:        ServerConfig{OAuth2Scopes: []string{"read:jira-work"}},
		discovery:  &OAuthDiscovery{ScopesSupported: []string{"read:jira-work", "offline_access"}},
	}
	got := rt.effectiveScopes()
	if !containsFold(got, "offline_access") {
		t.Errorf("esperava offline_access nos scopes, got %v", got)
	}
}

func TestEffectiveScopes_NotAddedWhenUnsupported(t *testing.T) {
	rt := &pkceRoundTripper{
		cfg:       ServerConfig{OAuth2Scopes: []string{"read"}},
		discovery: &OAuthDiscovery{ScopesSupported: []string{"read", "write"}},
	}
	if containsFold(rt.effectiveScopes(), "offline_access") {
		t.Error("não deveria adicionar offline_access quando o servidor não o anuncia")
	}
}

func TestEffectiveScopes_NoDiscoveryDoesNotAdd(t *testing.T) {
	rt := &pkceRoundTripper{cfg: ServerConfig{OAuth2Scopes: []string{"read"}}}
	if containsFold(rt.effectiveScopes(), "offline_access") {
		t.Error("sem discovery (config manual) não deve adicionar offline_access")
	}
}

func TestEffectiveScopes_NoDuplicateWhenAlreadyConfigured(t *testing.T) {
	rt := &pkceRoundTripper{
		cfg:       ServerConfig{OAuth2Scopes: []string{"offline_access", "read"}},
		discovery: &OAuthDiscovery{ScopesSupported: []string{"offline_access"}},
	}
	count := 0
	for _, s := range rt.effectiveScopes() {
		if strings.EqualFold(s, "offline_access") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("offline_access não deveria duplicar, got %v", rt.effectiveScopes())
	}
}

// ============ persistTokens robustez (#193) ============

func TestPersistTokens_PreservesRefreshTokenWhenEmpty(t *testing.T) {
	credMgr := newTestCredMgr()
	rt := &pkceRoundTripper{credMgr: credMgr, serverSlug: "srv"}
	ctx := context.Background()

	rt.persistTokens(ctx, &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"})
	// Refresh non-rotativo: novo access_token, refresh_token ausente na resposta.
	rt.persistTokens(ctx, &oauth2.Token{AccessToken: "a2"})

	loaded := loadUserTokens(ctx, credMgr, "srv")
	if loaded == nil {
		t.Fatal("loadUserTokens retornou nil")
	}
	if loaded.RefreshToken != "r1" {
		t.Errorf("refresh_token deveria ser preservado: got %q want r1", loaded.RefreshToken)
	}
	if loaded.AccessToken != "a2" {
		t.Errorf("access_token deveria atualizar: got %q want a2", loaded.AccessToken)
	}
}

func TestPersistTokens_PersistsExpiry(t *testing.T) {
	credMgr := newTestCredMgr()
	rt := &pkceRoundTripper{credMgr: credMgr, serverSlug: "srv"}
	ctx := context.Background()

	exp := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	rt.persistTokens(ctx, &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: exp})

	loaded := loadUserTokens(ctx, credMgr, "srv")
	if loaded == nil {
		t.Fatal("loadUserTokens retornou nil")
	}
	if loaded.Expiry.Unix() != exp.Unix() {
		t.Errorf("expiry deveria ser persistida: got %v want %v", loaded.Expiry, exp)
	}
}

func TestTrySilentRefresh_UsesStoreRefreshTokenWhenMemoryLacksIt(t *testing.T) {
	// Refresh non-rotativo: o token em memória não tem refresh_token, mas o store
	// tem. O reload-before-refresh deve recarregar do store em vez de falhar com
	// "no refresh token available" (issue #193).
	var gotRefresh string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"token_type":    "Bearer",
			"refresh_token": "rotated-refresh",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	credMgr := newTestCredMgr()
	ctx := context.Background()
	rt := &pkceRoundTripper{credMgr: credMgr, serverSlug: "srv"}
	rt.persistTokens(ctx, &oauth2.Token{AccessToken: "old-access", RefreshToken: "stored-refresh"})

	rt.oauthCfg = &oauth2.Config{
		ClientID: "c",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	}
	// Token em memória SEM refresh_token (caminho que antes retornava cedo).
	rt.tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "old-access"})

	if err := rt.trySilentRefresh(ctx); err != nil {
		t.Fatalf("trySilentRefresh deveria suceder usando o refresh do store: %v", err)
	}
	if gotRefresh != "stored-refresh" {
		t.Errorf("deveria usar o refresh_token do store, got %q", gotRefresh)
	}
	loaded := loadUserTokens(ctx, credMgr, "srv")
	if loaded == nil || loaded.AccessToken != "new-access" {
		t.Errorf("novo access_token deveria ser persistido, got %+v", loaded)
	}
	if loaded.RefreshToken != "rotated-refresh" {
		t.Errorf("refresh rotacionado deveria persistir, got %q", loaded.RefreshToken)
	}
}

// ============ single-flight por servidor (#194) ============

// seqTokenSource devolve tokens em sequência: simula que, entre a captura do token
// rejeitado e a reverificação pós-arbiter, OUTRO flow trocou o token.
type seqTokenSource struct {
	mu     sync.Mutex
	tokens []*oauth2.Token
	i      int
}

func (s *seqTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok := s.tokens[s.i]
	if s.i < len(s.tokens)-1 {
		s.i++
	}
	return tok, nil
}

func TestAuthorizeSkipsWhenAnotherFlowRenewedToken(t *testing.T) {
	// Caso #194: ao adquirir o arbiter, o token foi substituído por OUTRO flow
	// (access_token diferente do rejeitado) — authorize pula a nova janela.
	rt := &pkceRoundTripper{
		serverSlug: "srv",
		tokenSource: &seqTokenSource{tokens: []*oauth2.Token{
			{AccessToken: "old-rejected", Expiry: time.Now().Add(time.Hour)},
			{AccessToken: "new-from-other-flow", Expiry: time.Now().Add(time.Hour)},
		}},
	}
	if err := rt.authorize(context.Background()); err != nil {
		t.Fatalf("authorize deveria pular e retornar nil quando outro flow renovou, got %v", err)
	}
}

func TestAuthorizeProceedsWhenTokenUnchanged(t *testing.T) {
	// Token rejeitado continua o mesmo (ex.: revogado, não-expirado): NÃO pode pular
	// — o usuário precisa reautenticar. Sem config válida, authorize deve falhar ao
	// tentar de fato o flow (prova que não tomou o atalho do single-flight).
	rt := &pkceRoundTripper{
		serverSlug:  "srv",
		tokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "still-rejected", Expiry: time.Now().Add(time.Hour)}),
	}
	err := rt.authorize(context.Background())
	if err == nil {
		t.Fatal("authorize não deveria pular quando o token rejeitado permanece; deveria prosseguir e falhar sem config")
	}
}
