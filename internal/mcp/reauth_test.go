package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/tools"
)

// capturingEmitter registra os eventos emitidos pelo Manager para asserção.
type capturingEmitter struct {
	mu     sync.Mutex
	events []capturedEvent
}

type capturedEvent struct {
	name string
	data any
}

func (c *capturingEmitter) emit(name string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, capturedEvent{name: name, data: data})
}

func (c *capturingEmitter) countByName(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.events {
		if e.name == name {
			n++
		}
	}
	return n
}

func (c *capturingEmitter) lastByName(name string) (capturedEvent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i].name == name {
			return c.events[i], true
		}
	}
	return capturedEvent{}, false
}

func newTestManagerWithEmit(emit emitFunc) *Manager {
	return NewManager(tools.NewRegistry(), credentials.NewManager(nil), emit)
}

func storeUserToken(t *testing.T, m *Manager, slug, access, refresh string, expiresAt int64) {
	t.Helper()
	auth := &credentials.AuthConfig{Source: "static",
		Type:       "oauth2",
		Token:      access,
		RefreshURL: refresh,
		ExpiresAt:  expiresAt,
	}
	if err := m.credMgr.RegisterPatternWithContext(context.Background(), userTokensPattern(slug), auth); err != nil {
		t.Fatalf("RegisterPatternWithContext: %v", err)
	}
}

// ============ GetEligibleNativeMCPServers: token expirado ============

func TestGetEligibleNativeMCPServers_ExpiredWithoutRefreshSignalsReauthAndSkips(t *testing.T) {
	emitter := &capturingEmitter{}
	m := newTestManagerWithEmit(emitter.emit)

	storeUserToken(t, m, "atlassian", "dead-token", "", time.Now().Add(-time.Hour).Unix())
	m.servers["atlassian"] = &ServerStatus{
		Slug:   "atlassian",
		Status: StatusConnected,
		Config: ServerConfig{
			Name:      "Atlassian",
			Transport: TransportSSE,
			URL:       "https://mcp.atlassian.com/v1/sse",
			AuthType:  AuthOAuth2PKCE,
		},
		Tools: []MCPToolInfo{{Name: "jira", FullName: "mcp_atlassian__jira"}},
	}

	result := m.GetEligibleNativeMCPServers()

	if len(result) != 0 {
		t.Fatalf("servidor com token morto NÃO deveria entrar no MCP nativo, got %#v", result)
	}
	m.mu.RLock()
	needs := m.servers["atlassian"].NeedsReauth
	m.mu.RUnlock()
	if !needs {
		t.Error("servidor deveria estar marcado como NeedsReauth")
	}
	if emitter.countByName("mcp:server_needs_reauth") != 1 {
		t.Errorf("esperava 1 evento mcp:server_needs_reauth, got %d", emitter.countByName("mcp:server_needs_reauth"))
	}
	ev, ok := emitter.lastByName("mcp:server_needs_reauth")
	if !ok {
		t.Fatal("evento mcp:server_needs_reauth não emitido")
	}
	payload, ok := ev.data.(MCPServerReauthEvent)
	if !ok {
		t.Fatalf("payload deveria ser MCPServerReauthEvent, got %T", ev.data)
	}
	if payload.Slug != "atlassian" || payload.Name != "Atlassian" {
		t.Errorf("payload inesperado: %#v", payload)
	}

	// Idempotência: uma segunda chamada não deve reemitir o evento (só transição).
	_ = m.GetEligibleNativeMCPServers()
	if emitter.countByName("mcp:server_needs_reauth") != 1 {
		t.Errorf("evento de reauth não deveria repetir por turno, got %d", emitter.countByName("mcp:server_needs_reauth"))
	}
}

func TestGetEligibleNativeMCPServers_ExpiredTokenRefreshedAndDelivered(t *testing.T) {
	var gotRefresh string
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "fresh-token",
			"token_type":    "Bearer",
			"refresh_token": "rotated-refresh",
			"expires_in":    3600,
		})
	}))
	defer tokenSrv.Close()

	emitter := &capturingEmitter{}
	m := newTestManagerWithEmit(emitter.emit)

	storeUserToken(t, m, "srv", "old-token", "stored-refresh", time.Now().Add(-time.Minute).Unix())
	m.servers["srv"] = &ServerStatus{
		Slug:   "srv",
		Status: StatusConnected,
		Config: ServerConfig{
			Name:           "Srv",
			Transport:      TransportSSE,
			URL:            "https://mcp.example.com/sse",
			AuthType:       AuthOAuth2PKCE,
			OAuth2ClientID: "client-x",
			OAuth2TokenURL: tokenSrv.URL + "/token",
		},
		Tools: []MCPToolInfo{{Name: "t", FullName: "mcp_srv__t"}},
	}

	result := m.GetEligibleNativeMCPServers()

	if len(result) != 1 {
		t.Fatalf("servidor deveria entrar no MCP nativo após refresh, got %#v", result)
	}
	if result[0].AuthToken != "fresh-token" {
		t.Errorf("token entregue deveria ser o renovado, got %q", result[0].AuthToken)
	}
	if gotRefresh != "stored-refresh" {
		t.Errorf("refresh deveria usar o refresh_token do cofre, got %q", gotRefresh)
	}
	m.mu.RLock()
	needs := m.servers["srv"].NeedsReauth
	m.mu.RUnlock()
	if needs {
		t.Error("servidor renovado não deveria estar marcado como NeedsReauth")
	}
	if emitter.countByName("mcp:server_needs_reauth") != 0 {
		t.Errorf("não deveria emitir reauth quando o refresh teve sucesso, got %d", emitter.countByName("mcp:server_needs_reauth"))
	}
}

