package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/jobs"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
	"assistente/internal/skills"
	"assistente/internal/speech"
	"assistente/internal/terminal"
	"assistente/internal/tools"
	deeplinktool "assistente/internal/tools/deeplink"
	"assistente/internal/tools/editor"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/history"
	questiontool "assistente/internal/tools/questionnaire"
	"assistente/internal/tools/shell"
	tasklisttool "assistente/internal/tools/tasklist"
	"assistente/internal/tools/web"
	"assistente/internal/updater"
	"assistente/internal/workspace"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	// AppVersion é a versão do aplicativo
	// Será injetada automaticamente pelo Wails a partir de wails.json info.productVersion
	// Em dev, permanece como "dev"
	AppVersion = "dev"
)

// Request structs for LLM Provider Management
type CreateLLMProviderRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	APIFormat    string `json:"api_format,omitempty"`
}

type TestLLMProviderRequest struct {
	Type       string `json:"type"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
}

type UpdateLLMProviderRequest struct {
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	APIFormat    string `json:"api_format,omitempty"`
}

// App struct
type App struct {
	ctx                   context.Context
	llmRegistry           *llm.ProviderRegistry // Registro de provedores LLM
	speechManager         *speech.SpeechManager
	hotkeyManager         *hotkey.Manager
	profileManager        *profiles.Manager
	toolRegistry          *tools.Registry             // Registro de ferramentas disponíveis
	toolExecutor          *tools.Executor             // Executor de ferramentas com paralelismo e timeout
	terminalMgr           *terminal.Manager           // Gerenciador de sessões PTY (pool compartilhado LLM + usuário)
	questionnaireMgr      *questionnaire.Manager      // Gerenciador de questionários (coleta estruturada)
	allowlistMgr          *allowlist.Manager          // Gerenciador de allowlists de comandos
	mcpMgr                *mcpmgr.Manager             // Gerenciador de servidores MCP
	skillMgr              *skills.Manager             // Gerenciador de skills
	responseNotifier      *messaging.ResponseNotifier // Notificador de respostas para mensageiros
	msgGateway            *messaging.Gateway          // Gateway de mensageria (Telegram, etc.)
	updater               *updater.Updater            // Gerenciador de atualizações automáticas
	voiceHotkeyID         int
	currentConversationID uint // ID da conversa atual

	credMgr   *credentials.Manager
	credStore credentials.Store

	// Throttle para hotkeys - evita disparo repetido quando segura a tecla
	hotkeyLastFired  map[uint]time.Time
	hotkeyThrottleMs int64 // tempo mínimo entre disparos (em ms)

	// Watcher de arquivos do editor (mudanças externas)
	editorWatchMu    sync.Mutex
	editorDirWatches map[string]*editorDirWatch

	// Workspace manager (unified tabs)
	workspaceMgr *workspace.Manager

	// Jobs manager (event-driven automation)
	jobMgr *jobs.Manager
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
		llmRegistry:      llm.NewProviderRegistry(),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Inicializa o banco de dados
	if err := InitDatabase(); err != nil {
		log.Printf("Erro ao inicializar banco de dados: %v", err)
	}
	if err := a.cleanupEditorOrphanDraftsOnStartup(); err != nil {
		log.Printf("Erro ao limpar drafts órfãos do editor no startup: %v", err)
	}

	// Instala/atualiza perfis embutidos em ~/.assistente/profiles/
	a.installBuiltinProfiles()

	// Garante que o diretório de perfis existe
	if err := a.profileManager.EnsureDefaults(); err != nil {
		log.Printf("Erro ao garantir diretório de perfis: %v", err)
	}

	// Inicializa Credential Manager PRIMEIRO (antes de qualquer uso)
	a.initCredentialManager()

	// Inicializa os provedores LLM (Provider Registry) ANTES do client
	a.initLLMProviders()

	// Inicializa o cliente LLM (usa credMgr + registry já populado)
	a.initLLMClient()

	// Migra config.json legado para novo sistema (se necessário)
	a.migrateLegacyConfig()

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

	// Inicializa o sistema de jobs (event-driven automation)
	a.initJobs()

	// Inicializa o workspace manager
	a.initWorkspace()

	// Inicializa o updater
	a.initUpdater()

	// Verifica atualizações no startup (não bloqueante)
	go a.checkForUpdatesOnStartup()

	// Restaura foco da janela no startup (resolve bug do Wails no Windows)
	// Deixa 400ms para garantir que a janela está completamente pronta
	go func() {
		time.Sleep(400 * time.Millisecond)
		runtime.WindowShow(a.ctx)
		log.Printf("[App] WindowShow chamado após startup")
	}()
}

// initLLMClient inicializa o cliente LLM usando o provider do perfil ativo
func (a *App) initLLMClient() {
	activeProfile, err := a.profileManager.GetActive()
	if err != nil || activeProfile == nil {
		log.Printf("[initLLMClient] Perfil ativo não encontrado: %v", err)
		return
	}
	activeProfile = a.resolveProfileDefaults(activeProfile)

	provider := a.llmRegistry.Get(activeProfile.Chat.LLMProvider)
	if provider == nil {
		log.Printf("[initLLMClient] Provedor LLM não encontrado: %s", activeProfile.Chat.LLMProvider)
		return
	}

	log.Printf("[initLLMClient] Provedor ativo: %s (api_format=%s)", provider.Name, provider.GetAPIFormat())
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
}

// getChatProviderForProvider retorna um ChatProvider para o provedor especificado.
// Usa NewChatProvider que roteia para o SDK correto via GetAPIFormat()
// (default: OpenAI SDK, que é compatível com todos os provedores OpenAI-compat).
func (a *App) getChatProviderForProvider(providerID string) (llm.ChatProvider, error) {
	if a.llmRegistry == nil {
		return nil, fmt.Errorf("registro de provedores não inicializado")
	}

	provider := a.llmRegistry.Get(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provedor LLM não encontrado: %s", providerID)
	}

	return llm.NewChatProvider(provider, a.credMgr), nil
}

// resolveProfileDefaults substitui sentinelas "$default" no profile pelo provedor/modelo
// default do sistema. Retorna uma cópia modificada — não altera o profile em disco.
// Se não houver default configurado, mantém os valores originais inalterados.
func (a *App) resolveProfileDefaults(p *profiles.Profile) *profiles.Profile {
	if p == nil {
		return nil
	}

	needsResolve := p.Chat.LLMProvider == profiles.DefaultProviderSentinel ||
		p.Chat.Model == profiles.DefaultProviderSentinel ||
		p.Voice.LLMProviderID == profiles.DefaultProviderSentinel ||
		p.Interaction.LLMProviderID == profiles.DefaultProviderSentinel
	if !needsResolve {
		return p
	}

	defaultProvider, err := database.GetDefaultProvider()
	if err != nil || defaultProvider == nil {
		log.Printf("[ResolveDefaults] Nenhum provedor default encontrado: %v", err)
		return p
	}

	resolved := *p
	resolved.Chat = p.Chat
	resolved.Voice = p.Voice
	resolved.Interaction = p.Interaction

	if resolved.Chat.LLMProvider == profiles.DefaultProviderSentinel {
		resolved.Chat.LLMProvider = defaultProvider.ID
	}
	if resolved.Chat.Model == profiles.DefaultProviderSentinel {
		resolved.Chat.Model = defaultProvider.DefaultModel
	}
	if resolved.Voice.LLMProviderID == profiles.DefaultProviderSentinel {
		resolved.Voice.LLMProviderID = defaultProvider.ID
	}
	if resolved.Interaction.LLMProviderID == profiles.DefaultProviderSentinel {
		resolved.Interaction.LLMProviderID = defaultProvider.ID
	}

	log.Printf("[ResolveDefaults] Resolvido $default → provider=%s, model=%s", defaultProvider.ID, defaultProvider.DefaultModel)
	return &resolved
}

// initLLMProviders inicializa o registro de provedores LLM com os provedores padrão
func (a *App) initLLMProviders() {
	// Tentar carregar provedores do SQLite
	if err := a.loadLLMProviders(); err == nil {
		return
	}

	// Se não houver provedores, verificar se já passou pelo wizard
	// Se não passou, o wizard criará o primeiro provedor
	count, err := database.CountLLMProviders()
	if err != nil || count == 0 {
		log.Printf("Nenhum provedor encontrado. Configure um provedor nas configurações ou crie um perfil.")
	}
}

// CreateDefaultLLMProvider cria o primeiro provedor durante o wizard
func (a *App) CreateDefaultLLMProvider(providerType, apiKey string) error {
	var provider *llm.ProviderConfig

	switch providerType {
	case "openai":
		provider = &llm.ProviderConfig{
			ID:                "openai-default",
			Name:              "OpenAI",
			Type:              llm.ProviderOpenAI,
			APIFormat:         llm.APIFormatOpenAIResponses,
			BaseURL:           "https://api.openai.com/v1",
			Model:             "gpt-4o-mini",
			Timeout:           180,
			CredentialPattern: "api.openai.com",
		}
	case "claude":
		provider = &llm.ProviderConfig{
			ID:                "anthropic-claude",
			Name:              "Claude (Anthropic)",
			Type:              llm.ProviderClaude,
			BaseURL:           "https://api.anthropic.com/v1",
			Model:             "claude-3-7-sonnet-20250219",
			Timeout:           180,
			CredentialPattern: "api.anthropic.com",
		}
	case "google":
		provider = &llm.ProviderConfig{
			ID:                "google-gemini",
			Name:              "Google (Gemini)",
			Type:              llm.ProviderOpenAI,
			BaseURL:           "https://generativelanguage.googleapis.com/v1beta/openai/",
			Model:             "gemini-2.0-flash",
			Timeout:           180,
			CredentialPattern: "generativelanguage.googleapis.com",
		}
	case "openrouter":
		provider = &llm.ProviderConfig{
			ID:                "openrouter-default",
			Name:              "OpenRouter",
			Type:              llm.ProviderOpenAI,
			BaseURL:           "https://openrouter.ai/api/v1",
			Model:             "openai/gpt-4o-mini",
			Timeout:           180,
			CredentialPattern: "openrouter.ai",
		}
	case "mistral":
		provider = &llm.ProviderConfig{
			ID:                "mistral-default",
			Name:              "Mistral AI",
			Type:              llm.ProviderMistral,
			BaseURL:           "https://api.mistral.ai/v1",
			Model:             "mistral-large-latest",
			Timeout:           180,
			CredentialPattern: "api.mistral.ai",
		}
	case "groq":
		provider = &llm.ProviderConfig{
			ID:                "groq-default",
			Name:              "Groq",
			Type:              llm.ProviderGroq,
			BaseURL:           "https://api.groq.com/openai/v1",
			Model:             "llama-3.3-70b-versatile",
			Timeout:           180,
			CredentialPattern: "api.groq.com",
		}
	case "together":
		provider = &llm.ProviderConfig{
			ID:                "together-default",
			Name:              "Together AI",
			Type:              llm.ProviderTogether,
			BaseURL:           "https://api.together.xyz/v1",
			Model:             "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Timeout:           180,
			CredentialPattern: "api.together.xyz",
		}
	case "fireworks":
		provider = &llm.ProviderConfig{
			ID:                "fireworks-default",
			Name:              "Fireworks AI",
			Type:              llm.ProviderFireworks,
			BaseURL:           "https://api.fireworks.ai/inference/v1",
			Model:             "accounts/fireworks/models/llama-v3p3-70b-instruct",
			Timeout:           180,
			CredentialPattern: "api.fireworks.ai",
		}
	case "perplexity":
		provider = &llm.ProviderConfig{
			ID:                "perplexity-default",
			Name:              "Perplexity",
			Type:              llm.ProviderPerplexity,
			BaseURL:           "https://api.perplexity.ai",
			Model:             "sonar",
			Timeout:           180,
			CredentialPattern: "api.perplexity.ai",
		}
	case "deepseek":
		provider = &llm.ProviderConfig{
			ID:                "deepseek-default",
			Name:              "DeepSeek",
			Type:              llm.ProviderDeepSeek,
			BaseURL:           "https://api.deepseek.com/v1",
			Model:             "deepseek-chat",
			Timeout:           180,
			CredentialPattern: "api.deepseek.com",
		}
	case "grok":
		provider = &llm.ProviderConfig{
			ID:                "xai-grok",
			Name:              "xAI (Grok)",
			Type:              llm.ProviderGrok,
			BaseURL:           "https://api.x.ai/v1",
			Model:             "grok-2",
			Timeout:           180,
			CredentialPattern: "api.x.ai",
		}
	case "ollama":
		provider = &llm.ProviderConfig{
			ID:                "ollama-local",
			Name:              "Ollama (Local)",
			Type:              llm.ProviderOllama,
			BaseURL:           "http://localhost:11434/api",
			Model:             "llama2",
			Timeout:           300,
			CredentialPattern: "",
		}
	default:
		return fmt.Errorf("tipo de provedor inválido: %s", providerType)
	}

	// Registrar no registry
	if err := a.llmRegistry.Register(provider); err != nil {
		return fmt.Errorf("erro ao registrar provedor: %w", err)
	}

	// Salvar API key se fornecida
	if apiKey != "" && provider.CredentialPattern != "" {
		authCfg := &credentials.AuthConfig{
			Type:  "bearer",
			Token: apiKey,
		}
		if err := a.credMgr.RegisterPatternWithContext(a.ctx, provider.CredentialPattern, authCfg); err != nil {
			return fmt.Errorf("erro ao salvar credencial: %w", err)
		}
	}

	// Salvar no SQLite
	if err := a.saveLLMProviders(); err != nil {
		return fmt.Errorf("erro ao salvar provedor: %w", err)
	}

	log.Printf("[Wizard] Provedor '%s' criado com sucesso", provider.ID)
	return nil
}

// initCredentialManager inicializa o gerenciador de credenciais com persistência
func (a *App) initCredentialManager() {
	a.credStore = credentials.NewDBStore()
	persist := true
	dek, err := credentials.LoadDEKFromKeychain()
	if err != nil {
		if !credentials.IsKeychainNotFound(err) {
			log.Printf("[Credentials] Erro ao acessar keychain: %v", err)
		}
		persist = false
		dek = nil
	}

	a.credMgr = credentials.NewManagerWithStore(dek, a.credStore, persist)
	if err := a.credMgr.LoadFromStore(context.Background()); err != nil {
		log.Printf("[Credentials] Erro ao carregar credenciais persistidas: %v", err)
	}
	a.registerEnvCredentials(a.credMgr)
}

// migrateLegacyConfig detecta config.json com campos legados e migra para novo sistema
// Migração:
// 1. Se APIKey existir → registra como credencial no credentials.Manager
// 2. Se APIKey existir → garante que provider default está usando as credenciais
// 3. Limpa campos legados do config.json
func (a *App) migrateLegacyConfig() {
	cfg, err := config.Load()
	if err != nil {
		// Sem config, sem migração necessária
		return
	}

	needsMigration := false
	migratedFields := []string{}

	// Verificar se tem APIKey (campo principal legado)
	if cfg.APIKey != "" {
		needsMigration = true
		migratedFields = append(migratedFields, "APIKey")

		// Extrair domínio do BaseURL
		baseURL := cfg.APIBaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}

		// Determinar pattern baseado no baseURL
		// Usa extractHostname para padrão exato, consistente com CreateLLMProvider
		pattern := ""
		if extractedHost, hostErr := extractHostname(baseURL); hostErr == nil && extractedHost != "" {
			pattern = extractedHost
		} else if strings.Contains(baseURL, "anthropic") {
			pattern = "api.anthropic.com"
		} else if strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") {
			pattern = "" // local, sem pattern
		} else {
			pattern = "api.openai.com" // fallback para OpenAI
		}

		// Registrar credencial no credentials.Manager
		if pattern != "" {
			authCfg := &credentials.AuthConfig{
				Type:  "bearer",
				Token: cfg.APIKey,
			}
			if err := a.credMgr.RegisterPatternWithContext(a.ctx, pattern, authCfg); err != nil {
				log.Printf("[Migration] Erro ao registrar credencial do config.json: %v", err)
			} else {
				log.Printf("[Migration] ✓ APIKey migrado para credentials.Manager (pattern: %s)", pattern)
			}
		}
	}

	// Verificar outros campos legados
	if cfg.APIBaseURL != "" && cfg.APIBaseURL != "https://api.openai.com/v1" {
		migratedFields = append(migratedFields, "APIBaseURL")
	}
	if cfg.DefaultModel != "" && cfg.DefaultModel != "gpt-4o-mini" {
		migratedFields = append(migratedFields, "DefaultModel")
	}
	if cfg.ResponseTimeout != 0 && cfg.ResponseTimeout != 180 {
		migratedFields = append(migratedFields, "ResponseTimeout")
	}
	if cfg.ActiveProfile != "" && cfg.ActiveProfile != "padrao" {
		migratedFields = append(migratedFields, "ActiveProfile")
	}

	if needsMigration {
		log.Printf("[Migration] Config.json legado detectado — campos migrados: %v", migratedFields)
		log.Printf("[Migration] Novas configurações devem ser feitas via Perfis e Provider Registry")
		log.Printf("[Migration] Os campos legados em config.json não serão mais usados")

		// Não vamos deletar o config.json ainda, apenas marcar como migrado
		// Isso permite que usuários vejam o arquivo e entendam a mudança
	}
}

// initTerminalAndAllowlists inicializa os managers de terminal, questionário e allowlists.
func (a *App) initTerminalAndAllowlists() {
	// Callback para emitir eventos Wails a partir dos managers
	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)
	}

	// Terminal Manager (pool compartilhado LLM + usuário)
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), emitEvent)

	// Questionnaire Manager (coleta de respostas estruturadas)
	a.questionnaireMgr = questionnaire.NewManager(emitEvent)

	// Allowlist Manager (CRUD de allowlists)
	a.allowlistMgr = allowlist.NewManager()
	if err := a.allowlistMgr.EnsureDefaults(); err != nil {
		log.Printf("[Allowlist] Erro ao garantir allowlist padrão: %v", err)
	}

	log.Printf("[Terminal] Managers de terminal, questionário e allowlist inicializados")
}

// initMCP inicializa o gerenciador de servidores MCP.
// Deve ser chamado após initToolRegistry (precisa do registry para registrar tools MCP).
func (a *App) initMCP() {
	emitEvent := func(event string, data any) {
		runtime.EventsEmit(a.ctx, event, data)

		// Quando o set de tools MCP muda, regenera o catalogo de jobs
		if event == "mcp:tools_changed" && a.jobMgr != nil {
			go func() {
				if err := a.jobMgr.RegenerateCatalog(); err != nil {
					log.Printf("[Jobs] Catalog regeneration on MCP change failed: %v", err)
				} else {
					log.Printf("[Jobs] Catalog regenerated after MCP tools change")
				}
			}()
		}
	}

	a.mcpMgr = mcpmgr.NewManager(a.toolRegistry, a.credMgr, emitEvent)

	// Carrega configs e auto-conecta servidores habilitados
	if err := a.mcpMgr.LoadConfigs(); err != nil {
		log.Printf("[MCP] Erro ao carregar configurações: %v", err)
	}

	// Observa mudanças externas nos arquivos de config
	go a.mcpMgr.WatchConfigs()

	log.Printf("[MCP] Manager inicializado")
}

// appTaskListManager adapta o App para a interface tasklisttool.TaskListManager
type appTaskListManager struct {
	ctx context.Context
}

// appDeepLinkEmitter emite deep links para o frontend via eventos Wails.
type appDeepLinkEmitter struct {
	ctx context.Context
}

func (e *appDeepLinkEmitter) EmitDeepLink(uri string) {
	runtime.EventsEmit(e.ctx, "deeplink:execute", uri)
}

func (m *appTaskListManager) CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error) {
	return database.CreateTaskList(title, description, templateWorkflow)
}

func (m *appTaskListManager) GetTaskList(id uint) (*database.TaskList, error) {
	return database.GetTaskList(id)
}

func (m *appTaskListManager) GetAllTaskLists() ([]database.TaskList, error) {
	return database.GetAllTaskLists()
}

func (m *appTaskListManager) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return database.GetTaskListStats(taskListID)
}

func (m *appTaskListManager) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error) {
	task, err := database.CreateTask(taskListID, title, description, code, link, parentID)
	if err == nil && task != nil && m.ctx != nil {
		runtime.EventsEmit(m.ctx, "task:created", task)
	}
	return task, err
}

func (m *appTaskListManager) CreateTaskFull(taskListID uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *uint) (*database.Task, error) {
	task, err := database.CreateTaskFull(taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
	if err == nil && task != nil && m.ctx != nil {
		runtime.EventsEmit(m.ctx, "task:created", task)
	}
	return task, err
}

func (m *appTaskListManager) GetTask(id uint) (*database.Task, error) {
	return database.GetTask(id)
}

func (m *appTaskListManager) FindTaskByCode(taskListID uint, code string) (*database.Task, error) {
	return database.FindTaskByCode(taskListID, code)
}

func (m *appTaskListManager) UpdateTask(id uint, title, description, code, link string) error {
	if err := database.UpdateTask(id, title, description, code, link); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	if err := database.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	if err := database.UpdateTaskAssignee(id, assigneeName, assigneeID); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) UpdateTaskStatus(id uint, newStatusID int) error {
	if err := database.UpdateTaskStatus(id, newStatusID); err != nil {
		return err
	}
	m.emitTaskUpdated(id)
	return nil
}

func (m *appTaskListManager) MoveTaskToList(taskID uint, targetTaskListID uint) (*database.Task, error) {
	oldTask, err := database.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	oldListID := oldTask.TaskListID

	task, err := database.MoveTaskToList(taskID, targetTaskListID)
	if err != nil {
		return nil, err
	}

	if m.ctx != nil && oldListID != targetTaskListID {
		runtime.EventsEmit(m.ctx, "task:updated", task)
		runtime.EventsEmit(m.ctx, "taskList:updated", oldListID)
		runtime.EventsEmit(m.ctx, "taskList:updated", targetTaskListID)
	}
	return task, err
}

func (m *appTaskListManager) emitTaskUpdated(id uint) {
	if m.ctx == nil {
		return
	}
	task, err := database.GetTask(id)
	if err == nil && task != nil {
		runtime.EventsEmit(m.ctx, "task:updated", task)
	}
}

func (m *appTaskListManager) DeleteTask(id uint) error {
	return database.DeleteTask(id)
}

func (m *appTaskListManager) GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error) {
	return database.GetWorkflow(taskListID)
}

func (m *appTaskListManager) CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	return database.CreateTaskNote(taskID, noteType, content, authorName, authorID)
}

func (m *appTaskListManager) UpdateTaskNote(noteID uint, content string) error {
	return database.UpdateTaskNote(noteID, content)
}

func (m *appTaskListManager) GetTaskNotes(taskID uint) ([]database.TaskNote, error) {
	return database.GetTaskNotes(taskID)
}

func (m *appTaskListManager) GetTaskNote(noteID uint) (*database.TaskNote, error) {
	return database.GetTaskNote(noteID)
}

func (m *appTaskListManager) UpdateTaskListFull(id uint, title, description, preferredViewMode string) error {
	return database.UpdateTaskListFull(id, title, description, preferredViewMode)
}

func (m *appTaskListManager) UpdateWorkflowFull(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	return database.UpdateWorkflowFull(taskListID, statuses, transitions, initialStatusID, statusMigration)
}

func (m *appTaskListManager) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	return database.GetTaskCountsByStatus(taskListID)
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
	a.toolRegistry.MustRegister(filesystem.NewMoveFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewCopyFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewDeleteFile(workDir))
	a.toolRegistry.MustRegister(filesystem.NewMakeDirectory(workDir))

	// Registra ferramentas web (credMgr já foi inicializado antes)
	a.toolRegistry.MustRegister(web.NewWebFetch(a.credMgr)) // GET simples, foco em leitura

	// HTTPRequest com CredentialManager (autenticação automática por domínio)
	httpReqTool := web.NewHTTPRequest(a.credMgr)

	// Confirmação para operações destrutivas
	httpReqTool.SetConfirmFunc(func(ctx context.Context, method, url, body string) (bool, error) {
		if a.questionnaireMgr == nil {
			return false, fmt.Errorf("questionnaire manager não inicializado")
		}
		bodyPreview := body
		if bodyPreview == "" {
			bodyPreview = "(sem body)"
		}
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       fmt.Sprintf("Confirmar operação %s", method),
			Description: fmt.Sprintf("O assistente quer executar:\n\n%s %s\n\nBody:\n%s", method, url, bodyPreview),
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Negar",
			Questions: []questionnaire.Question{
				{
					ID:       "approve",
					Type:     "boolean",
					Prompt:   fmt.Sprintf("Permitir esta operação %s?", method),
					Required: true,
				},
			},
		})
		if err != nil {
			return false, err
		}
		if resp.Cancelled {
			return false, nil
		}
		approved, ok := resp.Answers["approve"].(bool)
		if !ok {
			return false, fmt.Errorf("resposta inválida para aprovação")
		}
		return approved, nil
	})
	a.toolRegistry.MustRegister(httpReqTool)

	a.toolRegistry.MustRegister(web.NewWebSearch(a.credMgr))

	// Registra ferramenta de shell (run_command)
	confirmFn := func(ctx context.Context, cmd, wd string) (bool, error) {
		if a.questionnaireMgr == nil {
			return false, fmt.Errorf("questionnaire manager não inicializado")
		}
		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Confirmar execução de comando",
			Description: fmt.Sprintf("O assistente quer executar:\n\n%s\n\nem: %s", cmd, wd),
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Negar",
			Questions: []questionnaire.Question{
				{
					ID:       "approve",
					Type:     "boolean",
					Prompt:   "Permitir a execução deste comando?",
					Required: true,
				},
			},
		})
		if err != nil {
			return false, err
		}
		if resp.Cancelled {
			return false, nil
		}
		approved, ok := resp.Answers["approve"].(bool)
		if !ok {
			return false, fmt.Errorf("resposta inválida para aprovação de comando")
		}
		return approved, nil
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

	// Registra ferramenta de questionário (collect_responses)
	a.toolRegistry.MustRegister(questiontool.NewCollectResponses(a.questionnaireMgr))

	// Registra ferramenta de edição de texto (opt-in: só disponível em perfis que a listam explicitamente)
	a.toolRegistry.MustRegisterOptIn(editor.NewTextEdit(a.questionnaireMgr))

	// Registra ferramenta de busca no histórico de conversas
	a.toolRegistry.MustRegister(history.NewSearchConversations())

	// Registra ferramentas de gerenciamento de task lists
	tlMgr := &appTaskListManager{ctx: a.ctx}
	a.toolRegistry.MustRegister(tasklisttool.NewTaskList(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTask(tlMgr))
	a.toolRegistry.MustRegister(tasklisttool.NewTaskNote(tlMgr))

	// Registra ferramenta de deep links
	a.toolRegistry.MustRegister(deeplinktool.NewOpenDeepLink(&appDeepLinkEmitter{ctx: a.ctx}))

	log.Printf("[Tools] Registry inicializado com %d ferramentas: %v", a.toolRegistry.Count(), a.toolRegistry.Names())
}

func (a *App) registerEnvCredentials(credMgr *credentials.Manager) {
	if credMgr == nil {
		return
	}

	// GITHUB_TOKEN -> *.github.com, github.com
	if ghToken := os.Getenv("GITHUB_TOKEN"); ghToken != "" {
		_ = credMgr.RegisterPattern("*.github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
		_ = credMgr.RegisterPattern("github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
	}

	// GITLAB_TOKEN -> *.gitlab.com, gitlab.com
	if glToken := os.Getenv("GITLAB_TOKEN"); glToken != "" {
		_ = credMgr.RegisterPattern("*.gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
		_ = credMgr.RegisterPattern("gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
	}

	// BITBUCKET_TOKEN -> *.bitbucket.org, bitbucket.org
	if bbToken := os.Getenv("BITBUCKET_TOKEN"); bbToken != "" {
		_ = credMgr.RegisterPattern("*.bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
		_ = credMgr.RegisterPattern("bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
	}

	// API genérica - X_API_KEY para qualquer host (fallback)
	// Usar com cuidado - preferir padrões específicos acima
	if apiKey := os.Getenv("GENERIC_API_KEY"); apiKey != "" {
		_ = credMgr.RegisterPattern("*", &credentials.AuthConfig{
			Type: "custom",
			Headers: map[string]string{
				"X-API-Key": apiKey,
			},
		})
	}
}

func (a *App) configureCredentialManager(dek []byte, persist bool) {
	if a.credStore == nil {
		a.credStore = credentials.NewDBStore()
	}
	if a.credMgr == nil {
		a.credMgr = credentials.NewManagerWithStore(dek, a.credStore, persist)
	} else {
		a.credMgr.Reset(dek, persist)
	}

	if err := a.credMgr.LoadFromStore(context.Background()); err != nil {
		log.Printf("[Credentials] Erro ao carregar credenciais persistidas: %v", err)
	}
	a.registerEnvCredentials(a.credMgr)
}

// ToolInfo é um resumo de uma ferramenta para listagem no frontend.
type ToolInfo struct {
	Name        string `json:"name"`         // id interno (namespaced para MCP, ex: mcp_github__create_issue)
	DisplayName string `json:"display_name"` // nome curto para exibição (ex: create_issue)
	Description string `json:"description"`
	SourceType  string `json:"source_type"`  // "local" | "mcp"
	SourceLabel string `json:"source_label"` // "Local" | nome amigável do servidor MCP
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
		name := t.Name()
		info := ToolInfo{
			Name:        name,
			DisplayName: name,
			Description: t.Description(),
			SourceType:  "local",
			SourceLabel: "Local",
		}

		if slug, originalName, ok := mcpmgr.ParseToolName(name); ok {
			info.DisplayName = originalName
			info.SourceType = "mcp"
			info.SourceLabel = slug
			if a.mcpMgr != nil {
				if cfg, err := a.mcpMgr.GetConfig(slug); err == nil && cfg.Name != "" {
					info.SourceLabel = cfg.Name
				}
			}
		}

		result[i] = info
	}
	return result
}

// shutdown é chamado quando o app fecha
func (a *App) shutdown(_ context.Context) {
	a.stopAllEditorWatches()

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

	// Encerra o sistema de jobs
	if a.jobMgr != nil {
		a.jobMgr.Stop()
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

// saveLLMProviders salva os provedores no SQLite
func (a *App) saveLLMProviders() error {
	providers := a.llmRegistry.List()

	for _, p := range providers {
		dbProvider := &database.LLMProvider{
			ID:                p.ID,
			Name:              p.Name,
			Type:              string(p.Type),
			APIFormat:         string(p.APIFormat),
			BaseURL:           p.BaseURL,
			Model:             p.Model,
			DefaultModel:      p.DefaultModel,
			IsDefault:         p.IsDefault,
			Timeout:           p.Timeout,
			CredentialPattern: p.CredentialPattern,
		}
		if err := database.SaveLLMProvider(dbProvider); err != nil {
			log.Printf("Erro ao salvar provedor %s: %v", p.ID, err)
			return err
		}
	}

	return nil
}

// loadLLMProviders carrega provedores do SQLite
func (a *App) loadLLMProviders() error {
	providers, err := database.GetLLMProviders()
	if err != nil {
		return err
	}

	if len(providers) == 0 {
		return fmt.Errorf("nenhum provedor encontrado")
	}

	for _, dbProvider := range providers {
		p := &llm.ProviderConfig{
			ID:                dbProvider.ID,
			Name:              dbProvider.Name,
			Type:              llm.ProviderType(dbProvider.Type),
			APIFormat:         llm.APIFormat(dbProvider.APIFormat),
			BaseURL:           dbProvider.BaseURL,
			Model:             dbProvider.Model,
			DefaultModel:      dbProvider.DefaultModel,
			IsDefault:         dbProvider.IsDefault,
			Timeout:           dbProvider.Timeout,
			CredentialPattern: dbProvider.CredentialPattern,
		}
		if err := a.llmRegistry.Register(p); err != nil {
			log.Printf("Erro ao registrar provedor %s: %v", p.ID, err)
		}
	}

	log.Printf("Provedores LLM carregados do SQLite: %d", len(providers))

	a.ensureDefaultProvider()

	return nil
}

// ensureDefaultProvider marks the first provider as default when none is.
// Handles migration for providers created before the IsDefault feature.
func (a *App) ensureDefaultProvider() {
	defaultProv, err := database.GetDefaultProvider()
	if err == nil && defaultProv != nil {
		return
	}

	allProviders := a.llmRegistry.List()
	if len(allProviders) == 0 {
		return
	}

	first := allProviders[0]
	log.Printf("[ProviderManager] Nenhum provedor default — marcando '%s' como default", first.Name)

	if err := database.SetDefaultProvider(first.ID); err != nil {
		log.Printf("[ProviderManager] Erro ao definir default: %v", err)
		return
	}
	first.IsDefault = true

	if first.DefaultModel == "" && first.Model != "" {
		first.DefaultModel = first.Model
		if dbProv, err := database.GetLLMProvider(first.ID); err == nil {
			dbProv.DefaultModel = first.Model
			database.SaveLLMProvider(dbProv)
		}
	}
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

// RespondQuestionnaire responde a uma solicitação de questionário.
// Chamado pelo frontend quando o usuário envia ou cancela o questionário.
func (a *App) RespondQuestionnaire(requestID string, answers map[string]any, cancelled bool) error {
	if a.questionnaireMgr == nil {
		return fmt.Errorf("questionnaire manager não inicializado")
	}
	return a.questionnaireMgr.Respond(requestID, answers, cancelled)
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
// Token Stats API
// ============================================================================

// ToolUsageBreakdownResult contém informações de uso de uma ferramenta
type ToolUsageBreakdownResult struct {
	ToolName              string `json:"toolName"`
	CallCount             int    `json:"callCount"`
	TotalPromptTokens     int    `json:"totalPromptTokens"`
	TotalCompletionTokens int    `json:"totalCompletionTokens"`
	TotalTokens           int    `json:"totalTokens"`
}

// TokenStatsResult representa estatísticas de tokens para o frontend
type TokenStatsResult struct {
	ConversationID   uint    `json:"conversationId"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	MessageCount     int     `json:"messageCount"`
	Model            string  `json:"model"`
	MostUsedModel    string  `json:"mostUsedModel"`
	ContextUsage     float64 `json:"contextUsage"` // Porcentagem de uso do contexto (0-100)
	ContextLimit     int     `json:"contextLimit"` // Limite de tokens do modelo
	IsNearLimit      bool    `json:"isNearLimit"`  // True se >= 80% do limite
	IsCritical       bool    `json:"isCritical"`   // True se >= 95% do limite

	// Breakdown detalhado de tokens
	SystemPromptEstimatedTokens int                        `json:"systemPromptEstimatedTokens"`
	SummaryTokens               int                        `json:"summaryTokens"`
	MessagesInContextCount      int                        `json:"messagesInContextCount"`
	MessagesInContextTokens     int                        `json:"messagesInContextTokens"`
	MessagesOutOfContextCount   int                        `json:"messagesOutOfContextCount"`
	MessagesOutOfContextTokens  int                        `json:"messagesOutOfContextTokens"`
	ToolsUsedCount              int                        `json:"toolsUsedCount"`
	ToolBreakdown               []ToolUsageBreakdownResult `json:"toolBreakdown"`
}

