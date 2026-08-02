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

	// fallbackWait limita o desfecho negativo de uma extensão. Ele deveria ser
	// imediato; o prazo existe só para um handler quebrado não transformar a
	// última linha de defesa em mais uma espera.
	fallbackWait = 5 * time.Second

	// Janela de espera entre tentativas de subir um agente que falhou, para um
	// binário quebrado não virar uma tempestade de spawn.
	backoffBase = 1 * time.Second
	backoffMax  = 30 * time.Second

	// authRequiredCode é o erro JSON-RPC que o ACP reserva para "faça login".
	authRequiredCode = -32000

	// handlerBackstop é o teto de tempo que damos a quem decide sobre um pedido
	// do agente. Não é o prazo da pergunta: esse é da camada que pergunta à
	// pessoa (AEP-0084 D9) e é bem menor. Este aqui é a última linha de defesa
	// do contrato de que todo pedido recebe resposta — o contexto que o SDK
	// entrega não traz prazo, então um handler que trava penduraria o agente
	// para sempre. Folgado de propósito: cortar antes do prazo da tela tiraria
	// da pessoa a chance de responder.
	handlerBackstop = 30 * time.Minute
)

type client struct {
	cfg     Config
	handler RequestHandler
	now     func() time.Time

	// life morre no Close e é o que interrompe um handshake em andamento sem
	// depender do mutex, que nessa hora está com quem está subindo o processo.
	life    context.Context
	endLife context.CancelFunc

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
	life, endLife := context.WithCancel(context.Background())
	return &client{cfg: cfg, handler: handler, now: time.Now, life: life, endLife: endLife}, nil
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

	cn, err := c.dialWithLifetime(ctx)
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

// dialWithLifetime sobe o processo abortando também quando o cliente fecha, e
// não só quando quem pediu desiste.
func (c *client) dialWithLifetime(ctx context.Context) (*conn, error) {
	dctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(c.life, cancel)
	defer stop()

	return dial(dctx, c.cfg, c.handler)
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

// CloseSession encerra uma sessão pelo identificador, sem exigir o objeto dela.
// De propósito não sobe processo: subir o agente para despedir uma sessão que
// morreu junto com o processo anterior seria pagar o spawn para não fazer nada.
func (c *client) CloseSession(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("identificador de sessão ACP vazio")
	}

	c.mu.Lock()
	cn, closed := c.conn, c.closed
	c.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if cn == nil || cn.isDead() {
		return nil
	}
	// Sessão que o app ainda conhece se despede pelo caminho normal, que
	// cancela o turno em andamento antes de encerrar.
	if sess := cn.session(sessionID); sess != nil {
		return sess.Close(ctx)
	}
	if !cn.caps.CloseSession {
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx, closeTimeout)
	defer cancel()
	_, err := sdk.SendRequest[sdk.CloseSessionResponse](cn.rpc, cctx, sdk.AgentMethodSessionClose,
		sdk.CloseSessionRequest{SessionId: sdk.SessionId(sessionID)})
	if err != nil {
		return wrapCallError(fmt.Sprintf("encerrar a sessão %q", sessionID), err)
	}
	return nil
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
	// Antes do lock: se o processo ainda está subindo, quem segura o mutex é o
	// handshake, e esperar por ele atrasaria o fechamento do app em até meio
	// minuto.
	c.endLife()

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
	// backstop é o teto de tempo de quem decide; campo, e não constante direta,
	// para que o teste não precise esperar meia hora.
	backstop time.Duration

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
		_ = stdin.Close()
		return nil, fmt.Errorf("abrir saída do agente %s: %w", describeAgent(cfg), err)
	}
	stderr := newStderrLogger(cfg.Command)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		kill()
		// Sem processo não haverá watch para fechar o cano, e o leitor de
		// stderr ficaria parado para sempre — uma goroutine por binário
		// quebrado que o usuário tentar usar.
		_ = stderr.Close()
		return nil, fmt.Errorf("iniciar agente %s: %w", describeAgent(cfg), err)
	}

	cn := &conn{
		cfg:      cfg,
		handler:  handler,
		cmd:      cmd,
		kill:     kill,
		stderr:   stderr,
		backstop: handlerBackstop,
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

// sessionOf descobre a que sessão um pedido pertence. Métodos de sessão do ACP
// — e as extensões do Cursor — carregam sessionId; os globais não, e para esses
// scoped volta falso.
func (c *conn) sessionOf(params json.RawMessage) (sess *session, scoped bool) {
	var payload struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, false
	}
	// Só em branco significa "pedido global"; fora isso a busca é pelo
	// identificador cru, como ele foi registrado. Aparar antes de procurar faria
	// um agente que manda o ID com espaço nas pontas parecer estar falando de
	// uma conversa que não existe, e a pergunta dele viraria "conversa
	// encerrada" sem nunca chegar a quem decide.
	if strings.TrimSpace(payload.SessionID) == "" {
		return nil, false
	}
	return c.session(payload.SessionID), true
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
			logging.Errorf(ctx, logComponent, "[ACP] pânico ao tratar %q: %v", method, r)
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
		logging.Debugf(ctx, logComponent, "[ACP] atualização de sessão desconhecida (%q) descartada", notification.SessionId)
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

	// Sessão que já não existe é conversa encerrada: não há a quem perguntar, e
	// o agente precisa de resposta agora, não quando alguém aparecer.
	sess := c.session(string(req.SessionId))
	if sess == nil {
		return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
	}

	// O ACP obriga quem manda session/cancel a responder "cancelado" a todo
	// pedido de permissão pendente. Além do protocolo, é o que fecha o diálogo
	// na tela: perguntar sobre um turno que a pessoa já abortou é ruído.
	cancelled := sess.cancelSignal()
	hctx, stopHandler := c.handlerContext(ctx, cancelled)
	defer stopHandler()

	// Aqui a falta de decisão não vira erro: negar a ação é um desfecho que o
	// método aceita e que deixa o agente seguir, enquanto um erro derrubaria o
	// turno inteiro por causa de um handler quebrado.
	outcome, _ := guard(hctx, PermissionOutcome{}, func() PermissionOutcome {
		return c.handler.RequestPermission(hctx, pedido)
	})
	if signalFired(cancelled) {
		return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
	}

	return sdk.RequestPermissionResponse{Outcome: permissionOutcomeToSDK(outcome, req.Options)}, nil
}

