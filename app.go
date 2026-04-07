package main

import (
	"context"
	"log"
	"sync"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/credentials"
	"assistente/internal/hotkey"
	"assistente/internal/jobs"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"assistente/internal/skills"
	"assistente/internal/speech"
	"assistente/internal/terminal"
	"assistente/internal/tools"
	"assistente/internal/updater"
	"assistente/internal/workspace"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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

	// Provider service (business logic para provedores LLM)
	providerSvc *providers.Service

	// Streaming context management (barge-in support)
	streamingMu       sync.Mutex
	streamingContexts map[uint]context.CancelFunc // conversationID → cancel
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
		hotkeyLastFired:   make(map[uint]time.Time),
		hotkeyThrottleMs:  1000,
		profileManager:    profiles.NewManager(),
		llmRegistry:       llm.NewProviderRegistry(),
		streamingContexts: make(map[uint]context.CancelFunc),
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

	// Inicializa o Provider Service (camada de negócio para provedores LLM)
	a.providerSvc = providers.NewService(providers.ServiceConfig{
		Registry: a.llmRegistry,
		CredMgr:  a.credMgr,
		Store:    providers.NewDBStore(),
	})

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