// GetConversationTokenStats retorna estatísticas de tokens de uma conversa
func (a *App) GetConversationTokenStats(conversationID uint) (*TokenStatsResult, error) {
	// Recuperar summaryUpToMessageID para cálculo de mensagens in/out of context
	summaryUpToMessageID := uint(0)
	summary, upToID, _ := database.GetConversationSummary(conversationID)
	if summary != "" {
		summaryUpToMessageID = upToID
	}

	detailedStats, err := database.GetDetailedTokenStats(conversationID, summaryUpToMessageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas de tokens: %w", err)
	}

	// Map tool usage breakdown
	toolBreakdown := make([]ToolUsageBreakdownResult, len(detailedStats.ToolBreakdown))
	for i, tool := range detailedStats.ToolBreakdown {
		toolBreakdown[i] = ToolUsageBreakdownResult{
			ToolName:              tool.ToolName,
			CallCount:             tool.CallCount,
			TotalPromptTokens:     tool.TotalPromptTokens,
			TotalCompletionTokens: tool.TotalCompletionTokens,
			TotalTokens:           tool.TotalTokens,
		}
	}

	result := &TokenStatsResult{
		ConversationID:              conversationID,
		PromptTokens:                detailedStats.PromptTokens,
		CompletionTokens:            detailedStats.CompletionTokens,
		TotalTokens:                 detailedStats.TotalTokens,
		MessageCount:                detailedStats.MessageCount,
		Model:                       detailedStats.Model,
		MostUsedModel:               detailedStats.Model,
		SystemPromptEstimatedTokens: detailedStats.SystemPromptEstimatedTokens,
		SummaryTokens:               detailedStats.SummaryTokens,
		MessagesInContextCount:      detailedStats.MessagesInContextCount,
		MessagesInContextTokens:     detailedStats.MessagesInContextTokens,
		MessagesOutOfContextCount:   detailedStats.MessagesOutOfContextCount,
		MessagesOutOfContextTokens:  detailedStats.MessagesOutOfContextTokens,
		ToolsUsedCount:              detailedStats.ToolsUsedCount,
		ToolBreakdown:               toolBreakdown,
	}

	// Busca informações do perfil ativo para obter o limite de contexto
	profile, err := a.profileManager.GetActive()
	if err == nil && profile != nil && profile.Chat.ContextWindow > 0 {
		contextLimit := profile.Chat.ContextWindow
		percentage, _, err := database.GetContextWindowUsage(conversationID, contextLimit)
		if err == nil {
			result.ContextUsage = percentage
			result.ContextLimit = contextLimit
			result.IsNearLimit = percentage >= 80.0
			result.IsCritical = percentage >= 95.0
		}
	}

	return result, nil
}

