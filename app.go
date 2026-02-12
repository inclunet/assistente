package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/confirmation"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/messaging/signal"
	"assistente/internal/messaging/telegram"
	msgtool "assistente/internal/tools/messaging"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/speech"
	"assistente/internal/terminal"
	"assistente/internal/tools"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/shell"
	"assistente/internal/tools/web"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx                   context.Context
	llmClient             *llm.SyncClient
	speechManager         *speech.SpeechManager
	hotkeyManager         *hotkey.Manager
	profileManager        *profiles.Manager
	toolRegistry          *tools.Registry       // Registro de ferramentas disponíveis
	toolExecutor          *tools.Executor       // Executor de ferramentas com paralelismo e timeout
	terminalMgr           *terminal.Manager     // Gerenciador de sessões PTY (pool compartilhado LLM + usuário)
	confirmationMgr       *confirmation.Manager // Gerenciador de confirmações de comandos
	allowlistMgr          *allowlist.Manager    // Gerenciador de allowlists de comandos
	mcpMgr                *mcpmgr.Manager       // Gerenciador de servidores MCP
	skillMgr              *skills.Manager       // Gerenciador de skills
	responseNotifier      *messaging.ResponseNotifier // Notificador de respostas para mensageiros
	msgGateway            *messaging.Gateway          // Gateway de mensageria (Telegram, etc.)
	voiceHotkeyID         int
	currentConversationID uint // ID da conversa atual

	// Throttle para hotkeys - evita disparo repetido quando segura a tecla
	hotkeyLastFired  map[uint]time.Time
	hotkeyThrottleMs int64 // tempo mínimo entre disparos (em ms)
}

// ==================== Tipos para Threads ====================

// EnrichedMessage é ChatMessage + campos derivados calculados no backend
type EnrichedMessage struct {
	ID               string    `json:"id"`
	ConversationID   uint      `json:"conversationId"`
	ParentID         *string   `json:"parentId,omitempty"`
	TurnID           *uint     `json:"turnId,omitempty"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`
	Media            string    `json:"media,omitempty"`
	ToolCalls        string    `json:"toolCalls,omitempty"`
	ToolCallID       string    `json:"toolCallId,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	Source           string    `json:"source,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	Timestamp        int64     `json:"timestamp"`
	IsStreaming      bool      `json:"isStreaming"`
	Internal         bool      `json:"internal"`
}

// MessageNode representa uma mensagem com seus filhos na hierarquia
type MessageNode struct {
	Message    EnrichedMessage `json:"message"`
	Children   []MessageNode   `json:"children,omitempty"`
	Level      int             `json:"level"`
	ChildCount int             `json:"childCount"`
}

// ConversationWithThreads representa uma conversa com mensagens organizadas em árvore
type ConversationWithThreads struct {
	ID      uint          `json:"id"`
	Title   string        `json:"title"`
	Threads []MessageNode `json:"threads"`
}

