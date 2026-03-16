package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/tools"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// configSubdir é o subdiretório de .assistente/ para configs MCP
	configSubdir = "mcp"

	// configExt é a extensão dos arquivos de configuração
	configExt = ".json"

	// connectTimeout é o timeout para conexão com um servidor MCP
	connectTimeout = 30 * time.Second

	// listToolsTimeout é o timeout para listar tools de um servidor
	listToolsTimeout = 15 * time.Second

	// healthCheckInterval é o intervalo entre health checks
	healthCheckInterval = 2 * time.Minute

	// healthCheckTimeout é o timeout para ping de health check
	healthCheckTimeout = 10 * time.Second

	// healthCheckFailThreshold é o número de falhas consecutivas antes de reconectar
	healthCheckFailThreshold = 2

	// maxRetries é o número máximo de tentativas rápidas de reconexão
	maxRetries = 3

	// baseRetryDelay é o delay inicial para retry (exponential backoff)
	baseRetryDelay = 15 * time.Second

	// maxRetryDelay é o delay máximo entre retries (usado no modo lento)
	maxRetryDelay = 5 * time.Minute
)

// emitFunc é a callback para emitir eventos Wails
type emitFunc func(event string, data any)

// serverConnection mantém o estado runtime de um servidor MCP conectado.
type serverConnection struct {
	client        *mcpsdk.Client
	session       *mcpsdk.ClientSession
	bridges       []*MCPToolBridge // tools registradas no registry
	cancelHealth  context.CancelFunc // cancela health check goroutine
	logHandler    func(LogEntry)    // handler para logs do servidor
	progressHandler func(ProgressNotification) // handler para progresso
	resourceSubHandler func(ResourceUpdated) // handler para resource updates
}

// Manager gerencia servidores MCP: configuração, conexão, discovery de tools.
// Thread-safe para uso concorrente.
type Manager struct {
	mu          sync.RWMutex
	resolver    *configdir.Resolver
	credMgr     *credentials.Manager
	registry    *tools.Registry
	emitEvent   emitFunc
	llmHandler  func(context.Context, SamplingRequest) (string, error) // handler para sampling requests
	servers     map[string]*ServerStatus     // slug -> status
	connections map[string]*serverConnection // slug -> connection ativa
	ctx         context.Context
	cancel      context.CancelFunc
	roots       []Root // workspace roots globais
	lastSelfWrite time.Time
}

