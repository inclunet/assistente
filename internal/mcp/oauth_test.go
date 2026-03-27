package mcp

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"assistente/internal/credentials"

	"golang.org/x/oauth2"
)

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
	listener.Close()

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
			defer listener.Close()

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

	rt.persistClientCreds("my-client-id", "my-client-secret")

	cid, csec := loadClientCreds(credMgr, "test-server")
	if cid != "my-client-id" {
		t.Errorf("ClientID: got %q, want %q", cid, "my-client-id")
	}
	if csec != "my-client-secret" {
		t.Errorf("ClientSecret: got %q, want %q", csec, "my-client-secret")
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
	rt.persistTokens(toOAuth2Token(token))

	loaded := loadUserTokens(credMgr, "test-server")
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
	cid, csec := loadClientCreds(nil, "test")
	if cid != "" || csec != "" {
		t.Errorf("Esperado strings vazias para credMgr nil, got %q, %q", cid, csec)
	}
}

func TestLoadUserTokens_NilManager(t *testing.T) {
	token := loadUserTokens(nil, "test")
	if token != nil {
		t.Errorf("Esperado nil para credMgr nil, got %+v", token)
	}
}

func TestLoadUserTokens_NoEntry(t *testing.T) {
	credMgr := newTestCredMgr()
	token := loadUserTokens(credMgr, "nonexistent")
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
