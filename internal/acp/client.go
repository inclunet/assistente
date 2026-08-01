package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/logging"
	"assistente/internal/osutil"

	sdk "github.com/coder/acp-go-sdk"
)

const (
	logComponent = "acp.client"

	// handshakeTimeout limita o initialize. Um agente que não se apresenta
	// nesse prazo está travado, e travar junto com ele deixaria a tela de
	// configuração pendurada.
	handshakeTimeout = 30 * time.Second

	// Janela de espera entre tentativas de subir um agente que falhou, para um
	// binário quebrado não virar uma tempestade de spawn.
	backoffBase = 1 * time.Second
	backoffMax  = 30 * time.Second

	// authRequiredCode é o erro JSON-RPC que o ACP reserva para "faça login".
	authRequiredCode = -32000
)

type client struct {
	cfg     Config
	handler RequestHandler
	now     func() time.Time

	mu          sync.Mutex
	conn        *conn
	closed      bool
	failures    int
	nextAttempt time.Time
	lastErr     error
}

// New cria um cliente para o agente descrito em cfg. O processo não sobe aqui:
// sobe no primeiro uso, seja ele um turno, uma consulta de modelos ou um health
// check (AEP-0084 D3).
//
// handler nulo significa negar todo pedido do agente na hora.
func New(cfg Config, handler RequestHandler) (Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		handler = denyAll{}
	}
	return &client{cfg: cfg, handler: handler, now: time.Now}, nil
}

// ensureConn devolve a conexão viva, subindo o processo se preciso. O lock fica
// segurado durante o spawn de propósito: dois turnos simultâneos numa conexão
// fria devem compartilhar um processo, não abrir dois agentes.
func (c *client) ensureConn(ctx context.Context) (*conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClosed
	}
	if c.conn != nil && !c.conn.isDead() {
		return c.conn, nil
	}
	if c.conn != nil {
		c.conn = nil
	}
	if wait := c.nextAttempt.Sub(c.now()); wait > 0 {
		return nil, fmt.Errorf("agente ACP indisponível; nova tentativa em %s: %w", wait.Round(time.Millisecond), c.lastErr)
	}

	cn, err := dial(ctx, c.cfg, c.handler)
	if err != nil {
		c.failures++
		c.lastErr = err
		c.nextAttempt = c.now().Add(backoffFor(c.failures))
		return nil, err
	}

	c.failures = 0
	c.lastErr = nil
	c.nextAttempt = time.Time{}
	c.conn = cn
	return cn, nil
}

func backoffFor(failures int) time.Duration {
	d := backoffBase
	for i := 1; i < failures && d < backoffMax; i++ {
		d *= 2
	}
	if d > backoffMax {
		d = backoffMax
	}
	return d
}

func (c *client) Capabilities(ctx context.Context) (Capabilities, error) {
	cn, err := c.ensureConn(ctx)
	if err != nil {
		return Capabilities{}, err
	}
	return cn.caps, nil
}

func (c *client) NewSession(ctx context.Context, cwd string) (Session, error) {
	cn, err := c.ensureConn(ctx)
	if err != nil {
		return nil, err
	}
	dir, err := absoluteDir(cwd)
	if err != nil {
		return nil, err
	}

	resp, err := sdk.SendRequest[sdk.NewSessionResponse](cn.rpc, ctx, sdk.AgentMethodSessionNew, sdk.NewSessionRequest{
		Cwd:        dir,
		McpServers: []sdk.McpServer{},
	})
	if err != nil {
		return nil, wrapCallError("abrir sessão no agente ACP", err)
	}
	if strings.TrimSpace(string(resp.SessionId)) == "" {
		return nil, errors.New("agente ACP devolveu sessão sem identificador")
	}

	options := withModeOption(configOptionsFrom(resp.ConfigOptions), resp.Modes)
	return cn.registerSession(string(resp.SessionId), dir, options), nil
}

