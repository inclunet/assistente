package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"assistente/internal/credentials"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// browserOpen opens a URL in the user's browser. Variable so tests can stub it.
var browserOpen = browser.OpenURL

// SessionExpiredError indica que a sessão Streamable HTTP expirou no servidor.
// O servidor retornou 404 ou 410, significando que o Mcp-Session-Id é inválido.
type SessionExpiredError struct {
	StatusCode int
}

func (e *SessionExpiredError) Error() string {
	return fmt.Sprintf("mcp session expired (HTTP %d)", e.StatusCode)
}

// persistingTokenSource wraps an oauth2.TokenSource and persists tokens
// to the credential manager whenever a new token is obtained (refresh).
type persistingTokenSource struct {
	inner     oauth2.TokenSource
	rt        *pkceRoundTripper
	mu        sync.Mutex
	lastToken string
}

func (pts *persistingTokenSource) Token() (*oauth2.Token, error) {
	token, err := pts.inner.Token()
	if err != nil {
		return nil, err
	}

	pts.mu.Lock()
	defer pts.mu.Unlock()

	if token.AccessToken != pts.lastToken {
		pts.lastToken = token.AccessToken
		pts.rt.persistTokens(token)
		log.Printf("[MCP:%s] Token renovado e persistido automaticamente", pts.rt.serverSlug)
	}

	return token, nil
}

func (rt *pkceRoundTripper) wrapWithPersistence(ts oauth2.TokenSource) oauth2.TokenSource {
	return &persistingTokenSource{
		inner: ts,
		rt:    rt,
	}
}

// trySilentRefresh tenta renovar o token usando o refresh_token,
// sem abrir o browser. Retorna nil se bem-sucedido.
func (rt *pkceRoundTripper) trySilentRefresh(ctx context.Context) error {
	if rt.oauthCfg == nil || rt.tokenSource == nil {
		return fmt.Errorf("no oauth config or token source")
	}

	token, err := rt.tokenSource.Token()
	if err != nil || token == nil || token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	expiredToken := &oauth2.Token{
		RefreshToken: token.RefreshToken,
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	newSource := rt.oauthCfg.TokenSource(refreshCtx, expiredToken)
	newToken, err := newSource.Token()
	if err != nil {
		return fmt.Errorf("silent refresh failed: %w", err)
	}

	rt.tokenSource = rt.wrapWithPersistence(rt.oauthCfg.TokenSource(ctx, newToken))
	rt.persistTokens(newToken)

	log.Printf("[MCP:%s] Token renovado silenciosamente via refresh_token", rt.serverSlug)
	return nil
}

// buildClientCredentialsHTTPClient cria um *http.Client que obtém tokens
// via OAuth2 Client Credentials Grant (machine-to-machine).
func buildClientCredentialsHTTPClient(cfg ServerConfig, clientSecret string) *http.Client {
	cc := &clientcredentials.Config{
		ClientID:     cfg.OAuth2ClientID,
		ClientSecret: clientSecret,
		TokenURL:     cfg.OAuth2TokenURL,
		Scopes:       cfg.OAuth2Scopes,
	}
	return cc.Client(context.Background())
}

// ============ OAuth Discovery (uses discovery.go infrastructure) ============

// OAuthDiscovery holds endpoints discovered via .well-known metadata.
type OAuthDiscovery struct {
	Resource                    string
	AuthorizationEndpoint       string
	TokenEndpoint               string
	RegistrationEndpoint        string
	DeviceAuthorizationEndpoint string
	GrantTypesSupported         []string
}

// discoverOAuthEndpoints uses the existing discovery infrastructure from
// discovery.go to fetch protected resource + auth server metadata.
func discoverOAuthEndpoints(mcpURL string) (*OAuthDiscovery, error) {
	origin, err := extractOrigin(mcpURL)
	if err != nil {
		return nil, err
	}

	authServerBase := origin
	var resource string

	prm, err := fetchProtectedResourceMetadata(mcpURL)
	if err != nil {
		return nil, fmt.Errorf("protected resource metadata unavailable: %w", err)
	}
	if len(prm.AuthorizationServers) > 0 {
		authServerBase = prm.AuthorizationServers[0]
	}
	resource = prm.Resource
	if resource == "" {
		resource = mcpURL
	}

	asm, err := fetchAuthServerMetadata(authServerBase)
	if err != nil {
		return nil, fmt.Errorf("auth server metadata unavailable: %w", err)
	}

	return &OAuthDiscovery{
		Resource:                    resource,
		AuthorizationEndpoint:       asm.AuthorizationEndpoint,
		TokenEndpoint:               asm.TokenEndpoint,
		RegistrationEndpoint:        asm.RegistrationEndpoint,
		DeviceAuthorizationEndpoint: asm.DeviceAuthorizationEndpoint,
		GrantTypesSupported:         asm.GrantTypesSupported,
	}, nil
}

// ============ Blocked endpoint workaround (Istio/mTLS) ============

// probeOAuthURL checks if an OAuth endpoint is reachable by sending a GET.
// Returns true if the endpoint responds with anything other than 401.
// A 401 with empty body from istio-envoy typically means the endpoint
// is blocked by an AuthorizationPolicy (mTLS required).
func probeOAuthURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode != http.StatusUnauthorized
}

