package mcp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/toolcatalog"
	"assistente/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type blockingUnavailableCatalog struct {
	entered chan struct{}
	release chan struct{}
}

func (c *blockingUnavailableCatalog) ListTools(context.Context, tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error) {
	return nil, nil
}

func (c *blockingUnavailableCatalog) MarkServerToolsUnavailable(context.Context, string, []string, string) (int, error) {
	close(c.entered)
	<-c.release
	return 0, nil
}

func (*blockingUnavailableCatalog) RecordToolTest(context.Context, string, string, string) error {
	return nil
}
func (*blockingUnavailableCatalog) RecordToolTestByID(context.Context, string, string, string) error {
	return nil
}
func (*blockingUnavailableCatalog) SyncBuiltins(context.Context, *tools.Registry) error { return nil }
func (*blockingUnavailableCatalog) SyncMCPServerTools(context.Context, string, string, []toolcatalog.MCPToolDescriptor) error {
	return nil
}

func registerLifecycleServer(m *Manager, slug string, cfg ServerConfig) {
	m.servers[slug] = &ServerStatus{
		Slug:   slug,
		Config: cfg,
		Status: StatusDisconnected,
		Tools:  []MCPToolInfo{},
	}
}

type contextFirstCheckObserved struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *contextFirstCheckObserved) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestBeginManagedAttemptNaoRegistraRetryCancelado(t *testing.T) {
	m := newLifecycleManager()
	slug := "oauth-cancelado"
	registerLifecycleServer(m, slug, ServerConfig{Enabled: true, Transport: TransportStreamable, OAuthAuthorizationID: "auth-1"})

	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	cancelOwner()
	_, done, err := m.beginManagedAttemptWithGuard(context.Background(), ownerCtx, slug, 0)
	if err == nil {
		done()
		t.Fatal("esperava rejeitar a tentativa cujo retry já foi cancelado")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.managedAttempts[slug]; ok {
		t.Fatal("tentativa OAuth foi registrada após o cancelamento do retry")
	}
	if got := m.servers[slug].Status; got != StatusDisconnected {
		t.Fatalf("status mudou apesar do retry cancelado: %s", got)
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
	listCtx, cancelList := context.WithTimeout(context.Background(), time.Second)
	defer cancelList()
	if _, err := conn.session.ListTools(listCtx, nil); err != nil {
		t.Fatalf("sessão morreu quando connectWithContext retornou: %v", err)
	}

	m.CloseAll()
}

func TestEncerramentoEspontaneoDaSessaoAtualizaStatus(t *testing.T) {
	m := newLifecycleManager()
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	registerLifecycleServer(m, "sessao-encerrada", ServerConfig{Enabled: true, Transport: TransportStdio})

	unhealthy := make(chan struct{}, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			unhealthy <- struct{}{}
		}
	}
	if err := m.Connect("sessao-encerrada"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	factory.mu.Lock()
	serverSession := factory.sessions[0]
	factory.mu.Unlock()
	if err := serverSession.Close(); err != nil {
		t.Fatalf("fechamento remoto da sessão: %v", err)
	}

	select {
	case <-unhealthy:
	case <-time.After(time.Second):
		t.Fatal("encerramento da sessão não emitiu server_unhealthy")
	}
	m.mu.RLock()
	status := m.servers["sessao-encerrada"].Status
	err := m.servers["sessao-encerrada"].Error
	m.mu.RUnlock()
	if status != StatusError || !strings.Contains(err, "sessão perdida") {
		t.Fatalf("encerramento espontâneo não refletido no status: status=%s error=%q", status, err)
	}
	m.CloseAll()
}

func TestEncerramentoDuranteDiscoveryAcionaEstadoIndisponivel(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "sessao-encerra-no-discovery", ServerConfig{Enabled: true, Transport: TransportStdio})
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "discovery-close-test", Version: "1.0.0"}, nil)
	server.AddResource(&mcpsdk.Resource{URI: "file:///discovery"}, nil)
	unhealthy := make(chan struct{}, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			unhealthy <- struct{}{}
		}
	}
	var serverSession *mcpsdk.ServerSession
	m.transportFactory = func(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		var err error
		serverSession, err = server.Connect(m.ctx, serverTransport, nil)
		if err != nil {
			return nil, err
		}
		return &closeOnResourceListTransport{inner: clientTransport, closeSession: func() { _ = serverSession.Close() }}, nil
	}
	defer func() {
		if serverSession != nil {
			_ = serverSession.Close()
		}
	}()

	_ = m.Connect("sessao-encerra-no-discovery")
	select {
	case <-unhealthy:
	case <-time.After(time.Second):
		t.Fatal("encerramento durante discovery não sinalizou a sessão indisponível")
	}
	m.mu.RLock()
	status := m.servers["sessao-encerra-no-discovery"].Status
	err := m.servers["sessao-encerra-no-discovery"].Error
	m.mu.RUnlock()
	if status != StatusError {
		t.Fatalf("encerramento não atualizou o status: status=%s error=%q", status, err)
	}
	m.CloseAll()
}

