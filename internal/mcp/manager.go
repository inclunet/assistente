package mcp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/tools"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
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

	// tokenRefreshCheckInterval é o intervalo para verificar expiração de tokens OAuth2
	tokenRefreshCheckInterval = 1 * time.Minute

	// tokenRefreshThreshold é a antecedência mínima para forçar refresh antes da expiração
	tokenRefreshThreshold = 2 * time.Minute
)

// emitFunc é a callback para emitir eventos Wails
type emitFunc func(event string, data any)

// serverConnection mantém o estado runtime de um servidor MCP conectado.
type serverConnection struct {
	client             *mcpsdk.Client
	session            *mcpsdk.ClientSession
	bridges            []*MCPToolBridge           // tools registradas no registry
	cancelHealth       context.CancelFunc         // cancela health check goroutine
	cancelTokenRefresh context.CancelFunc         // cancela token refresh goroutine (OAuth2)
	logHandler         func(LogEntry)             // handler para logs do servidor
	progressHandler    func(ProgressNotification) // handler para progresso
	resourceSubHandler func(ResourceUpdated)      // handler para resource updates
}

// Manager gerencia servidores MCP: configuração, conexão, discovery de tools.
// Thread-safe para uso concorrente.
type Manager struct {
	mu             sync.RWMutex
	resolver       *configdir.Resolver
	repo           Repository
	credMgr        *credentials.Manager
	registry       *tools.Registry
	emitEvent      emitFunc
	llmHandler     func(context.Context, SamplingRequest) (string, error) // handler para sampling requests
	servers        map[string]*ServerStatus                               // slug -> status
	connections    map[string]*serverConnection                           // slug -> connection ativa
	connectCancels map[string]context.CancelFunc                          // slug -> cancel for in-flight Connect()
	ctx            context.Context
	cancel         context.CancelFunc
	authContext    func() context.Context
	roots          []Root // workspace roots globais
	lastSelfWrite  time.Time
}