// NewManager cria um novo gerenciador de servidores MCP.
func NewManager(registry *tools.Registry, credMgr *credentials.Manager, emitEvent emitFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		resolver:    configdir.NewResolver(configSubdir),
		credMgr:     credMgr,
		registry:    registry,
		emitEvent:   emitEvent,
		servers:     make(map[string]*ServerStatus),
		connections: make(map[string]*serverConnection),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// SetSamplingHandler configura o handler para requisições de sampling dos servidores.
func (m *Manager) SetSamplingHandler(handler func(context.Context, SamplingRequest) (string, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llmHandler = handler
}

// SetWorkspaceRoots configura os diretórios raiz do workspace.
// Servidores conectados serão notificados.
func (m *Manager) SetWorkspaceRoots(roots []Root) error {
	m.mu.Lock()
	m.roots = roots
	
	// Notifica todos os servidores conectados
	for slug, conn := range m.connections {
		if conn.session != nil {
			// Envia notificação em background
			go func(s string, session *mcpsdk.ClientSession, r []Root) {
				// Converte roots para formato do SDK
				sdkRoots := make([]mcpsdk.Root, len(r))
				for i, root := range r {
					sdkRoots[i] = mcpsdk.Root{
						URI:  root.URI,
						Name: root.Name,
					}
				}
				
				// TODO: Quando SDK suportar, usar session.NotifyRootsListChanged(ctx)
				log.Printf("[MCP] Roots disponíveis para '%s': %v", s, sdkRoots)
			}(slug, conn.session, roots)
		}
		
		// Atualiza status
		if status, ok := m.servers[slug]; ok {
			status.Roots = roots
		}
	}
	m.mu.Unlock()
	
	log.Printf("[MCP] Workspace roots atualizados: %d roots", len(roots))
	m.emit("mcp:roots_changed", map[string]any{"rootCount": len(roots)})
	
	return nil
}

// GetWorkspaceRoots retorna os workspace roots configurados.
func (m *Manager) GetWorkspaceRoots() []Root {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.roots
}

// LoadConfigs carrega todas as configurações de servidores MCP e conecta os que têm auto_connect.
func (m *Manager) LoadConfigs() error {
	files, err := m.resolver.List()
	if err != nil {
		log.Printf("[MCP] Nenhuma configuração encontrada: %v", err)
		return nil
	}

	for _, f := range files {
		if f.Filename[len(f.Filename)-5:] != configExt {
			continue
		}

		data, _, err := m.resolver.Read(f.Filename)
		if err != nil {
			log.Printf("[MCP] Erro ao ler %s: %v", f.Filename, err)
			continue
		}

		var cfg ServerConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Printf("[MCP] Erro ao parsear %s: %v", f.Filename, err)
			continue
		}

		slug := f.Name
		m.mu.Lock()
		m.servers[slug] = &ServerStatus{
			Slug:   slug,
			Config: cfg,
			Status: StatusDisconnected,
			Tools:  []MCPToolInfo{},
		}
		m.mu.Unlock()

		log.Printf("[MCP] Servidor carregado: %s (%s, transport=%s, enabled=%v, auto_connect=%v)",
			slug, cfg.Name, cfg.Transport, cfg.Enabled, cfg.AutoConnect)

		// Auto-connect se habilitado
		if cfg.Enabled && cfg.AutoConnect {
			go func(s string) {
				if err := m.Connect(s); err != nil {
					log.Printf("[MCP] Erro ao conectar '%s': %v", s, err)
				}
			}(slug)
		}
	}

	return nil
}

// Connect conecta a um servidor MCP pelo slug.
func (m *Manager) Connect(slug string) error {
	m.mu.Lock()
	status, ok := m.servers[slug]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("servidor MCP '%s' não encontrado", slug)
	}

	if !status.Config.Enabled {
		m.mu.Unlock()
		return fmt.Errorf("servidor MCP '%s' está desabilitado", slug)
	}

	// Se já conectado, desconecta primeiro
	if _, connected := m.connections[slug]; connected {
		m.mu.Unlock()
		m.Disconnect(slug)
		m.mu.Lock()
	}

	status.Status = StatusConnecting
	status.Error = ""
	cfg := status.Config
	m.mu.Unlock()

	m.emit("mcp:server_connecting", map[string]string{"slug": slug})

	// Cria o cliente MCP
	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{
			Name:    "assistente",
			Version: "1.0.0",
		},
		nil,
	)

	// Cria o transport com base no tipo (inclui autenticação se configurada)
	transport, err := m.createTransport(slug, cfg)
	if err != nil {
		m.setError(slug, fmt.Sprintf("erro ao criar transport: %v", err))
		return err
	}

	// Conecta ao servidor com timeout
	connectCtx, connectCancel := context.WithTimeout(m.ctx, connectTimeout)
	defer connectCancel()

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		m.setError(slug, fmt.Sprintf("erro ao conectar: %v", err))
		return fmt.Errorf("falha ao conectar ao servidor MCP '%s': %w", slug, err)
	}

	// Captura capabilities do servidor
	// TODO: Quando SDK expor ServerCapabilities(), capturar capabilities reais
	capabilities := ServerCapabilities{}
	log.Printf("[MCP] Servidor '%s' conectado (capabilities não disponíveis no SDK v1.3.0)", slug)

	// Descobre as ferramentas do servidor
	toolsCtx, toolsCancel := context.WithTimeout(m.ctx, listToolsTimeout)
	defer toolsCancel()

	toolsResult, err := session.ListTools(toolsCtx, nil)
	if err != nil {
		session.Close()
		m.setError(slug, fmt.Sprintf("erro ao listar ferramentas: %v", err))
		return fmt.Errorf("falha ao listar tools do servidor MCP '%s': %w", slug, err)
	}

	// Cria bridges e registra no registry
	var bridges []*MCPToolBridge
	var toolInfos []MCPToolInfo

	for _, mcpTool := range toolsResult.Tools {
		bridge := NewMCPToolBridge(slug, mcpTool, session)
		bridges = append(bridges, bridge)
		toolInfos = append(toolInfos, bridge.ToMCPToolInfo())

		// Registra no registry (ignora erro se já existe — pode ser reconexão)
		if err := m.registry.Register(bridge); err != nil {
			log.Printf("[MCP] Aviso: tool '%s' já registrada: %v", bridge.Name(), err)
		}
	}

	// Descobre resources do servidor
	var resourceInfos []MCPResourceInfo
	resourcesResult, err := session.ListResources(toolsCtx, nil)
	if err == nil && resourcesResult != nil {
		for _, res := range resourcesResult.Resources {
			resourceInfos = append(resourceInfos, MCPResourceInfo{
				URI:         res.URI,
				Name:        res.Name,
				Description: res.Description,
				MIMEType:    res.MIMEType,
				ServerSlug:  slug,
			})
		}
		log.Printf("[MCP] Servidor '%s': %d resources descobertos", slug, len(resourceInfos))
	}

	// Descobre prompts do servidor
	var promptInfos []MCPPromptInfo
	promptsResult, err := session.ListPrompts(toolsCtx, nil)
	if err == nil && promptsResult != nil {
		for _, prompt := range promptsResult.Prompts {
			args := make([]MCPPromptArgument, len(prompt.Arguments))
			for i, arg := range prompt.Arguments {
				args[i] = MCPPromptArgument{
					Name:        arg.Name,
					Description: arg.Description,
					Required:    arg.Required,
				}
			}
			promptInfos = append(promptInfos, MCPPromptInfo{
				Name:        prompt.Name,
				Description: prompt.Description,
				Arguments:   args,
				ServerSlug:  slug,
			})
		}
		log.Printf("[MCP] Servidor '%s': %d prompts descobertos", slug, len(promptInfos))
	}

	now := time.Now()

	// TODO: Quando SDK suportar, configurar handlers:
	// - session.SetLogHandler(logHandler)
	// - session.SetProgressHandler(progressHandler)
	// - session.SetResourceUpdatedHandler(resourceSubHandler)
	
	logHandler := m.createLogHandler(slug)
	progressHandler := m.createProgressHandler(slug)
	resourceSubHandler := m.createResourceUpdateHandler(slug)

	// Se temos roots configurados, armazenar para futuro uso
	m.mu.RLock()
	currentRoots := m.roots
	m.mu.RUnlock()
	
	if len(currentRoots) > 0 {
		log.Printf("[MCP] Roots disponíveis para '%s': %d roots", slug, len(currentRoots))
		// TODO: Quando SDK suportar, enviar via session.NotifyRootsListChanged
	}

	// Inicia health check goroutine
	healthCtx, healthCancel := context.WithCancel(m.ctx)
	go m.healthCheckLoop(healthCtx, slug)

	// Atualiza estado
	m.mu.Lock()
	m.connections[slug] = &serverConnection{
		client:             client,
		session:            session,
		bridges:            bridges,
		cancelHealth:       healthCancel,
		logHandler:         logHandler,
		progressHandler:    progressHandler,
		resourceSubHandler: resourceSubHandler,
	}
	if s, ok := m.servers[slug]; ok {
		s.Status = StatusConnected
		s.Error = ""
		s.Tools = toolInfos
		s.Capabilities = capabilities
		s.Roots = currentRoots
		s.Resources = resourceInfos
		s.Prompts = promptInfos
		s.ConnectedAt = &now
		s.LastPing = &now
		s.RetryCount = 0
	}
	m.mu.Unlock()

	log.Printf("[MCP] Servidor '%s' conectado: %d ferramentas, %d resources, %d prompts", 
		slug, len(bridges), len(resourceInfos), len(promptInfos))
	for _, t := range toolInfos {
		log.Printf("[MCP]   - tool: %s (%s)", t.FullName, t.Name)
	}

	m.emit("mcp:server_connected", map[string]any{
		"slug":          slug,
		"toolCount":     len(bridges),
		"resourceCount": len(resourceInfos),
		"promptCount":   len(promptInfos),
	})
	m.emit("mcp:tools_changed", nil)

	return nil
}