// tryAPIPrefix rewrites /oauth/... to /api/oauth/... in the URL path.
// Many workforce auth proxies expose machine-accessible endpoints under /api/
// while browser-facing /oauth/ paths are behind mTLS/service mesh.
func tryAPIPrefix(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if strings.HasPrefix(u.Path, "/oauth/") {
		u.Path = "/api" + u.Path
		return u.String()
	}
	return rawURL
}

// fixBlockedEndpoint probes an OAuth endpoint and, if blocked (401),
// tries the /api/ prefixed version as a workaround.
func fixBlockedEndpoint(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	if probeOAuthURL(rawURL) {
		return rawURL
	}
	alt := tryAPIPrefix(rawURL)
	if alt != rawURL && probeOAuthURL(alt) {
		log.Printf("[OAuth] Endpoint bloqueado reescrito: %s → %s", rawURL, alt)
		return alt
	}
	return rawURL
}

// ============ pkceRoundTripper ============

// pkceRoundTripper é um http.RoundTripper que implementa OAuth2
// Authorization Code + PKCE com suporte a Device Flow (RFC 8628).
//
// Suporta:
// - OAuth discovery automático via .well-known (RFC 9728)
// - Device Authorization Flow (RFC 8628) para servidores workforce/corporativos
// - Dynamic Client Registration (RFC 7591) quando registration_endpoint existe
// - Client secret para servidores que exigem client_secret_post (ex: Slack)
// - Porta de callback fixa (oauth2_callback_port) para redirect_uri determinístico
// - Parâmetro resource (RFC 8707)
type pkceRoundTripper struct {
	base       http.RoundTripper
	credMgr    *credentials.Manager
	cfg        ServerConfig
	emitEvent  emitFunc
	serverSlug string

	// onConfigUpdate é chamado para persistir mudanças no config (ex: porta após DCR).
	onConfigUpdate func(ServerConfig)

	mu          sync.Mutex
	tokenSource oauth2.TokenSource
	oauthCfg    *oauth2.Config

	// Resolved client credentials — from DCR, credential manager, or config
	resolvedClientID     string
	resolvedClientSecret string

	// resourceURL is the MCP server URL used as the "resource" parameter (RFC 8707).
	resourceURL string

	// discovery caches discovered endpoints (nil = not yet attempted).
	discovery *OAuthDiscovery
}

func (rt *pkceRoundTripper) effectiveClientID() string {
	if rt.cfg.OAuth2ClientID != "" {
		return rt.cfg.OAuth2ClientID
	}
	return rt.resolvedClientID
}

func (rt *pkceRoundTripper) effectiveClientSecret() string {
	return rt.resolvedClientSecret
}