func (c *client) LoadSession(ctx context.Context, sessionID, cwd string) (Session, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("identificador de sessão ACP vazio")
	}
	cn, err := c.ensureConn(ctx)
	if err != nil {
		return nil, err
	}
	if !cn.caps.LoadSession {
		return nil, fmt.Errorf("agente %s não suporta retomar sessões", describeAgent(c.cfg))
	}
	dir, err := absoluteDir(cwd)
	if err != nil {
		return nil, err
	}

	// A sessão precisa existir no registro antes da chamada: o agente pode
	// mandar atualizações do histórico enquanto ainda está respondendo o load,
	// e uma sessão desconhecida faria o transporte descartá-las.
	sess := cn.registerSession(sessionID, dir, nil)

	resp, err := sdk.SendRequest[sdk.LoadSessionResponse](cn.rpc, ctx, sdk.AgentMethodSessionLoad, sdk.LoadSessionRequest{
		SessionId:  sdk.SessionId(sessionID),
		Cwd:        dir,
		McpServers: []sdk.McpServer{},
	})
	if err != nil {
		cn.removeSession(sessionID)
		return nil, wrapCallError("retomar sessão no agente ACP", err)
	}

	sess.setConfigOptions(withModeOption(configOptionsFrom(resp.ConfigOptions), resp.Modes))
	return sess, nil
}

