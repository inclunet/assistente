package mcp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerLifecycleServer(m *Manager, slug string, cfg ServerConfig) {
	m.servers[slug] = &ServerStatus{
		Slug:   slug,
		Config: cfg,
		Status: StatusDisconnected,
		Tools:  []MCPToolInfo{},
	}
}

type inMemoryMCPFactory struct {
	t        *testing.T
	server   *mcpsdk.Server
	ctx      context.Context
	mu       sync.Mutex
	sessions []*mcpsdk.ServerSession
	count    atomic.Int32
}

func newInMemoryMCPFactory(t *testing.T, ctx context.Context) *inMemoryMCPFactory {
	t.Helper()
	return &inMemoryMCPFactory{
		t:      t,
		ctx:    ctx,
		server: mcpsdk.NewServer(&mcpsdk.Implementation{Name: "lifecycle-test", Version: "1.0.0"}, nil),
	}
}

func (f *inMemoryMCPFactory) transport(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := f.server.Connect(f.ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.sessions = append(f.sessions, serverSession)
	f.mu.Unlock()
	f.count.Add(1)
	return clientTransport, nil
}

func (f *inMemoryMCPFactory) close() {
	f.mu.Lock()
	sessions := append([]*mcpsdk.ServerSession(nil), f.sessions...)
	f.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
}

func TestConnectWithContextMantemSessaoAposRetorno(t *testing.T) {
	m := newLifecycleManager()
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	registerLifecycleServer(m, "persistente", ServerConfig{Enabled: true, Transport: TransportStdio})

	parentCtx, cancelParent := context.WithCancel(context.Background())
	if err := m.connectWithContext(parentCtx, "persistente"); err != nil {
		t.Fatalf("connectWithContext: %v", err)
	}
	cancelParent()

	m.mu.RLock()
	conn := m.connections["persistente"]
	m.mu.RUnlock()
	if conn == nil {
		t.Fatal("sessão não foi publicada")
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), time.Second)
	defer cancelPing()
	if err := conn.session.Ping(pingCtx, nil); err != nil {
		t.Fatalf("sessão morreu quando connectWithContext retornou: %v", err)
	}

	m.CloseAll()
}

func TestHealthCheckNaoFechaSessaoPorCancelamentoLocal(t *testing.T) {
	m := newLifecycleManager()
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	registerLifecycleServer(m, "health", ServerConfig{Enabled: true, Transport: TransportStdio})

	if err := m.Connect("health"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	m.performHealthCheck("health")

	m.mu.RLock()
	status := m.servers["health"]
	failures := status.ConsecutiveHealthFailures
	state := status.Status
	m.mu.RUnlock()
	if failures != 0 || state != StatusConnected {
		t.Fatalf("health check degradou sessão válida: status=%s failures=%d error=%q", state, failures, status.Error)
	}
	m.CloseAll()
}

// TestMCPHelperProcess é reexecutado como subprocesso por
// TestHandshakeTimeoutEncerraProcessoStdio.
func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER_PROCESS") != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestHandshakeTimeoutEncerraProcessoStdio(t *testing.T) {
	m := newLifecycleManager()
	m.connectTimeout = 50 * time.Millisecond
	registerLifecycleServer(m, "stdio-timeout", ServerConfig{Enabled: true, Transport: TransportStdio})

	var cmd *exec.Cmd
	m.transportFactory = func(ctx context.Context, _ string, _ ServerConfig) (mcpsdk.Transport, error) {
		cmd = exec.CommandContext(ctx, os.Args[0], "-test.run=TestMCPHelperProcess", "--", "hang")
		cmd.Env = append(os.Environ(), "GO_WANT_MCP_HELPER_PROCESS=1")
		return &mcpsdk.CommandTransport{Command: cmd, TerminateDuration: 100 * time.Millisecond}, nil
	}

	start := time.Now()
	err := m.Connect("stdio-timeout")
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("Connect err=%v, esperado deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout não interrompeu o handshake a tempo: %v", elapsed)
	}
	if cmd == nil || cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatalf("processo stdio não foi coletado após timeout: cmd=%#v state=%#v", cmd, cmd.ProcessState)
	}
	m.mu.RLock()
	_, connected := m.connections["stdio-timeout"]
	_, connecting := m.connectCancels["stdio-timeout"]
	m.mu.RUnlock()
	if connected || connecting {
		t.Fatalf("timeout deixou recursos publicados: connected=%v connecting=%v", connected, connecting)
	}
	m.CloseAll()
}