// GetTurnTokenStats retorna estatísticas de tokens para um turno específico
func (a *App) GetTurnTokenStats(conversationID uint, turnID uint) (*TokenStatsResult, error) {
	stats, err := database.GetTurnTokenStats(conversationID, turnID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar estatísticas do turno: %w", err)
	}

	return &TokenStatsResult{
		PromptTokens:     stats.PromptTokens,
		CompletionTokens: stats.CompletionTokens,
		TotalTokens:      stats.TotalTokens,
		MessageCount:     stats.MessageCount,
	}, nil
}

// GetRecentMessagesTokenCount retorna o total de tokens das N mensagens mais recentes
// Útil para estimar quanto contexto será enviado na próxima requisição
func (a *App) GetRecentMessagesTokenCount(conversationID uint, messageLimit int) (int, error) {
	return database.GetRecentMessagesTokenCount(conversationID, messageLimit)
}

// CheckContextWindowThreshold verifica se a conversa está próxima do limite de contexto
// Retorna true e a porcentagem se estiver acima do threshold (padrão 80%)
func (a *App) CheckContextWindowThreshold(conversationID uint, threshold float64) (bool, float64, error) {
	if threshold <= 0 {
		threshold = 80.0 // Padrão: 80%
	}

	// Busca informações do perfil ativo para obter o limite de contexto
	profile, err := a.profileManager.GetActive()
	if err != nil {
		return false, 0, fmt.Errorf("erro ao obter perfil ativo: %w", err)
	}

	if profile == nil || profile.Chat.ContextWindow <= 0 {
		return false, 0, fmt.Errorf("limite de contexto não configurado no perfil")
	}

	contextLimit := profile.Chat.ContextWindow
	percentage, totalTokens, err := database.GetContextWindowUsage(conversationID, contextLimit)
	if err != nil {
		return false, 0, fmt.Errorf("erro ao calcular uso do contexto: %w", err)
	}

	log.Printf("[TokenStats] Conversa %d: %d tokens de %d (%0.1f%%)",
		conversationID, totalTokens, contextLimit, percentage)

	return percentage >= threshold, percentage, nil
}