func TestEncerramentoDuranteReconexaoMantemDonoDoRetry(t *testing.T) {
	m := newLifecycleManager()
	conn := &serverConnection{sessionEnded: true, sessionEndErr: errors.New("transporte fechado")}
	m.connections["reconnect-ended"] = conn
	m.servers["reconnect-ended"] = &ServerStatus{
		Slug:               "reconnect-ended",
		Config:             ServerConfig{Enabled: true},
		Status:             StatusConnected,
		Reconnecting:       true,
		reconnectAttemptID: 1,
	}

	if m.finishReconnectAttempt("reconnect-ended", 1) {
		t.Fatal("reconexão atual encerrou em vez de manter o retry após queda imediata")
	}
	m.mu.RLock()
	status := m.servers["reconnect-ended"].Status
	reconnecting := m.servers["reconnect-ended"].Reconnecting
	m.mu.RUnlock()
	if status != StatusError || !reconnecting {
		t.Fatalf("ownership do retry perdido: status=%s reconnecting=%v", status, reconnecting)
	}
	m.CloseAll()
}

func TestCleanupDeRetryAntigoNaoLiberaNovoDono(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "retry-handoff", ServerConfig{Enabled: true})
	m.mu.Lock()
	status := m.servers["retry-handoff"]
	firstAttemptID := m.beginReconnectAttemptLocked(status)
	m.mu.Unlock()
	m.releaseReconnectAttempt("retry-handoff", firstAttemptID)

	m.mu.Lock()
	secondAttemptID := m.beginReconnectAttemptLocked(status)
	m.mu.Unlock()

	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		m.releaseReconnectAttempt("retry-handoff", firstAttemptID)
	}()
	<-cleanupDone

	m.mu.RLock()
	reconnecting := m.servers["retry-handoff"].Reconnecting
	ownerID := m.servers["retry-handoff"].reconnectAttemptID
	m.mu.RUnlock()
	if !reconnecting || ownerID != secondAttemptID {
		t.Fatalf("cleanup antigo removeu o dono novo: reconnecting=%v owner=%d", reconnecting, ownerID)
	}
	m.CloseAll()
}

func TestDisconnectDuranteBackoffCancelaReconexaoPendente(t *testing.T) {
	for _, disconnectAll := range []bool{false, true} {
		name := "Disconnect"
		if disconnectAll {
			name = "DisconnectAll"
		}
		t.Run(name, func(t *testing.T) {
			m := newLifecycleManager()
			slug := "retry-cancelado"
			registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
			m.servers[slug].Status = StatusError
			var transportCalls atomic.Int32
			m.transportFactory = func(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
				transportCalls.Add(1)
				return nil, errors.New("retry não deveria iniciar")
			}

			retryDone := make(chan struct{})
			m.goTracked(func() {
				defer close(retryDone)
				m.reconnectWithRetry(slug)
			})
			deadline := time.Now().Add(time.Second)
			for {
				m.mu.RLock()
				status := m.servers[slug]
				started := status.Reconnecting && m.reconnectCancels[slug] != nil && status.RetryCount > 0
				m.mu.RUnlock()
				if started {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("retry não entrou no backoff")
				}
				time.Sleep(time.Millisecond)
			}

			if disconnectAll {
				m.DisconnectAll()
			} else if err := m.Disconnect(slug); err != nil {
				t.Fatalf("Disconnect: %v", err)
			}
			select {
			case <-retryDone:
			case <-time.After(time.Second):
				t.Fatal("desconexão não cancelou o retry em backoff")
			}

			m.mu.RLock()
			status := m.servers[slug]
			m.mu.RUnlock()
			if status.Status != StatusDisconnected || status.Reconnecting {
				t.Fatalf("retry alterou o estado após desconexão: status=%s reconnecting=%v", status.Status, status.Reconnecting)
			}
			if got := transportCalls.Load(); got != 0 {
				t.Fatalf("retry abriu %d transports após a desconexão explícita", got)
			}
			m.CloseAll()
		})
	}
}