// handlerContext prepara o contexto entregue a quem decide sobre um pedido do
// agente: ele morre junto com o turno cancelado e tem um teto de tempo, para
// que nem um handler travado deixe o agente esperando para sempre.
func (c *conn) handlerContext(ctx context.Context, cancelled <-chan struct{}) (context.Context, context.CancelFunc) {
	limit := c.backstop
	if limit <= 0 {
		limit = handlerBackstop
	}
	tctx, stopClock := context.WithTimeout(ctx, limit)
	hctx, stop := bindCancel(tctx, cancelled)
	return hctx, func() {
		stop()
		stopClock()
	}
}

// bindCancel amarra o contexto entregue a quem decide ao cancelamento do turno.
// Sem isso, o diálogo continuaria na tela e o agente esperando uma resposta que
// já não interessa a ninguém.
func bindCancel(ctx context.Context, cancelled <-chan struct{}) (context.Context, context.CancelFunc) {
	hctx, stop := context.WithCancel(ctx)
	if cancelled == nil {
		return hctx, stop
	}
	go func() {
		select {
		case <-cancelled:
			stop()
		case <-hctx.Done():
		}
	}()
	return hctx, stop
}

func signalFired(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// handleCustom trata as extensões fora do padrão. As bloqueantes do Cursor
// (cursor/ask_question, cursor/create_plan) pertencem a um turno, e por isso
// morrem com ele: a pessoa não deve continuar sendo perguntada sobre um turno
// cancelado, nem o agente esperando por essa resposta (AEP-0084 D9).
func (c *conn) handleCustom(ctx context.Context, method string, params json.RawMessage) (any, *sdk.RequestError) {
	sess, scoped := c.sessionOf(params)
	if scoped && sess == nil {
		return nil, sdk.NewRequestCancelled(map[string]any{"error": "sessão encerrada"})
	}
	var cancelled <-chan struct{}
	if sess != nil {
		cancelled = sess.cancelSignal()
	}
	hctx, stopHandler := c.handlerContext(ctx, cancelled)
	defer stopHandler()

	type custom struct {
		result  any
		handled bool
	}
	out, decided := guard(hctx, custom{}, func() custom {
		result, handled := c.handler.HandleCustom(hctx, method, params)
		return custom{result: result, handled: handled}
	})
	if signalFired(cancelled) {
		return nil, sdk.NewRequestCancelled(map[string]any{"error": "turno cancelado"})
	}
	// Pânico ou contexto morto não são falta de suporte: responder "método não
	// encontrado" faria o agente riscar a extensão da lista e nunca mais tentar.
	if !decided {
		if fallback, ok := c.customFallback(ctx, method); ok {
			return fallback, nil
		}
		return nil, sdk.NewInternalError(map[string]any{"error": "falha interna do cliente ao tratar o pedido"})
	}
	if !out.handled {
		logging.Debugf(ctx, logComponent, "[ACP] método %q não tratado; respondendo método não encontrado", method)
		return nil, sdk.NewMethodNotFound(method)
	}
	return out.result, nil
}

// customFallback pede a quem implementa a extensão o desfecho negativo que o
// método aceita, para quando ninguém decidiu a tempo (AEP-0084 D9). O contexto
// é próprio e curto: o de quem pediu pode já estar morto, e esta resposta
// precisa sair de qualquer jeito — mas quem não decidiu antes também não pode
// prender o agente aqui.
func (c *conn) customFallback(ctx context.Context, method string) (any, bool) {
	fctx, stop := context.WithTimeout(context.WithoutCancel(ctx), fallbackWait)
	defer stop()

	type answer struct {
		result any
		ok     bool
	}
	out, decided := guard(fctx, answer{}, func() answer {
		result, ok := c.handler.CustomFallback(method)
		return answer{result: result, ok: ok}
	})
	return out.result, decided && out.ok
}

// guard executa fn e devolve fallback se ela entrar em pânico ou se o contexto
// do pedido morrer antes da decisão. É o que garante resposta de protocolo: um
// handler que demora demais atrasa o agente, mas não o pendura — o contexto
// vem com teto de tempo de handlerContext justamente para isso. A goroutine de
// fn pode sobreviver à resposta; é o preço de não confiar em código de fora.
//
// O segundo retorno diz se houve decisão de verdade. Sem ele, quem chama não
// consegue distinguir "o handler decidiu isso" de "não houve decisão", e acaba
// respondendo ao agente uma coisa pela outra.
func guard[T any](ctx context.Context, fallback T, fn func() T) (T, bool) {
	done := make(chan T, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Errorf(ctx, logComponent, "[ACP] pânico no tratamento do pedido do agente: %v", r)
				close(done)
			}
		}()
		done <- fn()
	}()

	return awaitDecision(ctx, done, fallback)
}