func (rt *pkceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	ts := rt.tokenSource
	rt.mu.Unlock()

	if ts != nil {
		token, err := ts.Token()
		if err == nil {
			cloned := req.Clone(req.Context())
			token.SetAuthHeader(cloned)
			resp, err := rt.base.RoundTrip(cloned)
			if err != nil {
				return nil, err
			}

			// 404/410 = sessão MCP expirou (Mcp-Session-Id inválido).
			// Não é problema de auth — propaga para o bridge disparar reconexão.
			if isSessionExpiredStatus(resp.StatusCode) {
				_ = resp.Body.Close()
				return nil, &SessionExpiredError{StatusCode: resp.StatusCode}
			}

			if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
				return resp, nil
			}
			_ = resp.Body.Close()

			// Token foi rejeitado — tenta renovar silenciosamente antes de abrir o browser
			rt.mu.Lock()
			silentErr := rt.trySilentRefresh(req.Context())
			rt.mu.Unlock()

			if silentErr == nil {
				rt.mu.Lock()
				ts = rt.tokenSource
				rt.mu.Unlock()
				if ts != nil {
					if newToken, err := ts.Token(); err == nil {
						retryReq := req.Clone(req.Context())
						newToken.SetAuthHeader(retryReq)
						resp, err := rt.base.RoundTrip(retryReq)
						if err != nil {
							return nil, err
						}
						if isSessionExpiredStatus(resp.StatusCode) {
							_ = resp.Body.Close()
							return nil, &SessionExpiredError{StatusCode: resp.StatusCode}
						}
						if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
							return resp, nil
						}
						_ = resp.Body.Close()
					}
				}
			}
		}
	}

	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if isSessionExpiredStatus(resp.StatusCode) {
		_ = resp.Body.Close()
		return nil, &SessionExpiredError{StatusCode: resp.StatusCode}
	}

	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}
	_ = resp.Body.Close()

	// Último recurso: autorização completa (abre browser)
	if err := rt.authorize(req.Context()); err != nil {
		return nil, fmt.Errorf("oauth2 pkce authorization failed: %w", err)
	}

	rt.mu.Lock()
	ts = rt.tokenSource
	rt.mu.Unlock()

	if ts == nil {
		return nil, fmt.Errorf("no token after authorization")
	}

	token, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get token after authorization: %w", err)
	}

	retryReq := req.Clone(req.Context())
	token.SetAuthHeader(retryReq)
	return rt.base.RoundTrip(retryReq)
}

// isSessionExpiredStatus retorna true para status HTTP que indicam sessão MCP expirada.
// 404 = sessão não encontrada, 410 = sessão encerrada deliberadamente.
func isSessionExpiredStatus(statusCode int) bool {
	return statusCode == http.StatusNotFound || statusCode == http.StatusGone
}

func (rt *pkceRoundTripper) authorize(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// 1. Discovery automático de endpoints OAuth (se URL MCP disponível)
	if rt.discovery == nil && rt.cfg.URL != "" {
		disc, err := discoverOAuthEndpoints(rt.cfg.URL)
		if err != nil {
			log.Printf("[MCP:%s] Discovery automático falhou (usando config manual): %v", rt.serverSlug, err)
		} else {
			rt.discovery = disc
			log.Printf("[MCP:%s] Discovery OK: resource=%s, device_endpoint=%s",
				rt.serverSlug, disc.Resource, disc.DeviceAuthorizationEndpoint)
		}
	}

	// 2. Merge: discovery preenche campos vazios, config manual tem prioridade
	rt.mergeDiscovery()

	// 2.5 Workaround: some workforce auth proxies block browser-facing /oauth/*
	// paths behind mTLS while /api/oauth/* paths are accessible. Probe and fix.
	if rt.cfg.OAuth2AuthURL != "" {
		if fixed := fixBlockedEndpoint(rt.cfg.OAuth2AuthURL); fixed != rt.cfg.OAuth2AuthURL {
			log.Printf("[MCP:%s] Authorization endpoint fix: %s → %s", rt.serverSlug, rt.cfg.OAuth2AuthURL, fixed)
			rt.cfg.OAuth2AuthURL = fixed
		}
	}

	// 3. Resolve client_id: existente (config/cred manager) ou DCR
	if err := rt.resolveClientID(ctx); err != nil {
		return err
	}

	clientID := rt.effectiveClientID()
	if clientID == "" {
		return fmt.Errorf("no client_id available (configure manualmente ou use um servidor com registration_endpoint)")
	}

	// 4. Se device_authorization_endpoint disponível → device flow
	if rt.cfg.OAuth2DeviceAuthURL != "" {
		log.Printf("[MCP:%s] Tentando Device Authorization Flow", rt.serverSlug)
		err := rt.authorizeDeviceFlow(ctx)
		if err == nil {
			return nil
		}

		// Se o client_id não tem grant device_code, re-registrar via DCR e tentar de novo
		if strings.Contains(err.Error(), "unauthorized_client") && rt.cfg.OAuth2RegistrationURL != "" {
			log.Printf("[MCP:%s] Client sem grant device_code — re-registrando via DCR", rt.serverSlug)
			if rerr := rt.reRegisterClient(ctx); rerr != nil {
				log.Printf("[MCP:%s] Re-registro falhou: %v", rt.serverSlug, rerr)
			} else {
				log.Printf("[MCP:%s] Re-registro OK — retentando Device Flow", rt.serverSlug)
				if err2 := rt.authorizeDeviceFlow(ctx); err2 == nil {
					return nil
				} else {
					log.Printf("[MCP:%s] Device flow falhou após re-registro: %v", rt.serverSlug, err2)
				}
			}
		} else {
			log.Printf("[MCP:%s] Device flow falhou: %v — tentando PKCE", rt.serverSlug, err)
		}
	}

	// 5. Fallback: PKCE Authorization Code (fluxo atual)
	return rt.authorizePKCE(ctx)
}