func TestDisconnectAntesDoInicioDoRetryCancelaReserva(t *testing.T) {
	m := newLifecycleManager()
	slug := "retry-reservado"
	registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
	m.servers[slug].Status = StatusError
	var transportCalls atomic.Int32
	m.transportFactory = func(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
		transportCalls.Add(1)
		return nil, errors.New("retry não deveria iniciar")
	}

	m.mu.Lock()
	retryCtx, attemptID := m.reserveReconnectAttemptLocked(slug, m.servers[slug])
	m.mu.Unlock()
	if err := m.Disconnect(slug); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if retryCtx.Err() == nil {
		t.Fatal("Disconnect não cancelou a reserva do retry ainda não iniciado")
	}

	// Simula a goroutine de recuperação começando depois que Disconnect retornou.
	m.runReconnectWithRetry(slug, retryCtx, attemptID)
	m.mu.RLock()
	status := m.servers[slug]
	_, retryStillRegistered := m.reconnectCancels[slug]
	m.mu.RUnlock()
	if status.Status != StatusDisconnected || status.Reconnecting || retryStillRegistered {
		t.Fatalf("retry reservado sobreviveu à desconexão: status=%s reconnecting=%v registered=%v", status.Status, status.Reconnecting, retryStillRegistered)
	}
	if got := transportCalls.Load(); got != 0 {
		t.Fatalf("retry abriu %d transports após a desconexão explícita", got)
	}
	m.CloseAll()
}

func TestRetryAntigoNaoDesconectaConexaoManualNova(t *testing.T) {
	m := newLifecycleManager()
	slug := "retry-antigo"
	registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
	m.servers[slug].Status = StatusError

	m.mu.Lock()
	retryCtx, attemptID := m.reserveReconnectAttemptLocked(slug, m.servers[slug])
	m.mu.Unlock()
	m.supersedeReconnectWithManualConnect(slug)
	if retryCtx.Err() == nil {
		t.Fatal("conexão manual não cancelou o retry antigo")
	}

	manualConn := &serverConnection{}
	m.mu.Lock()
	m.connections[slug] = manualConn
	m.servers[slug].Status = StatusConnected
	m.mu.Unlock()

	disconnected, err := m.disconnectForRetry(slug, attemptID)
	if err != nil {
		t.Fatalf("disconnectForRetry: %v", err)
	}
	if disconnected {
		t.Fatal("retry cancelado manteve posse para desconectar uma sessão nova")
	}
	m.mu.RLock()
	gotConn := m.connections[slug]
	gotStatus := m.servers[slug].Status
	m.mu.RUnlock()
	if gotConn != manualConn || gotStatus != StatusConnected {
		t.Fatalf("retry antigo afetou a conexão manual: conn=%p status=%s", gotConn, gotStatus)
	}
	m.CloseAll()
}

func TestCancelamentoLocalDuranteDiscoveryNaoAgendaRecuperacao(t *testing.T) {
	m := newLifecycleManager()
	slug := "discovery-cancelado"
	registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
	baseCtx, cancelSession := context.WithCancel(context.Background())
	sessionCtx := &contextFirstCheckObserved{Context: baseCtx, checked: make(chan struct{})}
	conn := &serverConnection{sessionCtx: sessionCtx}
	m.connections[slug] = conn
	m.servers[slug].Status = StatusConnecting

	events := make(chan string, 2)
	m.emitEvent = func(event string, _ any) { events <- event }
	// Segura Manager.mu para cancelar depois da primeira verificação do watcher,
	// mas antes de ele conseguir validar a conexão e reservar uma recuperação.
	m.mu.Lock()
	finished := make(chan struct{})
	go func() {
		m.handleSessionEnded(slug, conn, errors.New("transport closed"))
		close(finished)
	}()
	select {
	case <-sessionCtx.checked:
	case <-time.After(time.Second):
		m.mu.Unlock()
		t.Fatal("watcher não verificou o contexto local")
	}
	cancelSession()
	m.mu.Unlock()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("handler de encerramento local não terminou")
	}

	m.mu.RLock()
	status := m.servers[slug].Status
	_, retryScheduled := m.reconnectCancels[slug]
	m.mu.RUnlock()
	if status != StatusConnecting || retryScheduled {
		t.Fatalf("cancelamento local foi tratado como queda remota: status=%s retry=%v", status, retryScheduled)
	}
	select {
	case event := <-events:
		t.Fatalf("cancelamento local emitiu evento inesperado %q", event)
	default:
	}
	m.CloseAll()
}