func (c *client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if strings.TrimSpace(method) == "" {
		return nil, errors.New("método JSON-RPC vazio")
	}
	cn, err := c.ensureConn(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := sdk.SendRequest[json.RawMessage](cn.rpc, ctx, method, params)
	if err != nil {
		return nil, wrapCallError(fmt.Sprintf("chamar %s no agente ACP", method), err)
	}
	return raw, nil
}

func (c *client) Close() error {
	c.mu.Lock()
	cn := c.conn
	c.conn = nil
	c.closed = true
	c.mu.Unlock()

	if cn == nil {
		return nil
	}
	cn.shutdown()
	return nil
}

// conn é um processo de agente vivo e as sessões multiplexadas nele.
type conn struct {
	cfg     Config
	handler RequestHandler
	cmd     *exec.Cmd
	rpc     *sdk.Connection
	caps    Capabilities
	kill    context.CancelFunc
	stderr  *io.PipeWriter

	mu       sync.Mutex
	sessions map[string]*session

	dead     chan struct{}
	deadOnce sync.Once
}

// dial sobe o processo do agente e faz o handshake.
func dial(ctx context.Context, cfg Config, handler RequestHandler) (*conn, error) {
	// O contexto do processo é independente do ctx da chamada: quem pediu o
	// primeiro turno não é dono do agente, que serve todas as conversas.
	procCtx, kill := context.WithCancel(context.Background())

	cmd := exec.CommandContext(procCtx, cfg.Command, cfg.Args...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = buildEnv(cfg.Env)
	// Sem isso, no Windows o agente abre uma janela de console que rouba o foco
	// e faz o leitor de telas anunciar o caminho do executável.
	osutil.HideConsoleWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		kill()
		return nil, fmt.Errorf("abrir entrada do agente %s: %w", describeAgent(cfg), err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		kill()
		return nil, fmt.Errorf("abrir saída do agente %s: %w", describeAgent(cfg), err)
	}
	stderr := newStderrLogger(cfg.Command)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		kill()
		return nil, fmt.Errorf("iniciar agente %s: %w", describeAgent(cfg), err)
	}

	cn := &conn{
		cfg:      cfg,
		handler:  handler,
		cmd:      cmd,
		kill:     kill,
		stderr:   stderr,
		sessions: make(map[string]*session),
		dead:     make(chan struct{}),
	}
	cn.rpc = sdk.NewConnection(cn.handleInbound, stdin, stdout)
	// Sem isso o SDK escreve o diagnóstico dele no logger padrão, fora do log
	// estruturado do app e sem dizer de que componente veio.
	cn.rpc.SetLogger(logging.Logger(context.Background(), logComponent))
	go cn.watch()

	caps, err := cn.handshake(ctx, cfg)
	if err != nil {
		cn.shutdown()
		return nil, err
	}
	cn.caps = caps

	logging.Infof(ctx, logComponent, "[ACP] agente %s conectado (loadSession=%t, imagem=%t)",
		describeAgent(cfg), caps.LoadSession, caps.PromptImage)
	return cn, nil
}

func (c *conn) handshake(ctx context.Context, cfg Config) (Capabilities, error) {
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	resp, err := sdk.SendRequest[sdk.InitializeResponse](c.rpc, hctx, sdk.AgentMethodInitialize, sdk.InitializeRequest{
		ProtocolVersion: sdk.ProtocolVersionNumber,
		ClientInfo: &sdk.Implementation{
			Name:    cfg.ClientName,
			Version: cfg.ClientVersion,
		},
		// O app não empresta ferramentas ao agente: nada de filesystem nem de
		// terminal do nosso lado. Ele usa as ferramentas dele (AEP-0084 D1).
		ClientCapabilities: sdk.ClientCapabilities{
			Fs:       sdk.FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
	})
	if err != nil {
		return Capabilities{}, fmt.Errorf("handshake com o agente %s: %w", describeAgent(cfg), err)
	}
	return capabilitiesFrom(resp), nil
}

// watch encerra a conexão quando o processo morre, colhe o filho e fecha o
// encaminhamento de stderr — que, aberto, seguraria uma goroutine por conexão.
func (c *conn) watch() {
	<-c.rpc.Done()
	c.markDead()
	c.kill()
	if err := c.cmd.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		logging.Warnf(context.Background(), logComponent, "[ACP] agente %s encerrou: %v", describeAgent(c.cfg), err)
	}
	_ = c.stderr.Close()
}

func (c *conn) markDead() {
	c.deadOnce.Do(func() { close(c.dead) })
}

// isDead olha também para o canal do SDK, e não só para a nossa marca: quem
// observa a queda primeiro é a conexão, e depender do watcher ter rodado
// deixaria uma janela em que a sessão parece viva com o processo já morto.
func (c *conn) isDead() bool {
	select {
	case <-c.dead:
		return true
	default:
	}
	select {
	case <-c.rpc.Done():
		return true
	default:
		return false
	}
}

func (c *conn) shutdown() {
	c.markDead()
	c.kill()
}

func (c *conn) registerSession(id, cwd string, options []ConfigOption) *session {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.sessions[id]; ok {
		if len(options) > 0 {
			existing.setConfigOptions(options)
		}
		return existing
	}
	sess := newSession(id, cwd, c, options)
	c.sessions[id] = sess
	return sess
}

func (c *conn) removeSession(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, id)
}

func (c *conn) session(id string) *session {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[id]
}

// handleInbound trata tudo que o agente manda para o app. Todo caminho aqui
// termina em resposta de protocolo: pânico vira erro interno, método
// desconhecido vira "não encontrado". O agente nunca fica esperando (D9).
func (c *conn) handleInbound(ctx context.Context, method string, params json.RawMessage) (result any, reqErr *sdk.RequestError) {
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf(ctx, logComponent, "[ACP] pânico ao tratar %s: %v", method, r)
			result = nil
			reqErr = sdk.NewInternalError(map[string]any{"error": "falha interna do cliente ao tratar o pedido"})
		}
	}()

	switch method {
	case sdk.ClientMethodSessionUpdate:
		var notification sdk.SessionNotification
		if err := json.Unmarshal(params, &notification); err != nil {
			logging.Warnf(ctx, logComponent, "[ACP] notificação de sessão ilegível: %v", err)
			return nil, sdk.NewInvalidParams(map[string]any{"error": err.Error()})
		}
		c.dispatchUpdate(ctx, notification)
		return nil, nil

	case sdk.ClientMethodSessionRequestPermission:
		return c.requestPermission(ctx, params)

	default:
		return c.handleCustom(ctx, method, params)
	}
}