// Disconnect desconecta de um servidor MCP.
func (m *Manager) Disconnect(slug string) error {
	m.mu.Lock()
	conn, ok := m.connections[slug]
	if !ok {
		m.mu.Unlock()
		return nil // não conectado, nada a fazer
	}

	// Cancela health check
	if conn.cancelHealth != nil {
		conn.cancelHealth()
	}

	// Remove tools do registry
	for _, bridge := range conn.bridges {
		m.registry.Unregister(bridge.Name())
	}

	delete(m.connections, slug)

	if s, ok := m.servers[slug]; ok {
		s.Status = StatusDisconnected
		s.Error = ""
		s.Tools = []MCPToolInfo{}
		s.Resources = []MCPResourceInfo{}
		s.Prompts = []MCPPromptInfo{}
		s.ConnectedAt = nil
		s.LastPing = nil
	}
	m.mu.Unlock()

	// Fecha a sessão (fora do lock para evitar deadlock)
	if conn.session != nil {
		if err := conn.session.Close(); err != nil {
			log.Printf("[MCP] Erro ao fechar sessão '%s': %v", slug, err)
		}
	}

	log.Printf("[MCP] Servidor '%s' desconectado", slug)

	m.emit("mcp:server_disconnected", map[string]string{"slug": slug})
	m.emit("mcp:tools_changed", nil)

	return nil
}