func TestConnectManualAguardaTentativaDoRetryAntesDeAssumir(t *testing.T) {
	for _, managedOAuth := range []bool{false, true} {
		name := "connect"
		if managedOAuth {
			name = "oauth-gerenciado"
		}
		t.Run(name, func(t *testing.T) {
			m := newLifecycleManager()
			slug := "transferencia-retry"
			registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
			m.servers[slug].Status = StatusError

			m.mu.Lock()
			retryCtx, retryID := m.reserveReconnectAttemptLocked(slug, m.servers[slug])
			m.mu.Unlock()

			attemptCtx, cancelAttempt := context.WithCancel(retryCtx)
			attemptDone := make(chan struct{})
			completion := attemptDone
			if managedOAuth {
				_, managedDone, err := m.beginManagedAttemptWithGuard(attemptCtx, retryCtx, slug, retryID)
				if err != nil {
					t.Fatalf("beginManagedAttemptWithGuard: %v", err)
				}
				m.mu.RLock()
				completion = m.managedAttempts[slug].done
				m.mu.RUnlock()
				go func() {
					<-attemptCtx.Done()
					managedDone()
				}()
			} else {
				m.mu.Lock()
				m.connectCancels[slug] = &connectionAttempt{
					reconnectAttemptID: retryID,
					cancel:             cancelAttempt,
					done:               attemptDone,
				}
				m.mu.Unlock()
				go func() {
					<-attemptCtx.Done()
					m.mu.Lock()
					delete(m.connectCancels, slug)
					m.mu.Unlock()
					close(attemptDone)
				}()
			}

			m.supersedeReconnectWithManualConnect(slug)
			select {
			case <-completion:
			default:
				t.Fatal("Connect manual retornou antes do cleanup da tentativa de retry")
			}
			if retryCtx.Err() == nil {
				t.Fatal("Connect manual não cancelou o contexto do retry")
			}
			m.mu.RLock()
			_, managedPending := m.managedAttempts[slug]
			_, connectPending := m.connectCancels[slug]
			m.mu.RUnlock()
			if managedPending || connectPending {
				t.Fatalf("tentativa do retry permaneceu registrada: managed=%v connect=%v", managedPending, connectPending)
			}
			cancelAttempt()
			m.CloseAll()
		})
	}
}

func TestConnectManualAguardaFimDoCleanupDeDesconexao(t *testing.T) {
	m := newLifecycleManager()
	slug := "cleanup-retry"
	registerLifecycleServer(m, slug, ServerConfig{Enabled: true, UserID: "test-user"})
	m.servers[slug].ID = "server-id"
	m.catalog = &blockingUnavailableCatalog{entered: make(chan struct{}), release: make(chan struct{})}
	catalog := m.catalog.(*blockingUnavailableCatalog)
	m.connections[slug] = &serverConnection{}
	m.mu.Lock()
	_, retryID := m.reserveReconnectAttemptLocked(slug, m.servers[slug])
	m.mu.Unlock()

	disconnectDone := make(chan struct{})
	go func() {
		_, _ = m.disconnectForRetry(slug, retryID)
		close(disconnectDone)
	}()
	select {
	case <-catalog.entered:
	case <-time.After(time.Second):
		t.Fatal("cleanup não chegou à atualização do catálogo")
	}

	manualConnectDone := make(chan struct{})
	manualConnectStarted := make(chan struct{})
	go func() {
		close(manualConnectStarted)
		m.supersedeReconnectWithManualConnect(slug)
		close(manualConnectDone)
	}()
	<-manualConnectStarted
	select {
	case <-manualConnectDone:
		t.Fatal("conexão manual avançou antes de terminar o cleanup anterior")
	case <-time.After(30 * time.Millisecond):
	}

	close(catalog.release)
	select {
	case <-disconnectDone:
	case <-time.After(time.Second):
		t.Fatal("cleanup de desconexão não terminou")
	}
	select {
	case <-manualConnectDone:
	case <-time.After(time.Second):
		t.Fatal("conexão manual não prosseguiu após o cleanup")
	}
	m.CloseAll()
}

func TestFalhaDaConexaoDuranteRetryPreservaEstadoDeErro(t *testing.T) {
	m := newLifecycleManager()
	slug := "falha-durante-retry"
	registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
	conn := &serverConnection{}
	m.connections[slug] = conn
	m.mu.Lock()
	status := m.servers[slug]
	status.Status = StatusConnected
	attemptID := m.beginReconnectAttemptLocked(status)
	m.mu.Unlock()

	events := make(chan string, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			events <- event
		}
	}
	m.handleToolCallErrorForConnection(slug, conn, &SessionExpiredError{StatusCode: http.StatusGone})

	m.mu.RLock()
	gotStatus := m.servers[slug].Status
	gotError := m.servers[slug].Error
	reconnecting := m.servers[slug].Reconnecting
	gotAttemptID := m.servers[slug].reconnectAttemptID
	m.mu.RUnlock()
	if gotStatus != StatusError || !strings.Contains(gotError, "sessão perdida") || !reconnecting || gotAttemptID != attemptID {
		t.Fatalf("falha da sessão foi descartada durante retry: status=%s error=%q reconnecting=%v attempt=%d", gotStatus, gotError, reconnecting, gotAttemptID)
	}
	select {
	case <-events:
	default:
		t.Fatal("falha da sessão durante retry não emitiu server_unhealthy")
	}
	if m.finishReconnectAttempt(slug, attemptID) {
		t.Fatal("retry deveria continuar após detectar que a sessão atual falhou")
	}
	m.CloseAll()
}

