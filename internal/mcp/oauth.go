package mcp

import (
	"assistente/internal/logging"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/oauthflow"
	"assistente/internal/oauthintegrations"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
)

// browserOpen opens a URL in the user's browser. Variable so tests can stub it.
var browserOpen = browser.OpenURL

var errOAuthPersistence = errors.New("oauth_legacy_persistence_failed")

// oauthFlowArbiter serializa flows OAuth interativos do MCP entre
// servidores diferentes. Sem ele, dois servidores que precisam reauth
// simultaneamente disparavam `browser.OpenURL` ao mesmo tempo,
// inundando o usuário com N janelas (sintoma reportado no incident
// AEP-0061: "todos os MCPs abrem juntos quando perdem autorização").
//
// Não tem timeout próprio: cada `authorize()` já tem timeout interno
// (5min PKCE, poll budget Device Flow). Um flow congelado bloqueia o
// próximo na fila, e isso é o comportamento desejado — não faz
// sentido empilhar fluxos abertos.
var oauthFlowArbiter = oauthflow.Interactive

// SessionExpiredError indica que a sessão Streamable HTTP expirou no servidor.
// O servidor retornou 404 ou 410, significando que o Mcp-Session-Id é inválido.
type SessionExpiredError struct {
	StatusCode int
}

func (e *SessionExpiredError) Error() string {
	return fmt.Sprintf("mcp session expired (HTTP %d)", e.StatusCode)
}

// ============ OAuth Discovery (uses discovery.go infrastructure) ============

type OAuthDiscovery = oauthflow.DiscoveryEndpoints

func discoverOAuthEndpoints(ctx context.Context, resourceURL string) (*OAuthDiscovery, error) {
	return oauthflow.DiscoverEndpoints(ctx, resourceURL)
}

// ============ Blocked endpoint workaround (Istio/mTLS) ============

