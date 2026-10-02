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
	"assistente/internal/database"
	"assistente/internal/oauthflow"

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
		if err := pts.rt.persistTokens(token); err != nil {
			return nil, err
		}
		pts.lastToken = token.AccessToken
		logging.Infof(context.Background(), "mcp.oauth", "[MCP:%s] Token renovado e persistido automaticamente", pts.rt.serverSlug)
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
func (rt *pkceRoundTripper) trySilentRefresh(ctx context.Context, rejected ...string) error {
	if rt.oauthCfg == nil || rt.tokenSource == nil {
		return fmt.Errorf("no oauth config or token source")
	}
	if rt.coordinatedLegacy() {
		_, err := rt.resolveLegacyToken(ctx, rt.oauthCfg, true, rejected...)
		return err
	}

	// Reload-before-refresh: prefere o refresh_token mais recente do store e cai para
	// o token em memória. Duas razões (issue #193):
	//   - outro caminho (reconexão, tool concorrente) pode ter rotacionado e
	//     persistido um novo refresh_token; reusar um já consumido (rotativo de uso
	//     único, ex.: Atlassian) falha e dispara reauth;
	//   - em refresh non-rotativo, o token em memória pode não reter o refresh_token,
	//     mas o store ainda o tem — não exigimos refresh_token em memória aqui.
	refreshToken := ""
	if stored := loadUserTokens(rt.authCtx(), rt.credMgr, rt.serverSlug); stored != nil {
		refreshToken = stored.RefreshToken
	}
	if refreshToken == "" {
		if token, err := rt.tokenSource.Token(); err == nil && token != nil {
			refreshToken = token.RefreshToken
		}
	}
	if refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	expiredToken := &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	refreshCtx := oauthflow.WithNetworkOperation(ctx)

	newSource := rt.oauthCfg.TokenSource(context.WithValue(refreshCtx, oauth2.HTTPClient, rt.oauthHTTPClient(15*time.Second)), expiredToken)
	newToken, err := newSource.Token()
	if err != nil {
		logging.Errorf(ctx, "mcp.oauth", "[MCP:%s] Silent refresh falhou: %v", rt.serverSlug, err)
		return fmt.Errorf("silent refresh failed: %w", err)
	}

	rt.tokenSource = rt.newTokenSource(newToken, rt.oauthCfg)
	if err := rt.persistTokens(newToken); err != nil {
		return err
	}

	rotated := newToken.RefreshToken != "" && newToken.RefreshToken != refreshToken
	logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Token renovado silenciosamente via refresh_token (rotacionado=%v)", rt.serverSlug, rotated)
	return nil
}

// buildClientCredentialsHTTPClient cria um *http.Client que obtém tokens
// via OAuth2 Client Credentials Grant (machine-to-machine).
func buildClientCredentialsHTTPClient(ctx context.Context, cfg ServerConfig, clientSecret string, authorize oauthflow.NetworkAuthorizer) *http.Client {
	return oauthflow.NewResourceHTTPClient(cfg.URL, &oauth2.Transport{
		Source: buildClientCredentialsTokenSource(ctx, cfg, clientSecret, authorize),
		Base:   oauthflow.NewResourceTransport(cfg.URL, authorize),
	})
}