// Reconnect desconecta e reconecta a um servidor.
func (m *Manager) Reconnect(slug string) error {
	m.Disconnect(slug)
	return m.Connect(slug)
}

// List retorna informações de todos os servidores (formato frontend-safe).
func (m *Manager) List() []ServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ServerInfo, 0, len(m.servers))
	for _, status := range m.servers {
		result = append(result, status.toServerInfo())
	}
	return result
}

// GetTools retorna as tools de um servidor específico.
func (m *Manager) GetTools(slug string) []MCPToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if s, ok := m.servers[slug]; ok {
		return s.Tools
	}
	return []MCPToolInfo{}
}

// GetConfig retorna a configuração de um servidor.
func (m *Manager) GetConfig(slug string) (*ServerConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if s, ok := m.servers[slug]; ok {
		cfg := s.Config // cópia
		return &cfg, nil
	}
	return nil, fmt.Errorf("servidor MCP '%s' não encontrado", slug)
}

// SaveConfig salva (cria ou atualiza) a configuração de um servidor MCP.
func (m *Manager) SaveConfig(slug string, cfg ServerConfig) error {
	m.mu.Lock()
	m.lastSelfWrite = time.Now()
	m.mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar config: %w", err)
	}

	filename := slug + configExt

	// Tenta escrever (atualiza se existe, cria se não)
	if m.resolver.Exists(filename) {
		if err := m.resolver.Write(filename, data); err != nil {
			return fmt.Errorf("erro ao salvar config: %w", err)
		}
	} else {
		if err := m.resolver.Create(filename, data); err != nil {
			return fmt.Errorf("erro ao criar config: %w", err)
		}
	}

	// Atualiza estado em memória
	m.mu.Lock()
	if existing, ok := m.servers[slug]; ok {
		existing.Config = cfg
	} else {
		m.servers[slug] = &ServerStatus{
			Slug:   slug,
			Config: cfg,
			Status: StatusDisconnected,
			Tools:  []MCPToolInfo{},
		}
	}
	m.mu.Unlock()

	log.Printf("[MCP] Configuração salva: %s", slug)
	m.emit("mcp:config_changed", map[string]string{"slug": slug})

	return nil
}

// DuplicateConfig cria uma copia da configuracao de um servidor MCP.
func (m *Manager) DuplicateConfig(slug string) (string, error) {
	cfg, err := m.GetConfig(slug)
	if err != nil {
		return "", err
	}

	newSlug := m.nextCopySlug(slug)
	newCfg := *cfg
	if newCfg.Name == "" {
		newCfg.Name = slug
	}
	newCfg.Name = fmt.Sprintf("%s (Copia)", newCfg.Name)

	if err := m.SaveConfig(newSlug, newCfg); err != nil {
		return "", err
	}

	return newSlug, nil
}

func (m *Manager) nextCopySlug(baseSlug string) string {
	if baseSlug == "" {
		baseSlug = "mcp"
	}

	if !m.slugExists(baseSlug + "-copia") {
		return baseSlug + "-copia"
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-copia-%d", baseSlug, i)
		if !m.slugExists(candidate) {
			return candidate
		}
	}
}

func (m *Manager) slugExists(slug string) bool {
	return m.resolver.Exists(slug + configExt)
}

// DeleteConfig remove a configuração de um servidor MCP.
// Desconecta automaticamente se estiver conectado.
func (m *Manager) DeleteConfig(slug string) error {
	m.mu.Lock()
	m.lastSelfWrite = time.Now()
	m.mu.Unlock()

	// Desconecta primeiro
	m.Disconnect(slug)

	filename := slug + configExt
	if m.resolver.Exists(filename) {
		if err := m.resolver.Delete(filename); err != nil {
			return fmt.Errorf("erro ao deletar config: %w", err)
		}
	}

	m.mu.Lock()
	delete(m.servers, slug)
	m.mu.Unlock()

	log.Printf("[MCP] Configuração removida: %s", slug)
	m.emit("mcp:config_changed", map[string]string{"slug": slug})

	return nil
}