// NewManager cria um novo gerenciador de servidores MCP.
func NewManager(registry *tools.Registry, credMgr *credentials.Manager, emitEvent emitFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		resolver:       configdir.NewResolver(configSubdir),
		credMgr:        credMgr,
		registry:       registry,
		emitEvent:      emitEvent,
		servers:        make(map[string]*ServerStatus),
		connections:    make(map[string]*serverConnection),
		connectCancels: make(map[string]context.CancelFunc),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// SetRepository configura o backing store persistido do Manager.
// Sem repository, o Manager mantém o caminho legado em arquivos JSON.
func (m *Manager) SetRepository(repo Repository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repo = repo
}

func (m *Manager) repository() Repository {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.repo
}

func (m *Manager) UsesRepository() bool {
	return m.repository() != nil
}

// SetAuthContextProvider configura o contexto usado para resolver credenciais
// user-scoped de servidores MCP.
func (m *Manager) SetAuthContextProvider(provider func() context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authContext = provider
}

func (m *Manager) credentialContext() context.Context {
	m.mu.RLock()
	provider := m.authContext
	m.mu.RUnlock()
	if provider == nil {
		return context.Background()
	}
	ctx := provider()
	if ctx == nil {
		return context.Background()
	}
	return ctx
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

// LoadConfigs carrega configs persistidas e popula o estado runtime do Manager.
// NÃO conecta aos servidores — para isso
// chame AutoConnectAll depois (tipicamente do reloadUserScopedRuntime
// pós-Login, quando as credenciais user-scoped já estão em memória).
//
// A separação é o fix do AEP-0061: a versão antiga disparava
// `go m.Connect(slug)` para cada server enabled+autoconnect dentro
// daqui, e como LoadConfigs roda no startup pré-login, todos os
// servidores OAuth perdiam a credencial em memória, caíam no fallback
// "sem token", abriam o navegador para reauth — N janelas em paralelo.
func (m *Manager) LoadConfigs() error {
	if repo := m.repository(); repo != nil {
		ctx := m.credentialContext()
		if _, err := database.RequireUserID(ctx); err != nil {
			log.Printf("[MCP] LoadConfigs aguardando usuário autenticado: %v", err)
			return nil
		}
		if err := m.migrateFilesystemConfigsToRepository(ctx, repo); err != nil {
			return err
		}
		configs, err := repo.ListServers(ctx)
		if err != nil {
			return err
		}
		next := make(map[string]*ServerStatus, len(configs))
		for _, cfg := range configs {
			cfg.applyDefaults(cfg.Slug)
			next[cfg.Slug] = &ServerStatus{
				ID:     cfg.ID,
				Slug:   cfg.Slug,
				Config: cfg,
				Status: StatusDisconnected,
				Tools:  []MCPToolInfo{},
				Roots:  m.GetWorkspaceRoots(),
			}
			log.Printf("[MCP] Servidor carregado do DB: %s (%s, transport=%s, enabled=%v, auto_connect=%v)",
				cfg.Slug, cfg.Name, cfg.Transport, cfg.Enabled, cfg.AutoConnect)
		}
		m.mu.Lock()
		m.servers = next
		m.mu.Unlock()
		return nil
	}

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

		slug := f.Name
		cfg, err := ParseServerConfig(data, slug)
		if err != nil {
			log.Printf("[MCP] Erro ao parsear %s: %v", f.Filename, err)
			continue
		}
		m.applyInlineAuthFromConfig(slug, &cfg, data)
		m.mu.Lock()
		m.servers[slug] = &ServerStatus{
			ID:     cfg.ID,
			Slug:   slug,
			Config: cfg,
			Status: StatusDisconnected,
			Tools:  []MCPToolInfo{},
		}
		m.mu.Unlock()

		log.Printf("[MCP] Servidor carregado: %s (%s, transport=%s, enabled=%v, auto_connect=%v)",
			slug, cfg.Name, cfg.Transport, cfg.Enabled, cfg.AutoConnect)
	}

	return nil
}

// AutoConnectAll conecta sequencialmente a todos os servidores
// `Enabled && AutoConnect`. É o caminho legítimo pós-login: o caller
// (reloadUserScopedRuntime) já garantiu que as credenciais user-scoped
// estão em memória, então cada Connect resolve o token sem cair no
// flow OAuth interativo.
//
// Ordem é determinística (slug ordenado), serializada (um Connect por
// vez) e cancela imediatamente se `ctx` for cancelado. Se algum
// servidor precisar de OAuth interativo (refresh expirado de
// verdade), o `oauthFlowArbiter` global mantém o serial — outras
// conexões não-OAuth seguem.
func (m *Manager) AutoConnectAll(ctx context.Context) {
	m.mu.RLock()
	slugs := make([]string, 0, len(m.servers))
	for slug, s := range m.servers {
		if s.Config.Enabled && s.Config.AutoConnect {
			slugs = append(slugs, slug)
		}
	}
	m.mu.RUnlock()

	sort.Strings(slugs)

	for _, slug := range slugs {
		select {
		case <-ctx.Done():
			log.Printf("[MCP] AutoConnectAll cancelado: %v", ctx.Err())
			return
		default:
		}
		if err := m.connectWithContext(ctx, slug); err != nil {
			log.Printf("[MCP] AutoConnectAll: erro ao conectar '%s': %v", slug, err)
		}
	}
}

// Connect conecta a um servidor MCP pelo slug.
func (m *Manager) Connect(slug string) error {
	return m.connectWithContext(m.ctx, slug)
}

func (m *Manager) connectWithContext(parentCtx context.Context, slug string) error {
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

	if status.Status == StatusConnecting {
		m.mu.Unlock()
		return fmt.Errorf("servidor MCP '%s' já está conectando — use Desconectar para cancelar", slug)
	}

	// Se já conectado, desconecta primeiro
	if _, connected := m.connections[slug]; connected {
		m.mu.Unlock()
		_ = m.Disconnect(slug)
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

	// Probe SSE: para Streamable HTTP, verifica se o servidor suporta SSE
	// antes de conectar, evitando esperar timeouts longos em 5 retries do SDK.
	if cfg.Transport == TransportStreamable && !cfg.DisableSSE && cfg.URL != "" {
		httpClient := m.buildAuthHTTPClient(slug, cfg)
		if sseSupported, reason := probeSSESupport(cfg.URL, httpClient); !sseSupported {
			log.Printf("[MCP:%s] SSE não suportado (%s) — usando polling", slug, reason)
			cfg.DisableSSE = true
		}
	}

	// Cria o transport com base no tipo (inclui autenticação se configurada)
	transport, err := m.createTransport(slug, cfg)
	if err != nil {
		m.setError(slug, fmt.Sprintf("erro ao criar transport: %v", err))
		return err
	}

	// Conecta ao servidor com timeout; registra cancel para permitir abortar via Disconnect
	connectCtx, connectCancel := context.WithTimeout(parentCtx, connectTimeout)
	m.mu.Lock()
	m.connectCancels[slug] = connectCancel
	m.mu.Unlock()
	defer func() {
		connectCancel()
		m.mu.Lock()
		delete(m.connectCancels, slug)
		m.mu.Unlock()
	}()

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		// SSE failure: auto-fallback to polling (disable SSE and retry once)
		if !cfg.DisableSSE && cfg.Transport == TransportStreamable &&
			strings.Contains(err.Error(), "standalone SSE") {
			log.Printf("[MCP:%s] SSE falhou — tentando reconectar sem SSE (polling)", slug)
			cfg.DisableSSE = true
			m.mu.Lock()
			if s, ok := m.servers[slug]; ok {
				s.Config.DisableSSE = true
			}
			m.mu.Unlock()
			_ = m.SaveConfig(slug, cfg)

			client = mcpsdk.NewClient(&mcpsdk.Implementation{Name: "assistente", Version: "1.0.0"}, nil)
			transport2, err2 := m.createTransport(slug, cfg)
			if err2 == nil {
				retryCtx, retryCancel := context.WithTimeout(parentCtx, connectTimeout)
				defer retryCancel()
				session, err = client.Connect(retryCtx, transport2, nil)
			}
		}
		if err != nil {
			m.setError(slug, fmt.Sprintf("erro ao conectar: %v", err))
			return fmt.Errorf("falha ao conectar ao servidor MCP '%s': %w", slug, err)
		}
	}

	// Captura capabilities do servidor
	// TODO: Quando SDK expor ServerCapabilities(), capturar capabilities reais
	capabilities := ServerCapabilities{}
	log.Printf("[MCP] Servidor '%s' conectado (capabilities não disponíveis no SDK v1.3.0)", slug)

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

	// Inicia token refresh proativo para servidores OAuth2
	var tokenRefreshCancel context.CancelFunc
	if cfg.AuthType == AuthOAuth2PKCE || cfg.AuthType == AuthOAuth2ClientCredentials {
		tokenCtx, cancel := context.WithCancel(m.ctx)
		tokenRefreshCancel = cancel
		go m.tokenRefreshLoop(tokenCtx, slug)
	}

	now := time.Now()

	// Atualiza estado da conexão (antes de descobrir offerings)
	m.mu.Lock()
	m.connections[slug] = &serverConnection{
		client:             client,
		session:            session,
		cancelHealth:       healthCancel,
		cancelTokenRefresh: tokenRefreshCancel,
		logHandler:         logHandler,
		progressHandler:    progressHandler,
		resourceSubHandler: resourceSubHandler,
	}
	if s, ok := m.servers[slug]; ok {
		s.Status = StatusConnected
		s.Error = ""
		s.Capabilities = capabilities
		s.Roots = currentRoots
		s.ConnectedAt = &now
		s.LastPing = &now
		s.RetryCount = 0
	}
	m.mu.Unlock()

	// Descobre tools, resources e prompts do servidor
	if err := m.refreshServerOfferingsWithContext(parentCtx, slug); err != nil {
		_ = m.Disconnect(slug)
		m.setError(slug, fmt.Sprintf("erro ao descobrir offerings: %v", err))
		return fmt.Errorf("falha ao descobrir offerings do servidor MCP '%s': %w", slug, err)
	}

	m.emit("mcp:server_connected", map[string]any{
		"slug": slug,
	})

	return nil
}

// refreshServerOfferings re-descobre tools, resources e prompts de um servidor
// já conectado, atualizando o registry e o ServerStatus.
// Usado pelo Connect inicial e pelo health check periódico.
func (m *Manager) refreshServerOfferings(slug string) error {
	return m.refreshServerOfferingsWithContext(m.ctx, slug)
}

func (m *Manager) refreshServerOfferingsWithContext(parentCtx context.Context, slug string) error {
	m.mu.RLock()
	conn, ok := m.connections[slug]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("servidor '%s' não está conectado", slug)
	}
	session := conn.session
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(parentCtx, listToolsTimeout)
	defer cancel()

	// Descobre tools
	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("erro ao listar ferramentas: %w", err)
	}

	var bridges []*MCPToolBridge
	var toolInfos []MCPToolInfo
	for _, mcpTool := range toolsResult.Tools {
		bridge := NewMCPToolBridge(slug, mcpTool, session)
		bridge.onSessionError = m.handleToolCallError
		bridges = append(bridges, bridge)
		toolInfos = append(toolInfos, bridge.ToMCPToolInfo())
	}

	// Descobre resources
	var resourceInfos []MCPResourceInfo
	resourcesResult, err := session.ListResources(ctx, nil)
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
	}

	// Descobre prompts
	var promptInfos []MCPPromptInfo
	promptsResult, err := session.ListPrompts(ctx, nil)
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
	}

	// Atualiza registry: remove bridges antigos, registra novos
	m.mu.Lock()
	conn, ok = m.connections[slug]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("servidor '%s' desconectou durante refresh", slug)
	}

	oldBridges := conn.bridges
	for _, old := range oldBridges {
		m.registry.Unregister(old.Name())
	}
	for _, bridge := range bridges {
		if err := m.registry.Register(bridge); err != nil {
			log.Printf("[MCP] Aviso: tool '%s' já registrada: %v", bridge.Name(), err)
		}
	}
	conn.bridges = bridges

	changed := false
	if s, ok := m.servers[slug]; ok {
		changed = len(s.Tools) != len(toolInfos) ||
			len(s.Resources) != len(resourceInfos) ||
			len(s.Prompts) != len(promptInfos)
		s.Tools = toolInfos
		s.Resources = resourceInfos
		s.Prompts = promptInfos
	}
	m.mu.Unlock()

	if changed {
		m.emit("mcp:tools_changed", nil)
	}
	m.syncMCPToolsBestEffort(m.credentialContext(), slug, toolInfos)

	log.Printf("[MCP] Servidor '%s' offerings atualizados: %d tools, %d resources, %d prompts",
		slug, len(toolInfos), len(resourceInfos), len(promptInfos))

	return nil
}

