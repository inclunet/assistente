package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/oauthflow"

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
			wantAddr: "[::1]:0",
		},
		{
			name:     "IPv6 fixed port",
			listenIP: "::1",
			port:     8080,
			wantAddr: "[::1]:8080",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := callbackListenAddr(tc.listenIP, tc.port)

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
	// Preserve the dynamic-port regression through real DCR/checkpoint/reload,
	// instead of duplicating the removed writer's if statement in this test.
	testManagedDCRCallbackPersistence(t, 0)
}

func TestDCRDoesNotOverwriteFixedPort(t *testing.T) {
	// Select an available port, then configure it as fixed. The shared scenario
	// asserts the same port in DCR, checkpoint, final grant and after restart.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	testManagedDCRCallbackPersistence(t, port)
}

func TestAuthorizePKCEReregistersWhenFixedCallbackPortIsBusy(t *testing.T) {
	for _, initialClient := range []string{"old-client", ""} {
		t.Run("client="+initialClient, func(t *testing.T) {
			occupied, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("net.Listen failed: %v", err)
			}
			defer func() { _ = occupied.Close() }()
			occupiedPort := occupied.Addr().(*net.TCPAddr).Port

			var (
				mu                 sync.Mutex
				registeredRedirect string
				savedConfig        *ServerConfig
			)

			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/register":
					var req oauthflow.RegistrationRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("DCR body inválido: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if len(req.RedirectURIs) != 1 {
						t.Errorf("redirect_uris: got %v, want 1 item", req.RedirectURIs)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					mu.Lock()
					registeredRedirect = req.RedirectURIs[0]
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"client_id":"new-client"}`)
				case "/token":
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"access_token":"access","token_type":"Bearer","expires_in":3600}`)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer authServer.Close()

			oldBrowserOpen := browserOpen
			browserOpen = func(rawURL string) error {
				u, err := url.Parse(rawURL)
				if err != nil {
					return err
				}
				redirectURI := u.Query().Get("redirect_uri")
				state := u.Query().Get("state")
				resp, err := http.Get(redirectURI + "?code=ok&state=" + url.QueryEscape(state))
				if err != nil {
					return err
				}
				body, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					return err
				}
				_, nonceHTML, found := strings.Cut(string(body), `<style nonce="`)
				nonce, _, closed := strings.Cut(nonceHTML, `"`)
				policy := resp.Header.Get("Content-Security-Policy")
				if !found || !closed || len(nonce) < 32 || !strings.Contains(string(body), `<script nonce="`+nonce+`">`) || !strings.Contains(policy, "style-src 'nonce-"+nonce+"'") || !strings.Contains(policy, "script-src 'nonce-"+nonce+"'") || strings.Contains(string(body), "style=") {
					t.Error("MCP callback assets do not match the restricted CSP")
				}
				return nil
			}
			defer func() { browserOpen = oldBrowserOpen }()

			var rt *oauthProtocol
			rt = &oauthProtocol{
				cfg: ServerConfig{
					URL:                   authServer.URL,
					OAuth2ClientID:        initialClient,
					OAuth2AuthURL:         authServer.URL + "/authorize",
					OAuth2TokenURL:        authServer.URL + "/token",
					OAuth2CallbackPort:    occupiedPort,
					OAuth2CallbackHost:    "127.0.0.1",
					OAuth2RegistrationURL: authServer.URL + "/register",
				},
				serverSlug: "test",
				registrationCheckpoint: func() error {
					mu.Lock()
					defer mu.Unlock()
					c := rt.cfg
					savedConfig = &c
					return nil
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := rt.authorize(ctx); err != nil {
				t.Fatalf("authorizePKCE failed: %v", err)
			}

			if rt.cfg.OAuth2ClientID != "new-client" {
				t.Fatalf("client_id: got %q, want new-client", rt.cfg.OAuth2ClientID)
			}
			if rt.cfg.OAuth2CallbackPort == 0 || rt.cfg.OAuth2CallbackPort == occupiedPort {
				t.Fatalf("callback port não foi substituída: got %d, busy %d", rt.cfg.OAuth2CallbackPort, occupiedPort)
			}

			mu.Lock()
			defer mu.Unlock()
			if savedConfig == nil {
				t.Fatal("checkpoint de registro não foi chamado")
			}
			if savedConfig.OAuth2CallbackPort != rt.cfg.OAuth2CallbackPort {
				t.Fatalf("porta persistida: got %d, want %d", savedConfig.OAuth2CallbackPort, rt.cfg.OAuth2CallbackPort)
			}
			redirectURL, err := url.Parse(registeredRedirect)
			if err != nil {
				t.Fatalf("redirect_uri registrado inválido: %q: %v", registeredRedirect, err)
			}
			if redirectURL.Port() == "" || redirectURL.Port() == fmt.Sprint(occupiedPort) {
				t.Fatalf("redirect_uri registrado usou porta inválida: %q", registeredRedirect)
			}
		})
	}
}