// CloseAll desconecta todos os servidores e cancela operações pendentes.
func (m *Manager) CloseAll() {
	m.cancel()

	m.mu.RLock()
	slugs := make([]string, 0, len(m.connections))
	for slug := range m.connections {
		slugs = append(slugs, slug)
	}
	m.mu.RUnlock()

	for _, slug := range slugs {
		if err := m.Disconnect(slug); err != nil {
			log.Printf("[MCP] Erro ao desconectar '%s' no shutdown: %v", slug, err)
		}
	}

	log.Printf("[MCP] Todos os servidores MCP desconectados")
}

// createTransport cria o transport apropriado para o tipo de servidor,
// incluindo autenticação integrada com o gerenciador de credenciais.
func (m *Manager) createTransport(slug string, cfg ServerConfig) (mcpsdk.Transport, error) {
	switch cfg.Transport {
	case TransportStdio:
		if cfg.Command == "" {
			return nil, fmt.Errorf("campo 'command' é obrigatório para transport stdio")
		}

		cmd := exec.CommandContext(m.ctx, cfg.Command, cfg.Args...)

		if len(cfg.Env) > 0 {
			cmd.Env = buildEnv(cfg.Env)
		}

		return &mcpsdk.CommandTransport{Command: cmd}, nil

	case TransportSSE:
		if cfg.URL == "" {
			return nil, fmt.Errorf("campo 'url' é obrigatório para transport sse")
		}

		httpClient := m.buildAuthHTTPClient(slug, cfg)

		return &mcpsdk.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}, nil

	case TransportStreamable:
		if cfg.URL == "" {
			return nil, fmt.Errorf("campo 'url' é obrigatório para transport streamable")
		}

		httpClient := m.buildAuthHTTPClient(slug, cfg)

		return &mcpsdk.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}, nil

	default:
		return nil, fmt.Errorf("transport desconhecido: '%s' (use 'stdio', 'sse' ou 'streamable')", cfg.Transport)
	}
}

// buildAuthHTTPClient cria um *http.Client com autenticação configurada
// com base no authType do servidor e credenciais do gerenciador.
// Retorna nil se nenhuma autenticação estiver configurada (o SDK usará http.DefaultClient).
func (m *Manager) buildAuthHTTPClient(slug string, cfg ServerConfig) *http.Client {
	switch cfg.AuthType {
	case AuthOAuth2PKCE:
		onConfigUpdate := func(updated ServerConfig) {
			if err := m.SaveConfig(slug, updated); err != nil {
				log.Printf("[MCP:%s] Erro ao persistir config após atualização OAuth: %v", slug, err)
			}
		}
		client := buildPKCEHTTPClient(cfg, m.credMgr, m.emitEvent, slug, onConfigUpdate)
		log.Printf("[MCP:%s] HTTP client configurado com OAuth2 PKCE", slug)
		return client

	case AuthOAuth2ClientCredentials:
		_, clientSecret := loadClientCreds(m.credMgr, slug)
		if clientSecret != "" {
			client := buildClientCredentialsHTTPClient(cfg, clientSecret)
			log.Printf("[MCP:%s] HTTP client configurado com OAuth2 Client Credentials", slug)
			return client
		}
		log.Printf("[MCP:%s] OAuth2 Client Credentials configurado mas sem client_secret no credential manager (mcp-client:%s)", slug, slug)
		return nil

	case AuthBearer:
		if m.credMgr != nil && cfg.URL != "" {
			if auth, err := m.credMgr.ResolveForURL(cfg.URL); err == nil && auth != nil && auth.Token != "" {
				client := &http.Client{
					Transport: &bearerRoundTripper{
						base:  http.DefaultTransport,
						token: auth.Token,
					},
				}
				log.Printf("[MCP:%s] HTTP client configurado com Bearer token", slug)
				return client
			}
		}
		log.Printf("[MCP:%s] Bearer auth configurado mas sem token no credential manager", slug)
		return nil

	case AuthBasic:
		if m.credMgr != nil && cfg.URL != "" {
			if auth, err := m.credMgr.ResolveForURL(cfg.URL); err == nil && auth != nil && auth.Username != "" {
				client := &http.Client{
					Transport: &basicAuthRoundTripper{
						base:     http.DefaultTransport,
						username: auth.Username,
						password: auth.Password,
					},
				}
				log.Printf("[MCP:%s] HTTP client configurado com Basic auth", slug)
				return client
			}
		}
		log.Printf("[MCP:%s] Basic auth configurado mas sem credenciais no credential manager", slug)
		return nil

	default:
		return nil
	}
}