func TestGetEligibleNativeMCPServers_ValidTokenDeliveredWithoutRefresh(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("token endpoint NÃO deveria ser chamado para token válido (path=%s)", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tokenSrv.Close()

	m := newTestManagerWithEmit(func(string, any) {})
	storeUserToken(t, m, "srv", "good-token", "r1", time.Now().Add(time.Hour).Unix())
	m.servers["srv"] = &ServerStatus{
		Slug:   "srv",
		Status: StatusConnected,
		Config: ServerConfig{
			Name:           "Srv",
			Transport:      TransportSSE,
			URL:            "https://mcp.example.com/sse",
			AuthType:       AuthOAuth2PKCE,
			OAuth2ClientID: "client-x",
			OAuth2TokenURL: tokenSrv.URL + "/token",
		},
		Tools: []MCPToolInfo{{Name: "t", FullName: "mcp_srv__t"}},
	}

	result := m.GetEligibleNativeMCPServers()
	if len(result) != 1 || result[0].AuthToken != "good-token" {
		t.Fatalf("token válido deveria ser entregue sem refresh, got %#v", result)
	}
}

// ============ ReauthorizeServer ============

func TestReauthorizeServer_RejectsNonOAuthServer(t *testing.T) {
	m := newTestManagerWithEmit(func(string, any) {})
	m.servers["bearer"] = &ServerStatus{
		Slug:   "bearer",
		Status: StatusConnected,
		Config: ServerConfig{AuthType: AuthBearer},
	}
	err := m.ReauthorizeServer(context.Background(), "bearer")
	if err == nil {
		t.Fatal("esperava erro ao reautorizar servidor não-OAuth")
	}
}

func TestReauthorizeServer_UnknownServer(t *testing.T) {
	m := newTestManagerWithEmit(func(string, any) {})
	if err := m.ReauthorizeServer(context.Background(), "missing"); err == nil {
		t.Fatal("esperava erro para servidor inexistente")
	}
}

func TestReauthorizeServer_RunsInteractiveFlowPersistsTokenAndReconnects(t *testing.T) {
	// Servidor OAuth: só precisa do /token (o /authorize é substituído pelo stub
	// de browser que chama o callback local com um code).
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"reauth-access","token_type":"Bearer","refresh_token":"reauth-refresh","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	browserHit := make(chan struct{}, 1)
	oldBrowserOpen := browserOpen
	browserOpen = func(rawURL string) error {
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		go func() {
			resp, err := http.Get(redirectURI + "?code=ok&state=" + url.QueryEscape(state))
			if err == nil {
				_ = resp.Body.Close()
			}
			select {
			case browserHit <- struct{}{}:
			default:
			}
		}()
		return nil
	}
	defer func() { browserOpen = oldBrowserOpen }()

	emitter := &capturingEmitter{}
	m := newTestManagerWithEmit(emitter.emit)

	// Transport in-memory para a reconexão pós-reauth ter sucesso sem rede.
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport

	m.servers["srv"] = &ServerStatus{
		Slug:        "srv",
		Status:      StatusDisconnected,
		NeedsReauth: true,
		Config: ServerConfig{
			Name:               "Srv",
			Transport:          TransportStdio, // in-memory factory; evita probe SSE/rede
			Enabled:            true,
			AuthType:           AuthOAuth2PKCE,
			OAuth2ClientID:     "client-x",
			OAuth2AuthURL:      tokenSrv.URL + "/authorize",
			OAuth2TokenURL:     tokenSrv.URL + "/token",
			OAuth2CallbackHost: "127.0.0.1",
		},
		Tools: []MCPToolInfo{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.ReauthorizeServer(ctx, "srv"); err != nil {
		t.Fatalf("ReauthorizeServer: %v", err)
	}

	select {
	case <-browserHit:
	case <-time.After(2 * time.Second):
		t.Error("browser não foi acionado no fluxo interativo")
	}

	loaded := loadUserTokens(context.Background(), m.credMgr, "srv")
	if loaded == nil || loaded.AccessToken != "reauth-access" {
		t.Fatalf("token novo deveria ter sido persistido, got %#v", loaded)
	}

	m.mu.RLock()
	needs := m.servers["srv"].NeedsReauth
	status := m.servers["srv"].Status
	m.mu.RUnlock()
	if needs {
		t.Error("NeedsReauth deveria ter sido limpo após reautorização")
	}
	if status != StatusConnected {
		t.Errorf("servidor deveria reconectar após reautorização, status=%s", status)
	}

	m.CloseAll()
}

func TestRefreshOAuthPersistsExplicitStaticSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"renewed","refresh_token":"next-refresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	m := newTestManagerWithEmit(func(string, any) {})
	storeUserToken(t, m, "source-test", "old", "refresh", time.Now().Add(-time.Hour).Unix())
	m.servers["source-test"] = &ServerStatus{Slug: "source-test", Config: ServerConfig{AuthType: AuthOAuth2PKCE, OAuth2TokenURL: server.URL, OAuth2ClientID: "client"}}
	refreshed, err := m.refreshOAuthTokenBestEffort(context.Background(), "source-test", true)
	if err != nil || !refreshed {
		t.Fatalf("refreshed=%v err=%v", refreshed, err)
	}
	auth, err := m.credMgr.GetByPatternWithContext(context.Background(), userTokensPattern("source-test"))
	if err != nil || auth.Source != "static" || auth.Token != "renewed" || auth.RefreshURL != "next-refresh" {
		t.Fatalf("refresh not persisted: %v", err)
	}
}