func TestIsAddressInUseDetectsListenCollision(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer func() { _ = occupied.Close() }()

	second, err := net.Listen("tcp", occupied.Addr().String())
	if err == nil {
		_ = second.Close()
		t.Fatal("segundo listener deveria falhar com porta em uso")
	}
	if !isAddressInUse(err) {
		t.Fatalf("esperava erro de porta em uso, got %v", err)
	}
	if isAddressInUse(fmt.Errorf("host inválido")) {
		t.Fatal("erro genérico não deveria ser tratado como porta em uso")
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
			addr := callbackListenAddr(listenIP, 0)

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
	m, _, ctx := managedFixture(t)
	cfg := managedConfig("https://resource.example")
	cfg.OAuth2ClientID = "my-client-id"
	if err := m.SaveConfigWithOAuthSecret("test-server", cfg, "my-client-secret"); err != nil {
		t.Fatal(err)
	}
	if err := m.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	_, _, record := loadManaged(t, m, ctx, "test-server")
	cid, csec := record.Client.ID, record.Client.Secret
	if cid != "my-client-id" {
		t.Errorf("ClientID: got %q, want %q", cid, "my-client-id")
	}
	if csec != "my-client-secret" {
		t.Errorf("ClientSecret: got %q, want %q", csec, "my-client-secret")
	}
	if legacyID, _ := loadClientCreds(ctx, m.credMgr, "test-server"); legacyID != "" {
		t.Fatal("composed creation wrote a legacy client row")
	}
}

func TestManagedCreationStoresClientIDFromConfig(t *testing.T) {
	m, _, ctx := managedFixture(t)
	credMgr := m.credMgr

	cid, _ := loadClientCreds(ctx, credMgr, "slack")
	if cid != "" {
		t.Fatal("precondition: cred manager should be empty")
	}

	cfg := ServerConfig{
		Transport:      TransportStreamable,
		AuthType:       AuthOAuth2PKCE,
		OAuthManaged:   true,
		URL:            "https://mcp.slack.com/mcp",
		OAuth2ClientID: "pre-registered-id",
		OAuth2AuthURL:  "https://slack.com/oauth/v2_user/authorize",
		OAuth2TokenURL: "https://slack.com/api/oauth.v2.user.access",
	}
	if err := m.SaveConfig("slack", cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.LoadConfigs(); err != nil {
		t.Fatal(err)
	}

	_, _, record := loadManaged(t, m, ctx, "slack")
	cid = record.Client.ID
	if cid != "pre-registered-id" {
		t.Errorf("expected client_id in explicitly created authorization, got %q", cid)
	}
	if legacyID, _ := loadClientCreds(ctx, credMgr, "slack"); legacyID != "" {
		t.Fatal("explicit creation produced a legacy client row")
	}
}

func TestBuildPKCEHTTPClient_DoesNotOverwriteExistingCreds(t *testing.T) {
	credMgr := newTestCredMgr()
	ctx := context.Background()

	if err := credMgr.RegisterPatternWithContext(ctx, clientCredPattern("test"), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "existing-id", ClientSecret: "existing-secret"}); err != nil {
		t.Fatal(err)
	}

	cfg := ServerConfig{
		AuthType:       AuthOAuth2PKCE,
		URL:            "https://example.com/mcp",
		OAuth2ClientID: "config-id-should-be-ignored",
		OAuth2AuthURL:  "https://example.com/auth",
		OAuth2TokenURL: "https://example.com/token",
	}
	m := &Manager{credMgr: credMgr}
	client := m.buildAuthHTTPClient(ctx, "test", cfg)
	if err := clientGrantGet(client, cfg.URL); !errors.Is(err, errOAuthMigrationRequired) {
		t.Fatalf("legacy transport was allowed: %v", err)
	}

	cid, csec := loadClientCreds(ctx, credMgr, "test")
	if cid != "existing-id" {
		t.Errorf("expected cred manager value to be preserved, got %q", cid)
	}
	if csec != "existing-secret" {
		t.Errorf("expected cred manager secret to be preserved, got %q", csec)
	}
}

func TestPersistAndLoadUserTokens(t *testing.T) {
	m, _, ctx := managedFixture(t)
	if err := m.SaveConfig("test-server", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}

	token := &oauth2Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
	}
	cfg, store, _ := loadManaged(t, m, ctx, "test-server")
	_, _, service, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeUsing(ctx, store, cfg.OAuthAuthorizationID, func(_ context.Context, r oauthflow.Record) (oauthflow.Record, error) {
		r.Tokens = oauthflow.Tokens{Access: token.AccessToken, Refresh: token.RefreshToken, Type: "Bearer"}
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := m.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	_, _, record := loadManaged(t, m, ctx, "test-server")
	if record.Tokens.Access != "access-123" {
		t.Errorf("AccessToken: got %q, want %q", record.Tokens.Access, "access-123")
	}
	if record.Tokens.Refresh != "refresh-456" {
		t.Errorf("RefreshToken: got %q, want %q", record.Tokens.Refresh, "refresh-456")
	}
	if legacy := loadUserTokens(ctx, m.credMgr, "test-server"); legacy != nil {
		t.Fatal("composed authorization produced a legacy token row")
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
	rt := &oauthProtocol{
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
	rt := &oauthProtocol{
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
	var prmURL string
	asSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                        prmURL,
				"authorization_endpoint":        prmURL + "/authorize",
				"token_endpoint":                prmURL + "/token",
				"registration_endpoint":         prmURL + "/register",
				"device_authorization_endpoint": prmURL + "/device/authorize",
				"grant_types_supported":         []string{"authorization_code", "urn:ietf:params:oauth:grant-type:device_code"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer asSrv.Close()

	prmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              prmURL + "/mcp",
				"authorization_servers": []string{prmURL},
			})
			return
		}
		asSrv.Config.Handler.ServeHTTP(w, r)
	}))
	defer prmSrv.Close()
	prmURL = prmSrv.URL

	disc, err := discoverOAuthEndpoints(context.Background(), prmSrv.URL+"/mcp")
	if err != nil {
		t.Fatalf("discoverOAuthEndpoints failed: %v", err)
	}

	if disc.AuthorizationEndpoint != prmURL+"/authorize" {
		t.Errorf("AuthorizationEndpoint: got %q", disc.AuthorizationEndpoint)
	}
	if disc.TokenEndpoint != prmURL+"/token" {
		t.Errorf("TokenEndpoint: got %q", disc.TokenEndpoint)
	}
	if disc.RegistrationEndpoint != prmURL+"/register" {
		t.Errorf("RegistrationEndpoint: got %q", disc.RegistrationEndpoint)
	}
	if disc.DeviceAuthorizationEndpoint != prmURL+"/device/authorize" {
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

	_, err := discoverOAuthEndpoints(context.Background(), srv.URL+"/mcp")
	if err == nil {
		t.Error("expected error when discovery returns 404")
	}

	// Config manual should still work (tested through authorize chain)
	rt := &oauthProtocol{
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

func TestDiscoverOAuth_AuthServerWithPath_FallbackToOriginRoot(t *testing.T) {
	// PRM aponta para auth server em /oauth,
	// mas ASM está publicado na raiz do origin.
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              srvURL + "/mcp/default",
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
	var prmSrvURL string
	// PRM não existe em /mcp/.well-known/..., só no origin root.
	asSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                           prmSrvURL,
				"authorization_endpoint":           prmSrvURL + "/authorize",
				"token_endpoint":                   prmSrvURL + "/token",
				"code_challenge_methods_supported": []string{"S256"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer asSrv.Close()

	prmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              prmSrvURL + "/mcp",
				"authorization_servers": []string{prmSrvURL},
			})
			return
		}
		asSrv.Config.Handler.ServeHTTP(w, r)
	}))
	defer prmSrv.Close()
	prmSrvURL = prmSrv.URL

	result := DiscoverOAuth(prmSrv.URL + "/mcp")
	if !result.Found {
		t.Fatalf("expected discovery to succeed, got error: %s", result.Error)
	}
	if result.AuthURL != prmSrvURL+"/authorize" {
		t.Errorf("AuthURL: got %q", result.AuthURL)
	}
	if result.TokenURL != prmSrvURL+"/token" {
		t.Errorf("TokenURL: got %q", result.TokenURL)
	}
}

