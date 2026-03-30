package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/tools"
)

// TestNewManager testa criação de novo manager
func TestNewManager(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	emitFunc := func(event string, data any) {}

	manager := NewManager(registry, credMgr, emitFunc)

	if manager == nil {
		t.Fatal("NewManager retornou nil")
	}

	if manager.registry != registry {
		t.Error("registry não foi atribuído corretamente")
	}

	if manager.credMgr != credMgr {
		t.Error("credMgr não foi atribuído corretamente")
	}

	if len(manager.servers) != 0 {
		t.Errorf("esperado 0 servers, got %d", len(manager.servers))
	}

	if len(manager.connections) != 0 {
		t.Errorf("esperado 0 connections, got %d", len(manager.connections))
	}
}

// TestSetSamplingHandler testa configuração do handler de sampling
func TestSetSamplingHandler(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Handler customizado
	handlerCalled := false
	customHandler := func(ctx context.Context, req SamplingRequest) (string, error) {
		handlerCalled = true
		return "response", nil
	}

	manager.SetSamplingHandler(customHandler)

	// Verifica que handler foi atribuído
	if manager.llmHandler == nil {
		t.Fatal("handler não foi configurado")
	}

	// Testa invocação do handler
	resp, err := manager.llmHandler(context.Background(), SamplingRequest{})
	if err != nil {
		t.Errorf("handler retornou erro: %v", err)
	}
	if resp != "response" {
		t.Errorf("esperado 'response', got %q", resp)
	}
	if !handlerCalled {
		t.Error("handler não foi chamado")
	}
}

// TestSetWorkspaceRoots testa configuração de workspace roots
func TestSetWorkspaceRoots(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	roots := []Root{
		{URI: "file:///home/user/project", Name: "my-project"},
		{URI: "file:///home/user/other", Name: "other"},
	}

	err := manager.SetWorkspaceRoots(roots)
	if err != nil {
		t.Errorf("SetWorkspaceRoots retornou erro: %v", err)
	}

	// Verifica que roots foram armazenados
	retrievedRoots := manager.GetWorkspaceRoots()
	if len(retrievedRoots) != len(roots) {
		t.Errorf("esperado %d roots, got %d", len(roots), len(retrievedRoots))
	}

	for i, root := range retrievedRoots {
		if root.URI != roots[i].URI || root.Name != roots[i].Name {
			t.Errorf("root %d não bate: %+v vs %+v", i, root, roots[i])
		}
	}
}

// TestGetWorkspaceRoots testa recuperação de workspace roots
func TestGetWorkspaceRoots(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Sem roots configurados
	roots := manager.GetWorkspaceRoots()
	if roots == nil {
		roots = []Root{}
	}
	if len(roots) != 0 {
		t.Errorf("esperado 0 roots inicialmente, got %d", len(roots))
	}

	// Depois de configurar
	newRoots := []Root{{URI: "file:///test", Name: "test"}}
	manager.SetWorkspaceRoots(newRoots)

	roots = manager.GetWorkspaceRoots()
	if len(roots) != 1 {
		t.Errorf("esperado 1 root, got %d", len(roots))
	}
}

// TestManagerConcurrency testa acesso concorrente ao manager
func TestManagerConcurrency(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Executa operações concorrentes
	done := make(chan bool, 10)

	// Goroutine 1: Set roots
	go func() {
		manager.SetWorkspaceRoots([]Root{
			{URI: "file:///test1", Name: "test1"},
		})
		done <- true
	}()

	// Goroutine 2: Get roots
	go func() {
		manager.GetWorkspaceRoots()
		done <- true
	}()

	// Goroutine 3: Set handler
	go func() {
		manager.SetSamplingHandler(func(ctx context.Context, req SamplingRequest) (string, error) {
			return "", nil
		})
		done <- true
	}()

	// Goroutine 4-10: More gets
	for i := 0; i < 7; i++ {
		go func() {
			manager.GetWorkspaceRoots()
			done <- true
		}()
	}

	// Aguarda todas as goroutinas
	for i := 0; i < 10; i++ {
		<-done
	}

	// Se chegou aqui sem deadlock/panic, test passou
}