func TestFalhaDeConexaoAntigaNaoAfetaSessaoReconectada(t *testing.T) {
	m := newLifecycleManager()
	slug := "falha-de-conexao-antiga"
	registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
	current := &serverConnection{}
	m.connections[slug] = current
	m.servers[slug].Status = StatusConnected
	old := &serverConnection{}

	events := make(chan string, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			events <- event
		}
	}
	m.handleToolCallErrorForConnection(slug, old, &SessionExpiredError{StatusCode: http.StatusGone})

	m.mu.RLock()
	status := m.servers[slug].Status
	m.mu.RUnlock()
	if status != StatusConnected {
		t.Fatalf("falha de conexão antiga afetou a atual: status=%s", status)
	}
	select {
	case event := <-events:
		t.Fatalf("falha de conexão antiga emitiu evento %q", event)
	default:
	}
	m.CloseAll()
}

func TestEncerramentoDuranteShutdownNaoEmiteFalha(t *testing.T) {
	m := newLifecycleManager()
	conn := &serverConnection{}
	m.connections["shutdown-ended"] = conn
	m.servers["shutdown-ended"] = &ServerStatus{
		Slug:   "shutdown-ended",
		Config: ServerConfig{Enabled: true},
		Status: StatusConnected,
	}
	events := make(chan string, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			events <- event
		}
	}
	m.cancel()

	m.handleSessionEnded("shutdown-ended", conn, errors.New("connection closed"))
	m.mu.RLock()
	status := m.servers["shutdown-ended"].Status
	m.mu.RUnlock()
	if status != StatusConnected {
		t.Fatalf("cleanup de shutdown alterou status para %s", status)
	}
	select {
	case event := <-events:
		t.Fatalf("cleanup de shutdown emitiu evento %q", event)
	default:
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
	// ProcessState não nil prova que Wait coletou o subprocesso. Exited()
	// retorna false no Unix quando o processo termina por sinal (inclusive o
	// SIGKILL esperado do exec.CommandContext), portanto não é portável aqui.
	if cmd == nil || cmd.ProcessState == nil {
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

type delayedErrorTransport struct {
	delay time.Duration
	err   error
}

func (t *delayedErrorTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	timer := time.NewTimer(t.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil, t.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type delayedWriteTransport struct {
	inner mcpsdk.Transport
	delay time.Duration
}

func (t *delayedWriteTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &delayedWriteConnection{Connection: conn, delay: t.delay}, nil
}

type delayedWriteConnection struct {
	mcpsdk.Connection
	delay time.Duration
}

func (c *delayedWriteConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	timer := time.NewTimer(c.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return c.Connection.Write(ctx, msg)
	case <-ctx.Done():
		return ctx.Err()
	}
}

type closeOnResourceListTransport struct {
	inner        mcpsdk.Transport
	closeSession func()
}

func (t *closeOnResourceListTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &closeOnResourceListConnection{Connection: conn, closeSession: t.closeSession}, nil
}

type closeOnResourceListConnection struct {
	mcpsdk.Connection
	closeSession func()
	once         sync.Once
}

func (c *closeOnResourceListConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	if request, ok := msg.(*jsonrpc.Request); ok && request.Method == "resources/list" {
		c.once.Do(c.closeSession)
	}
	return c.Connection.Write(ctx, msg)
}

type closeOnToolsListTransport struct {
	inner        mcpsdk.Transport
	closeSession func()
	failure      error
	readBlocked  chan struct{}
	allowRead    <-chan struct{}
}

func (t *closeOnToolsListTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &closeOnToolsListConnection{
		Connection:   conn,
		closeSession: t.closeSession,
		failure:      t.failure,
		readBlocked:  t.readBlocked,
		allowRead:    t.allowRead,
	}, nil
}

type closeOnToolsListConnection struct {
	mcpsdk.Connection
	closeSession func()
	failure      error
	readBlocked  chan struct{}
	allowRead    <-chan struct{}
	closeOnce    sync.Once
	readOnce     sync.Once
}

func (c *closeOnToolsListConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.Connection.Read(ctx)
	if err != nil {
		c.readOnce.Do(func() {
			close(c.readBlocked)
			<-c.allowRead
		})
	}
	return message, err
}

func (c *closeOnToolsListConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	if request, ok := message.(*jsonrpc.Request); ok && request.Method == "tools/list" {
		c.closeOnce.Do(c.closeSession)
		if c.failure != nil {
			return c.failure
		}
		return mcpsdk.ErrConnectionClosed
	}
	return c.Connection.Write(ctx, message)
}

type failOnMethodTransport struct {
	inner   mcpsdk.Transport
	method  string
	failure error
}

func (t *failOnMethodTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &failOnMethodConnection{Connection: conn, method: t.method, failure: t.failure}, nil
}

type failOnMethodConnection struct {
	mcpsdk.Connection
	method  string
	failure error
}

func (c *failOnMethodConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	if request, ok := message.(*jsonrpc.Request); ok && request.Method == c.method {
		return c.failure
	}
	return c.Connection.Write(ctx, message)
}

func TestEncerramentoDuranteToolsListPreservaFalhaAntesDoWatcher(t *testing.T) {
	m := newLifecycleManager()
	registerLifecycleServer(m, "sessao-encerra-tools-list", ServerConfig{Enabled: true})
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "tools-list-close-test", Version: "1.0.0"}, nil)
	var serverSession *mcpsdk.ServerSession
	readBlocked := make(chan struct{})
	allowRead := make(chan struct{})
	var releaseOnce sync.Once
	releaseRead := func() { releaseOnce.Do(func() { close(allowRead) }) }
	m.transportFactory = func(ctx context.Context, _ string, _ ServerConfig) (mcpsdk.Transport, error) {
		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		var err error
		serverSession, err = server.Connect(ctx, serverTransport, nil)
		if err != nil {
			return nil, err
		}
		return &closeOnToolsListTransport{
			inner: clientTransport,
			closeSession: func() {
				_ = serverSession.Close()
			},
			failure:     &SessionExpiredError{StatusCode: http.StatusGone},
			readBlocked: readBlocked,
			allowRead:   allowRead,
		}, nil
	}
	defer func() {
		if serverSession != nil {
			_ = serverSession.Close()
		}
	}()
	defer m.CloseAll()
	defer releaseRead()

	unhealthy := make(chan struct{}, 1)
	m.emitEvent = func(event string, _ any) {
		if event == "mcp:server_unhealthy" {
			unhealthy <- struct{}{}
		}
	}
	connectDone := make(chan error, 1)
	go func() { connectDone <- m.Connect("sessao-encerra-tools-list") }()

	select {
	case <-readBlocked:
	case <-time.After(time.Second):
		t.Fatal("watcher não alcançou a barreira de leitura após tools/list falhar")
	}
	select {
	case <-unhealthy:
	case <-time.After(time.Second):
		t.Fatal("falha de transporte durante tools/list não sinalizou sessão indisponível antes do watcher")
	}
	m.mu.RLock()
	status := m.servers["sessao-encerra-tools-list"].Status
	err := m.servers["sessao-encerra-tools-list"].Error
	m.mu.RUnlock()
	if status != StatusError || !strings.Contains(err, "sessão perdida") {
		t.Fatalf("falha remota durante discovery não preservada: status=%s error=%q", status, err)
	}

	releaseRead()
	select {
	case err := <-connectDone:
		if err == nil {
			t.Fatal("Connect deveria retornar erro após encerramento durante tools/list")
		}
	case <-time.After(time.Second):
		t.Fatal("Connect não concluiu cleanup após liberar o watcher")
	}
}

func TestClassificaFalhasDeTransporteDuranteDiscovery(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "erro de leitura de rede",
			err:  &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")},
			want: true,
		},
		{name: "conexão de rede fechada", err: net.ErrClosed, want: true},
		{name: "arquivo fechado", err: os.ErrClosed, want: true},
		{name: "sessão HTTP expirada 404", err: &SessionExpiredError{StatusCode: http.StatusNotFound}, want: true},
		{name: "sessão HTTP expirada 410", err: &SessionExpiredError{StatusCode: http.StatusGone}, want: true},
		{name: "sentinela MCP", err: mcpsdk.ErrConnectionClosed, want: true},
		{name: "erro de resposta MCP", err: &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "falha da ferramenta"}, want: false},
		{name: "cancelamento local", err: context.Canceled, want: false},
		{name: "prazo local", err: context.DeadlineExceeded, want: false},
		{name: "erro comum", err: errors.New("falha ao listar tools"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSessionTerminationError(tt.err); got != tt.want {
				t.Fatalf("isSessionTerminationError(%v) = %t, esperado %t", tt.err, got, tt.want)
			}
		})
	}
}