func TestDiscoverOAuth_PRMAtResourcePath(t *testing.T) {
	var srvURL string
	// PRM existe no path do recurso (ex: /api/.well-known/oauth-protected-resource)
	asSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srvURL,
				"authorization_endpoint": srvURL + "/authorize",
				"token_endpoint":         srvURL + "/token",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer asSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              srvURL + "/api",
				"authorization_servers": []string{srvURL},
			})
			return
		}
		asSrv.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	srvURL = srv.URL

	disc, err := discoverOAuthEndpoints(context.Background(), srv.URL+"/api")
	if err != nil {
		t.Fatalf("expected discovery to succeed: %v", err)
	}
	if disc.AuthorizationEndpoint != srvURL+"/authorize" {
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

	disc, err := discoverOAuthEndpoints(context.Background(), srv.URL+"/mcp")
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
		if r.URL.RawQuery != "" {
			t.Errorf("verification code sent before browser: %s", r.URL.RequestURI())
		}
		switch r.URL.Path {
		case "/device/authorize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "DEV-CODE-123",
				"user_code":                 "ABCD-1234",
				"verification_uri":          "http://" + r.Host + "/verify",
				"verification_uri_complete": "http://" + r.Host + "/verify?user_code=ABCD-1234",
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

	rt := &oauthProtocol{
		cfg: ServerConfig{
			URL:                 srv.URL,
			OAuth2DeviceAuthURL: srv.URL + "/device/authorize",
			OAuth2TokenURL:      srv.URL + "/token",
			OAuth2AuthURL:       "https://auth.example.com/authorize",
		},
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

	if rt.issuedToken == nil {
		t.Error("issuedToken should be set after successful device flow")
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
				"verification_uri": "http://" + r.Host + "/verify",
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

	rt := &oauthProtocol{
		cfg: ServerConfig{
			URL:                 srv.URL,
			OAuth2DeviceAuthURL: srv.URL + "/device/authorize",
			OAuth2TokenURL:      srv.URL + "/token",
			OAuth2AuthURL:       "https://example.com/authorize",
		},
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
				"verification_uri": "http://" + r.Host + "/verify",
				"expires_in":       2,
				"interval":         1,
			})
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		}
	}))
	defer srv.Close()

	rt := &oauthProtocol{
		cfg: ServerConfig{
			URL:                 srv.URL,
			OAuth2DeviceAuthURL: srv.URL + "/device/authorize",
			OAuth2TokenURL:      srv.URL + "/token",
			OAuth2AuthURL:       "https://example.com/authorize",
		},
		serverSlug:       "test-timeout",
		resolvedClientID: "test-client",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := rt.authorizeDeviceFlow(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
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
		URL:                   srv.URL,
		OAuth2RegistrationURL: srv.URL + "/register",
	}
	_, err := registerDynamicClient(context.Background(), cfg, "http://localhost:9999/callback", nil)
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
	rt := &oauthProtocol{
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
	rt := &oauthProtocol{
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
	rt := &oauthProtocol{
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

	// The shared protocol retains manually configured endpoints, but binds the
	// grant audience to the resource declared by protected-resource discovery.
	if rt.resourceURL != "https://discovered.example.com/mcp" {
		t.Errorf("discovered audience was not adopted: got %q", rt.resourceURL)
	}
	if rt.cfg.OAuth2AuthURL != "https://manual.example.com/authorize" {
		t.Errorf("manual OAuth2AuthURL should not be overwritten: got %q", rt.cfg.OAuth2AuthURL)
	}
	if rt.cfg.OAuth2TokenURL != "https://manual.example.com/token" {
		t.Errorf("manual OAuth2TokenURL should not be overwritten: got %q", rt.cfg.OAuth2TokenURL)
	}
}

func TestMergeDiscovery_NilDiscovery(t *testing.T) {
	rt := &oauthProtocol{
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
	rt := &oauthProtocol{
		serverSlug: "atlassian",
		cfg:        ServerConfig{OAuth2Scopes: []string{"read:jira-work"}},
		discovery:  &OAuthDiscovery{ScopesSupported: []string{"read:jira-work", "offline_access"}},
	}
	got := rt.effectiveScopes()
	if !containsFold(got, "offline_access") {
		t.Errorf("esperava offline_access nos scopes, got %v", got)
	}
}

func TestAuthorizePKCEScopeParameterPreservesEmptyAndExplicitScopes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		scopes []string
		want   string
	}{
		{"empty", nil, ""},
		{"resource", []string{"sql"}, "sql offline_access"},
		{"explicit-refresh-only", []string{"offline_access"}, "offline_access"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/token" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"test-access","token_type":"Bearer"}`)
			}))
			defer server.Close()
			opened := false
			previous := browserOpen
			browserOpen = func(raw string) error {
				opened = true
				u, err := url.Parse(raw)
				if err != nil {
					return err
				}
				if u.Query().Get("scope") != tt.want || (tt.want == "" && u.Query().Has("scope")) {
					t.Errorf("scope sent=%q want=%q", u.Query().Get("scope"), tt.want)
				}
				resp, err := http.Get(u.Query().Get("redirect_uri") + "?code=test&state=" + url.QueryEscape(u.Query().Get("state")))
				if err != nil {
					return err
				}
				_ = resp.Body.Close()
				return nil
			}
			defer func() { browserOpen = previous }()
			rt := &oauthProtocol{
				cfg:       ServerConfig{URL: server.URL, OAuth2ClientID: "client", OAuth2AuthURL: server.URL + "/authorize", OAuth2TokenURL: server.URL + "/token", OAuth2Scopes: tt.scopes},
				discovery: &OAuthDiscovery{ScopesSupported: []string{"offline_access", "admin"}},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := rt.authorizePKCE(ctx); err != nil {
				t.Fatal(err)
			}
			if !opened || rt.issuedToken == nil {
				t.Fatal("PKCE flow did not complete")
			}
		})
	}
}