// TestEmitEvent testa emissão de eventos
func TestEmitEvent(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)

	eventChan := make(chan struct { name string; data any }, 10)
	emitFunc := func(event string, data any) {
		eventChan <- struct {
			name string
			data any
		}{event, data}
	}

	manager := NewManager(registry, credMgr, emitFunc)

	// SetWorkspaceRoots deve emitir evento
	manager.SetWorkspaceRoots([]Root{
		{URI: "file:///test", Name: "test"},
	})

	// Tenta receber evento (non-blocking)
	select {
	case event := <-eventChan:
		if event.name != "mcp:roots_changed" {
			t.Errorf("esperado 'mcp:roots_changed', got %q", event.name)
		}
	default:
		t.Error("nenhum evento foi emitido")
	}
}

// TestManagerCancel testa cancelamento de context
func TestManagerCancel(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Context deve estar válido inicialmente
	select {
	case <-manager.ctx.Done():
		t.Fatal("context estava cancelado no início")
	default:
		// OK
	}

	// Cancelar
	manager.cancel()

	// Context deve estar cancelado agora
	select {
	case <-manager.ctx.Done():
		// OK
	default:
		t.Fatal("context não foi cancelado após Cancel()")
	}
}

// TestManagerStateConsistency testa consistência de estado
func TestManagerStateConsistency(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Configura roots
	roots1 := []Root{{URI: "file:///test1", Name: "test1"}}
	manager.SetWorkspaceRoots(roots1)

	// Obtém valores
	retrieved1 := manager.GetWorkspaceRoots()
	if len(retrieved1) != 1 || retrieved1[0].URI != "file:///test1" {
		t.Error("state inconsistency: roots1 não foram recuperados corretamente")
	}

	// Atualiza roots
	roots2 := []Root{
		{URI: "file:///test2", Name: "test2"},
		{URI: "file:///test3", Name: "test3"},
	}
	manager.SetWorkspaceRoots(roots2)

	// Obtém valores novamente
	retrieved2 := manager.GetWorkspaceRoots()
	if len(retrieved2) != 2 {
		t.Errorf("state inconsistency: esperado 2 roots, got %d", len(retrieved2))
	}

	// Verifica que são os novos
	if retrieved2[0].URI != "file:///test2" {
		t.Errorf("state inconsistency: esperado test2, got %s", retrieved2[0].URI)
	}
}

// TestSetMultipleHandlers testa múltiplas configurações de handler
func TestSetMultipleHandlers(t *testing.T) {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	manager := NewManager(registry, credMgr, func(string, any) {})

	// Handler 1
	manager.SetSamplingHandler(func(ctx context.Context, req SamplingRequest) (string, error) {
		return "handler1", nil
	})

	resp1, _ := manager.llmHandler(context.Background(), SamplingRequest{})
	if resp1 != "handler1" {
		t.Errorf("esperado 'handler1', got %q", resp1)
	}

	// Handler 2 (substitui)
	manager.SetSamplingHandler(func(ctx context.Context, req SamplingRequest) (string, error) {
		return "handler2", nil
	})

	resp2, _ := manager.llmHandler(context.Background(), SamplingRequest{})
	if resp2 != "handler2" {
		t.Errorf("esperado 'handler2', got %q", resp2)
	}
}

// ==================== checkAndRefreshToken Tests ====================

func newTestManager() *Manager {
	registry := tools.NewRegistry()
	credMgr := credentials.NewManager(nil)
	return NewManager(registry, credMgr, func(string, any) {})
}

func newTestManagerWithTempDir(t *testing.T) *Manager {
	t.Helper()
	m := newTestManager()
	m.resolver = configdir.NewResolverWithBase(t.TempDir())
	return m
}

func TestCheckAndRefreshToken_SkipsNonExistentServer(t *testing.T) {
	m := newTestManager()
	m.checkAndRefreshToken("nonexistent")
}

func TestCheckAndRefreshToken_SkipsReconnectingServer(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:         "test",
		Config:       ServerConfig{AuthType: AuthOAuth2PKCE},
		Reconnecting: true,
	}
	m.checkAndRefreshToken("test")
}

func TestCheckAndRefreshToken_SkipsNonOAuth2Server(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:   "test",
		Config: ServerConfig{AuthType: AuthBearer},
	}
	m.checkAndRefreshToken("test")
}