type blockingConnection struct {
	closed    chan struct{}
	closeOnce sync.Once
	wrote     chan struct{}
	writeOnce sync.Once
}

func newBlockingConnection() *blockingConnection {
	return &blockingConnection{closed: make(chan struct{}), wrote: make(chan struct{})}
}

func (c *blockingConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	select {
	case <-c.closed:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *blockingConnection) Write(context.Context, jsonrpc.Message) error {
	c.writeOnce.Do(func() { close(c.wrote) })
	return nil
}

func (c *blockingConnection) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (*blockingConnection) SessionID() string { return "" }

type blockingTransport struct {
	conn *blockingConnection
}

func (t *blockingTransport) Connect(context.Context) (mcpsdk.Connection, error) {
	return t.conn, nil
}

func TestDisconnectCancelaConnectEmAndamento(t *testing.T) {
	m := newLifecycleManager()
	m.connectTimeout = 5 * time.Second
	registerLifecycleServer(m, "pendente", ServerConfig{Enabled: true, Transport: TransportStdio})

	blocked := newBlockingConnection()
	m.transportFactory = func(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
		return &blockingTransport{conn: blocked}, nil
	}

	connectDone := make(chan error, 1)
	go func() { connectDone <- m.Connect("pendente") }()
	select {
	case <-blocked.wrote:
	case <-time.After(time.Second):
		t.Fatal("handshake não iniciou")
	}

	disconnectDone := make(chan error, 1)
	go func() { disconnectDone <- m.Disconnect("pendente") }()
	select {
	case err := <-disconnectDone:
		if err != nil {
			t.Fatalf("Disconnect: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Disconnect não aguardou cleanup cancelável")
	}
	select {
	case err := <-connectDone:
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("Connect err=%v, esperado cancelamento", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Connect não terminou após Disconnect")
	}
	select {
	case <-blocked.closed:
	default:
		t.Fatal("transport não foi fechado")
	}
	m.CloseAll()
}

func TestDisconnectCancelaSessaoAtiva(t *testing.T) {
	m := newLifecycleManager()
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	registerLifecycleServer(m, "ativa", ServerConfig{Enabled: true, Transport: TransportStdio})

	if err := m.Connect("ativa"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	m.mu.RLock()
	conn := m.connections["ativa"]
	healthDone := conn.healthDone
	m.mu.RUnlock()

	if err := m.Disconnect("ativa"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	select {
	case <-healthDone:
	case <-time.After(time.Second):
		t.Fatal("Disconnect retornou antes de encerrar o health loop")
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), time.Second)
	defer cancelPing()
	if err := conn.session.Ping(pingCtx, nil); err == nil {
		t.Fatal("sessão continuou utilizável após Disconnect")
	}
	m.CloseAll()
}

func TestReconnectNaoDuplicaConexaoNemHealthLoop(t *testing.T) {
	m := newLifecycleManager()
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	registerLifecycleServer(m, "reconnect", ServerConfig{Enabled: true, Transport: TransportStdio})

	if err := m.Connect("reconnect"); err != nil {
		t.Fatalf("primeiro Connect: %v", err)
	}
	m.mu.RLock()
	first := m.connections["reconnect"]
	firstHealthDone := first.healthDone
	m.mu.RUnlock()

	if err := m.Reconnect("reconnect"); err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	select {
	case <-firstHealthDone:
	case <-time.After(time.Second):
		t.Fatal("health loop antigo permaneceu ativo")
	}

	m.mu.RLock()
	second := m.connections["reconnect"]
	connectionCount := len(m.connections)
	attemptCount := len(m.connectCancels)
	m.mu.RUnlock()
	if second == nil || second == first || connectionCount != 1 || attemptCount != 0 {
		t.Fatalf("reconnect inconsistente: second=%p first=%p connections=%d attempts=%d", second, first, connectionCount, attemptCount)
	}
	if got := factory.count.Load(); got != 2 {
		t.Fatalf("transport connections=%d, esperado 2", got)
	}
	m.CloseAll()
}

func TestSSELegadoMantemLifecycleAposConnectRetornar(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "sse-test", Version: "1.0.0"}, nil)
	httpServer := httptest.NewServer(mcpsdk.NewSSEHandler(func(*http.Request) *mcpsdk.Server { return server }, nil))
	defer httpServer.Close()

	m := newLifecycleManager()
	registerLifecycleServer(m, "sse", ServerConfig{
		Enabled:   true,
		Transport: TransportSSE,
		URL:       httpServer.URL,
		AuthType:  AuthNone,
	})
	if err := m.Connect("sse"); err != nil {
		t.Fatalf("Connect SSE: %v", err)
	}

	m.mu.RLock()
	session := m.connections["sse"].session
	m.mu.RUnlock()
	pingCtx, cancelPing := context.WithTimeout(context.Background(), time.Second)
	defer cancelPing()
	if err := session.Ping(pingCtx, nil); err != nil {
		t.Fatalf("SSE legado morreu após Connect retornar: %v", err)
	}
	m.CloseAll()
}

func TestStreamableFallbackSemSSEMantemLifecycle(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "streamable-test", Version: "1.0.0"}, nil)
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{JSONResponse: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	m := newLifecycleManager()
	registerLifecycleServer(m, "polling", ServerConfig{
		Enabled:   true,
		Transport: TransportStreamable,
		URL:       httpServer.URL,
		AuthType:  AuthNone,
	})
	if err := m.Connect("polling"); err != nil {
		t.Fatalf("Connect streamable sem SSE: %v", err)
	}

	m.mu.RLock()
	session := m.connections["polling"].session
	m.mu.RUnlock()
	pingCtx, cancelPing := context.WithTimeout(context.Background(), time.Second)
	defer cancelPing()
	if err := session.Ping(pingCtx, nil); err != nil {
		t.Fatalf("fallback polling não preservou lifecycle: %v", err)
	}
	m.CloseAll()
}

func TestCloseAllEncerraSessoesLoopsETentativas(t *testing.T) {
	m := newLifecycleManager()
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	for _, slug := range []string{"a", "b"} {
		registerLifecycleServer(m, slug, ServerConfig{Enabled: true, Transport: TransportStdio})
		if err := m.Connect(slug); err != nil {
			t.Fatalf("Connect(%s): %v", slug, err)
		}
	}

	m.mu.RLock()
	connections := []*serverConnection{m.connections["a"], m.connections["b"]}
	m.mu.RUnlock()
	m.CloseAll()

	for _, conn := range connections {
		select {
		case <-conn.healthDone:
		default:
			t.Fatal("CloseAll retornou antes de um health loop terminar")
		}
	}
	m.mu.RLock()
	connectionCount := len(m.connections)
	attemptCount := len(m.connectCancels)
	m.mu.RUnlock()
	if connectionCount != 0 || attemptCount != 0 {
		t.Fatalf("CloseAll deixou estado runtime: connections=%d attempts=%d", connectionCount, attemptCount)
	}
}

func TestCloseAllCancelaConnectEmAndamento(t *testing.T) {
	m := newLifecycleManager()
	m.connectTimeout = 5 * time.Second
	registerLifecycleServer(m, "pendente", ServerConfig{Enabled: true, Transport: TransportStdio})
	blocked := newBlockingConnection()
	m.transportFactory = func(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
		return &blockingTransport{conn: blocked}, nil
	}

	connectDone := make(chan error, 1)
	go func() { connectDone <- m.Connect("pendente") }()
	select {
	case <-blocked.wrote:
	case <-time.After(time.Second):
		t.Fatal("handshake não iniciou")
	}

	closeDone := make(chan struct{})
	go func() {
		m.CloseAll()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("CloseAll não cancelou a tentativa em andamento")
	}
	select {
	case err := <-connectDone:
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("Connect err=%v, esperado cancelamento", err)
		}
	default:
		t.Fatal("CloseAll retornou antes de fazer join do Connect")
	}
	select {
	case <-blocked.closed:
	default:
		t.Fatal("CloseAll não fechou o transport pendente")
	}
	m.mu.RLock()
	connectionCount := len(m.connections)
	attemptCount := len(m.connectCancels)
	m.mu.RUnlock()
	if connectionCount != 0 || attemptCount != 0 {
		t.Fatalf("CloseAll deixou estado runtime: connections=%d attempts=%d", connectionCount, attemptCount)
	}
}

type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (*captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, record.Clone())
	h.mu.Unlock()
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func TestReconexaoBemSucedidaUsaNivelInfo(t *testing.T) {
	handler := &captureHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	logReconnectSuccess("semantic-log")

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.records) != 1 {
		t.Fatalf("records=%d, esperado 1", len(handler.records))
	}
	if got := handler.records[0].Level; got != slog.LevelInfo {
		t.Fatalf("nível=%s, esperado INFO", got)
	}
	if !strings.Contains(handler.records[0].Message, "Reconexão bem-sucedida") {
		t.Fatalf("mensagem inesperada: %q", handler.records[0].Message)
	}
}
