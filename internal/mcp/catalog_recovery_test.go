package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/oauthflow"
	"assistente/internal/toolcatalog"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCatalogRecoveryAfterStartupRequiresUseAndFreshDiscovery(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	sqlDB, err := repo.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	catalog := toolcatalog.NewService(toolcatalog.NewDBRepository(repo.db))
	m.SetCatalog(catalog)
	m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		return d.IPs, true, nil
	})
	var browsers, executions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"ACCESS","refresh_token":"REFRESH","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	cfg := managedConfig(server.URL)
	cfg.DisableSSE = true
	cfg.AutoConnect = true
	if err := m.SaveConfig("remote", cfg); err != nil {
		t.Fatal(err)
	}
	current, err := m.GetConfig("remote")
	if err != nil {
		t.Fatal(err)
	}
	name := BuildToolName("remote", "search")
	if err := catalog.SyncMCPServerTools(ctx, "remote", current.ID, []toolcatalog.MCPToolDescriptor{{FullName: name, Name: "search", Description: "old", Schema: json.RawMessage(`{"type":"object"}`)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.MarkServerToolsUnavailable(ctx, current.ID, nil, "server disconnected"); err != nil {
		t.Fatal(err)
	}
	factory := newInMemoryMCPFactory(t, context.Background())
	defer factory.close()
	factory.server.AddTool(&mcpsdk.Tool{Name: "search", Description: "fresh", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		executions.Add(1)
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}}, nil
	})
	m.transportFactory = func(callCtx context.Context, slug string, cfg ServerConfig) (mcpsdk.Transport, error) {
		if _, err := m.resolveManagedOAuthSilent(callCtx, cfg, ""); err != nil {
			return nil, err
		}
		return factory.transport(callCtx, slug, cfg)
	}
	original := browserOpen
	defer func() { browserOpen = original }()
	browserOpen = func(raw string) error {
		browsers.Add(1)
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		response, err := http.Get(q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&code=CODE")
		if err != nil {
			return err
		}
		return response.Body.Close()
	}
	if err := m.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	m.AutoConnectAll(ctx)
	entries, err := m.DiscoverableTools(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("discoverable=%v err=%v", entries, err)
	}
	if browsers.Load() != 0 || m.registry.Has(name) {
		t.Fatal("startup/discovery became interactive or executable")
	}
	turn := WithInteractiveOAuth(ctx)
	if err := m.PrepareCatalogTool(turn, entries[0]); err != nil {
		t.Fatal(err)
	}
	if browsers.Load() != 1 || executions.Load() != 0 {
		t.Fatal("preparation replayed a tool or duplicated consent")
	}
	tool, ok := m.registry.Get(name)
	if !ok || tool.Description() != "fresh" {
		t.Fatal("fresh schema was not published")
	}
	result, err := tool.Execute(turn, json.RawMessage(`{"query":"hello"}`))
	if err != nil || result.IsError || executions.Load() != 1 {
		t.Fatalf("execution: %+v %v", result, err)
	}
	if err := m.PrepareCatalogTool(turn, entries[0]); err != nil {
		t.Fatal(err)
	}
	if browsers.Load() != 1 || factory.count.Load() != 1 {
		t.Fatal("existing connection was not reused")
	}
	removed := entries[0]
	removed.Name = "removed_tool"
	if err := m.PrepareCatalogTool(turn, removed); err == nil {
		t.Fatal("accepted tool absent from current discovery")
	}
	canceled, cancel := context.WithCancel(turn)
	cancel()
	if err := m.PrepareCatalogTool(canceled, entries[0]); err == nil {
		t.Fatal("canceled preparation succeeded")
	}
	m.mu.Lock()
	m.servers["remote"].Config.Enabled = false
	m.mu.Unlock()
	if err := m.PrepareCatalogTool(turn, entries[0]); err == nil {
		t.Fatal("disabled server was prepared")
	}
	if visible, err := m.DiscoverableTools(ctx); err != nil || len(visible) != 0 {
		t.Fatal("disabled server remained discoverable")
	}
	wrongUser := entries[0]
	wrongUser.UserID = "other-user"
	if err := m.PrepareCatalogTool(turn, wrongUser); err == nil {
		t.Fatal("accepted another user's tool")
	}
}

func TestCatalogRecoveryConcurrentConnectionIsReused(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "parallel", ServerConfig{Enabled: true})
	factory := newInMemoryMCPFactory(t, context.Background())
	defer factory.close()
	defer m.CloseAll()
	m.transportFactory = factory.transport
	start := make(chan struct{})
	results := make(chan error, 12)
	for i := 0; i < cap(results); i++ {
		go func() { <-start; results <- m.ConnectWithContext(context.Background(), "parallel") }()
	}
	close(start)
	for i := 0; i < cap(results); i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := m.connectWithContextForRetry(context.Background(), "parallel", 0, true); err != nil {
		t.Fatal(err)
	}
	if factory.count.Load() != 1 {
		t.Fatalf("connections=%d", factory.count.Load())
	}
}