func TestCheckAndRefreshToken_SkipsWhenNoTokenStored(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:   "test",
		Config: ServerConfig{AuthType: AuthOAuth2PKCE},
	}
	m.checkAndRefreshToken("test")
}

func TestCheckAndRefreshToken_SkipsWhenTokenFarFromExpiry(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:   "test",
		Config: ServerConfig{AuthType: AuthOAuth2PKCE},
	}

	farFuture := time.Now().Add(1 * time.Hour).Unix()
	_ = m.credMgr.RegisterPatternWithContext(context.Background(), userTokensPattern("test"), &credentials.AuthConfig{
		Type:       "oauth2",
		Token:      "access-valid",
		RefreshURL: "refresh-123",
		ExpiresAt:  farFuture,
	})

	m.checkAndRefreshToken("test")

	auth, _ := m.credMgr.GetByPattern(userTokensPattern("test"))
	if auth.Token != "access-valid" {
		t.Errorf("token should not have changed, got %q", auth.Token)
	}
}

func TestCheckAndRefreshToken_SkipsWhenNoRefreshToken(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:   "test",
		Config: ServerConfig{AuthType: AuthOAuth2PKCE},
	}

	soonExpiry := time.Now().Add(30 * time.Second).Unix()
	_ = m.credMgr.RegisterPatternWithContext(context.Background(), userTokensPattern("test"), &credentials.AuthConfig{
		Type:      "oauth2",
		Token:     "access-expiring",
		ExpiresAt: soonExpiry,
	})

	m.checkAndRefreshToken("test")

	auth, _ := m.credMgr.GetByPattern(userTokensPattern("test"))
	if auth.Token != "access-expiring" {
		t.Errorf("token should not have changed (no refresh token), got %q", auth.Token)
	}
}

func TestCheckAndRefreshToken_RefreshesExpiringToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug: "test",
		Config: ServerConfig{
			AuthType:       AuthOAuth2PKCE,
			OAuth2ClientID: "test-client",
			OAuth2TokenURL: tokenServer.URL,
			OAuth2AuthURL:  "http://unused/auth",
		},
	}

	soonExpiry := time.Now().Add(30 * time.Second).Unix()
	_ = m.credMgr.RegisterPatternWithContext(context.Background(), userTokensPattern("test"), &credentials.AuthConfig{
		Type:       "oauth2",
		Token:      "old-access-token",
		RefreshURL: "old-refresh-token",
		ExpiresAt:  soonExpiry,
	})

	m.checkAndRefreshToken("test")

	auth, err := m.credMgr.GetByPattern(userTokensPattern("test"))
	if err != nil {
		t.Fatalf("error reading token: %v", err)
	}
	if auth.Token != "new-access-token" {
		t.Errorf("expected refreshed token 'new-access-token', got %q", auth.Token)
	}
	if auth.RefreshURL != "new-refresh-token" {
		t.Errorf("expected refreshed refresh_token 'new-refresh-token', got %q", auth.RefreshURL)
	}
}

func TestCheckAndRefreshToken_HandlesRefreshFailure(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer tokenServer.Close()

	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug: "test",
		Config: ServerConfig{
			AuthType:       AuthOAuth2PKCE,
			OAuth2ClientID: "test-client",
			OAuth2TokenURL: tokenServer.URL,
			OAuth2AuthURL:  "http://unused/auth",
		},
	}

	soonExpiry := time.Now().Add(30 * time.Second).Unix()
	_ = m.credMgr.RegisterPatternWithContext(context.Background(), userTokensPattern("test"), &credentials.AuthConfig{
		Type:       "oauth2",
		Token:      "old-access-token",
		RefreshURL: "old-refresh-token",
		ExpiresAt:  soonExpiry,
	})

	m.checkAndRefreshToken("test")

	auth, _ := m.credMgr.GetByPattern(userTokensPattern("test"))
	if auth.Token != "old-access-token" {
		t.Errorf("token should remain unchanged after failed refresh, got %q", auth.Token)
	}
}

