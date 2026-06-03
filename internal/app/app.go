package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"assistente/controllers"
	"assistente/internal/agent"
	"assistente/internal/allowlist"
	"assistente/internal/auth"
	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/connstatus"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/jobs"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/profiles"
	"assistente/internal/prompt"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"assistente/internal/skills"
	"assistente/internal/speech"
	"assistente/internal/subagent"
	"assistente/internal/summarization"
	"assistente/internal/tasklist"
	"assistente/internal/terminal"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/updater"
	"assistente/internal/workspace"
)

// Request structs for LLM Provider Management — type aliases para controllers.
// Mantém compatibilidade com código e testes existentes durante a migração.
type CreateLLMProviderRequest = controllers.CreateLLMProviderRequest
type TestLLMProviderRequest = controllers.TestLLMProviderRequest
type UpdateLLMProviderRequest = controllers.UpdateLLMProviderRequest

// SkillCreateRequest — type alias para controllers.
type SkillCreateRequest = controllers.SkillCreateRequest

// ChannelInfo — type alias para controllers.
type ChannelInfo = controllers.ChannelInfo

// App struct
type App struct {
	ctx               context.Context
	llmRegistry       *llm.ProviderRegistry // Registro de provedores LLM
	profileManager    *profiles.Manager
	toolRegistry      *tools.Registry             // Registro de ferramentas disponíveis
	toolExecutor      *tools.Executor             // Executor de ferramentas com paralelismo e timeout
	toolInvocationSvc *toolinvocations.Service    // Persistência e execução comum de tool calls
	terminalMgr       *terminal.Manager           // Gerenciador de sessões PTY (pool compartilhado LLM + usuário)
	questionnaireMgr  *questionnaire.Manager      // Gerenciador de questionários (coleta estruturada)
	allowlistMgr      *allowlist.Manager          // Gerenciador de allowlists de comandos
	mcpMgr            *mcpmgr.Manager             // Gerenciador de servidores MCP
	skillMgr          *skills.Manager             // Gerenciador de skills
	responseNotifier  *messaging.ResponseNotifier // Notificador de respostas para mensageiros
	msgGateway        *messaging.Gateway          // Gateway de mensageria (Telegram, etc.)
	updater           *updater.Updater            // Gerenciador de atualizações automáticas

	credMgr           *credentials.Manager
	credStore         credentials.Store
	vaultSvc          *auth.VaultService
	identitySvc       *auth.IdentityService
	sessionSvc        *auth.SessionService
	httpAPIServer     *http.Server
	authMu            sync.RWMutex
	authSessionMu     sync.Mutex
	currentUserID     string
	currentAuthUser   *AuthUser
	authKeyringLoad   func() (string, error)
	authKeyringSave   func(string) error
	authKeyringDelete func() error

	// Watcher de arquivos do editor (mudanças externas)
	editorWatchMu    sync.Mutex
	editorDirWatches map[string]*editorDirWatch

	// Workspace manager (unified tabs)
	workspaceMgr *workspace.Manager

	// Jobs manager (event-driven automation)
	jobMgr *jobs.Manager

	// Subagent manager (sub-agentes em sub-conversas — AEP-0068)
	subagentMgr *subagent.Manager

	// Contexto cancelável do runtime user-scoped (ex.: loops de auto-connect).
	userRuntimeMu     sync.Mutex
	userRuntimeCtx    context.Context
	userRuntimeCancel context.CancelFunc

	// Monitor de status de conexão com a API LLM (health check periódico).
	connMu      sync.Mutex
	connMonitor *connstatus.Monitor
	connCancel  context.CancelFunc

	// Provider service (business logic para provedores LLM)
	providerSvc *providers.Service

	// Token service (estatísticas de tokens e janela de contexto)
	tokenSvc *chat.TokenService

	// TaskList service (business logic para listas de tarefas)
	taskSvc *tasklist.Service

	// Audio repository (persistência de áudio de mensagens)
	audioSvc speech.AudioRepository

	// Conversation repository (metadados de conversa)
	convSvc chat.ConversationRepository

	// Message repository (criação e consulta de mensagens)
	msgRepo chat.MessageRepository

	// ChatInteractor (orquestra validações, perfil, renaming — livre de Wails)
	chatInteractor *chat.Interactor

	// Summary service (sumarização de conversas em background)
	summarySvc *summarization.Service

	// Agent service (agentic loop sem dependências do Wails)
	agentSvc *agent.Service

	// Prompt builder (monta system prompt — puro, sem Wails)
	promptBuilder *prompt.Builder

	// Settings service (config CRUD e reset de dados — sem Wails)
	settingsSvc *config.SettingsService

	// Speech service (TTS/STT business logic — sem Wails)
	speechSvc *speech.Service

	// StreamingManager gerencia contextos canceláveis por conversa (barge-in)
	streamMgr *chat.StreamingManager

	// Emitter abstrai runtime.EventsEmit para desacoplar lógica de negócio do Wails
	emitter events.Emitter

	// Ports de infraestrutura (Wails em produção, noop em testes/CLI)
	windowPort ports.WindowPort
	dialogPort ports.SystemDialogPort

	// Controllers (Inbound Adapters — camada Fase 2 da migração para Clean Arch)
	msgCtrl         *controllers.MessagingController
	mcpCtrl         *controllers.MCPController
	profilesCtrl    *controllers.ProfilesController
	llmCtrl         *controllers.LLMController
	skillsCtrl      *controllers.SkillsController
	settingsCtrl    *controllers.SettingsController
	chatCtrl        *controllers.ChatController
	taskListCtrl    *controllers.TaskListController
	speechCtrl      *controllers.SpeechController
	jobsCtrl        *controllers.JobsController
	workspaceCtrl   *controllers.WorkspaceController
	tokensCtrl      *controllers.TokensController
	toolsCtrl       *controllers.ToolsController
	updaterCtrl     *controllers.UpdaterController
	credentialsCtrl *controllers.CredentialsController
	welcomeCtrl     *controllers.WelcomeController
	terminalCtrl    *controllers.TerminalController
	allowlistCtrl   *controllers.AllowlistController
	signalCtrl      *controllers.SignalController
	hotkeyCtrl      *controllers.HotkeysController
}