func TestCatalogRecoveryReplacesEndedSessionBeforeBackoff(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "ended", ServerConfig{Enabled: true})
	factory := newInMemoryMCPFactory(t, context.Background())
	defer factory.close()
	defer m.CloseAll()
	m.transportFactory = factory.transport
	unhealthy := make(chan struct{}, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			select {
			case unhealthy <- struct{}{}:
			default:
			}
		}
	}
	if err := m.ConnectWithContext(context.Background(), "ended"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	previous := m.connections["ended"]
	m.mu.RUnlock()
	factory.mu.Lock()
	remote := factory.sessions[0]
	factory.mu.Unlock()
	if err := remote.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-unhealthy:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not observe session termination")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.ConnectWithContext(ctx, "ended"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	current := m.connections["ended"]
	ready := current != nil && current != previous && !current.sessionEnded
	m.mu.RUnlock()
	if !ready || factory.count.Load() != 2 {
		t.Fatal("ended connection was reused or recovery was duplicated")
	}
}

func TestCatalogRecoveryRestoresRetryAfterTransientFailure(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "ended", ServerConfig{Enabled: true})
	factory := newInMemoryMCPFactory(t, context.Background())
	defer factory.close()
	defer m.CloseAll()
	var fail atomic.Bool
	m.transportFactory = func(ctx context.Context, slug string, cfg ServerConfig) (mcpsdk.Transport, error) {
		if fail.Load() {
			return nil, errors.New("temporary transport failure")
		}
		return factory.transport(ctx, slug, cfg)
	}
	unhealthy := make(chan struct{}, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			select {
			case unhealthy <- struct{}{}:
			default:
			}
		}
	}
	if err := m.ConnectWithContext(context.Background(), "ended"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	factory.mu.Lock()
	remote := factory.sessions[0]
	factory.mu.Unlock()
	if err := remote.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-unhealthy:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not observe session termination")
	}
	m.mu.RLock()
	previousID := m.servers["ended"].reconnectAttemptID
	m.mu.RUnlock()
	if err := m.ConnectWithContext(context.Background(), "ended"); err == nil {
		t.Fatal("transient failure was ignored")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	attempt := m.reconnectCancels["ended"]
	status := m.servers["ended"]
	if attempt == nil || attempt.id == previousID || !status.Reconnecting || status.reconnectAttemptID != attempt.id {
		t.Fatal("on-demand failure left the server without background recovery")
	}
}

func TestCatalogRecoveryDisconnectCancelsAnticipatedConnection(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "ended", ServerConfig{Enabled: true})
	defer m.CloseAll()
	m.connections["ended"] = &serverConnection{sessionEnded: true}
	m.servers["ended"].Status = StatusError
	started := make(chan struct{})
	m.transportFactory = func(ctx context.Context, _ string, _ ServerConfig) (mcpsdk.Transport, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	done := make(chan error, 1)
	go func() { done <- m.ConnectWithContext(context.Background(), "ended") }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("connection did not start")
	}
	if err := m.Disconnect("ended"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connection did not finish")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.reconnectCancels["ended"] != nil || m.servers["ended"].Reconnecting {
		t.Fatal("explicit disconnect restarted automatic recovery")
	}
}

func TestCatalogRecoveryDisconnectClearsFailureWithoutActiveAttempt(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "failed", ServerConfig{Enabled: true})
	defer m.CloseAll()
	m.servers["failed"].Status = StatusError
	m.servers["failed"].Error = "temporary failure"
	if err := m.Disconnect("failed"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.servers["failed"].Status != StatusDisconnected || m.servers["failed"].Error != "" {
		t.Fatal("explicit disconnect left the failure eligible for another retry")
	}
}