func TestCheckAndRefreshToken_UsesStoredClientCreds(t *testing.T) {
	var receivedAuth string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refreshed",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug: "test",
		Config: ServerConfig{
			AuthType:       AuthOAuth2PKCE,
			OAuth2ClientID: "",
			OAuth2TokenURL: tokenServer.URL,
			OAuth2AuthURL:  "http://unused/auth",
		},
	}

	rt := &pkceRoundTripper{credMgr: m.credMgr, serverSlug: "test"}
	rt.persistClientCreds("stored-client-id", "stored-secret")

	soonExpiry := time.Now().Add(30 * time.Second).Unix()
	_ = m.credMgr.RegisterPatternWithContext(context.Background(), userTokensPattern("test"), &credentials.AuthConfig{
		Type:       "oauth2",
		Token:      "old",
		RefreshURL: "refresh-tok",
		ExpiresAt:  soonExpiry,
	})

	m.checkAndRefreshToken("test")

	if receivedAuth == "" {
		t.Error("expected Authorization header with stored client credentials")
	}

	auth, _ := m.credMgr.GetByPattern(userTokensPattern("test"))
	if auth.Token != "refreshed" {
		t.Errorf("expected refreshed token, got %q", auth.Token)
	}
}

// ==================== handleToolCallError Tests ====================

func TestHandleToolCallError_SetsErrorAndTriggersReconnect(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:   "test",
		Config: ServerConfig{Enabled: true},
		Status: StatusConnected,
	}
	m.connections["test"] = &serverConnection{}

	events := make(chan string, 5)
	m.emitEvent = func(event string, data any) {
		events <- event
	}

	m.handleToolCallError("test", fmt.Errorf("connection reset"))

	s := m.servers["test"]
	if s.Status != StatusError {
		t.Errorf("expected StatusError, got %v", s.Status)
	}
	if s.ConsecutiveHealthFailures != healthCheckFailThreshold {
		t.Errorf("expected failures=%d, got %d", healthCheckFailThreshold, s.ConsecutiveHealthFailures)
	}

	select {
	case event := <-events:
		if event != "mcp:server_unhealthy" {
			t.Errorf("expected mcp:server_unhealthy event, got %q", event)
		}
	case <-time.After(1 * time.Second):
		t.Error("no event emitted within timeout")
	}
}

func TestHandleToolCallError_SkipsDisabledServer(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:   "test",
		Config: ServerConfig{Enabled: false},
		Status: StatusConnected,
	}

	m.handleToolCallError("test", fmt.Errorf("error"))

	if m.servers["test"].Status != StatusConnected {
		t.Error("should not change status for disabled server")
	}
}

func TestHandleToolCallError_SkipsReconnectingServer(t *testing.T) {
	m := newTestManager()
	m.servers["test"] = &ServerStatus{
		Slug:         "test",
		Config:       ServerConfig{Enabled: true},
		Status:       StatusError,
		Reconnecting: true,
	}

	m.handleToolCallError("test", fmt.Errorf("error"))

	if m.servers["test"].ConsecutiveHealthFailures != 0 {
		t.Error("should not increment failures for reconnecting server")
	}
}

func TestHandleToolCallError_SkipsNonExistentServer(t *testing.T) {
	m := newTestManager()
	m.handleToolCallError("nonexistent", fmt.Errorf("error"))
}

// ============ Auto-Detect Transport Tests ============

func TestAutoDetectTransport_URL(t *testing.T) {
	data := []byte(`{"url": "https://mcp.example.com/mcp"}`)
	cfg, err := ParseServerConfig(data, "example")
	if err != nil {
		t.Fatalf("ParseServerConfig failed: %v", err)
	}
	if cfg.Transport != TransportStreamable {
		t.Errorf("expected streamable transport, got %q", cfg.Transport)
	}
	if cfg.Name != "Example" {
		t.Errorf("expected name 'Example', got %q", cfg.Name)
	}
	if !cfg.Enabled {
		t.Error("expected enabled=true by default")
	}
	if !cfg.AutoConnect {
		t.Error("expected auto_connect=true by default")
	}
}

func TestAutoDetectTransport_Command(t *testing.T) {
	data := []byte(`{"command": "node", "args": ["server.js"]}`)
	cfg, err := ParseServerConfig(data, "my-server")
	if err != nil {
		t.Fatalf("ParseServerConfig failed: %v", err)
	}
	if cfg.Transport != TransportStdio {
		t.Errorf("expected stdio transport, got %q", cfg.Transport)
	}
	if cfg.Name != "My server" {
		t.Errorf("expected name 'My server', got %q", cfg.Name)
	}
}