// probeOAuthURL checks if an OAuth endpoint is reachable by sending a GET.
// Returns true if the endpoint responds with anything other than 401.
// A 401 with empty body from istio-envoy typically means the endpoint
// is blocked by an AuthorizationPolicy (mTLS required).
func (rt *oauthProtocol) probeOAuthURL(ctx context.Context, rawURL string) (bool, error) {
	if rawURL == "" {
		return false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := rt.oauthHTTPClient(5 * time.Second).Do(req)
	if err != nil {
		if errors.Is(err, oauthflow.ErrNetworkAuthorization) || ctx.Err() != nil {
			return false, err
		}
		return false, nil
	}
	_ = resp.Body.Close()
	return resp.StatusCode != http.StatusUnauthorized, nil
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
func (rt *oauthProtocol) fixBlockedEndpoint(ctx context.Context, rawURL string) (string, error) {
	if rawURL == "" {
		return rawURL, nil
	}
	reachable, err := rt.probeOAuthURL(ctx, rawURL)
	if err != nil {
		return "", err
	}
	if reachable {
		return rawURL, nil
	}
	alt := tryAPIPrefix(rawURL)
	if alt != rawURL {
		reachable, err = rt.probeOAuthURL(ctx, alt)
		if err != nil {
			return "", err
		}
		if reachable {
			logging.Infof(context.Background(), "mcp.oauth", "[OAuth] Endpoint bloqueado reescrito: %s → %s", rawURL, alt)
			return alt, nil
		}
	}
	return rawURL, nil
}

// ============ oauthProtocol ============

// oauthProtocol executa OAuth2 Authorization Code + PKCE e Device Flow
// (RFC 8628) como adaptador do serviço compartilhado, sem transportar recursos.
//
// Suporta:
// - OAuth discovery automático via .well-known (RFC 9728)
// - Device Authorization Flow (RFC 8628) para servidores workforce/corporativos
// - Dynamic Client Registration (RFC 7591) quando registration_endpoint existe
// - Client secret para servidores que exigem client_secret_post (ex: Slack)
// - Porta de callback fixa (oauth2_callback_port) para redirect_uri determinístico
// - Parâmetro resource (RFC 8707)
// oauthProtocol performs only interactive authorization. The shared service owns
// token resolution, leases, persistence and the resource HTTP transport.
type oauthProtocol struct {
	cfg                    ServerConfig
	serverSlug             string
	emitEvent              emitFunc
	registrationCheckpoint func() error
	clientAuthMethod       string
	issuedToken            *oauth2.Token
	networkAuthorizer      oauthflow.NetworkAuthorizer
	mu                     sync.Mutex
	callback               *oauthflow.LoopbackCallback
	oauthCfg               *oauth2.Config
	resolvedClientID       string
	resolvedClientSecret   string
	clientGrantType        string
	resourceURL            string
	discovery              *OAuthDiscovery
}

func (rt *oauthProtocol) effectiveClientID() string {
	if rt.cfg.OAuth2ClientID != "" {
		return rt.cfg.OAuth2ClientID
	}
	return rt.resolvedClientID
}

func (rt *oauthProtocol) effectiveClientSecret() string {
	return rt.resolvedClientSecret
}

// effectiveScopes devolve os scopes a pedir na autorização. Acrescenta
// "offline_access" quando o auth server o anuncia em scopes_supported e ele ainda
// não está configurado — sem isso, provedores como o Atlassian NÃO emitem
// refresh_token e o usuário precisa reautenticar a cada expiração (issue #193).
//
// Sem scopes configurados, preservamos a ausência do parâmetro scope para não
// substituir os defaults do provedor por apenas offline_access.
// Só adicionamos quando o servidor declara suporte para evitar invalid_scope em
// servidores que não conhecem o scope (ex.: alguns que usam access_type=offline).
func (rt *oauthProtocol) effectiveScopes() []string {
	scopes := append([]string(nil), rt.cfg.OAuth2Scopes...)
	if len(scopes) == 0 || containsFold(scopes, "offline_access") {
		return scopes
	}
	if rt.discovery != nil && containsFold(rt.discovery.ScopesSupported, "offline_access") {
		scopes = append(scopes, "offline_access")
		logging.Infof(context.Background(), "mcp.oauth", "[MCP:%s] offline_access adicionado aos scopes (anunciado pelo auth server)", rt.serverSlug)
	}
	return scopes
}

func containsFold(list []string, target string) bool {
	for _, s := range list {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}

// isSessionExpiredStatus retorna true para status HTTP que indicam sessão MCP expirada.
// 404 = sessão não encontrada, 410 = sessão encerrada deliberadamente.
func isSessionExpiredStatus(statusCode int) bool {
	return statusCode == http.StatusNotFound || statusCode == http.StatusGone
}

func (rt *oauthProtocol) authorize(ctx context.Context) error {
	ctx = oauthflow.WithNetworkOperation(ctx)
	endInteraction := beginOAuthInteraction(ctx)
	defer endInteraction()
	if rt.networkAuthorizer != nil {
		ctx = oauthflow.WithNetworkAuthorizer(ctx, rt.networkAuthorizer)
	}
	// The service arbitrates each authorization; this global gate also serializes
	// browser interactions across different consumers.
	release, err := oauthFlowArbiter.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	// 1. Discovery automático de endpoints OAuth (se URL MCP disponível)
	if rt.discovery == nil && rt.cfg.URL != "" {
		disc, err := discoverOAuthEndpoints(ctx, rt.cfg.URL)
		if err != nil {
			if errors.Is(err, oauthflow.ErrNetworkAuthorization) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return err
			}
			logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Discovery automático falhou (usando config manual): %v", rt.serverSlug, err)
		} else {
			rt.discovery = disc
			logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Discovery OK: resource=%s, device_endpoint=%s",
				rt.serverSlug, disc.Resource, disc.DeviceAuthorizationEndpoint)
		}
	}

	// 2. Merge: discovery preenche campos vazios, config manual tem prioridade
	rt.mergeDiscovery()

	// 2.5 Workaround: some workforce auth proxies block browser-facing /oauth/*
	// paths behind mTLS while /api/oauth/* paths are accessible. Probe and fix.
	if rt.cfg.OAuth2AuthURL != "" {
		fixed, err := rt.fixBlockedEndpoint(ctx, rt.cfg.OAuth2AuthURL)
		if err != nil {
			return err
		}
		if fixed != rt.cfg.OAuth2AuthURL {
			logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Authorization endpoint fix: %s → %s", rt.serverSlug, rt.cfg.OAuth2AuthURL, fixed)
			rt.cfg.OAuth2AuthURL = fixed
		}
	}

	defer rt.closeCallback()

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
		logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Tentando Device Authorization Flow", rt.serverSlug)
		err := rt.authorizeDeviceFlow(ctx)
		if terminalDeviceGrantError(ctx, err) {
			return err
		}
		if err == nil {
			return nil
		}

		// Se o client_id não tem grant device_code, re-registrar via DCR e tentar de novo
		if oauthflow.DeviceGrantErrorCode(err) == "unauthorized_client" && rt.cfg.OAuth2RegistrationURL != "" {
			logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Client sem grant device_code — re-registrando via DCR", rt.serverSlug)
			if rerr := rt.reRegisterClient(ctx); rerr != nil {
				if terminalOAuthNetworkError(ctx, rerr) {
					return rerr
				}
				logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Re-registro falhou: %v", rt.serverSlug, rerr)
			} else {
				logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Re-registro OK — retentando Device Flow", rt.serverSlug)
				if err2 := rt.authorizeDeviceFlow(ctx); err2 == nil {
					return nil
				} else {
					if terminalDeviceGrantError(ctx, err2) {
						return err2
					}
					logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Device flow falhou após re-registro: %v", rt.serverSlug, err2)
				}
			}
		} else {
			logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Device flow falhou: %v — tentando PKCE", rt.serverSlug, err)
		}
	}

	// 5. Fallback: PKCE Authorization Code (fluxo atual)
	return rt.authorizePKCE(ctx)
}

