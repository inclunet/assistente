package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"assistente/internal/configdir"
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
)

// emitFunc é a callback para emitir eventos Wails
type emitFunc func(event string, data any)

// serverConnection mantém o estado runtime de um servidor MCP conectado.
type serverConnection struct {
	client  *mcpsdk.Client
	session *mcpsdk.ClientSession
	bridges []*MCPToolBridge // tools registradas no registry
}

// Manager gerencia servidores MCP: configuração, conexão, discovery de tools.
// Thread-safe para uso concorrente.
type Manager struct {
	mu          sync.RWMutex
	resolver    *configdir.Resolver
	registry    *tools.Registry
	emitEvent   emitFunc
	servers     map[string]*ServerStatus     // slug -> status
	connections map[string]*serverConnection // slug -> connection ativa
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewManager cria um novo gerenciador de servidores MCP.
func NewManager(registry *tools.Registry, emitEvent emitFunc) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		resolver:    configdir.NewResolver(configSubdir),
		registry:    registry,
		emitEvent:   emitEvent,
		servers:     make(map[string]*ServerStatus),
		connections: make(map[string]*serverConnection),
		ctx:         ctx,
		cancel:      cancel,
	}
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

	// Cria o transport com base no tipo
	transport, err := m.createTransport(cfg)
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

	now := time.Now()

	// Atualiza estado
	m.mu.Lock()
	m.connections[slug] = &serverConnection{
		client:  client,
		session: session,
		bridges: bridges,
	}
	if s, ok := m.servers[slug]; ok {
		s.Status = StatusConnected
		s.Error = ""
		s.Tools = toolInfos
		s.ConnectedAt = &now
	}
	m.mu.Unlock()

	log.Printf("[MCP] Servidor '%s' conectado: %d ferramentas descobertas", slug, len(bridges))
	for _, t := range toolInfos {
		log.Printf("[MCP]   - %s (%s)", t.FullName, t.Name)
	}

	m.emit("mcp:server_connected", map[string]any{
		"slug":      slug,
		"toolCount": len(bridges),
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

	// Remove tools do registry
	for _, bridge := range conn.bridges {
		m.registry.Unregister(bridge.Name())
	}

	delete(m.connections, slug)

	if s, ok := m.servers[slug]; ok {
		s.Status = StatusDisconnected
		s.Error = ""
		s.Tools = []MCPToolInfo{}
		s.ConnectedAt = nil
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
func (m *Manager) createTransport(cfg ServerConfig) (mcpsdk.Transport, error) {
	switch cfg.Transport {
	case TransportStdio:
		if cfg.Command == "" {
			return nil, fmt.Errorf("campo 'command' é obrigatório para transport stdio")
		}

		cmd := exec.CommandContext(m.ctx, cfg.Command, cfg.Args...)

		// Herda ambiente do sistema e adiciona vars extras do config
		if len(cfg.Env) > 0 {
			cmd.Env = buildEnv(cfg.Env)
		}

		return &mcpsdk.CommandTransport{Command: cmd}, nil

	case TransportSSE:
		if cfg.URL == "" {
			return nil, fmt.Errorf("campo 'url' é obrigatório para transport sse")
		}

		return &mcpsdk.SSEClientTransport{
			Endpoint: cfg.URL,
		}, nil

	default:
		return nil, fmt.Errorf("transport desconhecido: '%s' (use 'stdio' ou 'sse')", cfg.Transport)
	}
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