func TestEffectiveScopes_NotAddedWhenUnsupported(t *testing.T) {
	rt := &oauthProtocol{
		cfg:       ServerConfig{OAuth2Scopes: []string{"read"}},
		discovery: &OAuthDiscovery{ScopesSupported: []string{"read", "write"}},
	}
	if containsFold(rt.effectiveScopes(), "offline_access") {
		t.Error("não deveria adicionar offline_access quando o servidor não o anuncia")
	}
}

func TestEffectiveScopes_NoDiscoveryDoesNotAdd(t *testing.T) {
	rt := &oauthProtocol{cfg: ServerConfig{OAuth2Scopes: []string{"read"}}}
	if containsFold(rt.effectiveScopes(), "offline_access") {
		t.Error("sem discovery (config manual) não deve adicionar offline_access")
	}
}

func TestEffectiveScopes_NoDuplicateWhenAlreadyConfigured(t *testing.T) {
	rt := &oauthProtocol{
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("refresh_token") != "r1" {
			t.Error("refresh did not use stored token")
		}
		w.Header().Set("Content-Type", "application/json")
		// Non-rotating refresh: the server omits refresh_token.
		_, _ = io.WriteString(w, `{"access_token":"a2","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()
	m, ctx, cfg := managedTokenTestFixture(t, srv.URL, "a1", "r1", time.Now().Add(time.Hour))
	if _, err := m.resolveManagedOAuth(ctx, cfg, "a1"); err != nil {
		t.Fatal(err)
	}

	loaded := loadManagedTestToken(t, m, ctx, "srv")
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
	m, ctx, cfg := managedTokenTestFixture(t, "https://resource.example", "old", "old-refresh", time.Now().Add(-time.Hour))
	store, _, service, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	exp := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	if _, err := service.AuthorizeUsing(ctx, store, cfg.OAuthAuthorizationID, func(_ context.Context, r oauthflow.Record) (oauthflow.Record, error) {
		r.Tokens = oauthflow.Tokens{Access: "a", Refresh: "r", ExpiresAt: exp, Type: "Bearer"}
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}

	loaded := loadManagedTestToken(t, m, ctx, "srv")
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

	m, ctx, cfg := managedTokenTestFixture(t, srv.URL, "old-access", "", time.Now().Add(time.Hour))
	previous, err := m.resolveManagedOAuth(ctx, cfg, "")
	if err != nil || previous.Tokens.Refresh != "" {
		t.Fatalf("expected prior in-memory grant without refresh: %v", err)
	}
	_, store, updated := loadManaged(t, m, ctx, "srv")
	updated.Tokens.Refresh = "stored-refresh"
	updated.Revision++
	if err := store.CompareAndSwap(ctx, updated, updated.Revision-1); err != nil {
		t.Fatal(err)
	}
	// A rejected token from the prior result must resolve using the fresh vault.
	if _, err := m.resolveManagedOAuth(ctx, cfg, previous.Tokens.Access); err != nil {
		t.Fatalf("trySilentRefresh deveria suceder usando o refresh do store: %v", err)
	}
	if gotRefresh != "stored-refresh" {
		t.Errorf("deveria usar o refresh_token do store, got %q", gotRefresh)
	}
	loaded := loadManagedTestToken(t, m, ctx, "srv")
	if loaded == nil || loaded.AccessToken != "new-access" {
		t.Errorf("novo access_token deveria ser persistido, got %+v", loaded)
	}
	if loaded.RefreshToken != "rotated-refresh" {
		t.Errorf("refresh rotacionado deveria persistir, got %q", loaded.RefreshToken)
	}
}

func TestStoredTokenSourceSurvivesOperationCtxCancel(t *testing.T) {
	// O token source ARMAZENADO deve renovar mesmo depois que o ctx da operação que
	// o criou é cancelado — senão refreshes futuros falham e forçam reauth (#193).
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		// expires_in baixo: com o expiryDelta do oauth2, o token é tratado como
		// expirado, forçando refresh a cada Token().
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  fmt.Sprintf("acc-%d", n),
			"token_type":    "Bearer",
			"refresh_token": fmt.Sprintf("ref-%d", n),
			"expires_in":    1,
		})
	}))
	defer srv.Close()

	m, ownerCtx, cfg := managedTokenTestFixture(t, srv.URL, "seed", "seed-ref", time.Now().Add(-time.Hour))
	ctx, cancel := context.WithCancel(ownerCtx)
	if _, err := m.resolveManagedOAuth(ctx, cfg, "seed"); err != nil {
		t.Fatalf("trySilentRefresh: %v", err)
	}
	// Cancela o ctx da operação: o token source armazenado NÃO deve depender dele.
	cancel()

	resolved, err := m.resolveManagedOAuth(ownerCtx, cfg, "")
	tok := &oauth2.Token{AccessToken: resolved.Tokens.Access, RefreshToken: resolved.Tokens.Refresh, Expiry: resolved.Tokens.ExpiresAt}
	if err != nil {
		t.Fatalf("refresh futuro falhou após cancelamento do ctx da operação: %v", err)
	}
	if tok == nil || tok.AccessToken == "" {
		t.Fatal("esperava access_token renovado")
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls < 2 {
		t.Errorf("esperava ao menos 2 chamadas ao token endpoint (refresh inicial + futuro), got %d", gotCalls)
	}
}

// Runtime fixtures create only composed authorizations. Results are reloaded
// from the vault so persistence assertions never inspect just a returned token.
func managedTokenTestFixture(t *testing.T, resource, access, refresh string, expiry time.Time) (*Manager, context.Context, ServerConfig) {
	t.Helper()
	m, _, ctx := managedFixture(t)
	cfg := managedConfig(resource)
	cfg.OAuth2TokenAuthMethod = "none"
	if err := m.SaveConfig("srv", cfg); err != nil {
		t.Fatal(err)
	}
	seedManagedRuntime(t, m, ctx, "srv", access, refresh, expiry)
	cfg, _, _ = loadManaged(t, m, ctx, "srv")
	return m, ctx, cfg
}

func loadManagedTestToken(t *testing.T, m *Manager, ctx context.Context, slug string) *oauth2.Token {
	t.Helper()
	if err := m.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	_, _, r := loadManaged(t, m, ctx, slug)
	return &oauth2.Token{AccessToken: r.Tokens.Access, RefreshToken: r.Tokens.Refresh, Expiry: r.Tokens.ExpiresAt, TokenType: r.Tokens.Type}
}

func TestLongLivedCtx_InjectsGuardedHTTPClient(t *testing.T) {
	// O protocolo usa o mesmo transporte protegido sem destacar o cancelamento
	// da operação. Uma renovação posterior recebe seu próprio contexto.
	rt := &oauthProtocol{serverSlug: "srv", cfg: ServerConfig{URL: "https://resource.example"}}
	client := rt.oauthHTTPClient(time.Second)
	if client.Transport == nil {
		t.Fatal("refresh sem transporte protegido e sem timeout por tentativa")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://resource.example/token", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := client.Do(req); response != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cliente protegido perdeu cancelamento: %v", err)
	}
}

// ============ single-flight por servidor (#194) ============

func TestAuthorizeSkipsWhenAnotherFlowRenewedToken(t *testing.T) {
	// Caso #194: outro flow já publicou um token diferente do rejeitado.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("token já renovado não deve emitir outro grant")
	}))
	defer srv.Close()
	m, ctx, cfg := managedTokenTestFixture(t, srv.URL, "old-rejected", "refresh", time.Now().Add(time.Hour))
	_, store, current := loadManaged(t, m, ctx, "srv")
	current.Tokens.Access = "new-from-other-flow"
	current.Revision++
	if err := store.CompareAndSwap(ctx, current, current.Revision-1); err != nil {
		t.Fatal(err)
	}
	oldBrowser := browserOpen
	browserOpen = func(string) error { t.Error("renovação concorrente abriu navegador"); return nil }
	defer func() { browserOpen = oldBrowser }()
	if r, err := m.resolveManagedOAuth(ctx, cfg, "old-rejected"); err != nil || r.Tokens.Access != "new-from-other-flow" {
		t.Fatalf("resolução deveria aproveitar a renovação concorrente: %v", err)
	}
}

func TestAuthorizeProceedsWhenTokenUnchanged(t *testing.T) {
	// Token rejeitado e sem refresh não pode ser devolvido nem iniciar login
	// incidental; a UI recebe a necessidade explícita de reconexão.
	m, ctx, cfg := managedTokenTestFixture(t, "https://resource.example", "still-rejected", "", time.Now().Add(time.Hour))
	oldBrowser := browserOpen
	browserOpen = func(string) error { t.Error("rejeição abriu navegador incidental"); return nil }
	defer func() { browserOpen = oldBrowser }()
	if r, err := m.resolveManagedOAuth(ctx, cfg, "still-rejected"); !errors.Is(err, oauthflow.ErrReauthorize) || r.Tokens.Access != "" {
		t.Fatalf("token rejeitado foi aceito: %v", err)
	}
}

func TestAuthorizeCanceledWhileWaitingForSharedArbiter(t *testing.T) {
	oauthFlowArbiter.Lock()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(oauthFlowArbiter.Unlock) }
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &oauthProtocol{}
	done := make(chan error, 1)
	go func() { done <- rt.authorize(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled MCP authorization waited for the shared arbiter")
	}
	release()
	if err := rt.authorize(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled authorization proceeded with free arbiter: %v", err)
	}
}

func TestDeviceVerificationProbesOnlyCodeFreeEndpoint(t *testing.T) {
	for _, mode := range []string{"reachable", "rewrite", "different_path", "missing_base", "base_query"} {
		t.Run(mode, func(t *testing.T) {
			var probes []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				probes = append(probes, r.URL.RequestURI())
				if r.URL.RawQuery != "" {
					t.Errorf("probe leaked code: %s", r.URL.RequestURI())
				}
				if mode != "reachable" && r.URL.Path == "/oauth/verify" {
					w.WriteHeader(http.StatusUnauthorized)
				}
			}))
			defer srv.Close()
			base := srv.URL + "/oauth/verify"
			complete := base + "?user_code=SECRET#confirm"
			want := complete
			wantProbes := 1
			switch mode {
			case "rewrite":
				want = srv.URL + "/api/oauth/verify?user_code=SECRET#confirm"
				wantProbes = 2
			case "different_path":
				complete = srv.URL + "/other?user_code=SECRET"
				want = complete
				wantProbes = 2
			case "missing_base":
				base = ""
				wantProbes = 0
			case "base_query":
				base += "?code=SECRET"
				wantProbes = 0
			}
			rt := &oauthProtocol{cfg: ServerConfig{URL: srv.URL}}
			got, err := rt.deviceVerificationURL(context.Background(), oauthflow.DeviceVerification{BaseURL: base, URL: complete, UserCode: "SECRET"})
			if err != nil || got != want || len(probes) != wantProbes {
				t.Fatalf("got=%s err=%v probes=%v", got, err, probes)
			}
		})
	}
}