// mergeDiscovery fills empty config fields from discovered endpoints.
func (rt *oauthProtocol) mergeDiscovery() {
	d := rt.discovery
	if d == nil {
		return
	}
	if rt.resourceURL == "" || d.Resource != "" {
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
func (rt *oauthProtocol) resolveClientID(ctx context.Context) error {
	if rt.effectiveClientID() != "" || rt.cfg.OAuth2RegistrationURL == "" {
		return nil
	}
	return rt.registerClient(ctx, rt.cfg.OAuth2DeviceAuthURL == "")
}

// Re-register exactly the selected grant. Device grants never reserve a listener.
func (rt *oauthProtocol) reRegisterClient(ctx context.Context) error {
	return rt.registerClient(ctx, rt.cfg.OAuth2DeviceAuthURL == "")
}
func (rt *oauthProtocol) registerClient(ctx context.Context, pkce bool) error {
	var result *oauthflow.RegistrationResponse
	var err error
	var callback *oauthflow.LoopbackCallback
	if pkce {
		callback, err = rt.reserveRegistrationCallback()
		if err != nil {
			return err
		}
		result, err = registerDynamicClient(ctx, rt.cfg, callback.RedirectURI(), rt.effectiveScopes())
	} else {
		result, err = oauthflow.RegisterDynamicClient(ctx, rt.cfg.URL, rt.cfg.OAuth2RegistrationURL, oauthflow.RegistrationRequest{
			ClientName: "Assistente", GrantTypes: []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"}, ResponseTypes: []string{}, TokenEndpointAuthMethod: "none", Scope: strings.Join(rt.effectiveScopes(), " "),
		})
	}
	if err != nil {
		return err
	}
	rt.resolvedClientID, rt.resolvedClientSecret = result.ClientID, result.ClientSecret
	// DCR explicitly registers a public client. Ignore an unsolicited secret.
	rt.clientAuthMethod, rt.resolvedClientSecret = "none", ""
	rt.clientGrantType = "urn:ietf:params:oauth:grant-type:device_code"
	if pkce {
		rt.clientGrantType = "authorization_code"
	}
	rt.cfg.OAuth2ClientID = result.ClientID
	if callback != nil {
		rt.cfg.OAuth2CallbackPort = callback.Port()
	}
	if rt.registrationCheckpoint != nil {
		if err := rt.registrationCheckpoint(); err != nil {
			return err
		}
	}
	logging.Infof(ctx, "mcp.oauth", "client_registration_completed server=%s pkce=%t", rt.serverSlug, pkce)
	return nil
}

// Only dynamically registered clients may move to another port on collision.
func (rt *oauthProtocol) reserveRegistrationCallback() (*oauthflow.LoopbackCallback, error) {
	callback, err := rt.reserveCallback()
	if err == nil || !isAddressInUse(err) || rt.cfg.OAuth2RegistrationURL == "" {
		return callback, err
	}
	callback, err = oauthflow.ReserveCallback(oauthflow.CallbackConfig{Host: rt.cfg.OAuth2CallbackHost, Path: "/callback", PortPolicy: "ephemeral"})
	if err == nil {
		rt.callback = callback
	}
	return callback, err
}

// ============ Device Authorization Flow (RFC 8628) ============

// Probe only the code-free endpoint. Never send the complete verification URI
// through the HTTP client: its session belongs to the browser.
func (rt *oauthProtocol) deviceVerificationURL(ctx context.Context, verification oauthflow.DeviceVerification) (string, error) {
	base, err := url.Parse(verification.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || base.RawQuery != "" || base.Fragment != "" {
		return verification.URL, nil
	}
	fixed, err := rt.fixBlockedEndpoint(ctx, verification.BaseURL)
	if err != nil {
		return "", err
	}
	complete, err := url.Parse(verification.URL)
	if err == nil && fixed != verification.BaseURL && complete.Scheme == base.Scheme && complete.Host == base.Host && complete.EscapedPath() == base.EscapedPath() {
		return tryAPIPrefix(verification.URL), nil
	}
	return verification.URL, nil
}

func (rt *oauthProtocol) authorizeDeviceFlow(ctx context.Context) error {
	clientID := rt.effectiveClientID()
	deviceScopes := rt.effectiveScopes()
	clientSecret, authMethod := rt.effectiveClientSecret(), rt.clientAuthMethod
	result, err := oauthflow.AuthorizeDevice(ctx, oauthflow.DeviceGrantConfig{
		ClientSecret: clientSecret, AuthMethod: authMethod, Resource: rt.cfg.URL, Audience: rt.resourceURL, ClientID: clientID, DeviceEndpoint: rt.cfg.OAuth2DeviceAuthURL, TokenEndpoint: rt.cfg.OAuth2TokenURL, Scopes: deviceScopes, AuthorizeNetwork: rt.networkAuthorizer,
	}, func(ctx context.Context, verification oauthflow.DeviceVerification) error {
		verifyURL, err := rt.deviceVerificationURL(ctx, verification)
		if err != nil {
			return err
		}
		if rt.emitEvent != nil {
			rt.emitEvent("mcp:oauth_device_verify", map[string]string{"slug": rt.serverSlug, "user_code": verification.UserCode, "url": verifyURL})
		}
		if err := browserOpen(verifyURL); err != nil {
			logging.Warnf(ctx, "mcp.oauth", "device_browser_unavailable server=%s", rt.serverSlug)
		}
		return nil
	})
	if err != nil {
		return err
	}
	oauthCfg := &oauth2.Config{ClientID: clientID, Endpoint: oauth2.Endpoint{AuthURL: rt.cfg.OAuth2AuthURL, TokenURL: rt.cfg.OAuth2TokenURL}, Scopes: result.Scopes}
	rt.oauthCfg = oauthCfg
	rt.issuedToken = result.Token
	if result.Token.RefreshToken == "" {
		logging.Warnf(ctx, "mcp.oauth", "device_refresh_token_missing server=%s", rt.serverSlug)
	}
	logging.Infof(ctx, "mcp.oauth", "device_authorization_completed server=%s", rt.serverSlug)
	return nil
}

// ============ PKCE Authorization Code (fluxo original) ============

func (rt *oauthProtocol) authorizePKCE(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	defer rt.closeCallback()
	// A previous Device-only DCR has no registered callback. A PKCE fallback
	// must register the exact reserved URI before opening the browser.
	if rt.clientGrantType == "urn:ietf:params:oauth:grant-type:device_code" {
		if rt.cfg.OAuth2RegistrationURL == "" {
			return errors.New("oauth_code_exchange_failed")
		}
		if err := rt.registerClient(ctx, true); err != nil {
			return err
		}
	}
	callback, err := rt.reserveCallback()
	if err != nil && isAddressInUse(err) && rt.cfg.OAuth2RegistrationURL != "" {
		err = rt.registerClient(ctx, true)
		callback = rt.callback
	}
	if err != nil {
		return err
	}
	redirectURL := callback.RedirectURI()
	// Return the effective callback to the shared service for persistence.
	rt.cfg.OAuth2CallbackPort = callback.Port()

	clientID := rt.effectiveClientID()
	clientSecret := rt.effectiveClientSecret()

	scopes := rt.effectiveScopes()
	logging.Infof(ctx, "mcp.oauth", "[MCP:%s] PKCE authorize: scopes=%v (offline_access=%v)", rt.serverSlug, scopes, containsFold(scopes, "offline_access"))

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  rt.cfg.OAuth2AuthURL,
			TokenURL: rt.cfg.OAuth2TokenURL,
		},
		RedirectURL: redirectURL,
		Scopes:      scopes,
	}

	oauthCfg.Endpoint.AuthStyle = oauth2.AuthStyleInParams
	if rt.clientAuthMethod == "client_secret_basic" && clientSecret != "" {
		oauthCfg.Endpoint.AuthStyle = oauth2.AuthStyleInHeader
	}
	codeVerifier := oauth2.GenerateVerifier()
	state := generateState()

	authURLOpts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(codeVerifier)}
	if rt.resourceURL != "" {
		authURLOpts = append(authURLOpts, oauth2.SetAuthURLParam("resource", rt.resourceURL))
	}
	authURL := oauthCfg.AuthCodeURL(state, authURLOpts...)

	if err := callback.Start(state, oauthflow.CallbackPage{Success: authSuccessHTML, Failure: authErrorHTML, ContentType: "text/html; charset=utf-8", HTMLNonce: true}); err != nil {
		return err
	}

	logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Abrindo browser para autorização OAuth2 PKCE (redirect=%s)", rt.serverSlug, redirectURL)
	if rt.emitEvent != nil {
		rt.emitEvent("mcp:oauth_authorize", map[string]string{
			"slug": rt.serverSlug,
			"url":  authURL,
		})
	}

	if err := browserOpen(authURL); err != nil {
		logging.Warnf(ctx, "mcp.oauth", "pkce_browser_unavailable server=%s", rt.serverSlug)
	}

	exchangeOpts := []oauth2.AuthCodeOption{oauth2.VerifierOption(codeVerifier)}
	if rt.resourceURL != "" {
		exchangeOpts = append(exchangeOpts, oauth2.SetAuthURLParam("resource", rt.resourceURL))
	}

	result, err := callback.Wait(ctx)
	if err != nil {
		return err
	}
	if result.Get("error") != "" {
		return errors.New("oauth_consent_declined")
	}
	token, err := oauthCfg.Exchange(context.WithValue(ctx, oauth2.HTTPClient, rt.oauthHTTPClient(30*time.Second)), result.Get("code"), exchangeOpts...)
	if err != nil {
		var rejected *oauth2.RetrieveError
		if errors.As(err, &rejected) {
			switch rejected.ErrorCode {
			case "invalid_client":
				return oauthflow.ErrClientConfiguration
			case "invalid_scope":
				return oauthflow.ErrPermission
			}
		}
		if terminalOAuthNetworkError(ctx, err) {
			return errors.Join(errors.New("oauth_code_exchange_failed"), safeOAuthExchangeError(ctx, err))
		}
		return errors.New("oauth_code_exchange_failed")
	}
	rt.oauthCfg = oauthCfg
	rt.issuedToken = token
	logging.Infof(ctx, "mcp.oauth", "pkce_authorization_completed server=%s", rt.serverSlug)
	return nil
}