// StreamEvent representa um evento de streaming simplificado
type StreamEvent struct {
	MessageID      uint   `json:"messageId"`
	ConversationId uint   `json:"conversationId"`
	Content        string `json:"content"`
	Done           bool   `json:"done"`
	FullResponse   string `json:"fullResponse,omitempty"`
	Error          string `json:"error,omitempty"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		hotkeyLastFired:  make(map[uint]time.Time),
		hotkeyThrottleMs: 1000,
		profileManager:   profiles.NewManager(),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Inicializa o banco de dados
	if err := InitDatabase(); err != nil {
		log.Printf("Erro ao inicializar banco de dados: %v", err)
	}

	// Garante perfis padrão em ~/.assistente/profiles/
	if err := a.profileManager.EnsureDefaults(); err != nil {
		log.Printf("Erro ao criar perfis padrão: %v", err)
	}

	// Inicializa o cliente LLM
	a.initLLMClient()

	// Inicializa managers de terminal, confirmação e allowlists
	a.initTerminalAndAllowlists()

	// Inicializa o registro de ferramentas (tool calling)
	a.initToolRegistry()

	// Inicializa o gerenciador de skills
	a.initSkills()

	// Garante que o diretório de memória existe no home
	a.initMemoryDir()

	// Inicializa o gerenciador de servidores MCP (após tool registry)
	a.initMCP()

	// Inicializa o gateway de mensageria (Telegram, etc.)
	a.initMessaging()

	// Inicializa hotkeys globais
	a.initGlobalHotkeys()

	// Registra hotkeys do perfil ativo
	a.registerActiveProfileHotkeys()
}

// initLLMClient inicializa o cliente LLM
func (a *App) initLLMClient() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Erro ao carregar config para LLM: %v", err)
		return
	}

	llm.ConfigureResponseTimeout(cfg.GetResponseTimeout())
	log.Printf("HTTP Response Timeout configurado para %d segundos", cfg.GetResponseTimeout())

	if cfg.APIKey == "" {
		log.Printf("API Key não configurada")
		return
	}

	a.llmClient = llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)
	log.Printf("LLM Client inicializado")
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
}

// initTerminalAndAllowlists inicializa os managers de terminal, confirmação e allowlists.
func (a *App) initTerminalAndAllowlists() {
	// Callback para emitir eventos Wails a partir dos managers
	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)
	}

	// Terminal Manager (pool compartilhado LLM + usuário)
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), emitEvent)

	// Confirmation Manager (confirmação de comandos)
	a.confirmationMgr = confirmation.NewManager(emitEvent)

	// Allowlist Manager (CRUD de allowlists)
	a.allowlistMgr = allowlist.NewManager()
	if err := a.allowlistMgr.EnsureDefaults(); err != nil {
		log.Printf("[Allowlist] Erro ao garantir allowlist padrão: %v", err)
	}

	log.Printf("[Terminal] Managers de terminal, confirmação e allowlist inicializados")
}

// initMCP inicializa o gerenciador de servidores MCP.
// Deve ser chamado após initToolRegistry (precisa do registry para registrar tools MCP).
func (a *App) initMCP() {
	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)
	}

	a.mcpMgr = mcpmgr.NewManager(a.toolRegistry, emitEvent)

	// Carrega configs e auto-conecta servidores habilitados
	if err := a.mcpMgr.LoadConfigs(); err != nil {
		log.Printf("[MCP] Erro ao carregar configurações: %v", err)
	}

	log.Printf("[MCP] Manager inicializado")
}

// initMessaging inicializa o gateway de mensageria (Telegram, futuramente Signal/WhatsApp).
func (a *App) initMessaging() {
	// ResponseNotifier — permite ao gateway capturar respostas para reenvio
	a.responseNotifier = messaging.NewResponseNotifier()

	// Carrega configuração de mensageria do config.json
	cfg, err := config.Load()
	if err != nil {
		log.Printf("[Messaging] Erro ao carregar config: %v", err)
		return
	}
	msgConfig := cfg.Messaging
	if msgConfig == nil {
		log.Printf("[Messaging] Nenhum mensageiro configurado (messaging ausente no config.json)")
		msgConfig = &config.MessagingConfig{} // vazio para evitar nil
	}

	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)
	}

	// Cria o gateway com referência para SendMessageFromChannel
	a.msgGateway = messaging.NewGateway(
		a.responseNotifier,
		msgConfig,
		a.SendMessageFromChannel,
		emitEvent,
	)

	// Telegram
	if msgConfig.Telegram != nil && msgConfig.Telegram.Enabled && msgConfig.Telegram.BotToken != "" {
		adapter := telegram.NewAdapter(msgConfig.Telegram.BotToken)
		a.msgGateway.Register("telegram", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Telegram: %v", err)
			}
		}()
		log.Printf("[Messaging] Telegram habilitado")
	} else {
		log.Printf("[Messaging] Telegram não configurado ou desabilitado")
	}

	// Signal (via signal-cli-rest-api HTTP + WebSocket)
	if msgConfig.Signal != nil && msgConfig.Signal.Enabled && msgConfig.Signal.Account != "" && msgConfig.Signal.APIURL != "" {
		adapter := signal.NewAdapter(msgConfig.Signal.APIURL, msgConfig.Signal.Account)
		a.msgGateway.Register("signal", adapter)
		go func() {
			if err := adapter.Connect(a.ctx); err != nil {
				log.Printf("[Messaging] Erro ao conectar Signal: %v", err)
			}
		}()
		log.Printf("[Messaging] Signal habilitado (api=%s, account=%s)", msgConfig.Signal.APIURL, msgConfig.Signal.Account)
	} else {
		log.Printf("[Messaging] Signal não configurado ou desabilitado")
	}

	// Registra a tool send_message no registry de ferramentas
	if a.toolRegistry != nil {
		sendMsgTool := msgtool.NewSendMessageTool(a.msgGateway)
		a.toolRegistry.MustRegister(sendMsgTool)
		log.Printf("[Messaging] Tool 'send_message' registrada")
	}

	log.Printf("[Messaging] Gateway inicializado")
}

// GetMessagingStatus retorna o status de todos os mensageiros conectados.
func (a *App) GetMessagingStatus() map[string]string {
	if a.msgGateway == nil {
		return map[string]string{}
	}
	status := a.msgGateway.GetStatus()
	result := make(map[string]string, len(status))
	for k, v := range status {
		result[k] = string(v)
	}
	return result
}

// GetMessagingConfig retorna a configuração de mensageria atual.
func (a *App) GetMessagingConfig() (*config.MessagingConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Messaging == nil {
		return &config.MessagingConfig{}, nil
	}
	return cfg.Messaging, nil
}

// SaveMessagingConfig salva a configuração de mensageria dentro do config.json.
func (a *App) SaveMessagingConfig(msgCfg *config.MessagingConfig) error {
	return config.Update(func(c *config.Config) *config.Config {
		c.Messaging = msgCfg
		return c
	})
}

// initToolRegistry inicializa o registro de ferramentas disponíveis
func (a *App) initToolRegistry() {
	a.toolRegistry = tools.NewRegistry()
	a.toolExecutor = tools.NewExecutor(a.toolRegistry, tools.DefaultExecutorConfig())

	// Determina diretório de trabalho para as tools de filesystem
	workDir, err := os.Getwd()
	if err != nil {
		log.Printf("[Tools] Erro ao obter diretório de trabalho: %v", err)
		workDir = "."
	}

	// Registra ferramentas de filesystem
	a.toolRegistry.MustRegister(filesystem.NewReadFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewListDirectory(workDir))
	a.toolRegistry.MustRegister(filesystem.NewSearchFiles(workDir))
	a.toolRegistry.MustRegister(filesystem.NewGrepSearch(workDir))
	a.toolRegistry.MustRegister(filesystem.NewWriteFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewEditFile(workDir))

	// Registra ferramentas web
	a.toolRegistry.MustRegister(web.NewWebFetch())
	a.toolRegistry.MustRegister(web.NewWebSearch())

	// Registra ferramenta de shell (run_command)
	confirmFn := func(ctx context.Context, cmd, wd string) (bool, error) {
		return a.confirmationMgr.RequestConfirmation(ctx, cmd, wd)
	}
	getAllowlistFn := func() *allowlist.Allowlist {
		activeProfile, err := a.profileManager.GetActive()
		if err != nil || activeProfile == nil {
			// Sem perfil ativo: usa allowlist padrão
			al, err := a.allowlistMgr.Get("padrao")
			if err != nil {
				return nil // sem allowlist = tudo requer confirmação
			}
			return al
		}
		if activeProfile.Chat.CommandAllowlist == "" {
			// Perfil sem allowlist configurada: usa a padrão
			al, err := a.allowlistMgr.Get("padrao")
			if err != nil {
				return nil
			}
			return al
		}
		al, err := a.allowlistMgr.Get(activeProfile.Chat.CommandAllowlist)
		if err != nil {
			log.Printf("[Tools] Allowlist '%s' não encontrada, usando confirmação para tudo", activeProfile.Chat.CommandAllowlist)
			return nil
		}
		return al
	}
	a.toolRegistry.MustRegister(shell.NewRunCommand(a.terminalMgr, confirmFn, getAllowlistFn, workDir))

	log.Printf("[Tools] Registry inicializado com %d ferramentas: %v", a.toolRegistry.Count(), a.toolRegistry.Names())
}

// ToolInfo é um resumo de uma ferramenta para listagem no frontend.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetAvailableTools retorna a lista de ferramentas registradas no registry.
// Usado pelo frontend para exibir checkboxes no editor de perfis.
func (a *App) GetAvailableTools() []ToolInfo {
	if a.toolRegistry == nil {
		return []ToolInfo{}
	}

	allTools := a.toolRegistry.All()
	result := make([]ToolInfo, len(allTools))
	for i, t := range allTools {
		result[i] = ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
		}
	}
	return result
}

// shutdown é chamado quando o app fecha
func (a *App) shutdown(ctx context.Context) {
	if a.hotkeyManager != nil {
		a.hotkeyManager.Stop()
	}

	// Encerra todos os servidores MCP
	if a.mcpMgr != nil {
		a.mcpMgr.CloseAll()
	}

	// Encerra todas as sessões de terminal
	if a.terminalMgr != nil {
		a.terminalMgr.CloseAll()
	}

	// Encerra todos os mensageiros
	if a.msgGateway != nil {
		a.msgGateway.Shutdown()
	}
}

// ============================================================================
// Terminal Management API (sessões PTY compartilhadas LLM + usuário)
// ============================================================================

// ListTerminalSessions retorna todas as sessões de terminal ativas.
func (a *App) ListTerminalSessions() []terminal.SessionInfo {
	if a.terminalMgr == nil {
		return []terminal.SessionInfo{}
	}
	return a.terminalMgr.List()
}

// CreateTerminalSession cria uma nova sessão de terminal.
func (a *App) CreateTerminalSession(name string) (*terminal.SessionInfo, error) {
	if a.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}

	workDir, _ := os.Getwd()
	session, err := a.terminalMgr.Create(name, workDir)
	if err != nil {
		return nil, err
	}

	info := session.Info()
	return &info, nil
}

// CloseTerminalSession encerra uma sessão de terminal.
func (a *App) CloseTerminalSession(sessionID string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.Close(sessionID)
}

// GetTerminalHistory retorna o histórico de comandos de uma sessão.
func (a *App) GetTerminalHistory(sessionID string) ([]terminal.HistoryEntry, error) {
	if a.terminalMgr == nil {
		return nil, fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.GetHistory(sessionID)
}

// RunTerminalCommand executa um comando com markers em uma sessão de terminal.
// Mantido para compatibilidade — usado internamente pelo LLM.
func (a *App) RunTerminalCommand(sessionID string, command string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}

	// Executa em goroutine para não bloquear o binding
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		_, err := a.terminalMgr.RunCommand(ctx, sessionID, command, 0, "user")
		if err != nil {
			log.Printf("[Terminal] Erro ao executar comando: %v", err)
		}
	}()

	return nil
}

// SendTerminalInput envia input raw para uma sessão de terminal (modo interativo).
// Diferente de RunTerminalCommand, não usa markers — o input vai direto ao PTY.
// Suporta comandos interativos (wsl, python, ssh, etc.) e input para programas em execução.
func (a *App) SendTerminalInput(sessionID string, input string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}

	_, err := a.terminalMgr.SendInput(sessionID, input)
	if err != nil {
		log.Printf("[Terminal] Erro ao enviar input: %v", err)
		return err
	}
	return nil
}

// InterruptTerminalCommand envia Ctrl+C para uma sessão de terminal.
func (a *App) InterruptTerminalCommand(sessionID string) error {
	if a.terminalMgr == nil {
		return fmt.Errorf("terminal manager não inicializado")
	}
	return a.terminalMgr.Interrupt(sessionID)
}

// GetTerminalStats retorna estatísticas do gerenciador de terminais.
func (a *App) GetTerminalStats() *terminal.ManagerStats {
	if a.terminalMgr == nil {
		return &terminal.ManagerStats{}
	}
	stats := a.terminalMgr.Stats()
	return &stats
}

// ============================================================================
// Command Confirmation API
// ============================================================================

// RespondCommandConfirmation responde a uma solicitação de confirmação de comando.
// Chamado pelo frontend quando o usuário aprova ou nega um comando.
func (a *App) RespondCommandConfirmation(requestID string, approved bool) error {
	if a.confirmationMgr == nil {
		return fmt.Errorf("confirmation manager não inicializado")
	}
	return a.confirmationMgr.Respond(requestID, approved)
}

// ============================================================================
// Allowlist Management API
// ============================================================================

// GetAllowlists retorna a lista de allowlists disponíveis.
func (a *App) GetAllowlists() ([]allowlist.AllowlistInfo, error) {
	if a.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.List()
}

// GetAllowlist retorna uma allowlist pelo slug.
func (a *App) GetAllowlist(slug string) (*allowlist.Allowlist, error) {
	if a.allowlistMgr == nil {
		return nil, fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Get(slug)
}

// CreateAllowlist cria uma nova allowlist.
func (a *App) CreateAllowlist(al allowlist.Allowlist) (string, error) {
	if a.allowlistMgr == nil {
		return "", fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Create(&al)
}

// UpdateAllowlist atualiza uma allowlist existente.
func (a *App) UpdateAllowlist(slug string, al allowlist.Allowlist) error {
	if a.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Update(slug, &al)
}

// DeleteAllowlist exclui uma allowlist.
func (a *App) DeleteAllowlist(slug string) error {
	if a.allowlistMgr == nil {
		return fmt.Errorf("allowlist manager não inicializado")
	}
	return a.allowlistMgr.Delete(slug)
}

// GetAllowlistSearchPaths retorna os caminhos de busca de allowlists.
func (a *App) GetAllowlistSearchPaths() []string {
	if a.allowlistMgr == nil {
		return []string{}
	}
	return a.allowlistMgr.GetSearchPaths()
}

// ============================================================================
// Skills Management API
// ============================================================================

// initSkills inicializa o gerenciador de skills
func (a *App) initSkills() {
	a.skillMgr = skills.NewManager()
	if err := a.skillMgr.EnsureDir(); err != nil {
		log.Printf("[Skills] Erro ao garantir diretório de skills: %v", err)
	}

	list, err := a.skillMgr.List()
	if err != nil {
		log.Printf("[Skills] Erro ao listar skills: %v", err)
	} else {
		log.Printf("[Skills] Manager inicializado com %d skills", len(list))
	}
}

// initMemoryDir garante que o diretório memory/ existe no home (~/.assistente/memory/)
// e cria o arquivo memory.md inicial se não existir.
func (a *App) initMemoryDir() {
	resolver := configdir.NewResolver("memory")

	if err := resolver.EnsureHomeDir(); err != nil {
		log.Printf("[Memory] Erro ao criar diretório de memória: %v", err)
		return
	}

	// Cria memory.md se não existir em nenhum diretório
	if !resolver.Exists("memory.md") {
		initial := []byte("## Sobre o Usuário\n\n(Ainda não há memórias salvas. Quando o usuário compartilhar informações pessoais ou pedir para lembrar algo, registre aqui.)\n")
		if err := resolver.Create("memory.md", initial); err != nil {
			log.Printf("[Memory] Erro ao criar memory.md: %v", err)
		} else {
			log.Printf("[Memory] memory.md criado em ~/.assistente/memory/")
		}
	} else {
		log.Printf("[Memory] memory.md encontrado")
	}

	// Garante que os subdiretórios de memória temporal existem no home
	homeDir := resolver.GetHomeDir()
	if homeDir != "" {
		for _, sub := range []string{"daily", "weekly", "monthly", "yearly"} {
			subPath := homeDir + string(os.PathSeparator) + sub
			if err := os.MkdirAll(subPath, 0755); err != nil {
				log.Printf("[Memory] Erro ao criar %s: %v", sub, err)
			}
		}
	}
}

// GetSkills retorna a lista de skills disponíveis (metadados apenas).
func (a *App) GetSkills() ([]skills.SkillInfo, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.List()
}

// GetSkill retorna um skill completo pelo slug.
func (a *App) GetSkill(slug string) (*skills.Skill, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.Get(slug)
}

// SkillCreateRequest é o payload para criar/atualizar um skill via frontend.
// Contém a SkillMetadata completa conforme spec + conteúdo Markdown.
type SkillCreateRequest struct {
	skills.SkillMetadata `json:",inline"`
	Content              string `json:"content"`
}

// CreateSkill cria um novo skill.
func (a *App) CreateSkill(req SkillCreateRequest) (string, error) {
	if a.skillMgr == nil {
		return "", fmt.Errorf("skill manager não inicializado")
	}

	meta := req.SkillMetadata
	slug, err := a.skillMgr.Create(&meta, req.Content)
	if err != nil {
		return "", err
	}

	runtime.EventsEmit(a.ctx, "skill:created", map[string]interface{}{
		"slug": slug,
		"name": req.Name,
	})

	return slug, nil
}

// UpdateSkill atualiza um skill existente.
func (a *App) UpdateSkill(slug string, req SkillCreateRequest) error {
	if a.skillMgr == nil {
		return fmt.Errorf("skill manager não inicializado")
	}

	meta := req.SkillMetadata
	if err := a.skillMgr.Update(slug, &meta, req.Content); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "skill:updated", map[string]interface{}{
		"slug": slug,
		"name": req.Name,
	})

	return nil
}

// DeleteSkill exclui um skill.
func (a *App) DeleteSkill(slug string) error {
	if a.skillMgr == nil {
		return fmt.Errorf("skill manager não inicializado")
	}

	if err := a.skillMgr.Delete(slug); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "skill:deleted", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// GetUserInvocableSkills retorna skills que o usuário pode invocar via /slash.
func (a *App) GetUserInvocableSkills() ([]skills.SkillInfo, error) {
	if a.skillMgr == nil {
		return nil, fmt.Errorf("skill manager não inicializado")
	}
	return a.skillMgr.GetUserInvocableSkills()
}

// GetSkillSearchPaths retorna os caminhos de busca de skills.
func (a *App) GetSkillSearchPaths() []string {
	if a.skillMgr == nil {
		return []string{}
	}
	return a.skillMgr.GetSearchPaths()
}

// ============================================================================
// MCP Server Management API
// ============================================================================

// ListMCPServers retorna informações de todos os servidores MCP configurados.
func (a *App) ListMCPServers() []mcpmgr.ServerInfo {
	if a.mcpMgr == nil {
		return []mcpmgr.ServerInfo{}
	}
	return a.mcpMgr.List()
}

// ConnectMCPServer conecta a um servidor MCP pelo slug.
func (a *App) ConnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Connect(slug)
}

// DisconnectMCPServer desconecta de um servidor MCP.
func (a *App) DisconnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Disconnect(slug)
}

// ReconnectMCPServer reconecta a um servidor MCP.
func (a *App) ReconnectMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.Reconnect(slug)
}

// SaveMCPServer salva (cria ou atualiza) a configuração de um servidor MCP.
func (a *App) SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SaveConfig(slug, cfg)
}

// DeleteMCPServer remove a configuração de um servidor MCP.
func (a *App) DeleteMCPServer(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DeleteConfig(slug)
}

// GetMCPServerTools retorna as ferramentas de um servidor MCP específico.
func (a *App) GetMCPServerTools(slug string) []mcpmgr.MCPToolInfo {
	if a.mcpMgr == nil {
		return []mcpmgr.MCPToolInfo{}
	}
	return a.mcpMgr.GetTools(slug)
}

// GetMCPServerConfig retorna a configuração de um servidor MCP.
func (a *App) GetMCPServerConfig(slug string) (*mcpmgr.ServerConfig, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.GetConfig(slug)
}

// initGlobalHotkeys inicializa o gerenciador de hotkeys
func (a *App) initGlobalHotkeys() {
	if !hotkey.IsSupported() {
		log.Println("[Hotkey] Hotkeys globais não suportados neste sistema")
		return
	}

	a.hotkeyManager = hotkey.GetManager()
	log.Println("[Hotkey] Manager inicializado. Hotkeys serão registrados pelos triggers dos perfis.")
}

// registerActiveProfileHotkeys registra os hotkeys do perfil ativo
func (a *App) registerActiveProfileHotkeys() {
	if a.hotkeyManager == nil {
		return
	}

	activeProfile, err := a.profileManager.GetActive()
	if err != nil {
		log.Printf("[Hotkey] Erro ao obter perfil ativo: %v", err)
		return
	}

	// Remove todos os hotkeys anteriores
	a.hotkeyManager.UnregisterAllProfileHotkeys()

	if activeProfile == nil || len(activeProfile.Interaction.Triggers) == 0 {
		return
	}

	hotkeyCount := 0
	for _, trigger := range activeProfile.Interaction.Triggers {
		if !trigger.Enabled || trigger.Hotkey == "" {
			continue
		}
		hotkeyCount++

		t := trigger // Captura variável para closure

		log.Printf("[Hotkey] Registrando hotkey '%s' para trigger tipo %s...", t.Hotkey, t.Type)
		_, err := a.hotkeyManager.RegisterProfileHotkey(
			1, // Profile ID fixo (perfil global)
			t.Hotkey,
			t.Type == profiles.TriggerTypeHotkey,
			t.HotkeyBringToFront,
			func() {
				// Throttle: ignora se disparou recentemente
				now := time.Now()
				triggerKey := uint(hotkeyCount) // Usa index como key
				if lastFired, ok := a.hotkeyLastFired[triggerKey]; ok {
					elapsed := now.Sub(lastFired).Milliseconds()
					if elapsed < a.hotkeyThrottleMs {
						return
					}
				}
				a.hotkeyLastFired[triggerKey] = now

				log.Printf("[Hotkey] HOTKEY ACIONADA! Trigger tipo %s", t.Type)
				runtime.EventsEmit(a.ctx, "interaction:hotkey:triggered", map[string]interface{}{
					"triggerType":  t.Type,
					"bringToFront": t.HotkeyBringToFront,
				})

				if t.HotkeyGlobal && t.HotkeyBringToFront {
					runtime.WindowShow(a.ctx)
				}
			},
		)
		if err != nil {
			log.Printf("[Hotkey] ERRO ao registrar hotkey '%s': %v", t.Hotkey, err)
		} else {
			log.Printf("[Hotkey] Hotkey '%s' registrada com sucesso", t.Hotkey)
		}
	}

	log.Printf("[Hotkey] Total: %d hotkeys registradas para perfil ativo", hotkeyCount)
}

// ============================================================================
// Global Hotkey API
// ============================================================================

// HotkeyInfo informações sobre um hotkey
type HotkeyInfo struct {
	ID          int    `json:"id"`
	Modifiers   string `json:"modifiers"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados
func (a *App) IsGlobalHotkeySupported() bool {
	return hotkey.IsSupported()
}

// ============================================================================
// SAPI5 Voice Methods (Windows only)
// ============================================================================

// SAPI5VoiceInfo representa informações de uma voz SAPI5 para o frontend
type SAPI5VoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Age         string `json:"age"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// GetSAPI5Voices retorna a lista de vozes SAPI5 instaladas
func (a *App) GetSAPI5Voices() ([]SAPI5VoiceInfo, error) {
	manager := speech.GetSAPI5Manager()

	if err := manager.Initialize(); err != nil {
		log.Printf("SAPI5 Initialize error (may be expected on non-Windows): %v", err)
		return []SAPI5VoiceInfo{}, nil
	}

	voices := manager.GetVoices()
	result := make([]SAPI5VoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = SAPI5VoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Language:    v.Language,
			Gender:      v.Gender,
			Age:         v.Age,
			Vendor:      v.Vendor,
			Description: v.Description,
			Source:      v.Source,
		}
	}

	return result, nil
}

// SpeakSAPI5 sintetiza texto usando uma voz SAPI5
func (a *App) SpeakSAPI5(text string, voiceName string) error {
	manager := speech.GetSAPI5Manager()
	return manager.Speak(text, voiceName)
}

// StopSAPI5 para a síntese SAPI5 atual
func (a *App) StopSAPI5() error {
	manager := speech.GetSAPI5Manager()
	return manager.Stop()
}

// SetSAPI5Volume define o volume (0-100)
func (a *App) SetSAPI5Volume(volume int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetVolume(volume)
}

// SetSAPI5Rate define a velocidade (-10 a 10, 0 é normal)
func (a *App) SetSAPI5Rate(rate int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetRate(rate)
}

// IsSAPI5Speaking verifica se está falando
func (a *App) IsSAPI5Speaking() bool {
	manager := speech.GetSAPI5Manager()
	return manager.IsSpeaking()
}

// ============================================================================
// OpenAI Speech API Methods (Whisper STT + OpenAI TTS)
// ============================================================================

// OpenAITTSVoiceInfo representa uma voz OpenAI TTS para o frontend
type OpenAITTSVoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Gender      string `json:"gender"`
	Provider    string `json:"provider"`
}

// TranscriptionResultInfo resultado da transcrição para o frontend
type TranscriptionResultInfo struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Provider string  `json:"provider"`
}

// SynthesisResultInfo resultado da síntese para o frontend
type SynthesisResultInfo struct {
	AudioBase64 string `json:"audioBase64"`
	Format      string `json:"format"`
	Provider    string `json:"provider"`
}

// InitSpeechManager inicializa o gerenciador de speech com as configurações
func (a *App) InitSpeechManager(apiKey, apiBaseURL, whisperLanguage, ttsVoice, ttsModel string) error {
	config := speech.SpeechConfig{
		STTProvider:      speech.STTProviderWhisper,
		TTSProvider:      speech.TTSProviderOpenAI,
		OpenAIAPIKey:     apiKey,
		OpenAIAPIBaseURL: apiBaseURL,
		WhisperModel:     "whisper-1",
		WhisperLanguage:  whisperLanguage,
		TTSModel:         ttsModel,
		TTSVoice:         ttsVoice,
	}

	a.speechManager = speech.NewSpeechManager(config)
	log.Printf("Speech Manager inicializado (STT: whisper, TTS: openai)")
	return nil
}

// TranscribeWhisper transcreve áudio usando OpenAI Whisper
func (a *App) TranscribeWhisper(audioBase64 string, filename string) (*TranscriptionResultInfo, error) {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
	}

	result, err := a.speechManager.Transcribe(audioBase64, filename)
	if err != nil {
		return nil, err
	}

	return &TranscriptionResultInfo{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
		Provider: result.Provider,
	}, nil
}

// SynthesizeOpenAI sintetiza texto usando OpenAI TTS
func (a *App) SynthesizeOpenAI(text string) (*SynthesisResultInfo, error) {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
	}

	result, err := a.speechManager.Synthesize(text)
	if err != nil {
		return nil, err
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// SynthesizeOpenAIWithVoice sintetiza texto usando OpenAI TTS com uma voz específica
func (a *App) SynthesizeOpenAIWithVoice(text string, voice string) (*SynthesisResultInfo, error) {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voice, "tts-1")
	}

	result, err := a.speechManager.SynthesizeWithVoice(text, voice)
	if err != nil {
		return nil, err
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// TTSStreamEvent evento de streaming de TTS
type TTSStreamEvent struct {
	SessionID   string `json:"sessionId"`
	ChunkBase64 string `json:"chunkBase64"`
	Format      string `json:"format"`
	Done        bool   `json:"done"`
	Error       string `json:"error"`
}

// SynthesizeOpenAIStream sintetiza texto usando OpenAI TTS com streaming
func (a *App) SynthesizeOpenAIStream(text string, voice string, sessionID string) error {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     "speech manager not initialized",
			})
			return fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     "API key not configured",
			})
			return fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voice, "tts-1")
	}

	if !a.speechManager.SupportsStreaming() {
		go func() {
			result, err := a.speechManager.SynthesizeWithVoice(text, voice)
			if err != nil {
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
				return
			}

			runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
				SessionID: sessionID,
				Format:    result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
				SessionID:   sessionID,
				ChunkBase64: result.AudioBase64,
				Format:      result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
				SessionID: sessionID,
				Done:      true,
			})
		}()
		return nil
	}

	go func() {
		runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
			SessionID: sessionID,
			Format:    "mp3",
		})

		callbacks := speech.StreamCallbacks{
			OnChunk: func(chunkBase64 string) {
				runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
					SessionID:   sessionID,
					ChunkBase64: chunkBase64,
					Format:      "mp3",
				})
			},
			OnDone: func() {
				runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
					SessionID: sessionID,
					Done:      true,
				})
			},
			OnError: func(err error) {
				log.Printf("[TTS] Stream error: %v", err)
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := a.speechManager.SynthesizeStream(ctx, text, voice, callbacks)
		if err != nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     err.Error(),
			})
		}
	}()

	return nil
}

// GetOpenAITTSVoices retorna as vozes disponíveis do OpenAI TTS
func (a *App) GetOpenAITTSVoices() []OpenAITTSVoiceInfo {
	voices := speech.GetAvailableVoices()
	result := make([]OpenAITTSVoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = OpenAITTSVoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Description: v.Description,
			Gender:      v.Gender,
			Provider:    v.Provider,
		}
	}

	return result
}

// SetOpenAITTSVoice altera a voz do OpenAI TTS
func (a *App) SetOpenAITTSVoice(voice string) {
	if a.speechManager != nil {
		a.speechManager.SetTTSVoice(voice)
	}
}

// SetOpenAITTSSpeed altera a velocidade do OpenAI TTS
func (a *App) SetOpenAITTSSpeed(rate int) {
	if a.speechManager != nil {
		a.speechManager.SetTTSSpeed(rate)
	}
}

// ============================================================================
// Unified Profile API (arquivo JSON via configdir)
// ============================================================================

// GetProfiles retorna todos os perfis disponíveis
func (a *App) GetProfiles() ([]profiles.ProfileInfo, error) {
	return a.profileManager.List()
}

// GetProfile retorna um perfil pelo slug
func (a *App) GetProfile(slug string) (*profiles.Profile, error) {
	return a.profileManager.Get(slug)
}

// GetActiveProfile retorna o perfil ativo global
func (a *App) GetActiveProfile() (*profiles.Profile, error) {
	return a.profileManager.GetActive()
}

// GetActiveProfileSlug retorna o slug do perfil ativo
func (a *App) GetActiveProfileSlug() string {
	return a.profileManager.GetActiveSlug()
}

// SetActiveProfile define o perfil ativo e re-registra hotkeys
func (a *App) SetActiveProfile(slug string) error {
	if err := a.profileManager.SetActive(slug); err != nil {
		return err
	}

	// Re-registra hotkeys do novo perfil
	a.registerActiveProfileHotkeys()

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "profile:changed", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// CreateProfile cria um novo perfil
func (a *App) CreateProfile(profile profiles.Profile) (string, error) {
	slug, err := a.profileManager.Create(&profile)
	if err != nil {
		return "", err
	}

	runtime.EventsEmit(a.ctx, "profile:created", map[string]interface{}{
		"slug": slug,
		"name": profile.Name,
	})

	return slug, nil
}

// UpdateProfile atualiza um perfil existente
func (a *App) UpdateProfile(slug string, profile profiles.Profile) error {
	if err := a.profileManager.Update(slug, &profile); err != nil {
		return err
	}

	// Se for o perfil ativo, re-registra hotkeys
	if slug == a.profileManager.GetActiveSlug() {
		a.registerActiveProfileHotkeys()
	}

	runtime.EventsEmit(a.ctx, "profile:updated", map[string]interface{}{
		"slug": slug,
		"name": profile.Name,
	})

	return nil
}

// DeleteProfile deleta um perfil
func (a *App) DeleteProfile(slug string) error {
	// Não permite deletar o perfil ativo
	if slug == a.profileManager.GetActiveSlug() {
		return fmt.Errorf("não é possível deletar o perfil ativo")
	}

	if err := a.profileManager.Delete(slug); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "profile:deleted", map[string]interface{}{
		"slug": slug,
	})

	return nil
}

// GetProfileSearchPaths retorna os caminhos de busca dos perfis
func (a *App) GetProfileSearchPaths() []string {
	return a.profileManager.GetSearchPaths()
}

// PreviewVoiceSettings reproduz um texto de teste com configurações ad-hoc
func (a *App) PreviewVoiceSettings(provider, voiceID string, rate, pitch, volume float64, sampleText string) error {
	if sampleText == "" {
		sampleText = "Este é um teste das configurações de voz"
	}

	log.Printf("[PreviewVoiceSettings] provider=%s, voiceID=%s, rate=%.2f", provider, voiceID, rate)

	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("erro ao carregar config: %w", err)
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("API key não configurada")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voiceID, "tts-1")
	}

	if provider == "openai" {
		a.speechManager.SetTTSVoice(voiceID)
	}

	result, err := a.speechManager.SynthesizeWithVoice(sampleText, voiceID)
	if err != nil {
		return fmt.Errorf("erro ao sintetizar: %w", err)
	}

	runtime.EventsEmit(a.ctx, "voice_profile:preview", map[string]interface{}{
		"audio_base64": result.AudioBase64,
		"format":       result.Format,
	})

	return nil
}

// ==================== Chat Tabs ====================

// GetAllTabs retorna todas as abas de chat
func (a *App) GetAllTabs() ([]database.ChatTab, error) {
	return database.GetAllTabs()
}

// GetActiveTab retorna a aba ativa
func (a *App) GetActiveTab() (*database.ChatTab, error) {
	return database.GetActiveTab()
}

// CreateTab cria uma nova aba de chat
func (a *App) CreateTab(title, icon string, setAsActive bool) (*database.ChatTab, error) {
	tab, err := database.CreateTab(title, icon, setAsActive)
	if err != nil {
		return nil, err
	}

	runtime.EventsEmit(a.ctx, "tab_created", map[string]interface{}{
		"id":       tab.ID,
		"title":    tab.Title,
		"icon":     tab.Icon,
		"position": tab.Position,
		"isActive": tab.IsActive,
	})

	return tab, nil
}

// CloseTab fecha uma aba
func (a *App) CloseTab(id uint) error {
	err := database.CloseTab(id)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "tab_closed", map[string]interface{}{
		"id": id,
	})

	return nil
}

// SetActiveTab define a aba ativa
func (a *App) SetActiveTab(id uint) error {
	err := database.SetActiveTab(id)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "tab_activated", map[string]interface{}{
		"id": id,
	})

	return nil
}

// UpdateTabTitle atualiza o título de uma aba e da conversa associada
func (a *App) UpdateTabTitle(id uint, title string) error {
	tab, err := database.GetTab(id)
	if err != nil {
		return err
	}

	err = database.UpdateTabTitle(id, title)
	if err != nil {
		return err
	}

	if tab.ConversationID != nil && *tab.ConversationID > 0 {
		runtime.EventsEmit(a.ctx, "conversation:renamed", map[string]interface{}{
			"conversation_id": *tab.ConversationID,
			"new_title":       title,
		})
	}

	return nil
}

// LoadConversationInTab carrega uma conversa em uma aba
func (a *App) LoadConversationInTab(tabId, conversationId uint) error {
	err := database.LoadConversationInTab(tabId, conversationId)
	if err != nil {
		return err
	}

	conv, err := database.GetConversation(conversationId)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "conversation_loaded_in_tab", map[string]interface{}{
		"tabId":          tabId,
		"conversationId": conv.ID,
		"title":          conv.Title,
	})

	return nil
}

// ClearTab limpa a conversa de uma aba
func (a *App) ClearTab(id uint) error {
	err := database.ClearTab(id)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "tab_cleared", map[string]interface{}{
		"id": id,
	})

	return nil
}

// ReorderTabs reordena as abas
func (a *App) ReorderTabs(orderedIds []uint) error {
	return database.ReorderTabs(orderedIds)
}