// ==================== Tipos para Threads ====================

type StreamEvent = events.StreamEvent

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		profileManager: profiles.NewManager(),
		llmRegistry:    llm.NewProviderRegistry(),
	}
}

// Context retorna o contexto da aplicação.
func (a *App) Context() context.Context {
	return a.ctx
}

// StartupWithAdapters inicializa o app com os adapters fornecidos.
// Reutilizado pelo Wails (main.go na raiz) e pelo CLI (cmd/asst/).
func (a *App) StartupWithAdapters(ctx context.Context, emitter events.Emitter, window ports.WindowPort, dialog ports.SystemDialogPort) error {
	a.ctx = ctx
	a.emitter = emitter
	a.windowPort = window
	a.dialogPort = dialog

	// Inicializa o banco de dados (falha crítica: sem DB nada funciona)
	if err := InitDatabase(); err != nil {
		return fmt.Errorf("erro ao inicializar banco de dados: %w", err)
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
	a.initAuthServices()

	// Inicializa o rate limiter das chamadas LLM (Issue #27 / AEP-0065).
	// Escopo por usuário (userID do contexto); defaults sensatos com override
	// por variáveis de ambiente. nil quando desabilitado.
	llmRateLimiter := llm.NewRateLimiter(llm.RateLimitConfigFromEnv())
	if llmRateLimiter != nil {
		llmRateLimiter.SetNearLimitHandler(func(key string, remaining float64) {
			log.Printf("[llm/ratelimit] usuário %s próximo do limite de chamadas LLM (%.0f tokens restantes)", key, remaining)
		})
	}
	// Chave do rate limit = userID do contexto (AEP-0052). Compartilhada entre
	// o provider service (chat) e a sumarização para usarem o mesmo bucket.
	llmRateLimitKeyFunc := func(ctx context.Context) string {
		if userID, ok := database.UserIDFromContext(ctx); ok {
			return userID
		}
		return ""
	}

	// Inicializa o Provider Service (camada de negócio para provedores LLM)
	a.providerSvc = providers.NewService(providers.ServiceConfig{
		Registry:         a.llmRegistry,
		CredMgr:          a.credMgr,
		Store:            providers.NewDBStore(),
		RateLimiter:      llmRateLimiter,
		RateLimitKeyFunc: llmRateLimitKeyFunc,
	})

	// Inicializa o Token Service (estatísticas de tokens)
	a.tokenSvc = chat.NewTokenService(chat.NewDBMessageStore())

	// Inicializa o TaskList Service (business logic de listas de tarefas)
	a.taskSvc = a.newTaskListService()

	// Inicializa repositórios de audio e conversa
	a.audioSvc = speech.NewDBAudioStore()
	a.convSvc = chat.NewDBConversationStore()
	a.msgRepo = chat.NewDBMessageStore()

	// Inicializa o Speech Service aqui (antes de initMessaging, que depende dele)
	a.speechSvc = speech.NewService(speech.ServiceConfig{
		Emitter:         a.emitter,
		Registry:        a.llmRegistry,
		ProfileProvider: profileProviderAdapter{app: a},
		CredMgr:         a.credMgr,
		AudioRepo:       a.audioSvc,
	})

	// Inicializa o Summary Service (sumarização de conversas)
	a.summarySvc = summarization.NewService(summarization.ServiceConfig{
		Repo:           summarization.NewDBStore(),
		Emitter:        a.emitter,
		LLMRegistry:    a.llmRegistry,
		CredMgr:        a.credMgr,
		ProfileManager: a.profileManager,
		ProfileResolver: func(ctx context.Context, p *profiles.Profile) *profiles.Profile {
			return a.providerSvc.ResolveProfileDefaults(ctx, p)
		},
		RateLimiter:      llmRateLimiter,
		RateLimitKeyFunc: llmRateLimitKeyFunc,
	})
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
	a.toolInvocationSvc = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), a.toolExecutor)

	// Inicializa o gateway de mensageria (Telegram, etc.)
	a.initMessaging()

	// Callback reutilizado pelo agent.Service e ChatController
	speechDispatcher := func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool) {
		if _, err := a.dispatchSpeechEvent(ChatSpeakRequest{
			ConversationID: conversationID,
			MessageID:      messageID,
			ProfileSlug:    profileSlug,
			Role:           role,
			Text:           text,
			Origin:         ChatSpeakOrigin(origin),
			Interrupt:      &interrupt,
		}); err != nil {
			log.Printf("[Speech] WARN: dispatchSpeechEvent falhou (conv=%s msg=%s): %v", conversationID, messageID, err)
		}
	}

	// Inicializa o Agent Service (agentic loop desacoplado do Wails)
	a.agentSvc = agent.NewService(agent.ServiceConfig{
		Emitter:          a.emitter,
		MsgRepo:          a.msgRepo,
		ToolExecutor:     a.toolExecutor,
		ToolInvocations:  a.toolInvocationSvc,
		ResponseNotifier: a.responseNotifier,
		GetTokenStats:    a.GetConversationTokenStats,
		TriggerSummarize: a.summarySvc.CheckAndTriggerSummarization,
		OnSpeechRequest:  speechDispatcher,
	})

	// Workspace antes do Prompt Builder: senão Workspace fica (*Manager)(nil) numa interface (typed nil)
	// e BuildTemplateData chama Active() → panic.
	a.initWorkspace()

	// StreamingManager: controla contextos canceláveis por conversa (barge-in)
	a.streamMgr = chat.NewStreamingManager(a.responseNotifier)

	// Inicializa o Prompt Builder (montagem de system prompt, sem Wails)
	a.promptBuilder = &prompt.Builder{
		Skills:          a.skillMgr,
		Workspace:       a.workspaceMgr,
		Tools:           a.toolRegistry,
		OpenEditorPaths: a.workspaceMgr.OpenEditorFilePaths,
	}

	// Inicializa o Settings Service (config CRUD e reset de dados)
	a.settingsSvc = config.NewSettingsService(config.SettingsServiceConfig{
		Emitter:        a.emitter,
		CredCleaner:    credentialCleanerAdapter{mgr: a.credMgr},
		ProfileCleaner: profileCleanerAdapter{app: a},
		SkillCleaner:   skillCleanerAdapter{app: a},
		ReloadLLM:      a.initLLMClient,
	})

	// Inicializa o ChatInteractor (após skillMgr e promptBuilder estarem prontos)
	a.chatInteractor = chat.NewInteractor(chat.InteractorConfig{
		Emitter:       a.emitter,
		Repo:          a.msgRepo,
		ConvRepo:      a.convSvc,
		ProviderSvc:   a.providerSvc,
		ProfileMgr:    a.profileManager,
		Workspace:     a.workspaceMgr,
		SkillMgr:      a.skillMgr,
		PromptBuilder: a.promptBuilder,
	})

	// Inicializa hotkeys globais
	a.hotkeyCtrl = controllers.NewHotkeysController(controllers.HotkeysControllerConfig{
		ProfileMgr: a.profileManager,
		Emitter:    a.emitter,
		WindowPort: a.windowPort,
	})
	a.initGlobalHotkeys()

	// Registra hotkeys do perfil ativo
	a.registerActiveProfileHotkeys()

	// Inicializa o sistema de jobs (event-driven automation)
	a.initJobs()

	// Liga a ponte de eventos de domínio das tasklists ao EventBus de jobs (AEP-0067).
	// O Service é criado antes do jobMgr, então o sink é injetado aqui.
	if a.taskSvc != nil {
		a.taskSvc.SetDomainEventSink(a.jobMgr)
	}

	// Inicializa o updater
	a.initUpdater()

	// Instancia os Controllers (Fase 2 — Inbound Adapters por domínio)
	a.mcpCtrl = controllers.NewMCPController(a.mcpMgr, a.jobMgr, a.emitter)
	a.profilesCtrl = controllers.NewProfilesController(controllers.ProfilesControllerConfig{
		ProfileMgr: a.profileManager,
		Emitter:    a.emitter,
		OnProfileChanged: func(slug string) {
			a.initLLMClient()
			if err := a.InitSpeechManagerFromProfile(); err != nil {
				log.Printf("[Profile] Erro ao inicializar speech manager para perfil %s: %v", slug, err)
			}
			a.registerActiveProfileHotkeys()
		},
	})
	a.llmCtrl = controllers.NewLLMController(controllers.LLMControllerConfig{
		LLMRegistry:      a.llmRegistry,
		ProfileMgr:       a.profileManager,
		ProviderSvc:      a.providerSvc,
		Emitter:          a.emitter,
		OnProviderChange: a.initLLMClient,
	})
	a.skillsCtrl = controllers.NewSkillsController(controllers.SkillsControllerConfig{
		SkillMgr: a.skillMgr,
		Emitter:  a.emitter,
	})
	a.settingsCtrl = controllers.NewSettingsController(controllers.SettingsControllerConfig{
		CredMgr:     a.credMgr,
		ProfileMgr:  a.profileManager,
		SkillMgr:    a.skillMgr,
		Emitter:     a.emitter,
		ProviderSvc: a.providerSvc,
		RestartChannel: func(channelName string) error {
			return a.RestartChannel(channelName)
		},
		GetModels: func() ([]string, error) {
			return a.GetModels()
		},
		InitLLMClient: a.initLLMClient,
	})

	a.chatCtrl = controllers.NewChatController(controllers.ChatControllerConfig{
		Emitter:          a.emitter,
		ChatInteractor:   a.chatInteractor,
		ToolRegistry:     a.toolRegistry,
		ProviderSvc:      a.providerSvc,
		MCPMgr:           a.mcpMgr,
		AgentSvc:         a.agentSvc,
		StreamMgr:        a.streamMgr,
		SpeechSvc:        a.speechSvc,
		SettingsSvc:      a.settingsSvc,
		ConvRepo:         a.convSvc,
		MsgGateway:       a.msgGateway,
		ResponseNotifier: a.responseNotifier,
		OnSpeechRequest:  speechDispatcher,
		OpenEditorPaths:  a.workspaceMgr.OpenEditorFilePaths,
	})
	// Subagent manager (AEP-0068): criado após o ChatController para reusar a
	// MESMA SendMessageUseCase (sem fluxo alternativo de envio — AEP-0040).
	a.subagentMgr = subagent.NewManager(subagent.ManagerConfig{
		Repo:     subagent.NewDBRepository(database.DB()),
		Notifier: a.responseNotifier,
		Send: func(ctx context.Context, p subagent.SendParams) (string, error) {
			return a.chatCtrl.SendForSubagent(ctx, p.ConversationID, p.Prompt, p.Media, p.ProfileSlug, p.Model)
		},
		Delivery:     &subagentParentDelivery{app: a},
		CancelStream: a.streamMgr.Cancel,
	})

	a.taskListCtrl = controllers.NewTaskListController(controllers.TaskListControllerConfig{
		TaskSvc: a.taskSvc,
	})
	a.speechCtrl = controllers.NewSpeechController(controllers.SpeechControllerConfig{
		SpeechSvc: a.speechSvc,
	})
	a.jobsCtrl = controllers.NewJobsController(controllers.JobsControllerConfig{
		JobMgr: a.jobMgr,
	})
	a.tokensCtrl = controllers.NewTokensController(controllers.TokensControllerConfig{
		ProfileMgr:  a.profileManager,
		TokenSvc:    a.tokenSvc,
		SettingsSvc: a.settingsSvc,
	})
	a.toolsCtrl = controllers.NewToolsController(controllers.ToolsControllerConfig{
		ToolRegistry: a.toolRegistry,
		MCPMgr:       a.mcpMgr,
	})
	a.updaterCtrl = controllers.NewUpdaterController(controllers.UpdaterControllerConfig{
		Updater:          a.updater,
		Emitter:          a.emitter,
		QuestionnaireMgr: a.questionnaireMgr,
		ProviderSvc:      a.providerSvc,
		AppVersion:       AppVersion,
	})
	a.credentialsCtrl = controllers.NewCredentialsController(controllers.CredentialsControllerConfig{
		CredMgr: a.credMgr,
	})
	a.welcomeCtrl = controllers.NewWelcomeController(controllers.WelcomeControllerConfig{
		QuestionnaireMgr:           a.questionnaireMgr,
		CredMgr:                    a.credMgr,
		ProviderSvc:                a.providerSvc,
		LLMRegistry:                a.llmRegistry,
		SettingsSvc:                a.settingsSvc,
		Updater:                    a.updater,
		UpdaterCtrl:                a.updaterCtrl,
		ConfigureCredentialManager: a.configureCredentialManager,
		InitLLMClient:              a.initLLMClient,
		SaveLLMProviders:           a.saveLLMProviders,
	})
	a.terminalCtrl = controllers.NewTerminalController(controllers.TerminalControllerConfig{
		TerminalMgr: a.terminalMgr,
	})
	a.allowlistCtrl = controllers.NewAllowlistController(controllers.AllowlistControllerConfig{
		AllowlistMgr:     a.allowlistMgr,
		QuestionnaireMgr: a.questionnaireMgr,
	})
	a.signalCtrl = controllers.NewSignalController()

	if err := a.startHTTPAPI(); err != nil {
		return err
	}

	// Verifica atualizações no startup (não bloqueante)
	go a.checkForUpdatesOnStartup()

	return nil
}

// ShowWindow torna a janela visível (delegado ao WindowPort).
func (a *App) ShowWindow() {
	a.windowPort.Show()
}

// Shutdown encerra todos os serviços do app.
func (a *App) Shutdown() {
	a.stopAllEditorWatches()
	a.stopConnectionMonitor()

	if a.httpAPIServer != nil {
		_ = a.httpAPIServer.Shutdown(context.Background())
	}

	if a.hotkeyCtrl != nil {
		a.hotkeyCtrl.Stop()
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