// bearerRoundTripper injeta Authorization: Bearer em todas as requisições.
type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (rt *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.base.RoundTrip(cloned)
}

// basicAuthRoundTripper injeta Authorization: Basic em todas as requisições.
type basicAuthRoundTripper struct {
	base     http.RoundTripper
	username string
	password string
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.SetBasicAuth(rt.username, rt.password)
	return rt.base.RoundTrip(cloned)
}

// setError atualiza o status de um servidor para erro.
func (m *Manager) setError(slug, errMsg string) {
	m.mu.Lock()
	if s, ok := m.servers[slug]; ok {
		s.Status = StatusError
		s.Error = errMsg
	}
	m.mu.Unlock()

	log.Printf("[MCP] Erro no servidor '%s': %s", slug, errMsg)
	m.emit("mcp:server_error", map[string]string{
		"slug":  slug,
		"error": errMsg,
	})
}

// emit emite um evento Wails.
func (m *Manager) emit(event string, data any) {
	if m.emitEvent != nil {
		m.emitEvent(event, data)
	}
}

// healthCheckLoop executa pings periódicos para verificar a saúde do servidor.
func (m *Manager) healthCheckLoop(ctx context.Context, slug string) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck(slug)
		}
	}
}

// performHealthCheck executa um ping no servidor e atualiza o status.
func (m *Manager) performHealthCheck(slug string) {
	m.mu.RLock()
	conn, ok := m.connections[slug]
	if !ok {
		m.mu.RUnlock()
		return
	}
	status, hasStatus := m.servers[slug]
	if hasStatus && status.Reconnecting {
		m.mu.RUnlock()
		return
	}
	session := conn.session
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(m.ctx, healthCheckTimeout)
	defer cancel()

	err := session.Ping(ctx, nil)

	// "Method not found" = servidor respondeu, apenas não suporta ping.
	// Trata como saudável — o canal de comunicação está funcionando.
	if err != nil && isMethodNotFound(err) {
		log.Printf("[MCP] Servidor '%s' não suporta ping, tratando como saudável", slug)
		err = nil
	}

	now := time.Now()
	m.mu.Lock()
	if s, ok := m.servers[slug]; ok {
		if err != nil {
			s.ConsecutiveHealthFailures++
			log.Printf("[MCP] Health check falhou para '%s' (%d/%d): %v",
				slug, s.ConsecutiveHealthFailures, healthCheckFailThreshold, err)

			if s.ConsecutiveHealthFailures >= healthCheckFailThreshold {
				s.Status = StatusError
				s.Error = fmt.Sprintf("health check falhou: %v", err)
				s.ConsecutiveHealthFailures = 0
				go m.reconnectWithRetry(slug)
			}
		} else {
			s.LastPing = &now
			s.ConsecutiveHealthFailures = 0
			if s.Status == StatusError {
				s.Status = StatusConnected
				s.Error = ""
			}
		}
	}
	m.mu.Unlock()

	if err != nil {
		m.emit("mcp:server_unhealthy", map[string]string{
			"slug":  slug,
			"error": err.Error(),
		})
	}
}

// isMethodNotFound detecta erros JSON-RPC "method not found" (código -32601).
// Servidores que não implementam ping retornam esse erro.
func isMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "method not found") || strings.Contains(msg, "-32601")
}