// Disconnect desconecta de um servidor MCP.
// Se o servidor está em StatusConnecting, cancela a tentativa de conexão.
func (m *Manager) Disconnect(slug string) error {
	m.mu.Lock()
	conn, ok := m.connections[slug]
	if !ok {
		// Not connected yet — but may be mid-Connect(). Cancel and reset status.
		if cancelFn, has := m.connectCancels[slug]; has {
			cancelFn()
			delete(m.connectCancels, slug)
			if s, exists := m.servers[slug]; exists {
				s.Status = StatusDisconnected
				s.Error = ""
			}
			m.mu.Unlock()
			log.Printf("[MCP] Conexão em andamento de '%s' cancelada pelo usuário", slug)
			m.emit("mcp:server_disconnected", map[string]string{"slug": slug})
			return nil
		}
		m.mu.Unlock()
		return nil
	}

	// Cancela goroutines
	if conn.cancelHealth != nil {
		conn.cancelHealth()
	}
	if conn.cancelTokenRefresh != nil {
		conn.cancelTokenRefresh()
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
	return m.reconnectWithContext(m.ctx, slug)
}

func (m *Manager) reconnectWithContext(ctx context.Context, slug string) error {
	_ = m.Disconnect(slug)
	return m.connectWithContext(ctx, slug)
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
	if s, ok := m.servers[slug]; ok {
		cfg := s.Config // cópia
		m.mu.RUnlock()
		return &cfg, nil
	}
	m.mu.RUnlock()

	if repo := m.repository(); repo != nil {
		return repo.GetServer(m.credentialContext(), slug)
	}
	return nil, fmt.Errorf("servidor MCP '%s' não encontrado", slug)
}

// SaveConfig salva (cria ou atualiza) a configuração de um servidor MCP.
func (m *Manager) SaveConfig(slug string, cfg ServerConfig) error {
	if repo := m.repository(); repo != nil {
		ctx := m.credentialContext()
		if _, err := database.RequireUserID(ctx); err != nil {
			return err
		}
		cfg.Slug = slug
		if err := repo.SaveServer(ctx, &cfg); err != nil {
			return fmt.Errorf("erro ao salvar config: %w", err)
		}
		m.mu.Lock()
		if existing, ok := m.servers[slug]; ok {
			existing.ID = cfg.ID
			existing.Config = cfg
		} else {
			m.servers[slug] = &ServerStatus{
				ID:     cfg.ID,
				Slug:   slug,
				Config: cfg,
				Status: StatusDisconnected,
				Tools:  []MCPToolInfo{},
			}
		}
		m.mu.Unlock()
		log.Printf("[MCP] Configuração salva no DB: %s", slug)
		m.emit("mcp:config_changed", map[string]string{"slug": slug})
		return nil
	}

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
			ID:     cfg.ID,
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
	newCfg.ID = ""
	newCfg.Slug = newSlug
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
	if repo := m.repository(); repo != nil {
		_, err := repo.GetServer(m.credentialContext(), slug)
		return err == nil
	}
	return m.resolver.Exists(slug + configExt)
}

// DeleteConfig remove a configuração de um servidor MCP.
// Desconecta automaticamente se estiver conectado.
func (m *Manager) DeleteConfig(slug string) error {
	if repo := m.repository(); repo != nil {
		ctx := m.credentialContext()
		if _, err := database.RequireUserID(ctx); err != nil {
			return err
		}
		_ = m.Disconnect(slug)
		if err := repo.DeleteServer(ctx, slug); err != nil {
			return fmt.Errorf("erro ao deletar config: %w", err)
		}
		m.mu.Lock()
		delete(m.servers, slug)
		m.mu.Unlock()
		log.Printf("[MCP] Configuração removida do DB: %s", slug)
		m.emit("mcp:config_changed", map[string]string{"slug": slug})
		return nil
	}

	m.mu.Lock()
	m.lastSelfWrite = time.Now()
	m.mu.Unlock()

	// Desconecta primeiro
	_ = m.Disconnect(slug)

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

// CloseAll desconecta todos os servidores e cancela operações
// pendentes. É shutdown DEFINITIVO do Manager — depois desta chamada
// o Manager não conecta mais (`m.cancel()` invalida o ctx base).
// Use no Stop do app. Para logout/troca de user use DisconnectAll.
func (m *Manager) CloseAll() {
	m.cancel()
	m.disconnectAllConnections("shutdown")
	log.Printf("[MCP] Todos os servidores MCP desconectados")
}

// DisconnectAll fecha todas as conexões abertas SEM derrubar o
// Manager. É o caminho do logout/troca de user: as conexões do user
// anterior precisam soltar (porque os tokens user-scoped vão sair de
// memória), mas o Manager continua vivo para conectar de novo
// quando o próximo user fizer login.
func (m *Manager) DisconnectAll() {
	m.disconnectAllConnections("logout")
}

func (m *Manager) disconnectAllConnections(reason string) {
	m.mu.RLock()
	slugs := make([]string, 0, len(m.connections))
	for slug := range m.connections {
		slugs = append(slugs, slug)
	}
	m.mu.RUnlock()

	for _, slug := range slugs {
		if err := m.Disconnect(slug); err != nil {
			log.Printf("[MCP] Erro ao desconectar '%s' (%s): %v", slug, reason, err)
		}
	}
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

		transport := &mcpsdk.StreamableClientTransport{
			Endpoint:             cfg.URL,
			HTTPClient:           httpClient,
			DisableStandaloneSSE: cfg.DisableSSE,
		}

		if cfg.DisableSSE {
			log.Printf("[MCP:%s] Standalone SSE desabilitado por configuração", slug)
		}

		return transport, nil

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
		client := buildPKCEHTTPClient(cfg, m.credMgr, m.emitEvent, slug, onConfigUpdate, m.credentialContext)
		log.Printf("[MCP:%s] HTTP client configurado com OAuth2 PKCE", slug)
		return client

	case AuthOAuth2ClientCredentials:
		_, clientSecret := loadClientCreds(m.credentialContext(), m.credMgr, slug)
		if clientSecret != "" {
			client := buildClientCredentialsHTTPClient(cfg, clientSecret)
			log.Printf("[MCP:%s] HTTP client configurado com OAuth2 Client Credentials", slug)
			return client
		}
		log.Printf("[MCP:%s] OAuth2 Client Credentials configurado mas sem client_secret no credential manager (mcp-client:%s)", slug, slug)
		return nil

	case AuthBearer:
		if m.credMgr != nil && cfg.URL != "" {
			if auth, err := m.credMgr.ResolveForURLWithContext(m.credentialContext(), cfg.URL); err == nil && auth != nil && auth.Token != "" {
				client := &http.Client{
					Transport: &bearerRoundTripper{
						base:  newMCPTransport(),
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
			if auth, err := m.credMgr.ResolveForURLWithContext(m.credentialContext(), cfg.URL); err == nil && auth != nil && auth.Username != "" {
				client := &http.Client{
					Transport: &basicAuthRoundTripper{
						base:     newMCPTransport(),
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

// sseAwareTransport routes SSE requests (long-lived GET with text/event-stream)
// through a transport with relaxed timeouts, while regular requests use strict
// timeouts. This avoids ResponseHeaderTimeout killing SSE connections behind
// corporate proxies that buffer responses.
type sseAwareTransport struct {
	regular *http.Transport
	sse     *http.Transport
}

func (t *sseAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet &&
		strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		cloned := req.Clone(req.Context())
		cloned.Header.Set("X-Accel-Buffering", "no")
		cloned.Header.Set("Cache-Control", "no-cache")
		return t.sse.RoundTrip(cloned)
	}
	return t.regular.RoundTrip(req)
}

func newMCPTransport() http.RoundTripper {
	regular := &http.Transport{
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	sse := &http.Transport{
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       0,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}

	return &sseAwareTransport{regular: regular, sse: sse}
}

// probeSSESupport sends a quick GET with Accept: text/event-stream to the MCP
// endpoint to check if the server supports SSE. Returns (true, "") if supported,
// or (false, reason) if not. Uses a short timeout so this doesn't slow down connect.
func probeSSESupport(mcpURL string, authClient *http.Client) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mcpURL, nil)
	if err != nil {
		return true, "" // can't probe, assume SSE is supported
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := authClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, "timeout — servidor não respondeu ao GET SSE"
		}
		return false, fmt.Sprintf("erro: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	ct := resp.Header.Get("Content-Type")
	switch {
	case resp.StatusCode == http.StatusMethodNotAllowed:
		return false, "405 Method Not Allowed"
	case resp.StatusCode == http.StatusNotFound:
		return false, "404 Not Found"
	case resp.StatusCode >= 400:
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	case strings.Contains(ct, "text/event-stream"):
		return true, ""
	default:
		log.Printf("[MCP:probe] SSE probe: HTTP %d, Content-Type: %s", resp.StatusCode, ct)
		return true, "" // ambiguous — assume supported, let SDK handle it
	}
}

// bearerRoundTripper injeta Authorization: Bearer em todas as requisições.
type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (rt *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", bearerAuthorizationHeader(rt.token))
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

// tokenRefreshLoop verifica periodicamente a expiração de tokens OAuth2
// e força refresh proativo antes que expirem.
func (m *Manager) tokenRefreshLoop(ctx context.Context, slug string) {
	ticker := time.NewTicker(tokenRefreshCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAndRefreshToken(slug)
		}
	}
}

// checkAndRefreshToken verifica se o token OAuth2 está próximo de expirar
// e força um refresh proativo usando o refresh_token.
func (m *Manager) checkAndRefreshToken(slug string) {
	refreshed, err := m.refreshOAuthTokenBestEffort(m.ctx, slug, false)
	if err != nil {
		log.Printf("[MCP:%s] Refresh proativo falhou: %v", slug, err)
		return
	}
	if refreshed {
		log.Printf("[MCP:%s] Token renovado proativamente", slug)
	}
}

func (m *Manager) refreshOAuthTokenBestEffort(ctx context.Context, slug string, force bool) (bool, error) {
	if m.credMgr == nil {
		return false, nil
	}

	m.mu.RLock()
	status, ok := m.servers[slug]
	if !ok || status.Reconnecting {
		m.mu.RUnlock()
		return false, nil
	}
	cfg := status.Config
	m.mu.RUnlock()

	if cfg.AuthType != AuthOAuth2PKCE {
		return false, nil
	}

	authCtx := m.credentialContext()
	auth, err := m.credMgr.GetByPatternWithContext(authCtx, userTokensPattern(slug))
	if err != nil || auth == nil {
		return false, nil
	}

	timeUntilExpiry := time.Duration(0)
	if auth.ExpiresAt != 0 {
		expiresAt := time.Unix(auth.ExpiresAt, 0)
		timeUntilExpiry = time.Until(expiresAt)
	}
	if !force {
		if auth.ExpiresAt == 0 {
			return false, nil
		}
		if timeUntilExpiry > tokenRefreshThreshold {
			return false, nil
		}
	}

	if force {
		log.Printf("[MCP:%s] Executando refresh OAuth best-effort (expiry em %v)", slug, timeUntilExpiry.Round(time.Second))
	} else {
		log.Printf("[MCP:%s] Token expira em %v — forçando refresh proativo", slug, timeUntilExpiry.Round(time.Second))
	}

	clientID, clientSecret := loadClientCreds(authCtx, m.credMgr, slug)
	if clientID == "" {
		clientID = cfg.OAuth2ClientID
	}

	token := loadUserTokens(authCtx, m.credMgr, slug)
	if token == nil || token.RefreshToken == "" {
		log.Printf("[MCP:%s] Sem refresh_token disponível para refresh", slug)
		return false, nil
	}

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  cfg.OAuth2AuthURL,
			TokenURL: cfg.OAuth2TokenURL,
		},
		Scopes: cfg.OAuth2Scopes,
	}

	expiredToken := &oauth2.Token{
		RefreshToken: token.RefreshToken,
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	newToken, err := oauthCfg.TokenSource(refreshCtx, expiredToken).Token()
	if err != nil {
		return false, err
	}

	newAuth := &credentials.AuthConfig{
		Type:       "oauth2",
		Token:      newToken.AccessToken,
		RefreshURL: newToken.RefreshToken,
	}
	if newToken.Expiry.After(time.Now()) {
		newAuth.ExpiresAt = newToken.Expiry.Unix()
	}
	if err := m.credMgr.RegisterPatternWithContext(refreshCtx, userTokensPattern(slug), newAuth); err != nil {
		return false, err
	}

	log.Printf("[MCP:%s] Token renovado (novo expiry: %v)", slug, newToken.Expiry.Format(time.RFC3339))
	return true, nil
}

// RecoverServerBestEffort tenta restaurar um servidor MCP para chamadas futuras do chat.
// A operação é best-effort: falhas são retornadas, mas não devem interromper a resposta atual.
func (m *Manager) RecoverServerBestEffort(ctx context.Context, slug string) RecoveryResult {
	result := RecoveryResult{}

	select {
	case <-ctx.Done():
		result.Err = ctx.Err()
		return result
	default:
	}

	m.mu.RLock()
	status, ok := m.servers[slug]
	if !ok {
		m.mu.RUnlock()
		result.Err = fmt.Errorf("servidor MCP '%s' não encontrado", slug)
		return result
	}
	if !status.Config.Enabled {
		m.mu.RUnlock()
		result.Err = fmt.Errorf("servidor MCP '%s' está desabilitado", slug)
		return result
	}
	currentStatus := status.Status
	m.mu.RUnlock()

	result.Attempted = true

	refreshed, refreshErr := m.refreshOAuthTokenBestEffort(ctx, slug, true)
	if refreshed {
		result.Refreshed = true
	}

	if currentStatus == StatusConnected {
		if err := m.refreshServerOfferingsWithContext(ctx, slug); err == nil {
			return result
		} else if refreshErr == nil {
			refreshErr = err
		} else {
			refreshErr = errors.Join(refreshErr, err)
		}
	}

	if err := m.reconnectWithContext(ctx, slug); err == nil {
		result.Reconnected = true
		return result
	} else if refreshErr == nil {
		refreshErr = err
	} else {
		refreshErr = errors.Join(refreshErr, err)
	}

	result.Err = refreshErr
	return result
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
// Para servidores que não suportam ping, faz fallback para refreshServerOfferings,
// que também mantém tools/resources/prompts sincronizados.
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

	if err != nil && isMethodNotFound(err) {
		// Ping não suportado — valida a sessão re-descobrindo offerings.
		// Isso também mantém as tools sincronizadas caso o servidor as altere.
		err = m.refreshServerOfferings(slug)
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

// handleToolCallError é chamado pelo MCPToolBridge quando um tool call falha
// com erro de transporte/sessão. Dispara reconexão imediata.
func (m *Manager) handleToolCallError(slug string, err error) {
	m.mu.Lock()
	status, ok := m.servers[slug]
	if !ok || !status.Config.Enabled || status.Reconnecting {
		m.mu.Unlock()
		return
	}

	log.Printf("[MCP] Erro de sessão/transporte em tool call para '%s': %v", slug, err)

	status.ConsecutiveHealthFailures = healthCheckFailThreshold
	status.Status = StatusError
	status.Error = fmt.Sprintf("sessão perdida: %v", err)
	m.mu.Unlock()

	m.emit("mcp:server_unhealthy", map[string]string{
		"slug":  slug,
		"error": err.Error(),
	})

	go m.reconnectWithRetry(slug)
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

		_ = m.Disconnect(slug)

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

// ImportFromMCPJSON parses Cursor/Claude MCP config formats and creates
// individual config files. Returns the number of servers imported.
// Expects {"mcpServers": {...}} (Cursor) or entries keyed directly.
// Skips servers that already exist (won't overwrite).
func (m *Manager) ImportFromMCPJSON(data []byte) (int, error) {
	type mcpEntry struct {
		Command     string            `json:"command"`
		Args        []string          `json:"args"`
		Env         map[string]string `json:"env"`
		URL         string            `json:"url"`
		Headers     map[string]string `json:"headers"`
		RequestInit struct {
			Headers map[string]string `json:"headers"`
		} `json:"requestInit"`
	}

	// Try Cursor/Claude format: {"mcpServers": {...}}
	var wrapper struct {
		MCPServers map[string]mcpEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return 0, fmt.Errorf("failed to parse MCP JSON: %w", err)
	}

	servers := wrapper.MCPServers
	if len(servers) == 0 {
		// Try flat format: {"name": {...}, ...}
		if err := json.Unmarshal(data, &servers); err != nil {
			return 0, fmt.Errorf("failed to parse MCP JSON (flat format): %w", err)
		}
	}

	if len(servers) == 0 {
		return 0, nil
	}

	imported := 0
	for name, entry := range servers {
		slug := sanitizeSlug(name)

		// Skip if already exists
		m.mu.RLock()
		_, exists := m.servers[slug]
		m.mu.RUnlock()
		if exists {
			log.Printf("[MCP:import] Servidor '%s' já existe — ignorando", slug)
			continue
		}

		cfg := ServerConfig{
			Command:     entry.Command,
			Args:        entry.Args,
			Env:         entry.Env,
			URL:         entry.URL,
			Enabled:     true,
			AutoConnect: true,
		}
		cfg.applyDefaults(slug)
		if token := extractBearerTokenFromHeaders(entry.RequestInit.Headers); token != "" {
			cfg.AuthType = AuthBearer
			m.importBearerCredential(slug, cfg.URL, token)
		} else if token := extractBearerTokenFromHeaders(entry.Headers); token != "" {
			cfg.AuthType = AuthBearer
			m.importBearerCredential(slug, cfg.URL, token)
		}

		cfgData, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			log.Printf("[MCP:import] Erro ao serializar config para '%s': %v", slug, err)
			continue
		}

		filename := slug + configExt
		if err := m.resolver.Write(filename, cfgData); err != nil {
			log.Printf("[MCP:import] Erro ao gravar config '%s': %v", filename, err)
			continue
		}

		log.Printf("[MCP:import] Servidor importado: %s (transport=%s)", slug, cfg.Transport)
		imported++
	}

	if imported > 0 {
		m.syncConfigsFromDisk()
	}

	return imported, nil
}

func sanitizeSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	var clean []byte
	for _, c := range []byte(slug) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			clean = append(clean, c)
		}
	}
	return string(clean)
}

// NativeMCPServer descreve um servidor MCP HTTP elegível para passthrough nativo.
// Usado pelo chat loop para montar MCPServerConfig e passar ao ChatProvider.
type NativeMCPServer struct {
	Slug      string
	Name      string
	URL       string
	AuthToken string
	ToolNames []string // nomes das tools registradas (namespaced, para filtragem)
}

// RecoveryResult descreve o resultado de uma tentativa best-effort de recuperação
// de um servidor MCP para chamadas futuras do chat.
type RecoveryResult struct {
	Attempted   bool
	Refreshed   bool
	Reconnected bool
	Err         error
}

// isNativeMCPEligibleURL verifica se a URL é segura para MCP nativo.
// HTTPS é sempre aceito. HTTP é aceito apenas para localhost/loopback (dev local).
// URLs HTTP com host remoto são rejeitadas: o LLM provider faz a conexão
// server-side, e enviar auth tokens por HTTP sem encriptação é inseguro.
func isNativeMCPEligibleURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, "https://") {
		return true
	}
	if strings.HasPrefix(rawURL, "http://") {
		host := hostnameFromURL(rawURL)
		return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
	}
	return false
}

// GetEligibleNativeMCPServers retorna servidores HTTP conectados e com tools,
// elegíveis para MCP nativo (SSE ou Streamable HTTP).
// Servidores STDIO são excluídos — não podem ser acessados remotamente.
// Servidores HTTP com URL insegura (HTTP em host não-local) são excluídos.
func (m *Manager) GetEligibleNativeMCPServers() []NativeMCPServer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []NativeMCPServer
	for slug, status := range m.servers {
		if status.Status != StatusConnected {
			continue
		}
		if status.Config.Transport != TransportSSE && status.Config.Transport != TransportStreamable {
			continue
		}
		if status.Config.URL == "" {
			continue
		}
		if len(status.Tools) == 0 {
			continue
		}
		if status.Config.PreferBridge {
			log.Printf("[MCP] servidor %q excluído do MCP nativo: prefer_bridge=true (bridge local será usado)",
				slug)
			continue
		}
		if !isNativeMCPEligibleURL(status.Config.URL) {
			log.Printf("[MCP] servidor %q excluído do MCP nativo: URL %q não é HTTPS nem localhost (adapter será usado)",
				slug, status.Config.URL)
			continue
		}

		srv := NativeMCPServer{
			Slug: slug,
			Name: status.Config.Name,
			URL:  status.Config.URL,
		}

		// Coleta nomes das tools registradas (fullName = namespaced)
		for _, t := range status.Tools {
			srv.ToolNames = append(srv.ToolNames, t.FullName)
		}

		// Resolve auth token se disponível (escopado pelo user vigente)
		if m.credMgr != nil {
			authCtx := m.credentialContext()
			if auth, err := m.credMgr.GetByPatternWithContext(authCtx, userTokensPattern(slug)); err == nil && auth != nil && auth.Token != "" {
				srv.AuthToken = auth.Token
				log.Printf("[MCP] servidor %q: token OAuth resolvido (pattern=%s, len=%d, expires=%d)",
					slug, userTokensPattern(slug), len(auth.Token), auth.ExpiresAt)
			} else {
				if hostname := hostnameFromURL(status.Config.URL); hostname != "" {
					if auth, err := m.credMgr.GetByPatternWithContext(authCtx, hostname); err == nil && auth != nil && auth.Token != "" {
						srv.AuthToken = auth.Token
						log.Printf("[MCP] servidor %q: token resolvido por hostname (pattern=%s)", slug, hostname)
					} else {
						log.Printf("[MCP] servidor %q: NENHUM token encontrado (oauth=%s, hostname=%s)",
							slug, userTokensPattern(slug), hostname)
					}
				} else {
					log.Printf("[MCP] servidor %q: NENHUM token encontrado (oauth=%s, sem hostname)",
						slug, userTokensPattern(slug))
				}
			}
		}

		result = append(result, srv)
	}
	return result
}