func TestFalhaDeSessaoEmReadResourceEGetPromptMarcaServidorIndisponivel(t *testing.T) {
	tests := []struct {
		name   string
		method string
		call   func(*Manager) error
	}{
		{
			name:   "resources/read",
			method: "resources/read",
			call: func(m *Manager) error {
				_, err := m.ReadResource("sessao-expirada-operacao", "file:///resource")
				return err
			},
		},
		{
			name:   "prompts/get",
			method: "prompts/get",
			call: func(m *Manager) error {
				_, err := m.GetPrompt("sessao-expirada-operacao", "prompt", nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newLifecycleManager()
			slug := "sessao-expirada-operacao"
			registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "expired-session-test", Version: "1.0.0"}, nil)
			var serverSession *mcpsdk.ServerSession
			m.transportFactory = func(ctx context.Context, _ string, _ ServerConfig) (mcpsdk.Transport, error) {
				clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
				var err error
				serverSession, err = server.Connect(ctx, serverTransport, nil)
				if err != nil {
					return nil, err
				}
				return &failOnMethodTransport{
					inner:   clientTransport,
					method:  tt.method,
					failure: &SessionExpiredError{StatusCode: http.StatusGone},
				}, nil
			}
			defer func() {
				if serverSession != nil {
					_ = serverSession.Close()
				}
			}()
			defer m.CloseAll()

			unhealthy := make(chan struct{}, 1)
			m.emitEvent = func(event string, _ any) {
				if event == "mcp:server_unhealthy" {
					unhealthy <- struct{}{}
				}
			}
			if err := m.Connect(slug); err != nil {
				t.Fatalf("Connect: %v", err)
			}
			if err := tt.call(m); err == nil {
				t.Fatal("operação deveria retornar o erro da sessão expirada")
			}

			select {
			case <-unhealthy:
			case <-time.After(time.Second):
				t.Fatal("erro de sessão na operação não sinalizou servidor indisponível")
			}
			m.mu.RLock()
			status := m.servers[slug].Status
			err := m.servers[slug].Error
			m.mu.RUnlock()
			if status != StatusError || !strings.Contains(err, "sessão perdida") {
				t.Fatalf("erro de sessão não refletido no status: status=%s error=%q", status, err)
			}
		})
	}
}