func buildClientCredentialsTokenSource(ctx context.Context, cfg ServerConfig, clientSecret string, authorize oauthflow.NetworkAuthorizer) oauth2.TokenSource {
	return oauthflow.ClientCredentialsTokenSource(ctx, oauthflow.ClientCredentialsConfig{
		Resource: cfg.URL, ClientID: cfg.OAuth2ClientID, ClientSecret: clientSecret, TokenEndpoint: cfg.OAuth2TokenURL, Scopes: cfg.OAuth2Scopes,
	}, authorize)
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
func (rt *pkceRoundTripper) probeOAuthURL(ctx context.Context, rawURL string) (bool, error) {
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
func (rt *pkceRoundTripper) fixBlockedEndpoint(ctx context.Context, rawURL string) (string, error) {
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
	legacyConfig  ServerConfig // last persisted identity, separate from discovered endpoints
	legacySession interface {
		SessionContext(context.Context) (context.Context, context.CancelFunc)
	}
	legacyOperation        *credentials.LegacyOAuthOperation
	explicitAuthorization  bool
	configPersistenceError error
	protocolOnly           bool
	registrationCheckpoint func() error
	clientAuthMethod       string
	issuedToken            *oauth2.Token
	base                   http.RoundTripper
	credMgr                *credentials.Manager
	cfg                    ServerConfig
	emitEvent              emitFunc
	serverSlug             string

	// onConfigUpdate é chamado para persistir mudanças no config (ex: porta após DCR).
	onConfigUpdate      func(ServerConfig)
	persistRegistration func(context.Context, ServerConfig, *credentials.LegacyOAuthOperation, *credentials.AuthConfig) error

	// authCtxProvider devolve o ctx user-scoped vigente. Usado por
	// persistTokens / persistClientCreds em refreshes assíncronos
	// (TokenSource.Token()) que não recebem ctx do caller. Sem ele os
	// refreshes gravavam credenciais com `user_id=''` por engano e a
	// primeira leitura via ctx user-scoped não achava nada — exato
	// vetor que fazia "perdi a autorização do MCP" depois de um
	// restart (ver AEP-0061).
	authCtxProvider   func() context.Context
	networkAuthorizer oauthflow.NetworkAuthorizer
	lifetimeCtx       context.Context

	mu            sync.Mutex
	networkDenied error
	callback      *oauthflow.LoopbackCallback
	tokenSource   oauth2.TokenSource
	oauthCfg      *oauth2.Config

	// Resolved client credentials — from DCR, credential manager, or config
	resolvedClientID     string
	resolvedClientSecret string
	clientGrantType      string

	// resourceURL is the MCP server URL used as the "resource" parameter (RFC 8707).
	resourceURL string

	// discovery caches discovered endpoints (nil = not yet attempted).
	discovery *OAuthDiscovery
}

// authCtx devolve o ctx user-scoped a usar nas operações em background
// (refresh assíncrono via TokenSource). Cai em context.Background()
// apenas se o RT foi construído sem provider — caso de testes que
// montam o RT manualmente sem Manager por trás.
func (rt *pkceRoundTripper) authCtx() context.Context {
	if rt.authCtxProvider == nil {
		return context.Background()
	}
	ctx := rt.authCtxProvider()
	return ctx
}

// Used only by the serialized interactive protocol, while rt.mu is held.
func (rt *pkceRoundTripper) persistenceCtx() context.Context {
	ctx := rt.authCtx()
	if rt.legacyOperation != nil {
		return rt.legacyOperation.Context(ctx)
	}
	return ctx
}

// longLivedCtx keeps refresh independent of individual HTTP requests while
// preserving the connection lifetime: Disconnect cancels network consent too.
// Direct legacy/test construction without a lifetime retains the auth context
// values; actual connections always supply lifetimeCtx before loading tokens.
func (rt *pkceRoundTripper) longLivedCtx() context.Context {
	ctx := rt.lifetimeCtx
	if ctx == nil {
		ctx = context.WithoutCancel(rt.authCtx())
	}
	return context.WithValue(ctx, oauth2.HTTPClient, rt.oauthHTTPClient(30*time.Second))
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

// effectiveScopes devolve os scopes a pedir na autorização. Acrescenta
// "offline_access" quando o auth server o anuncia em scopes_supported e ele ainda
// não está configurado — sem isso, provedores como o Atlassian NÃO emitem
// refresh_token e o usuário precisa reautenticar a cada expiração (issue #193).
//
// Só adicionamos quando o servidor declara suporte para evitar invalid_scope em
// servidores que não conhecem o scope (ex.: alguns que usam access_type=offline).
func (rt *pkceRoundTripper) effectiveScopes() []string {
	scopes := append([]string(nil), rt.cfg.OAuth2Scopes...)
	if containsFold(scopes, "offline_access") {
		return scopes
	}
	if rt.discovery != nil && containsFold(rt.discovery.ScopesSupported, "offline_access") {
		scopes = append(scopes, "offline_access")
		logging.Infof(context.Background(), "mcp.oauth", "[MCP:%s] offline_access adicionado aos scopes (anunciado pelo auth server)", rt.serverSlug)
	}
	return scopes
}

// cachedAccessToken devolve o access_token atual do token source (string vazia se
// não houver). Usado para fotografar o token rejeitado antes de esperar no arbiter
// e, depois, detectar se outro flow o substituiu. Adquire rt.mu internamente, logo
// NÃO deve ser chamado com rt.mu já retido.
func (rt *pkceRoundTripper) cachedAccessToken() string {
	tok, _, err := rt.currentToken()
	if err != nil || tok == nil {
		return ""
	}
	return tok.AccessToken
}

// currentToken serializes source selection and resolution with discovery/DCR.
// Callers already holding rt.mu must use tokenSource.Token directly.
func (rt *pkceRoundTripper) currentToken() (*oauth2.Token, bool, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.networkDenied != nil {
		return nil, true, rt.networkDenied
	}
	if rt.tokenSource == nil {
		return nil, false, nil
	}
	token, err := rt.tokenSource.Token()
	return token, true, err
}

func containsFold(list []string, target string) bool {
	for _, s := range list {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}

func (rt *pkceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	sent := false
	send := func(token *oauth2.Token) (*http.Response, error) {
		cloned := req.Clone(req.Context())
		if sent && req.Body != nil && req.Body != http.NoBody {
			if req.GetBody == nil {
				return nil, errors.New("oauth_request_not_replayable")
			}
			var err error
			cloned.Body, err = req.GetBody()
			if err != nil {
				return nil, errors.New("oauth_request_not_replayable")
			}
		}
		if token != nil {
			token.SetAuthHeader(cloned)
		}
		sent = true
		return rt.base.RoundTrip(cloned)
	}
	token, hasSource, tokenErr := rt.currentToken()
	if terminalOAuthNetworkError(req.Context(), tokenErr) {
		return nil, tokenErr
	}

	if hasSource {
		if tokenErr == nil {
			resp, err := send(token)
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
			silentErr := rt.trySilentRefresh(req.Context(), token.AccessToken)
			rt.mu.Unlock()

			if terminalOAuthNetworkError(req.Context(), silentErr) {
				return nil, silentErr
			}
			if silentErr == nil {
				newToken, hasSource, err := rt.currentToken()
				if hasSource {
					if err == nil {
						resp, err := send(newToken)
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
					} else if terminalOAuthNetworkError(req.Context(), err) {
						return nil, err
					}
				}
			}
		}
	}

	resp, err := send(nil)
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

	token, hasSource, err = rt.currentToken()
	if !hasSource {
		return nil, fmt.Errorf("no token after authorization")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get token after authorization: %w", err)
	}

	return send(token)
}

// isSessionExpiredStatus retorna true para status HTTP que indicam sessão MCP expirada.
// 404 = sessão não encontrada, 410 = sessão encerrada deliberadamente.
func isSessionExpiredStatus(statusCode int) bool {
	return statusCode == http.StatusNotFound || statusCode == http.StatusGone
}

func (rt *pkceRoundTripper) authorize(ctx context.Context) (resultErr error) {
	if rt.legacySession != nil {
		var cancel context.CancelFunc
		ctx, cancel = rt.legacySession.SessionContext(ctx)
		defer cancel()
	}
	ctx = oauthflow.WithNetworkOperation(ctx)
	endInteraction := beginOAuthInteraction(ctx)
	defer endInteraction()
	defer func() {
		if errors.Is(resultErr, oauthflow.ErrNetworkAuthorization) {
			rt.mu.Lock()
			rt.networkDenied = resultErr
			rt.mu.Unlock()
		}
	}()
	if rt.networkAuthorizer != nil {
		ctx = oauthflow.WithNetworkAuthorizer(ctx, rt.networkAuthorizer)
	}
	// authorize() só é chamado após um 401/403, ou seja, o token atual ACABOU de ser
	// rejeitado. Capturamos o access_token rejeitado ANTES de esperar no arbiter para
	// detectar, depois, se OUTRO flow o substituiu enquanto aguardávamos.
	rejected := ""
	if !rt.explicitAuthorization {
		rejected = rt.cachedAccessToken()
	}

	// Serializa entre servidores: enquanto outro MCP estiver fazendo
	// flow OAuth interativo, este espera. rt.mu (logo abaixo) protege
	// dentro DE UM servidor — não substitui o arbiter global.
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

	// Single-flight por servidor: pula a janela SOMENTE se outro flow concorrente
	// instalou um token NOVO e válido (diferente do que foi rejeitado) enquanto
	// esperávamos no arbiter (issue #194). Não basta o token estar "não expirado":
	// ele pode ter sido revogado e rejeitado com 401 — nesse caso o usuário precisa
	// mesmo reautenticar, então seguimos o flow.
	if rt.tokenSource != nil {
		if !rt.explicitAuthorization {
			tok, err := rt.tokenSource.Token()
			if terminalOAuthNetworkError(ctx, err) {
				return err
			}
			if err == nil && tok != nil && tok.Valid() && tok.AccessToken != rejected {
				logging.Infof(ctx, "mcp.oauth", "[MCP:%s] Token renovado por outro flow enquanto aguardávamos o arbiter — pulando nova janela de autorização", rt.serverSlug)
				return nil
			}
		}
	}
	if rt.coordinatedLegacy() {
		ctx = database.WithUserID(ctx, rt.cfg.UserID)
		op, latest, err := rt.credMgr.BeginLegacyOAuth(ctx, rt.serverSlug, rt.cfg.ID, true, rt.explicitAuthorization, rt.validateLegacyConsumer)
		if err != nil {
			return err
		}
		rt.legacyOperation = op
		rt.configPersistenceError = nil
		rt.resolvedClientID = latest.ClientID
		if rt.resolvedClientID == "" {
			rt.resolvedClientID = rt.cfg.OAuth2ClientID
		}
		rt.resolvedClientSecret = latest.ClientSecret
		rt.clientGrantType = latest.ClientGrantType
		defer func() { op.End(); rt.legacyOperation = nil }()
		var cancel context.CancelFunc
		ctx, cancel = op.SessionContext(ctx)
		defer cancel()
		ctx, cancel = context.WithDeadline(ctx, op.Deadline())
		defer cancel()
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
func (rt *pkceRoundTripper) mergeDiscovery() {
	d := rt.discovery
	if d == nil {
		return
	}
	if rt.resourceURL == "" || (rt.protocolOnly && d.Resource != "") {
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
	if rt.effectiveClientID() != "" || rt.cfg.OAuth2RegistrationURL == "" {
		return nil
	}
	return rt.registerClient(ctx, rt.cfg.OAuth2DeviceAuthURL == "")
}

// Re-register exactly the selected grant. Device grants never reserve a listener.
func (rt *pkceRoundTripper) reRegisterClient(ctx context.Context) error {
	return rt.registerClient(ctx, rt.cfg.OAuth2DeviceAuthURL == "")
}
func (rt *pkceRoundTripper) registerClient(ctx context.Context, pkce bool) error {
	previousConfig := rt.cfg
	previousID, previousSecret, previousGrant := rt.resolvedClientID, rt.resolvedClientSecret, rt.clientGrantType
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
	if rt.protocolOnly {
		// Managed DCR explicitly registers a public client, even if the response
		// contains an unsolicited secret. Never authenticate that client with it.
		rt.clientAuthMethod, rt.resolvedClientSecret = "none", ""
	}
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
	if rt.legacyOperation != nil && rt.persistRegistration != nil {
		rt.configPersistenceError = rt.persistRegistration(rt.persistenceCtx(), rt.cfg, rt.legacyOperation, &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: result.ClientID, ClientSecret: result.ClientSecret, ClientGrantType: rt.clientGrantType})
	} else {
		rt.persistClientCreds(result.ClientID, result.ClientSecret)
	}
	if rt.configPersistenceError != nil {
		rt.cfg = previousConfig
		rt.resolvedClientID, rt.resolvedClientSecret, rt.clientGrantType = previousID, previousSecret, previousGrant
		return errOAuthPersistence
	}
	if rt.onConfigUpdate != nil && (rt.legacyOperation == nil || rt.persistRegistration == nil) {
		rt.onConfigUpdate(rt.cfg)
	}
	if rt.configPersistenceError != nil {
		return errOAuthPersistence
	}
	if rt.coordinatedLegacy() && (rt.persistRegistration != nil || rt.onConfigUpdate != nil) {
		rt.legacyConfig = persistedLegacyConfig(rt.cfg)
	}
	logging.Infof(ctx, "mcp.oauth", "client_registration_completed server=%s pkce=%t", rt.serverSlug, pkce)
	return nil
}

// Only dynamically registered clients may move to another port on collision.
func (rt *pkceRoundTripper) reserveRegistrationCallback() (*oauthflow.LoopbackCallback, error) {
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
func (rt *pkceRoundTripper) deviceVerificationURL(ctx context.Context, verification oauthflow.DeviceVerification) (string, error) {
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

func (rt *pkceRoundTripper) authorizeDeviceFlow(ctx context.Context) error {
	clientID := rt.effectiveClientID()
	deviceScopes := rt.effectiveScopes()
	var clientSecret, authMethod string
	if rt.protocolOnly {
		clientSecret, authMethod = rt.effectiveClientSecret(), rt.clientAuthMethod
	}
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
	rt.tokenSource = rt.newTokenSource(result.Token, oauthCfg)
	if err := rt.persistTokens(result.Token); err != nil {
		return err
	}
	if result.Token.RefreshToken == "" {
		logging.Warnf(ctx, "mcp.oauth", "device_refresh_token_missing server=%s", rt.serverSlug)
	}
	logging.Infof(ctx, "mcp.oauth", "device_authorization_completed server=%s", rt.serverSlug)
	return nil
}

// ============ PKCE Authorization Code (fluxo original) ============

func (rt *pkceRoundTripper) authorizePKCE(ctx context.Context) error {
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

	if rt.protocolOnly {
		oauthCfg.Endpoint.AuthStyle = oauth2.AuthStyleInParams
		if rt.clientAuthMethod == "client_secret_basic" && clientSecret != "" {
			oauthCfg.Endpoint.AuthStyle = oauth2.AuthStyleInHeader
		}
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
		if terminalOAuthNetworkError(ctx, err) {
			return errors.Join(errors.New("oauth_code_exchange_failed"), safeOAuthExchangeError(ctx, err))
		}
		return errors.New("oauth_code_exchange_failed")
	}
	rt.oauthCfg = oauthCfg
	rt.tokenSource = rt.newTokenSource(token, oauthCfg)
	if err := rt.persistTokens(token); err != nil {
		return err
	}
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

func (rt *pkceRoundTripper) closeCallback() {
	if rt.callback != nil {
		rt.callback.Close()
		rt.callback = nil
	}
}
func (rt *pkceRoundTripper) reserveCallback() (*oauthflow.LoopbackCallback, error) {
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

// ============ Persist (duas entradas separadas no credential manager) ============

func clientCredPattern(slug string) string { return "mcp-client:" + slug }
func userTokensPattern(slug string) string { return "mcp-tokens:" + slug }

// persistClientCreds salva dados de registro do app (client_id + client_secret) no credential manager.
//
// Usa SEMPRE rt.authCtx() (user-scoped) internamente para gravar com o
// `user_id` correto. Os call sites NÃO devem passar o ctx da operação
// (request-scoped ou derivado de context.Background() nos flows device/PKCE),
// pois esses contextos podem não carregar o `user_id` e gravariam a credencial
// como instance-scoped por engano — qualquer leitura subsequente via ctx
// user-scoped não acharia nada (classe de bug AEP-0061).
func (rt *pkceRoundTripper) persistClientCreds(clientID, clientSecret string) {
	if rt.credMgr == nil {
		return
	}
	auth := &credentials.AuthConfig{Source: "static",
		Type:            "oauth2",
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		ClientGrantType: rt.clientGrantType,
	}
	if rt.legacyOperation != nil {
		if err := rt.legacyOperation.SaveClient(rt.authCtx(), auth); err != nil {
			rt.configPersistenceError = err
		}
		return
	}
	if err := rt.credMgr.RegisterPatternWithContext(rt.persistenceCtx(), clientCredPattern(rt.serverSlug), auth); err != nil {
		rt.configPersistenceError = err
		logging.Errorf(context.Background(), "mcp.oauth", "[MCP:%s] Erro ao salvar credenciais do cliente: %v", rt.serverSlug, err)
	}
}

// persistTokens salva tokens da sessão do usuário (access_token + refresh_token) no credential manager.
//
// Usa SEMPRE rt.authCtx() (user-scoped) internamente pelo mesmo motivo de
// persistClientCreds: a credencial só é útil para o user que executou o flow,
// e o ctx da operação pode não carregar o `user_id` (ex.: device flow deriva de
// context.Background()).
func (rt *pkceRoundTripper) persistTokens(token *oauth2.Token) error {
	if rt.protocolOnly {
		rt.issuedToken = token
		return nil
	}
	if rt.credMgr == nil || token == nil {
		return nil
	}
	ctx := rt.persistenceCtx()
	if rt.legacyOperation != nil {
		auth := &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: token.AccessToken, RefreshURL: token.RefreshToken}
		if !token.Expiry.IsZero() {
			auth.ExpiresAt = token.Expiry.Unix()
		}
		if err := rt.legacyOperation.Commit(ctx, auth); err != nil {
			return errOAuthPersistence
		}
		return nil
	}
	refresh := token.RefreshToken
	if refresh == "" {
		// Refresh non-rotativo: o provedor pode não reenviar o refresh_token numa
		// renovação. Preserva o já armazenado para não perdê-lo — perder o
		// refresh_token força reautenticação interativa (issue #193).
		if existing := loadUserTokens(ctx, rt.credMgr, rt.serverSlug); existing != nil && existing.RefreshToken != "" {
			refresh = existing.RefreshToken
		}
	}
	auth := &credentials.AuthConfig{Source: "static",
		Type:       "oauth2",
		Token:      token.AccessToken,
		RefreshURL: refresh,
	}
	// Persiste a expiração sempre que conhecida (inclusive no passado): expiry zero
	// faz o oauth2 tratar o token como "nunca expira" e nunca renovar proativamente,
	// levando a 401 + refresh reativo a cada request.
	if !token.Expiry.IsZero() {
		auth.ExpiresAt = token.Expiry.Unix()
	}
	if err := rt.credMgr.RegisterPatternWithContext(ctx, userTokensPattern(rt.serverSlug), auth); err != nil {
		if errors.Is(err, oauthflow.ErrConflict) {
			return errors.Join(errOAuthPersistence, oauthflow.ErrConflict)
		}
		return errOAuthPersistence
	}
	return nil
}

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

// buildPKCERoundTripper monta o pkceRoundTripper com o bootstrap de client_id e
// tokens do credential manager. É a base compartilhada por buildPKCEHTTPClient
// (conexão normal) e pela reautorização interativa sob demanda
// (Manager.ReauthorizeServer, AEP-0105), que chama rt.authorize diretamente sem
// depender de um 401 incidental.
//
// `authCtxProvider` devolve o ctx user-scoped vigente — usado para
// gravar/ler credenciais com o user_id correto. Em background, o
// `oauth2.TokenSource` chama `persistTokens` no refresh assíncrono;
// sem `authCtxProvider` aqueles refreshes gravavam como instance e a
// próxima leitura via ctx user-scoped não achava nada (AEP-0061).
//
// `onConfigUpdate` é chamado quando o config precisa ser persistido
// (ex: porta após DCR).
func buildPKCERoundTripper(cfg ServerConfig, credMgr *credentials.Manager, emitEvent emitFunc, slug string, onConfigUpdate func(ServerConfig), authCtxProvider func() context.Context, networkAuthorizer oauthflow.NetworkAuthorizer, lifetimeCtx context.Context) *pkceRoundTripper {
	rt := &pkceRoundTripper{
		legacyConfig:      persistedLegacyConfig(cfg),
		base:              oauthflow.NewResourceTransport(cfg.URL, networkAuthorizer),
		credMgr:           credMgr,
		cfg:               cfg,
		emitEvent:         emitEvent,
		serverSlug:        slug,
		onConfigUpdate:    onConfigUpdate,
		resourceURL:       cfg.URL,
		authCtxProvider:   authCtxProvider,
		networkAuthorizer: networkAuthorizer,
		lifetimeCtx:       lifetimeCtx,
	}

	bootstrapCtx := rt.authCtx()
	if rt.coordinatedLegacy() {
		if store, err := credMgr.OAuthStore(bootstrapCtx); err == nil {
			rt.legacySession, _ = store.(interface {
				SessionContext(context.Context) (context.Context, context.CancelFunc)
			})
		}
	}

	// Entrada 1: dados do cliente (mcp-client:{slug}) → client_id + client_secret
	clientID, clientSecret := loadClientCreds(bootstrapCtx, credMgr, slug)
	if clientID == "" && cfg.OAuth2ClientID != "" {
		clientID = cfg.OAuth2ClientID
		if !rt.coordinatedLegacy() {
			rt.persistClientCreds(clientID, "")
			logging.Infof(context.Background(), "mcp.oauth", "[MCP:%s] client_id importado do config para credential manager", slug)
		}
	}
	rt.resolvedClientID = clientID
	rt.resolvedClientSecret = clientSecret
	if credMgr != nil {
		auth, err := credMgr.GetByPatternWithContext(bootstrapCtx, clientCredPattern(slug))
		if err == nil && auth != nil && auth.ClientID == rt.effectiveClientID() {
			rt.clientGrantType = auth.ClientGrantType
		}
	}

	// Entrada 2: tokens do usuário (mcp-tokens:{slug}) → access_token + refresh_token
	token := loadUserTokens(bootstrapCtx, credMgr, slug)
	if rt.coordinatedLegacy() || token != nil && clientID != "" {
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
		// Token source persistido: ctx long-lived para não morrer com o bootstrap.
		rt.tokenSource = rt.newTokenSource(token, oauthCfg)
	}

	return rt
}

// buildPKCEHTTPClient cria um *http.Client que implementa OAuth2 PKCE.
// Tenta reutilizar tokens e credenciais do credential manager.
func buildPKCEHTTPClient(cfg ServerConfig, credMgr *credentials.Manager, emitEvent emitFunc, slug string, onConfigUpdate func(ServerConfig), authCtxProvider func() context.Context) *http.Client {
	rt := buildPKCERoundTripper(cfg, credMgr, emitEvent, slug, onConfigUpdate, authCtxProvider, nil, nil)
	return oauthflow.NewResourceHTTPClient(cfg.URL, rt)
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

func (rt *pkceRoundTripper) oauthHTTPClient(timeout time.Duration) *http.Client {
	return oauthflow.NewNetworkHTTPClient(rt.cfg.URL, rt.networkAuthorizer, timeout)
}

func terminalOAuthNetworkError(ctx context.Context, err error) bool {
	return err != nil && (errors.Is(err, oauthflow.ErrReauthorize) || errors.Is(err, oauthflow.ErrTransient) || errors.Is(err, oauthflow.ErrConflict) || errors.Is(err, errOAuthPersistence) || errors.Is(err, oauthflow.ErrNetworkAuthorization) || errors.Is(err, context.Canceled) || ctx.Err() != nil)
}

func terminalDeviceGrantError(ctx context.Context, err error) bool {
	code := oauthflow.DeviceGrantErrorCode(err)
	return terminalOAuthNetworkError(ctx, err) || errors.Is(err, context.DeadlineExceeded) || code == "access_denied" || code == "expired_token"
}