func (c *conn) dispatchUpdate(ctx context.Context, notification sdk.SessionNotification) {
	sess := c.session(string(notification.SessionId))
	if sess == nil {
		logging.Debugf(ctx, logComponent, "[ACP] atualização de sessão desconhecida (%s) descartada", notification.SessionId)
		return
	}
	update, ok := updateFrom(notification.Update)
	if !ok {
		return
	}
	sess.deliver(update)
}

func (c *conn) requestPermission(ctx context.Context, params json.RawMessage) (any, *sdk.RequestError) {
	var req sdk.RequestPermissionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, sdk.NewInvalidParams(map[string]any{"error": err.Error()})
	}

	pedido := PermissionRequest{
		SessionID: string(req.SessionId),
		ToolCall:  toolCallFromUpdate(req.ToolCall),
		Options:   permissionOptionsFrom(req.Options),
	}

	outcome := guard(ctx, PermissionOutcome{}, func() PermissionOutcome {
		return c.handler.RequestPermission(ctx, pedido)
	})

	return sdk.RequestPermissionResponse{Outcome: permissionOutcomeToSDK(outcome, req.Options)}, nil
}

func (c *conn) handleCustom(ctx context.Context, method string, params json.RawMessage) (any, *sdk.RequestError) {
	type custom struct {
		result  any
		handled bool
	}
	out := guard(ctx, custom{}, func() custom {
		result, handled := c.handler.HandleCustom(ctx, method, params)
		return custom{result: result, handled: handled}
	})
	if !out.handled {
		logging.Debugf(ctx, logComponent, "[ACP] método %s não tratado; respondendo método não encontrado", method)
		return nil, sdk.NewMethodNotFound(method)
	}
	return out.result, nil
}

// guard executa fn e devolve fallback se ela entrar em pânico ou se o contexto
// do pedido morrer antes da decisão. É o que garante resposta de protocolo:
// um handler que demora demais atrasa o agente, mas não o pendura.
func guard[T any](ctx context.Context, fallback T, fn func() T) T {
	done := make(chan T, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Errorf(ctx, logComponent, "[ACP] pânico no tratamento do pedido do agente: %v", r)
				done <- fallback
			}
		}()
		done <- fn()
	}()

	select {
	case value := <-done:
		return value
	case <-ctx.Done():
		return fallback
	}
}

func absoluteDir(cwd string) (string, error) {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return "", errors.New("diretório de trabalho da sessão ACP não informado")
	}
	dir, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolver diretório %q: %w", trimmed, err)
	}
	return dir, nil
}

// wrapCallError distingue "faça login" de falha de conexão: o primeiro tem
// instrução acionável e não deve ser exibido como erro genérico (AEP-0084 D12).
func wrapCallError(action string, err error) error {
	var reqErr *sdk.RequestError
	if errors.As(err, &reqErr) && reqErr.Code == authRequiredCode {
		return fmt.Errorf("%s: %w", action, ErrNotAuthenticated)
	}
	return fmt.Errorf("%s: %w", action, err)
}

// buildEnv herda o ambiente do processo pai e acrescenta as variáveis extras,
// para o agente enxergar PATH, HOME e o que mais precisar para se autenticar.
func buildEnv(extra map[string]string) []string {
	base := os.Environ()
	for k, v := range extra {
		base = append(base, fmt.Sprintf("%s=%s", k, v))
	}
	return base
}

// newStderrLogger encaminha o stderr do agente para o log, linha a linha. Sem
// isso, o diagnóstico de um agente que não sobe se perde. Quem fecha o writer
// encerra a goroutine de leitura.
func newStderrLogger(command string) *io.PipeWriter {
	reader, writer := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 4096), 64*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			logging.Warnf(context.Background(), logComponent, "[ACP] %s: %s", filepath.Base(command), line)
		}
		_ = reader.Close()
	}()
	return writer
}