// mergeDiscovery fills empty config fields from discovered endpoints.
func (rt *pkceRoundTripper) mergeDiscovery() {
	d := rt.discovery
	if d == nil {
		return
	}
	if rt.resourceURL == "" {
		rt.resourceURL = d.Resource
	}
	if rt.cfg.OAuth2AuthURL == "" {
		rt.cfg.OAuth2AuthURL = d.AuthorizationEndpoint
	}
	if rt.cfg.OAuth2TokenURL == "" {
		rt.cfg.OAuth2TokenURL = d.TokenEndpoint
	}
	if rt.cfg.OAuth2RegistrationURL == "" && d.RegistrationEndpoint != "" {
		rt.cfg.OAuth2RegistrationURL = d.RegistrationEndpoint
	}
	if rt.cfg.OAuth2DeviceAuthURL == "" && d.DeviceAuthorizationEndpoint != "" {
		rt.cfg.OAuth2DeviceAuthURL = d.DeviceAuthorizationEndpoint
	}
}

// resolveClientID performs DCR if no client_id is available and registration endpoint exists.
func (rt *pkceRoundTripper) resolveClientID(ctx context.Context) error {
	if rt.effectiveClientID() != "" {
		return nil
	}
	if rt.cfg.OAuth2RegistrationURL == "" {
		return nil
	}

	log.Printf("[MCP:%s] Sem client_id — tentando Dynamic Client Registration", rt.serverSlug)

	callbackHost, listenIP := resolveCallbackHost(rt.cfg.OAuth2CallbackHost)
	port := rt.cfg.OAuth2CallbackPort
	if port == 0 {
		l, err := net.Listen("tcp", listenIP+":0")
		if err != nil {
			return fmt.Errorf("failed to allocate port for DCR redirect_uri: %w", err)
		}
		port = l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
	}
	redirectURL := fmt.Sprintf("http://%s:%d/callback", callbackHost, port)

	dcrResult, err := registerDynamicClient(rt.cfg, redirectURL)
	if err != nil {
		return fmt.Errorf("dynamic client registration failed: %w", err)
	}

	rt.resolvedClientID = dcrResult.ClientID
	rt.resolvedClientSecret = dcrResult.ClientSecret
	rt.persistClientCreds(dcrResult.ClientID, dcrResult.ClientSecret)

	rt.cfg.OAuth2ClientID = dcrResult.ClientID
	if rt.cfg.OAuth2CallbackPort == 0 {
		rt.cfg.OAuth2CallbackPort = port
	}
	log.Printf("[MCP:%s] DCR concluído: client_id=%s, porta=%d", rt.serverSlug, dcrResult.ClientID, rt.cfg.OAuth2CallbackPort)
	if rt.onConfigUpdate != nil {
		rt.onConfigUpdate(rt.cfg)
	}
	return nil
}