func safeOAuthExchangeError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, oauthflow.ErrNetworkAuthorization) {
		return oauthflow.ErrNetworkAuthorization
	}
	return context.Canceled
}

func (rt *oauthProtocol) closeCallback() {
	if rt.callback != nil {
		rt.callback.Close()
		rt.callback = nil
	}
}
func (rt *oauthProtocol) reserveCallback() (*oauthflow.LoopbackCallback, error) {
	if rt.callback != nil {
		return rt.callback, nil
	}
	policy := "ephemeral"
	if rt.cfg.OAuth2CallbackPort != 0 {
		policy = "fixed"
	}
	callback, err := oauthflow.ReserveCallback(oauthflow.CallbackConfig{Host: rt.cfg.OAuth2CallbackHost, Port: rt.cfg.OAuth2CallbackPort, Path: "/callback", PortPolicy: policy})
	if err == nil {
		rt.callback = callback
	}
	return callback, err
}

func isAddressInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "address already in use") ||
		strings.Contains(lower, "only one usage of each socket address")
}

// ============ Dynamic Client Registration (RFC 7591) ============

func registerDynamicClient(ctx context.Context, cfg ServerConfig, redirectURL string, scopes []string) (*oauthflow.RegistrationResponse, error) {
	return oauthflow.RegisterDynamicClient(ctx, cfg.URL, cfg.OAuth2RegistrationURL, oauthflow.RegistrationRequest{
		RedirectURIs:            []string{redirectURL},
		ClientName:              "Assistente",
		GrantTypes:              []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
		Scope:                   strings.Join(scopes, " "),
	})
}