// GetLLMSettings retorna as configurações atuais da API LLM
func (a *App) GetLLMSettings() (*LLMSettings, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar config: %w", err)
	}

	return &LLMSettings{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.APIBaseURL,
	}, nil
}

// SetMCPWorkspaceRoots configura os diretórios raiz do workspace para servidores MCP.
func (a *App) SetMCPWorkspaceRoots(roots []mcpmgr.Root) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SetWorkspaceRoots(roots)
}

// GetMCPWorkspaceRoots retorna os workspace roots configurados.
func (a *App) GetMCPWorkspaceRoots() []mcpmgr.Root {
	if a.mcpMgr == nil {
		return []mcpmgr.Root{}
	}
	return a.mcpMgr.GetWorkspaceRoots()
}

// SubscribeToMCPResource inscreve para receber notificações de um resource.
func (a *App) SubscribeToMCPResource(slug, uri string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.SubscribeToResource(slug, uri)
}

// UnsubscribeFromMCPResource cancela inscrição de um resource.
func (a *App) UnsubscribeFromMCPResource(slug, uri string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.UnsubscribeFromResource(slug, uri)
}

// SaveMCPServerAuth salva credenciais de autenticação para um servidor MCP.
// As credenciais são armazenadas de forma segura no credential manager,
// usando o hostname da URL do servidor como padrão de resolução.
func (a *App) SaveMCPServerAuth(slug, authType, token, username, password, clientSecret string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	if a.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	return a.mcpMgr.SaveServerAuth(slug, authType, token, username, password, clientSecret)
}