// reRegisterClient forces a new DCR, replacing the old client_id.
// Used when the existing client lacks required grant types (e.g. device_code).
func (rt *pkceRoundTripper) reRegisterClient(ctx context.Context) error {
	callbackHost, listenIP := resolveCallbackHost(rt.cfg.OAuth2CallbackHost)
	port := rt.cfg.OAuth2CallbackPort
	if port == 0 {
		l, err := net.Listen("tcp", listenIP+":0")
		if err != nil {
			return fmt.Errorf("failed to allocate port for DCR redirect_uri: %w", err)
		}
		port = l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
	}
	redirectURL := fmt.Sprintf("http://%s:%d/callback", callbackHost, port)

	dcrResult, err := registerDynamicClient(rt.cfg, redirectURL)
	if err != nil {
		return fmt.Errorf("re-registration failed: %w", err)
	}

	rt.resolvedClientID = dcrResult.ClientID
	rt.resolvedClientSecret = dcrResult.ClientSecret
	rt.cfg.OAuth2ClientID = dcrResult.ClientID
	rt.persistClientCreds(dcrResult.ClientID, dcrResult.ClientSecret)

	if rt.cfg.OAuth2CallbackPort == 0 {
		rt.cfg.OAuth2CallbackPort = port
	}
	if rt.onConfigUpdate != nil {
		rt.onConfigUpdate(rt.cfg)
	}
	return nil
}

// ============ Device Authorization Flow (RFC 8628) ============

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type deviceTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

func (rt *pkceRoundTripper) authorizeDeviceFlow(parentCtx context.Context) error {
	// Device flow needs user interaction (browser auth) — use a dedicated
	// long timeout independent of the parent connect timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	clientID := rt.effectiveClientID()

	// POST device_authorization_endpoint
	form := url.Values{
		"client_id": {clientID},
	}
	if rt.resourceURL != "" {
		form.Set("resource", rt.resourceURL)
	}
	if len(rt.cfg.OAuth2Scopes) > 0 {
		scope := ""
		for i, s := range rt.cfg.OAuth2Scopes {
			if i > 0 {
				scope += " "
			}
			scope += s
		}
		form.Set("scope", scope)
	}

	resp, err := discoveryHTTPClient.PostForm(rt.cfg.OAuth2DeviceAuthURL, form)
	if err != nil {
		return fmt.Errorf("device authorization request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("failed to read device authorization response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("device authorization returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var devResp deviceAuthResponse
	if err := json.Unmarshal(body, &devResp); err != nil {
		return fmt.Errorf("failed to parse device authorization response: %w", err)
	}

	if devResp.DeviceCode == "" {
		return fmt.Errorf("device authorization response missing device_code")
	}

	// Abrir browser com verification_uri_complete (ou verification_uri)
	verifyURL := devResp.VerificationURIComplete
	if verifyURL == "" {
		verifyURL = devResp.VerificationURI
	}

	// Workaround: verification_uri may be blocked by service mesh (mTLS).
	// Probe and try /api/ prefix version if the original returns 401.
	if verifyURL != "" {
		if fixed := fixBlockedEndpoint(verifyURL); fixed != verifyURL {
			log.Printf("[MCP:%s] Verification URI fix: %s → %s", rt.serverSlug, verifyURL, fixed)
			verifyURL = fixed
		}
	}

	log.Printf("[MCP:%s] Device flow: user_code=%s, verification_uri=%s", rt.serverSlug, devResp.UserCode, verifyURL)

	if rt.emitEvent != nil {
		rt.emitEvent("mcp:oauth_device_verify", map[string]string{
			"slug":      rt.serverSlug,
			"user_code": devResp.UserCode,
			"url":       verifyURL,
		})
	}

	if verifyURL != "" {
		if err := browserOpen(verifyURL); err != nil {
			log.Printf("[MCP:%s] Erro ao abrir browser para device flow: %v", rt.serverSlug, err)
		}
	}

	// Token polling
	interval := time.Duration(devResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	expiresIn := devResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("device flow timed out (%ds)", expiresIn)
		}

		tokenResp, err := rt.pollDeviceToken(clientID, devResp.DeviceCode)
		if err != nil {
			return err
		}

		switch tokenResp.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "":
			token := &oauth2.Token{
				AccessToken:  tokenResp.AccessToken,
				TokenType:    tokenResp.TokenType,
				RefreshToken: tokenResp.RefreshToken,
			}
			if tokenResp.ExpiresIn > 0 {
				token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
			}

			oauthCfg := &oauth2.Config{
				ClientID: clientID,
				Endpoint: oauth2.Endpoint{
					AuthURL:  rt.cfg.OAuth2AuthURL,
					TokenURL: rt.cfg.OAuth2TokenURL,
				},
				Scopes: rt.cfg.OAuth2Scopes,
			}
			rt.oauthCfg = oauthCfg
			rt.tokenSource = rt.wrapWithPersistence(oauthCfg.TokenSource(ctx, token))
			rt.persistTokens(token)

			log.Printf("[MCP:%s] Device Authorization Flow concluído com sucesso", rt.serverSlug)
			return nil
		default:
			return fmt.Errorf("device flow token error: %s", tokenResp.Error)
		}
	}
}

func (rt *pkceRoundTripper) pollDeviceToken(clientID, deviceCode string) (*deviceTokenResponse, error) {
	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
		"client_id":   {clientID},
	}
	if rt.resourceURL != "" {
		form.Set("resource", rt.resourceURL)
	}

	resp, err := discoveryHTTPClient.PostForm(rt.cfg.OAuth2TokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("device token poll failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read device token response: %w", err)
	}

	var tokenResp deviceTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse device token response: %w", err)
	}

	return &tokenResp, nil
}