// awaitDecision espera a decisão sem deixar o fim do prazo atropelar uma que já
// chegou. Com os dois canais prontos o select escolhe ao acaso, e o acaso aqui
// significa responder "negado" a quem acabou de autorizar — ou pior, engolir um
// "sempre permitir" e perguntar tudo de novo na próxima. A decisão vem primeiro.
func awaitDecision[T any](ctx context.Context, done <-chan T, fallback T) (T, bool) {
	select {
	case value, ok := <-done:
		if !ok {
			return fallback, false
		}
		return value, true
	case <-ctx.Done():
		if value, ok := tryReceive(done); ok {
			return value, true
		}
		return fallback, false
	}
}

// tryReceive olha o canal sem esperar. É como se desempata um select em que o
// outro caso é um prazo ou um aviso: o que já chegou vale mais. Canal fechado
// não é valor recebido — no guard é justamente o handler que entrou em pânico.
func tryReceive[T any](ch <-chan T) (T, bool) {
	var zero T
	select {
	case value, ok := <-ch:
		if !ok {
			return zero, false
		}
		return value, true
	default:
		return zero, false
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
//
// As extras vão no fim porque é assim que elas vencem: o os/exec deduplica o
// ambiente antes de criar o processo, mantendo a última ocorrência de cada
// chave (sem diferenciar maiúsculas no Windows). Nenhuma chave repetida chega
// ao processo-filho.
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
// Uma linha de stderr maior que isto tem o começo registrado e o resto
// descartado. O limite existe para um agente que despeja megabytes numa linha
// só não virar memória do app; descartar o excesso, e não parar de ler, é o que
// mantém o diagnóstico das linhas seguintes.
const stderrLineLimit = 64 * 1024

func newStderrLogger(command string) *io.PipeWriter {
	name := filepath.Base(command)
	return newStderrLoggerTo(func(line string) {
		logging.Warnf(context.Background(), logComponent, "[ACP] %s: %s", name, line)
	})
}

// newStderrLoggerTo é o miolo, separado de onde o texto vai parar para que o
// teste possa observar o que sairia no log.
func newStderrLoggerTo(emit func(string)) *io.PipeWriter {
	reader, writer := io.Pipe()
	go func() {
		buffered := bufio.NewReaderSize(reader, stderrLineLimit)
		for {
			chunk, tooLong, err := buffered.ReadLine()
			if line := strings.TrimSpace(string(chunk)); line != "" {
				if tooLong {
					line += " […linha truncada]"
				}
				emit(line)
			}
			// Joga fora o resto da linha comprida. Sem isso, os pedaços dela
			// virariam linhas soltas no log, cada uma parecendo diagnóstico
			// novo.
			for tooLong && err == nil {
				_, tooLong, err = buffered.ReadLine()
			}
			if err != nil {
				break
			}
		}
		_ = reader.Close()
	}()
	return writer
}