// DeleteMCPServerAuth remove credenciais de autenticação de um servidor MCP.
func (a *App) DeleteMCPServerAuth(slug string) error {
	if a.mcpMgr == nil {
		return fmt.Errorf("MCP manager não inicializado")
	}
	return a.mcpMgr.DeleteServerAuth(slug)
}

// GetMCPServerAuthInfo retorna informações sobre a autenticação de um servidor MCP
// (tipo e se existe, sem expor valores sensíveis).
func (a *App) GetMCPServerAuthInfo(slug string) (map[string]any, error) {
	if a.mcpMgr == nil {
		return nil, fmt.Errorf("MCP manager não inicializado")
	}
	authType, hasAuth, err := a.mcpMgr.GetServerAuthInfo(slug)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"hasAuth":  hasAuth,
		"authType": authType,
	}, nil
}

// DiscoverMCPServerAuth consulta os endpoints well-known de um servidor MCP
// para auto-discovery de configuração OAuth.
func (a *App) DiscoverMCPServerAuth(serverURL string) mcpmgr.OAuthDiscoveryResult {
	return mcpmgr.DiscoverOAuth(serverURL)
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

// ==================== Auto Update ====================

// initUpdater inicializa o gerenciador de atualizações
func (a *App) initUpdater() {
	// AppVersion é injetada via ldflags durante o build
	// Em dev, permanece como "dev"
	a.updater = updater.New(AppVersion, a.credMgr)

	// Configura callback de progresso
	a.updater.SetProgressCallback(func(bytesDownloaded, totalBytes int64, phase string) {
		if a.ctx == nil {
			return
		}

		var percentage float64
		if totalBytes > 0 {
			percentage = float64(bytesDownloaded) / float64(totalBytes) * 100
		}

		runtime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{
			"phase":           phase,
			"bytesDownloaded": bytesDownloaded,
			"totalBytes":      totalBytes,
			"percentage":      percentage,
		})
	})

	// Configura callback de elevação (solicita permissão ao usuário)
	a.updater.SetElevationCallback(func() bool {
		if a.questionnaireMgr == nil {
			log.Printf("[Updater] Questionnaire manager não disponível para solicitar elevação")
			return false
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
			Title:       "Permissão Necessária",
			Description: "Para atualizar o aplicativo, precisamos de permissões de administrador para substituir o arquivo executável.\n\nDeseja permitir?",
			Questions: []questionnaire.Question{
				{
					ID:       "allow",
					Type:     "boolean",
					Prompt:   "Permitir atualização com privilégios de administrador?",
					Required: true,
					Default:  true,
				},
			},
			AllowCancel: true,
			SubmitLabel: "Permitir",
			CancelLabel: "Cancelar",
		})

		if err != nil {
			log.Printf("[Updater] Erro ao solicitar confirmação de elevação: %v", err)
			return false
		}

		if resp.Cancelled {
			log.Printf("[Updater] Usuário cancelou a solicitação de elevação")
			return false
		}

		if allow, ok := resp.Answers["allow"].(bool); ok && allow {
			log.Printf("[Updater] Usuário autorizou elevação")
			return true
		}

		return false
	})

	log.Printf("[Updater] Inicializado (versão atual: %s)", AppVersion)
}