// ============ PKCE Authorization Code (fluxo original) ============

func (rt *pkceRoundTripper) authorizePKCE(ctx context.Context) error {
	callbackHost, listenIP := resolveCallbackHost(rt.cfg.OAuth2CallbackHost)

	listenAddr := listenIP + ":0"
	if rt.cfg.OAuth2CallbackPort > 0 {
		listenAddr = fmt.Sprintf("%s:%d", listenIP, rt.cfg.OAuth2CallbackPort)
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		if rt.cfg.OAuth2CallbackPort > 0 {
			return fmt.Errorf("porta %d em uso — verifique se outro processo está usando-a: %w",
				rt.cfg.OAuth2CallbackPort, err)
		}
		return fmt.Errorf("failed to start loopback listener: %w", err)
	}
	defer func() { _ = listener.Close() }()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://%s:%d/callback", callbackHost, port)

	clientID := rt.effectiveClientID()
	clientSecret := rt.effectiveClientSecret()

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  rt.cfg.OAuth2AuthURL,
			TokenURL: rt.cfg.OAuth2TokenURL,
		},
		RedirectURL: redirectURL,
		Scopes:      rt.cfg.OAuth2Scopes,
	}

	codeVerifier := oauth2.GenerateVerifier()
	state := generateState()

	authURLOpts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(codeVerifier)}
	if rt.resourceURL != "" {
		authURLOpts = append(authURLOpts, oauth2.SetAuthURLParam("resource", rt.resourceURL))
	}
	authURL := oauthCfg.AuthCodeURL(state, authURLOpts...)

	resultCh := make(chan *authCallbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errParam := q.Get("error"); errParam != "" {
			resultCh <- &authCallbackResult{
				err: fmt.Errorf("authorization error: %s - %s", errParam, q.Get("error_description")),
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, authErrorHTML)
			return
		}

		code := q.Get("code")
		returnedState := q.Get("state")
		if returnedState != state {
			resultCh <- &authCallbackResult{err: fmt.Errorf("state mismatch")}
			return
		}

		resultCh <- &authCallbackResult{code: code}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, authSuccessHTML)
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(context.Background()) }()

	log.Printf("[MCP:%s] Abrindo browser para autorização OAuth2 PKCE (redirect=%s)", rt.serverSlug, redirectURL)
	if rt.emitEvent != nil {
		rt.emitEvent("mcp:oauth_authorize", map[string]string{
			"slug": rt.serverSlug,
			"url":  authURL,
		})
	}

	if err := browserOpen(authURL); err != nil {
		log.Printf("[MCP:%s] Erro ao abrir browser: %v. URL: %s", rt.serverSlug, err, authURL)
	}

	exchangeOpts := []oauth2.AuthCodeOption{oauth2.VerifierOption(codeVerifier)}
	if rt.resourceURL != "" {
		exchangeOpts = append(exchangeOpts, oauth2.SetAuthURLParam("resource", rt.resourceURL))
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}

		token, err := oauthCfg.Exchange(ctx, result.code, exchangeOpts...)
		if err != nil {
			return fmt.Errorf("token exchange failed: %w", err)
		}

		rt.oauthCfg = oauthCfg
		rt.tokenSource = rt.wrapWithPersistence(oauthCfg.TokenSource(ctx, token))
		rt.persistTokens(token)

		log.Printf("[MCP:%s] Autorização OAuth2 PKCE concluída com sucesso", rt.serverSlug)
		return nil

	case <-ctx.Done():
		return ctx.Err()

	case <-time.After(5 * time.Minute):
		return fmt.Errorf("authorization timed out (5 min)")
	}
}

