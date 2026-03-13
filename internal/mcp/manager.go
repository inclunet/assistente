package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	healthCheckInterval = 30 * time.Second

	// healthCheckTimeout é o timeout para ping de health check
	healthCheckTimeout = 5 * time.Second

	// maxRetries é o número máximo de tentativas de reconexão
	maxRetries = 5

	// baseRetryDelay é o delay inicial para retry (exponential backoff)
	baseRetryDelay = 1 * time.Second

	// maxRetryDelay é o delay máximo entre retries
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
	registry    *tools.Registry
	credMgr     *credentials.Manager
	emitEvent   emitFunc
	llmHandler  func(context.Context, SamplingRequest) (string, error)
	servers     map[string]*ServerStatus
	connections map[string]*serverConnection
	ctx         context.Context
	cancel      context.CancelFunc
	roots       []Root

	// lastSelfWrite rastreia quando o próprio manager escreveu configs,
	// para o file watcher ignorar eventos causados pelo app.
	lastSelfWrite time.Time
}

// NewManager cria um novo gerenciador de servidores MCP.
func NewManager(registry *tools.Registry, credMgr *credentials.Manager, emitEvent emitFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		resolver:    configdir.NewResolver(configSubdir),
		registry:    registry,
		credMgr:     credMgr,
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
			Slug:    slug,
			Config:  cfg,
			Status:  StatusDisconnected,
			Tools:   []MCPToolInfo{},
			HasAuth: m.hasAuthForURL(cfg.URL),
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

	// Cria o transport com base no tipo
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

	m.mu.Lock()
	m.lastSelfWrite = time.Now()
	m.mu.Unlock()

	log.Printf("[MCP] Configuração salva: %s", slug)
	m.emit("mcp:config_changed", map[string]string{"slug": slug})

	return nil
}

// DeleteConfig remove a configuração de um servidor MCP.
// Desconecta automaticamente se estiver conectado.
func (m *Manager) DeleteConfig(slug string) error {
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

	m.mu.Lock()
	m.lastSelfWrite = time.Now()
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

// createTransport cria o transport apropriado para o tipo de servidor.
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

		httpClient, err := m.buildHTTPClientForAuth(slug, cfg)
		if err != nil {
			return nil, err
		}

		return &mcpsdk.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}, nil

	case TransportStreamable:
		if cfg.URL == "" {
			return nil, fmt.Errorf("campo 'url' é obrigatório para transport streamable")
		}

		httpClient, err := m.buildHTTPClientForAuth(slug, cfg)
		if err != nil {
			return nil, err
		}

		return &mcpsdk.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}, nil

	default:
		return nil, fmt.Errorf("transport desconhecido: '%s' (use 'stdio', 'sse' ou 'streamable')", cfg.Transport)
	}
}

// buildHTTPClientForAuth cria o http.Client adequado ao auth_type configurado.
func (m *Manager) buildHTTPClientForAuth(slug string, cfg ServerConfig) (*http.Client, error) {
	switch cfg.AuthType {
	case AuthOAuth2ClientCredentials:
		clientSecret, err := m.resolveClientSecret(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("client_secret não encontrado para OAuth2 Client Credentials: %w", err)
		}
		log.Printf("[MCP:%s] Usando OAuth2 Client Credentials (client_id=%s)", slug, cfg.OAuth2ClientID)
		return buildClientCredentialsHTTPClient(cfg, clientSecret), nil

	case AuthOAuth2PKCE:
		log.Printf("[MCP:%s] Usando OAuth2 Authorization Code + PKCE (client_id=%s)", slug, cfg.OAuth2ClientID)
		onConfigUpdate := func(updated ServerConfig) {
			if err := m.SaveConfig(slug, updated); err != nil {
				log.Printf("[MCP:%s] Erro ao persistir config após DCR: %v", slug, err)
			}
		}
		return buildPKCEHTTPClient(cfg, m.credMgr, m.emitEvent, slug, onConfigUpdate), nil

	default:
		return m.buildAuthHTTPClient(), nil
	}
}

// resolveClientSecret busca o client_secret armazenado no credential manager.
func (m *Manager) resolveClientSecret(serverURL string) (string, error) {
	if m.credMgr == nil {
		return "", fmt.Errorf("credential manager não disponível")
	}
	auth, err := m.credMgr.ResolveForURL(serverURL)
	if err != nil {
		return "", err
	}
	if auth == nil {
		return "", fmt.Errorf("nenhuma credencial encontrada")
	}
	if auth.ClientSecret != "" {
		return auth.ClientSecret, nil
	}
	if auth.Token != "" {
		return auth.Token, nil
	}
	return "", fmt.Errorf("client_secret não encontrado na credencial")
}

// credentialRoundTripper é um http.RoundTripper que resolve e injeta
// credenciais do credential manager centralizado em cada request HTTP.
// Usa o mesmo padrão do interceptor HTTP em internal/tools/http/client.go.
type credentialRoundTripper struct {
	base    http.RoundTripper
	credMgr *credentials.Manager
}

func (rt *credentialRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.credMgr != nil && req.Header.Get("Authorization") == "" {
		if auth, err := rt.credMgr.ResolveForURL(req.URL.String()); err == nil && auth != nil {
			switch auth.Type {
			case "bearer":
				if auth.Token != "" {
					if !strings.HasPrefix(auth.Token, "Bearer ") {
						req.Header.Set("Authorization", "Bearer "+auth.Token)
					} else {
						req.Header.Set("Authorization", auth.Token)
					}
				}
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					req.SetBasicAuth(auth.Username, auth.Password)
				}
			case "custom":
				for key, val := range auth.Headers {
					req.Header.Set(key, val)
				}
			}
		}
	}
	return rt.base.RoundTrip(req)
}