// checkForUpdatesOnStartup verifica atualizações ao iniciar (não bloqueante)
func (a *App) checkForUpdatesOnStartup() {
	// Pula verificação de updates em modo desenvolvimento
	if AppVersion == "dev" {
		log.Printf("[Updater] Modo desenvolvimento detectado (AppVersion=%s): pulando verificação de updates", AppVersion)
		return
	}

	// Aguarda 5 segundos após startup para não interferir com inicialização
	time.Sleep(5 * time.Second)

	// Só verifica atualizações se LLM estiver configurado
	cfg, err := config.Load()
	if err != nil || cfg.APIKey == "" || cfg.APIBaseURL == "" {
		log.Printf("[Updater] Pulando verificação de atualizações: LLM não configurado")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := a.updater.CheckForUpdates(ctx)
	if err != nil {
		log.Printf("[Updater] Erro ao verificar atualizações: %v", err)
		return
	}

	if !info.Available {
		log.Printf("[Updater] Aplicativo está atualizado (v%s)", info.CurrentVersion)
		return
	}

	log.Printf("[Updater] Nova versão disponível: v%s -> v%s", info.CurrentVersion, info.LatestVersion)

	// Pergunta ao usuário se deseja atualizar usando o sistema de questionário
	go a.promptForUpdate(info)
}

// promptForUpdate pergunta ao usuário se deseja atualizar
func (a *App) promptForUpdate(info *updater.UpdateInfo) {
	if a.questionnaireMgr == nil {
		log.Printf("[Updater] Questionnaire manager não disponível")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	description := fmt.Sprintf("Versão atual: %s\nNova versão: %s", info.CurrentVersion, info.LatestVersion)
	if info.ReleaseNotes != "" {
		description += "\n\nNotas da versão:\n" + info.ReleaseNotes
	}
	if info.DownloadSize > 0 {
		sizeMB := float64(info.DownloadSize) / (1024 * 1024)
		description += fmt.Sprintf("\n\nTamanho do download: %.2f MB", sizeMB)
	}

	resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       "Atualização Disponível",
		Description: description,
		Questions: []questionnaire.Question{
			{
				ID:       "confirm",
				Type:     "boolean",
				Prompt:   "Deseja atualizar agora?",
				Required: true,
				Default:  true,
			},
		},
		AllowCancel: true,
		SubmitLabel: "Atualizar",
		CancelLabel: "Mais Tarde",
	})

	if err != nil {
		log.Printf("[Updater] Erro ao solicitar confirmação: %v", err)
		return
	}

	if resp.Cancelled {
		log.Printf("[Updater] Usuário cancelou a atualização")
		return
	}

	if confirm, ok := resp.Answers["confirm"].(bool); ok && confirm {
		// Navega para página de atualização
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "navigate:update", nil)
		}
		go a.applyUpdateWithProgress()
	}
}