// ============ Dynamic Client Registration (RFC 7591) ============

type dcrRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope,omitempty"`
}

type dcrResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

var dcrHTTPClient = &http.Client{Timeout: 10 * time.Second}

func registerDynamicClient(cfg ServerConfig, redirectURL string) (*dcrResponse, error) {
	scope := ""
	for i, s := range cfg.OAuth2Scopes {
		if i > 0 {
			scope += " "
		}
		scope += s
	}

	reqBody := dcrRequest{
		RedirectURIs:            []string{redirectURL},
		ClientName:              "Assistente",
		GrantTypes:              []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
		Scope:                   scope,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DCR request: %w", err)
	}

	log.Printf("[MCP:dcr] POST %s with redirect_uris=%v", cfg.OAuth2RegistrationURL, reqBody.RedirectURIs)

	req, err := http.NewRequest("POST", cfg.OAuth2RegistrationURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := dcrHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DCR request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read DCR response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("DCR returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result dcrResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse DCR response: %w", err)
	}

	if result.ClientID == "" {
		return nil, fmt.Errorf("DCR response missing client_id")
	}

	return &result, nil
}

// ============ Persist (duas entradas separadas no credential manager) ============

func clientCredPattern(slug string) string { return "mcp-client:" + slug }
func userTokensPattern(slug string) string { return "mcp-tokens:" + slug }