func TestFalhaDeSessaoEmListResourcesEListPromptsInterrompeDiscovery(t *testing.T) {
	for _, method := range []string{"resources/list", "prompts/list"} {
		t.Run(method, func(t *testing.T) {
			m := newLifecycleManager()
			slug := "sessao-expirada-discovery"
			registerLifecycleServer(m, slug, ServerConfig{Enabled: true})
			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "expired-discovery-test", Version: "1.0.0"}, nil)
			var serverSession *mcpsdk.ServerSession
			m.transportFactory = func(ctx context.Context, _ string, _ ServerConfig) (mcpsdk.Transport, error) {
				clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
				var err error
				serverSession, err = server.Connect(ctx, serverTransport, nil)
				if err != nil {
					return nil, err
				}
				return &failOnMethodTransport{
					inner:   clientTransport,
					method:  method,
					failure: &SessionExpiredError{StatusCode: http.StatusNotFound},
				}, nil
			}
			defer func() {
				if serverSession != nil {
					_ = serverSession.Close()
				}
			}()
			defer m.CloseAll()

			unhealthy := make(chan struct{}, 1)
			m.emitEvent = func(event string, _ any) {
				if event == "mcp:server_unhealthy" {
					unhealthy <- struct{}{}
				}
			}
			if err := m.Connect(slug); err == nil {
				t.Fatal("Connect deveria falhar após o término da sessão no discovery")
			}
			select {
			case <-unhealthy:
			case <-time.After(time.Second):
				t.Fatal("término de sessão no discovery não sinalizou servidor indisponível")
			}
			m.mu.RLock()
			status := m.servers[slug].Status
			err := m.servers[slug].Error
			m.mu.RUnlock()
			if status != StatusError || !strings.Contains(err, "sessão perdida") {
				t.Fatalf("término de sessão não refletido no status: status=%s error=%q", status, err)
			}
		})
	}
}