func TestAutoDetectTransport_ExplicitOverride(t *testing.T) {
	data := []byte(`{"url": "https://example.com", "transport": "sse", "enabled": false, "auto_connect": false, "name": "Custom Name"}`)
	cfg, err := ParseServerConfig(data, "example")
	if err != nil {
		t.Fatalf("ParseServerConfig failed: %v", err)
	}
	if cfg.Transport != TransportSSE {
		t.Errorf("expected explicit sse transport, got %q", cfg.Transport)
	}
	if cfg.Name != "Custom Name" {
		t.Errorf("expected explicit name, got %q", cfg.Name)
	}
	if cfg.Enabled {
		t.Error("expected enabled=false when explicitly set")
	}
	if cfg.AutoConnect {
		t.Error("expected auto_connect=false when explicitly set")
	}
}

func TestParseServerConfig_DefaultsForMinimalURL(t *testing.T) {
	data := []byte(`{"url": "https://mcp.ist.nubank.world/mcp"}`)
	cfg, err := ParseServerConfig(data, "nu-mcp")
	if err != nil {
		t.Fatalf("ParseServerConfig failed: %v", err)
	}

	if cfg.Transport != TransportStreamable {
		t.Errorf("transport: got %q, want streamable", cfg.Transport)
	}
	if !cfg.Enabled {
		t.Error("enabled should default to true")
	}
	if !cfg.AutoConnect {
		t.Error("auto_connect should default to true")
	}
	if cfg.Name != "Nu mcp" {
		t.Errorf("name: got %q, want 'Nu mcp'", cfg.Name)
	}
}

// ============ ImportFromMCPJSON Tests ============

func TestImportFromMCPJSON_CursorFormat(t *testing.T) {
	m := newTestManagerWithTempDir(t)

	mcpJSON := []byte(`{
		"mcpServers": {
			"my-server": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem"],
				"env": {"HOME": "/tmp"}
			},
			"remote-server": {
				"url": "https://mcp.example.com/sse"
			}
		}
	}`)

	count, err := m.ImportFromMCPJSON(mcpJSON)
	if err != nil {
		t.Fatalf("ImportFromMCPJSON failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 imported, got %d", count)
	}

	// Check that servers are now loaded
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.servers["my-server"]; !ok {
		t.Error("my-server should be loaded after import")
	}
	if _, ok := m.servers["remote-server"]; !ok {
		t.Error("remote-server should be loaded after import")
	}
}

func TestImportFromMCPJSON_SkipsExisting(t *testing.T) {
	m := newTestManagerWithTempDir(t)
	m.servers["existing-server"] = &ServerStatus{
		Slug:   "existing-server",
		Config: ServerConfig{Name: "Existing"},
		Status: StatusDisconnected,
	}

	mcpJSON := []byte(`{
		"mcpServers": {
			"existing-server": {
				"command": "node",
				"args": ["new-server.js"]
			},
			"new-server": {
				"command": "node",
				"args": ["new.js"]
			}
		}
	}`)

	count, err := m.ImportFromMCPJSON(mcpJSON)
	if err != nil {
		t.Fatalf("ImportFromMCPJSON failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 imported (skipping existing), got %d", count)
	}
}

func TestImportFromMCPJSON_EmptyInput(t *testing.T) {
	m := newTestManagerWithTempDir(t)

	count, err := m.ImportFromMCPJSON([]byte(`{}`))
	if err != nil {
		t.Fatalf("ImportFromMCPJSON failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 imported for empty input, got %d", count)
	}
}

func TestImportFromMCPJSON_InvalidJSON(t *testing.T) {
	m := newTestManagerWithTempDir(t)

	_, err := m.ImportFromMCPJSON([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSanitizeSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Server", "my-server"},
		{"my_server", "my-server"},
		{"server-123", "server-123"},
		{"Server With Spaces", "server-with-spaces"},
		{"UPPERCASE", "uppercase"},
		{"special!@#chars", "specialchars"},
	}
	for _, tc := range tests {
		got := sanitizeSlug(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeSlug(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}