// Historical readers used only by recovery and tests. Runtime uses OAuthStore.

func clientCredPattern(slug string) string { return "mcp-client:" + slug }
func userTokensPattern(slug string) string { return "mcp-tokens:" + slug }

// loadClientCreds carrega client_id e client_secret do credential
// manager respeitando o escopo de usuário do `ctx`. Sem `ctx`
// user-scoped, o lookup em `Manager.GetByPatternWithContext` ignora
// credenciais user-scoped por construção (anti-leak), então o caller
// PRECISA passar o ctx do user logado para encontrar a credencial.
func loadClientCreds(ctx context.Context, credMgr *credentials.Manager, slug string) (clientID, clientSecret string) {
	if credMgr == nil {
		return
	}
	auth, err := credMgr.GetByPatternWithContext(ctx, clientCredPattern(slug))
	if err != nil || auth == nil {
		return
	}
	return auth.ClientID, auth.ClientSecret
}

// loadUserTokens carrega tokens do usuário do credential manager
// respeitando o escopo de usuário do `ctx`. Mesma observação de
// loadClientCreds: sem ctx user-scoped o lookup nunca acha tokens
// user-scoped.
func loadUserTokens(ctx context.Context, credMgr *credentials.Manager, slug string) *oauth2.Token {
	if credMgr == nil {
		return nil
	}
	auth, err := credMgr.GetByPatternWithContext(ctx, userTokensPattern(slug))
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

func callbackListenAddr(listenIP string, port int) string {
	return net.JoinHostPort(listenIP, fmt.Sprint(port))
}

func generateState() string {
	h := sha256.New()
	b := make([]byte, 32)
	rand.Read(b)
	h.Write(b)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

const authSuccessHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Autorização concluída</title>
<style nonce="` + oauthflow.CallbackNoncePlaceholder + `">body{font-family:sans-serif;text-align:center;padding:40px}</style></head>
<body>
<h2>Autorização concluída!</h2>
<p>Pode fechar esta janela e retornar ao Assistente.</p>
<script nonce="` + oauthflow.CallbackNoncePlaceholder + `">setTimeout(function(){window.close()},3000)</script>
</body></html>`

const authErrorHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Erro de autorização</title>
<style nonce="` + oauthflow.CallbackNoncePlaceholder + `">body{font-family:sans-serif;text-align:center;padding:40px}</style></head>
<body>
<h2>Erro na autorização</h2>
<p>Verifique os logs no Assistente para mais detalhes.</p>
</body></html>`

func (rt *oauthProtocol) oauthHTTPClient(timeout time.Duration) *http.Client {
	return oauthintegrations.MCPHTTPClient(oauthflow.NewNetworkHTTPClient(rt.cfg.URL, rt.networkAuthorizer, timeout), rt.cfg.URL)
}

func terminalOAuthNetworkError(ctx context.Context, err error) bool {
	return err != nil && (errors.Is(err, oauthflow.ErrReauthorize) || errors.Is(err, oauthflow.ErrTransient) || errors.Is(err, oauthflow.ErrConflict) || errors.Is(err, errOAuthPersistence) || errors.Is(err, oauthflow.ErrNetworkAuthorization) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil)
}

func terminalDeviceGrantError(ctx context.Context, err error) bool {
	code := oauthflow.DeviceGrantErrorCode(err)
	return terminalOAuthNetworkError(ctx, err) || errors.Is(err, context.DeadlineExceeded) || code == "access_denied" || code == "expired_token"
}