// CheckForUpdates verifica manualmente se há atualizações disponíveis (chamado pelo frontend)
func (a *App) CheckForUpdates() (*updater.UpdateInfo, error) {
	if a.updater == nil {
		return nil, fmt.Errorf("updater não inicializado")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.updater.CheckForUpdates(ctx)
}

// ApplyUpdate aplica a atualização (chamado pelo frontend)
func (a *App) ApplyUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	go a.applyUpdateWithProgress()
	return nil
}

// StartUpdate inicia o processo de atualização (navega para página e inicia)
func (a *App) StartUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater não inicializado")
	}

	// Emite evento para navegar para página de atualização
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "navigate:update", nil)
	}

	// Aguarda um pouco para garantir que a navegação ocorreu
	time.Sleep(500 * time.Millisecond)

	go a.applyUpdateWithProgress()
	return nil
}

// applyUpdateWithProgress aplica a atualização com feedback de progresso
func (a *App) applyUpdateWithProgress() {
	// Emite evento de início
	runtime.EventsEmit(a.ctx, "update:started", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Printf("[Updater] Iniciando download e aplicação da atualização...")

	err := a.updater.ApplyUpdate(ctx)
	if err != nil {
		log.Printf("[Updater] Erro ao aplicar atualização: %v", err)
		runtime.EventsEmit(a.ctx, "update:error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	log.Printf("[Updater] Atualização aplicada com sucesso. Reinicie o aplicativo.")
	runtime.EventsEmit(a.ctx, "update:completed", map[string]interface{}{
		"message": "Atualização instalada com sucesso! Feche e reabra o aplicativo para aplicar as mudanças.",
	})
}

// GetAppVersion retorna a versão atual do aplicativo
func (a *App) GetAppVersion() string {
	return AppVersion
}

// ==================== Workspace ====================

func (a *App) initWorkspace() {
	homeDir := configdir.GetHomeDir()
	a.workspaceMgr = workspace.NewManager(homeDir)

	workDir := ""
	if wd, err := os.Getwd(); err == nil {
		workDir = wd
	}

	if err := a.workspaceMgr.Initialize(workDir); err != nil {
		log.Printf("Erro ao inicializar workspace: %v", err)
	} else if ws := a.workspaceMgr.Active(); ws != nil {
		log.Printf("Workspace ativo: %s (%s)", ws.Name, ws.ID)
	}
}

// GetActiveWorkspace retorna o workspace ativo.
func (a *App) GetActiveWorkspace() *workspace.Workspace {
	if a.workspaceMgr == nil {
		return nil
	}
	return a.workspaceMgr.Active()
}

// ListWorkspaces retorna todos os workspaces conhecidos.
func (a *App) ListWorkspaces() ([]workspace.WorkspaceInfo, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.List()
}

// CreateWorkspace cria um novo workspace avulso.
func (a *App) CreateWorkspace(name string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.Create(name)
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:created", ws)
	return ws, nil
}

// SwitchWorkspace alterna para outro workspace.
func (a *App) SwitchWorkspace(workspaceID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.Switch(workspaceID)
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:switched", ws)
	return ws, nil
}

// RenameWorkspace renomeia o workspace ativo.
func (a *App) RenameWorkspace(newName string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.Rename(newName); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:renamed", a.workspaceMgr.Active())
	return nil
}

// DeleteWorkspace remove um workspace (não pode ser o ativo).
func (a *App) DeleteWorkspace(workspaceID string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.Delete(workspaceID); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:deleted", workspaceID)
	return nil
}

// SetWorkspaceProfile define o perfil base do workspace ativo.
func (a *App) SetWorkspaceProfile(profileSlug string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.SetProfile(profileSlug)
}

// SaveWorkspace persiste o estado do workspace ativo.
func (a *App) SaveWorkspace() error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.Save()
}

// --- Workspace Tab APIs ---

// AddWorkspaceTab adiciona uma aba ao workspace ativo.
func (a *App) AddWorkspaceTab(tab workspace.Tab) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.AddTab(tab); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_added", ws)
	return ws, nil
}

// RemoveWorkspaceTab remove uma aba do workspace ativo.
func (a *App) RemoveWorkspaceTab(tabID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.RemoveTab(tabID); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_removed", ws)
	return ws, nil
}

// SetActiveWorkspaceTab define a aba ativa no workspace.
func (a *App) SetActiveWorkspaceTab(tabID string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:tab_activated", tabID)
	return nil
}

// UpdateWorkspaceTab atualiza campos de uma aba.
func (a *App) UpdateWorkspaceTab(tabID string, updates map[string]any) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.UpdateTab(tabID, updates)
}

// ReorderWorkspaceTabs reordena as abas do workspace.
func (a *App) ReorderWorkspaceTabs(orderedIDs []string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.ReorderTabs(orderedIDs)
}

// MoveWorkspaceTabTo move uma aba do workspace ativo para outro workspace.
func (a *App) MoveWorkspaceTabTo(tabID, targetWorkspaceID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.MoveTabToWorkspace(tabID, targetWorkspaceID); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_removed", ws)
	return ws, nil
}

// ExportWorkspace exporta o workspace ativo como YAML.
func (a *App) ExportWorkspace() (string, error) {
	if a.workspaceMgr == nil {
		return "", fmt.Errorf("workspace manager not initialized")
	}
	data, err := a.workspaceMgr.ExportWorkspace()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ImportWorkspace importa um workspace a partir de YAML.
func (a *App) ImportWorkspace(yamlData string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.ImportWorkspace([]byte(yamlData))
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:created", ws)
	return ws, nil
}