// persistClientCreds salva dados de registro do app (client_id + client_secret) no credential manager.
func (rt *pkceRoundTripper) persistClientCreds(clientID, clientSecret string) {
	if rt.credMgr == nil {
		return
	}
	auth := &credentials.AuthConfig{
		Type:         "oauth2",
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	if err := rt.credMgr.RegisterPatternWithContext(context.Background(), clientCredPattern(rt.serverSlug), auth); err != nil {
		log.Printf("[MCP:%s] Erro ao salvar credenciais do cliente: %v", rt.serverSlug, err)
	}
}

// persistTokens salva tokens da sessão do usuário (access_token + refresh_token) no credential manager.
func (rt *pkceRoundTripper) persistTokens(token *oauth2.Token) {
	if rt.credMgr == nil || token == nil {
		return
	}
	auth := &credentials.AuthConfig{
		Type:       "oauth2",
		Token:      token.AccessToken,
		RefreshURL: token.RefreshToken,
	}
	if token.Expiry.After(time.Now()) {
		auth.ExpiresAt = token.Expiry.Unix()
	}
	if err := rt.credMgr.RegisterPatternWithContext(context.Background(), userTokensPattern(rt.serverSlug), auth); err != nil {
		log.Printf("[MCP:%s] Erro ao salvar tokens do usuário: %v", rt.serverSlug, err)
	}
}

// loadClientCreds carrega client_id e client_secret do credential manager.
func loadClientCreds(credMgr *credentials.Manager, slug string) (clientID, clientSecret string) {
	if credMgr == nil {
		return
	}
	auth, err := credMgr.GetByPattern(clientCredPattern(slug))
	if err != nil || auth == nil {
		return
	}
	return auth.ClientID, auth.ClientSecret
}

// loadUserTokens carrega tokens do usuário do credential manager.
func loadUserTokens(credMgr *credentials.Manager, slug string) *oauth2.Token {
	if credMgr == nil {
		return nil
	}
	auth, err := credMgr.GetByPattern(userTokensPattern(slug))
	if err != nil || auth == nil || auth.Token == "" {
		return nil
	}
	token := &oauth2.Token{
		AccessToken:  auth.Token,
		RefreshToken: auth.RefreshURL,
		TokenType:    "Bearer",
	}
	if auth.ExpiresAt > 0 {
		token.Expiry = time.Unix(auth.ExpiresAt, 0)
	}
	return token
}

type authCallbackResult struct {
	code string
	err  error
}

func hostnameFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// resolveCallbackHost retorna o hostname para o redirect_uri e o IP real
// para net.Listen. O hostname é o que o authorization server valida;
// o IP é o endereço de bind efetivo (localhost não é um endereço válido para bind).
func resolveCallbackHost(configured string) (host, listenIP string) {
	host = configured
	if host == "" {
		host = "localhost"
	}
	listenIP = "127.0.0.1"
	if host == "[::1]" {
		listenIP = "::1"
	}
	return
}

func generateState() string {
	h := sha256.New()
	b := make([]byte, 32)
	rand.Read(b)
	h.Write(b)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// buildPKCEHTTPClient cria um *http.Client que implementa OAuth2 PKCE.
// Tenta reutilizar tokens e credenciais do credential manager.
// onConfigUpdate é chamado quando o config precisa ser persistido (ex: porta após DCR).
func buildPKCEHTTPClient(cfg ServerConfig, credMgr *credentials.Manager, emitEvent emitFunc, slug string, onConfigUpdate func(ServerConfig)) *http.Client {
	rt := &pkceRoundTripper{
		base:           newMCPTransport(),
		credMgr:        credMgr,
		cfg:            cfg,
		emitEvent:      emitEvent,
		serverSlug:     slug,
		onConfigUpdate: onConfigUpdate,
		resourceURL:    cfg.URL,
	}

	// Entrada 1: dados do cliente (mcp-client:{slug}) → client_id + client_secret
	clientID, clientSecret := loadClientCreds(credMgr, slug)
	if clientID == "" && cfg.OAuth2ClientID != "" {
		clientID = cfg.OAuth2ClientID
		rt.persistClientCreds(clientID, "")
		log.Printf("[MCP:%s] client_id importado do config para credential manager", slug)
	}
	rt.resolvedClientID = clientID
	rt.resolvedClientSecret = clientSecret

	// Entrada 2: tokens do usuário (mcp-tokens:{slug}) → access_token + refresh_token
	token := loadUserTokens(credMgr, slug)
	if token != nil && clientID != "" {
		oauthCfg := &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  cfg.OAuth2AuthURL,
				TokenURL: cfg.OAuth2TokenURL,
			},
			Scopes: cfg.OAuth2Scopes,
		}
		rt.oauthCfg = oauthCfg
		rt.tokenSource = rt.wrapWithPersistence(oauthCfg.TokenSource(context.Background(), token))
	}

	return &http.Client{Transport: rt}
}

const authSuccessHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Autorização concluída</title></head>
<body style="font-family:sans-serif;text-align:center;padding:40px">
<h2>Autorização concluída!</h2>
<p>Pode fechar esta janela e retornar ao Assistente.</p>
<script>setTimeout(function(){window.close()},3000)</script>
</body></html>`

const authErrorHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Erro de autorização</title></head>
<body style="font-family:sans-serif;text-align:center;padding:40px">
<h2>Erro na autorização</h2>
<p>Verifique os logs no Assistente para mais detalhes.</p>
</body></html>`