// reconnectWithRetry tenta reconectar ao servidor com exponential backoff.
// Após esgotar as tentativas rápidas, entra em modo lento (a cada maxRetryDelay)
// até conseguir reconectar ou o servidor ser desabilitado/removido.
func (m *Manager) reconnectWithRetry(slug string) {
	m.mu.Lock()
	status, ok := m.servers[slug]
	if !ok || !status.Config.Enabled {
		m.mu.Unlock()
		return
	}

	if status.Reconnecting || status.Status == StatusConnecting || status.Status == StatusConnected {
		m.mu.Unlock()
		return
	}

	status.Reconnecting = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		if s, ok := m.servers[slug]; ok {
			s.Reconnecting = false
		}
		m.mu.Unlock()
	}()

	for {
		m.mu.RLock()
		status, ok := m.servers[slug]
		if !ok || !status.Config.Enabled {
			m.mu.RUnlock()
			return
		}
		retryCount := status.RetryCount
		m.mu.RUnlock()

		var delay time.Duration
		if retryCount >= maxRetries {
			delay = maxRetryDelay
			log.Printf("[MCP] Retries rápidos esgotados para '%s', nova tentativa em %v", slug, delay)
		} else {
			delay = baseRetryDelay * time.Duration(1<<uint(retryCount))
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			log.Printf("[MCP] Tentando reconectar '%s' em %v (tentativa %d/%d)",
				slug, delay, retryCount+1, maxRetries)
		}

		m.mu.Lock()
		if s, ok := m.servers[slug]; ok && retryCount < maxRetries {
			s.RetryCount++
		}
		m.mu.Unlock()

		select {
		case <-time.After(delay):
		case <-m.ctx.Done():
			return
		}

		m.mu.RLock()
		status, ok = m.servers[slug]
		if !ok || !status.Config.Enabled || status.Status == StatusConnected {
			m.mu.RUnlock()
			return
		}
		m.mu.RUnlock()

		m.Disconnect(slug)

		if err := m.Connect(slug); err != nil {
			log.Printf("[MCP] Falha ao reconectar '%s': %v", slug, err)
			continue
		}

		log.Printf("[MCP] Reconexão bem-sucedida para '%s'", slug)
		m.mu.Lock()
		if s, ok := m.servers[slug]; ok {
			s.RetryCount = 0
		}
		m.mu.Unlock()
		return
	}
}

// ReadResource lê o conteúdo de um resource MCP.
func (m *Manager) ReadResource(slug, uri string) (string, error) {
	m.mu.RLock()
	conn, ok := m.connections[slug]
	if !ok {
		m.mu.RUnlock()
		return "", fmt.Errorf("servidor MCP '%s' não está conectado", slug)
	}
	session := conn.session
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(m.ctx, listToolsTimeout)
	defer cancel()

	result, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{
		URI: uri,
	})
	if err != nil {
		return "", fmt.Errorf("erro ao ler resource: %w", err)
	}

	if len(result.Contents) == 0 {
		return "", fmt.Errorf("resource vazio")
	}

	// Extrai texto do primeiro conteúdo
	if result.Contents[0].Text != "" {
		return result.Contents[0].Text, nil
	}

	return "", fmt.Errorf("resource não contém texto")
}

// GetPrompt executa um prompt MCP e retorna as mensagens geradas.
func (m *Manager) GetPrompt(slug, name string, arguments map[string]string) ([]string, error) {
	m.mu.RLock()
	conn, ok := m.connections[slug]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("servidor MCP '%s' não está conectado", slug)
	}
	session := conn.session
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(m.ctx, listToolsTimeout)
	defer cancel()

	result, err := session.GetPrompt(ctx, &mcpsdk.GetPromptParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao obter prompt: %w", err)
	}

	var messages []string
	for _, msg := range result.Messages {
		if tc, ok := msg.Content.(*mcpsdk.TextContent); ok {
			messages = append(messages, tc.Text)
		}
	}

	return messages, nil
}

// createLogHandler cria um handler para logs do servidor MCP.
func (m *Manager) createLogHandler(slug string) func(LogEntry) {
	return func(entry LogEntry) {
		entry.ServerSlug = slug
		entry.Timestamp = time.Now()
		
		// Log local
		log.Printf("[MCP:%s] [%s] %v", slug, entry.Level, entry.Data)
		
		// Emite para frontend
		m.emit("mcp:log", entry)
	}
}

// createProgressHandler cria um handler para notificações de progresso.
func (m *Manager) createProgressHandler(slug string) func(ProgressNotification) {
	return func(progress ProgressNotification) {
		log.Printf("[MCP:%s] Progress: %.1f%%", slug, progress.Progress)
		
		// Emite para frontend
		m.emit("mcp:progress", map[string]any{
			"slug":     slug,
			"token":    progress.ProgressToken.Value,
			"progress": progress.Progress,
			"total":    progress.Total,
		})
	}
}