func TestFallbackSSERecebeNovoTimeoutDeHandshake(t *testing.T) {
	m := newLifecycleManager()
	m.connectTimeout = 800 * time.Millisecond
	registerLifecycleServer(m, "fallback-timeout", ServerConfig{
		Enabled:   true,
		Transport: TransportStreamable,
	})

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fallback-test", Version: "1.0.0"}, nil)
	var calls atomic.Int32
	m.transportFactory = func(ctx context.Context, _ string, _ ServerConfig) (mcpsdk.Transport, error) {
		if calls.Add(1) == 1 {
			return &delayedErrorTransport{
				delay: 450 * time.Millisecond,
				err:   errors.New("standalone SSE request failed"),
			}, nil
		}
		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		serverSession, err := server.Connect(
			m.ctx,
			&delayedWriteTransport{inner: serverTransport, delay: 450 * time.Millisecond},
			nil,
		)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = serverSession.Close() })
		return clientTransport, nil
	}

	if err := m.Connect("fallback-timeout"); err != nil {
		t.Fatalf("fallback deveria receber timeout próprio: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("tentativas=%d, esperado 2", got)
	}
	m.CloseAll()
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
	sessionDone := conn.sessionDone
	m.mu.RUnlock()

	if err := m.Disconnect("ativa"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	select {
	case _, ok := <-sessionDone:
		if ok {
			t.Fatal("Disconnect retornou sem consumir o resultado de session.Wait")
		}
	default:
		t.Fatal("Disconnect retornou antes de session.Wait terminar")
	}
	if _, err := conn.session.ListTools(context.Background(), nil); err == nil {
		t.Fatal("sessão continuou utilizável após Disconnect")
	}
	m.CloseAll()
}

func TestWatcherDaSessaoIniciaComBackgroundFechado(t *testing.T) {
	m := newLifecycleManager()
	factory := newInMemoryMCPFactory(t, m.ctx)
	defer factory.close()
	m.transportFactory = factory.transport
	registerLifecycleServer(m, "watcher-direto", ServerConfig{Enabled: true, Transport: TransportStdio})

	m.bgMu.Lock()
	m.bgClosed = true
	m.bgMu.Unlock()

	if err := m.Connect("watcher-direto"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	m.mu.RLock()
	sessionDone := m.connections["watcher-direto"].sessionDone
	m.mu.RUnlock()
	select {
	case <-sessionDone:
		t.Fatal("sessionDone fechou sem aguardar session.Wait")
	default:
	}

	if err := m.Disconnect("watcher-direto"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	select {
	case _, ok := <-sessionDone:
		if ok {
			t.Fatal("resultado de session.Wait não foi consumido no cleanup")
		}
	default:
		t.Fatal("Disconnect retornou antes do watcher direto")
	}
}

func TestReconnectSubstituiSessaoSemDuplicarConexao(t *testing.T) {
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
	m.mu.RUnlock()

	if err := m.Reconnect("reconnect"); err != nil {
		t.Fatalf("Reconnect: %v", err)
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

func TestCancelamentoDoCallerInterrompeProbeSSE(t *testing.T) {
	probeStarted := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(probeStarted)
		<-r.Context().Done()
	}))
	defer httpServer.Close()

	m := newLifecycleManager()
	defer m.CloseAll()
	var transportCalls atomic.Int32
	m.transportFactory = func(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
		transportCalls.Add(1)
		return nil, errors.New("transport não deveria ser criado")
	}
	registerLifecycleServer(m, "probe-cancelado", ServerConfig{
		Enabled:   true,
		Transport: TransportStreamable,
		URL:       httpServer.URL,
		AuthType:  AuthNone,
	})

	parentCtx, cancelParent := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- m.connectWithContext(parentCtx, "probe-cancelado")
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("probe SSE não iniciou")
	}
	cancelParent()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("connectWithContext retornou %v, esperado context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelamento do caller não interrompeu o probe SSE")
	}
	if got := transportCalls.Load(); got != 0 {
		t.Fatalf("transport criado %d vez(es) após cancelamento durante probe", got)
	}
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
	if _, err := session.ListTools(context.Background(), nil); err != nil {
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
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatalf("fallback polling não preservou lifecycle: %v", err)
	}
	m.CloseAll()
}

func TestCloseAllEncerraSessoesETentativas(t *testing.T) {
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

	m.CloseAll()

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
	case <-time.After(time.Second):
		t.Fatal("Connect não retornou após o join interno de CloseAll")
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

// TestStandaloneSSEDesabilitadoUsaNivelInfo garante que a desabilitação do SSE
// standalone é reportada em INFO (estado benigno de configuração) e preserva a
// mensagem original com o slug do servidor.
func TestStandaloneSSEDesabilitadoUsaNivelInfo(t *testing.T) {
	handler := &captureHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	logStandaloneSSEDisabled("atlassian")

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.records) != 1 {
		t.Fatalf("records=%d, esperado 1", len(handler.records))
	}
	if got := handler.records[0].Level; got != slog.LevelInfo {
		t.Fatalf("nível=%s, esperado INFO", got)
	}
	// A normalização de logs remove o prefixo legado [MCP:slug] da mensagem
	// armazenada (mesmo comportamento de logReconnectSuccess), então validamos
	// apenas o núcleo preservado da mensagem e o nível.
	if !strings.Contains(handler.records[0].Message, "Standalone SSE desabilitado por configuração") {
		t.Fatalf("mensagem inesperada: %q", handler.records[0].Message)
	}
}

func TestExitStatusSoEhEsperadoParaSessaoJaEncerrada(t *testing.T) {
	err := errors.New("exit status 1")
	if isExpectedSessionCloseError(err, false) {
		t.Fatal("exit status de sessão ativa não pode ser mascarado")
	}
	if !isExpectedSessionCloseError(err, true) {
		t.Fatal("exit status de sessão comprovadamente encerrada deveria ser cleanup esperado")
	}
}

func TestWaitConnectionSessionPropagaErroDoWatcher(t *testing.T) {
	want := errors.New("falha inesperada do transport")
	done := make(chan error, 1)
	done <- want
	close(done)

	got, completed := waitConnectionSession(&serverConnection{sessionDone: done})
	if !errors.Is(got, want) {
		t.Fatalf("waitConnectionSession=%v, esperado %v", got, want)
	}
	if !completed {
		t.Fatal("waitConnectionSession não confirmou execução do watcher")
	}

	closedWithoutResult := make(chan error)
	close(closedWithoutResult)
	if _, completed := waitConnectionSession(&serverConnection{sessionDone: closedWithoutResult}); completed {
		t.Fatal("canal fechado sem resultado não pode comprovar session.Wait")
	}
}