// buildAuthHTTPClient cria um *http.Client que injeta credenciais via credential manager.
func (m *Manager) buildAuthHTTPClient() *http.Client {
	return &http.Client{
		Transport: &credentialRoundTripper{
			base:    http.DefaultTransport,
			credMgr: m.credMgr,
		},
	}
}

// hostnameFromURL extrai o hostname (sem porta) de uma URL.
func hostnameFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// hasAuthForURL verifica se existe credencial registrada para o hostname da URL.
func (m *Manager) hasAuthForURL(rawURL string) bool {
	if m.credMgr == nil || rawURL == "" {
		return false
	}
	auth, err := m.credMgr.ResolveForURL(rawURL)
	return err == nil && auth != nil
}

// SaveServerAuth salva credenciais para um servidor MCP no credential manager.
// Usa o hostname da URL como padrão de credencial.
func (m *Manager) SaveServerAuth(slug string, authType, token, username, password, clientSecret string) error {
	m.mu.RLock()
	status, ok := m.servers[slug]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("servidor MCP '%s' não encontrado", slug)
	}
	serverURL := status.Config.URL
	m.mu.RUnlock()

	hostname := hostnameFromURL(serverURL)
	if hostname == "" {
		return fmt.Errorf("servidor '%s' não tem URL configurada", slug)
	}

	auth := &credentials.AuthConfig{
		Type:         authType,
		Token:        token,
		Username:     username,
		Password:     password,
		ClientSecret: clientSecret,
	}

	if err := m.credMgr.RegisterPatternWithContext(context.Background(), hostname, auth); err != nil {
		return fmt.Errorf("erro ao salvar credencial: %w", err)
	}

	m.mu.Lock()
	status.HasAuth = true
	m.mu.Unlock()

	log.Printf("[MCP] Credenciais salvas para '%s' (pattern: %s)", slug, hostname)
	m.emit("mcp:auth_changed", map[string]string{"slug": slug})
	return nil
}

// DeleteServerAuth remove credenciais de um servidor MCP do credential manager.
func (m *Manager) DeleteServerAuth(slug string) error {
	m.mu.RLock()
	status, ok := m.servers[slug]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("servidor MCP '%s' não encontrado", slug)
	}
	serverURL := status.Config.URL
	m.mu.RUnlock()

	hostname := hostnameFromURL(serverURL)
	if hostname == "" {
		return nil
	}

	if err := m.credMgr.DeletePattern(context.Background(), hostname); err != nil {
		return fmt.Errorf("erro ao remover credencial: %w", err)
	}

	m.mu.Lock()
	status.HasAuth = false
	m.mu.Unlock()

	log.Printf("[MCP] Credenciais removidas para '%s' (pattern: %s)", slug, hostname)
	m.emit("mcp:auth_changed", map[string]string{"slug": slug})
	return nil
}

// GetServerAuthInfo retorna informações sobre a autenticação de um servidor
// (tipo e se existe, sem expor valores sensíveis).
func (m *Manager) GetServerAuthInfo(slug string) (authType string, hasAuth bool, err error) {
	m.mu.RLock()
	status, ok := m.servers[slug]
	if !ok {
		m.mu.RUnlock()
		return "", false, fmt.Errorf("servidor MCP '%s' não encontrado", slug)
	}
	serverURL := status.Config.URL
	m.mu.RUnlock()

	if m.credMgr == nil || serverURL == "" {
		return "", false, nil
	}

	auth, resolveErr := m.credMgr.ResolveForURL(serverURL)
	if resolveErr != nil || auth == nil {
		return "", false, nil
	}

	return auth.Type, true, nil
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
	session := conn.session
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(m.ctx, healthCheckTimeout)
	defer cancel()

	err := session.Ping(ctx, nil)
	
	now := time.Now()
	m.mu.Lock()
	if s, ok := m.servers[slug]; ok {
		if err != nil {
			log.Printf("[MCP] Health check falhou para '%s': %v", slug, err)
			s.Status = StatusError
			s.Error = fmt.Sprintf("health check falhou: %v", err)
			
			// Inicia reconnect em background
			go m.reconnectWithRetry(slug)
		} else {
			s.LastPing = &now
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

// reconnectWithRetry tenta reconectar ao servidor com exponential backoff.
func (m *Manager) reconnectWithRetry(slug string) {
	m.mu.Lock()
	status, ok := m.servers[slug]
	if !ok || !status.Config.Enabled {
		m.mu.Unlock()
		return
	}
	
	// Se já está tentando reconectar ou conectado, não faz nada
	if status.Status == StatusConnecting || status.Status == StatusConnected {
		m.mu.Unlock()
		return
	}
	
	retryCount := status.RetryCount
	if retryCount >= maxRetries {
		log.Printf("[MCP] Número máximo de retries atingido para '%s'", slug)
		status.Error = fmt.Sprintf("máximo de %d tentativas de reconexão excedido", maxRetries)
		m.mu.Unlock()
		return
	}
	
	status.RetryCount++
	m.mu.Unlock()

	// Calcula delay com exponential backoff
	delay := baseRetryDelay * time.Duration(1<<uint(retryCount))
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	log.Printf("[MCP] Tentando reconectar '%s' em %v (tentativa %d/%d)", 
		slug, delay, retryCount+1, maxRetries)

	time.Sleep(delay)

	// Desconecta antes de reconectar
	m.Disconnect(slug)

	// Tenta reconectar
	if err := m.Connect(slug); err != nil {
		log.Printf("[MCP] Falha ao reconectar '%s': %v", slug, err)
		// reconnectWithRetry será chamado novamente no próximo health check
	} else {
		log.Printf("[MCP] Reconexão bem-sucedida para '%s'", slug)
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