// createResourceUpdateHandler cria um handler para notificações de resource updates.
func (m *Manager) createResourceUpdateHandler(slug string) func(ResourceUpdated) {
	return func(update ResourceUpdated) {
		log.Printf("[MCP:%s] Resource updated: %s", slug, update.URI)
		
		// Emite para frontend
		m.emit("mcp:resource_updated", map[string]any{
			"slug": slug,
			"uri":  update.URI,
		})
		
		// Re-lista resources para atualizar cache
		go func() {
			m.mu.RLock()
			conn, ok := m.connections[slug]
			m.mu.RUnlock()
			
			if !ok {
				return
			}
			
			ctx, cancel := context.WithTimeout(m.ctx, listToolsTimeout)
			defer cancel()
			
			resourcesResult, err := conn.session.ListResources(ctx, nil)
			if err != nil {
				log.Printf("[MCP:%s] Erro ao re-listar resources: %v", slug, err)
				return
			}
			
			var resourceInfos []MCPResourceInfo
			for _, res := range resourcesResult.Resources {
				resourceInfos = append(resourceInfos, MCPResourceInfo{
					URI:         res.URI,
					Name:        res.Name,
					Description: res.Description,
					MIMEType:    res.MIMEType,
					ServerSlug:  slug,
				})
			}
			
			m.mu.Lock()
			if s, ok := m.servers[slug]; ok {
				s.Resources = resourceInfos
			}
			m.mu.Unlock()
			
			log.Printf("[MCP:%s] Resources atualizados: %d total", slug, len(resourceInfos))
		}()
	}
}

// SubscribeToResource inscreve para receber notificações de um resource.
// TODO: Implementar quando SDK suportar session.SubscribeResource
func (m *Manager) SubscribeToResource(slug, uri string) error {
	m.mu.RLock()
	_, ok := m.connections[slug]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("servidor MCP '%s' não está conectado", slug)
	}
	
	status, ok := m.servers[slug]
	if !ok || status.Capabilities.Resources == nil || !status.Capabilities.Resources.Subscribe {
		m.mu.RUnlock()
		return fmt.Errorf("servidor '%s' não suporta resource subscriptions", slug)
	}
	m.mu.RUnlock()

	// TODO: Implementar quando SDK expor SubscribeResource
	log.Printf("[MCP:%s] Subscriptions ainda não suportadas pelo SDK (v1.3.0)", slug)
	return fmt.Errorf("resource subscriptions não disponíveis no SDK atual")
}

// UnsubscribeFromResource cancela inscrição de um resource.
// TODO: Implementar quando SDK suportar session.UnsubscribeResource
func (m *Manager) UnsubscribeFromResource(slug, uri string) error {
	m.mu.RLock()
	_, ok := m.connections[slug]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("servidor MCP '%s' não está conectado", slug)
	}
	m.mu.RUnlock()

	// TODO: Implementar quando SDK expor UnsubscribeResource
	log.Printf("[MCP:%s] Unsubscribe ainda não suportado pelo SDK (v1.3.0)", slug)
	return fmt.Errorf("resource subscriptions não disponíveis no SDK atual")
}

// HandleSamplingRequest processa uma requisição de sampling de um servidor MCP.
// Requer que um handler LLM tenha sido configurado via SetSamplingHandler.
func (m *Manager) HandleSamplingRequest(ctx context.Context, slug string, request SamplingRequest) (string, error) {
	m.mu.RLock()
	handler := m.llmHandler
	m.mu.RUnlock()
	
	if handler == nil {
		return "", fmt.Errorf("nenhum handler LLM configurado para sampling")
	}
	
	log.Printf("[MCP:%s] Processando sampling request com %d mensagens", slug, len(request.Messages))
	
	response, err := handler(ctx, request)
	if err != nil {
		return "", fmt.Errorf("erro no handler LLM: %w", err)
	}
	
	log.Printf("[MCP:%s] Sampling completado, resposta: %d chars", slug, len(response))
	return response, nil
}

// GetNativeServerInfo retorna informações para uso nativo de MCP por modelos.
// Permite que modelos com suporte MCP nativo acessem os servidores diretamente.
func (m *Manager) GetNativeServerInfo() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var servers []map[string]any
	for slug, status := range m.servers {
		if status.Status != StatusConnected {
			continue
		}

		conn, ok := m.connections[slug]
		if !ok {
			continue
		}

		serverInfo := map[string]any{
			"slug":        slug,
			"name":        status.Config.Name,
			"description": status.Config.Description,
			"transport":   status.Config.Transport,
			"capabilities": map[string]any{
				"tools":     len(status.Tools) > 0,
				"resources": len(status.Resources) > 0,
				"prompts":   len(status.Prompts) > 0,
			},
		}

		// Inclui informação de transporte para acesso direto
		if status.Config.Transport == TransportSSE || status.Config.Transport == TransportStreamable {
			serverInfo["endpoint"] = status.Config.URL
		}

		// Inclui session ID para uso direto
		if conn.session != nil {
			serverInfo["sessionId"] = conn.session.ID()
		}

		servers = append(servers, serverInfo)
	}

	return servers
}
