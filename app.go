package main

import (
	"context"
	"log"
	"sync"
	"time"

	"assistente/adapters/wails"
	"assistente/controllers"
	"assistente/internal/agent"
	"assistente/internal/allowlist"
	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/events"
	"assistente/internal/hotkey"
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
	"assistente/internal/summarization"
	"assistente/internal/tasklist"
	"assistente/internal/terminal"
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

// App struct
type App struct {
	ctx              context.Context
	llmRegistry      *llm.ProviderRegistry // Registro de provedores LLM
	hotkeyManager    *hotkey.Manager
	profileManager   *profiles.Manager
	toolRegistry     *tools.Registry             // Registro de ferramentas disponíveis
	toolExecutor     *tools.Executor             // Executor de ferramentas com paralelismo e timeout
	terminalMgr      *terminal.Manager           // Gerenciador de sessões PTY (pool compartilhado LLM + usuário)
	questionnaireMgr *questionnaire.Manager      // Gerenciador de questionários (coleta estruturada)
	allowlistMgr     *allowlist.Manager          // Gerenciador de allowlists de comandos
	mcpMgr           *mcpmgr.Manager             // Gerenciador de servidores MCP
	skillMgr         *skills.Manager             // Gerenciador de skills
	responseNotifier *messaging.ResponseNotifier // Notificador de respostas para mensageiros
	msgGateway       *messaging.Gateway          // Gateway de mensageria (Telegram, etc.)
	updater          *updater.Updater            // Gerenciador de atualizações automáticas

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
}

// ==================== Tipos para Threads ====================

type StreamEvent = events.StreamEvent

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
	a.emitter = appEmitter{ctx: ctx}
	a.windowPort = wails.NewWindowAdapter(ctx)
	a.dialogPort = wails.NewDialogAdapter(ctx)

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

	// Inicializa o Token Service (estatísticas de tokens)
	a.tokenSvc = chat.NewTokenService(chat.NewDBMessageStore())

	// Inicializa o TaskList Service (business logic de listas de tarefas)
	a.taskSvc = a.newTaskListService()

	// Inicializa repositórios de audio e conversa
	a.audioSvc = speech.NewDBAudioStore()
	a.convSvc = chat.NewDBConversationStore()
	a.msgRepo = chat.NewDBMessageStore()

	// Inicializa o Summary Service (sumarização de conversas)
	a.summarySvc = summarization.NewService(summarization.ServiceConfig{
		Repo:            summarization.NewDBStore(),
		Emitter:         a.emitter,
		LLMRegistry:     a.llmRegistry,
		CredMgr:         a.credMgr,
		ProfileManager:  a.profileManager,
		ProfileResolver: a.resolveProfileDefaults,
	})
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

	// Inicializa o Speech Service (TTS/STT business logic) — antes de initMessaging
	// porque o canal SIP precisa do speechSvc para criar SpeechManagers
	a.speechSvc = speech.NewService(speech.ServiceConfig{
		Emitter:         a.emitter,
		Registry:        a.llmRegistry,
		ProfileProvider: profileProviderAdapter{app: a},
		CredMgr:         a.credMgr,
		AudioRepo:       a.audioSvc,
	})

	// Inicializa o gateway de mensageria (Telegram, SIP, etc.)
	a.initMessaging()

	// Injeta speechManager no SIP adapter baseado no perfil DO CANAL (não do global)
	a.injectSIPSpeechManager()

	// Inicializa o Agent Service (agentic loop desacoplado do Wails)
	a.agentSvc = agent.NewService(agent.ServiceConfig{
		Emitter:          a.emitter,
		MsgRepo:          a.msgRepo,
		ToolExecutor:     a.toolExecutor,
		ResponseNotifier: a.responseNotifier,
		GetTokenStats:    a.GetConversationTokenStats,
		TriggerSummarize: a.summarySvc.CheckAndTriggerSummarization,
	})

	// Workspace antes do Prompt Builder: senão Workspace fica (*Manager)(nil) numa interface (typed nil)
	// e BuildTemplateData chama Active() → panic.
	a.initWorkspace()

	// StreamingManager: controla contextos canceláveis por conversa (barge-in)
	a.streamMgr = chat.NewStreamingManager(a.responseNotifier)

	// Inicializa o Prompt Builder (montagem de system prompt, sem Wails)
	a.promptBuilder = &prompt.Builder{
		Skills:    a.skillMgr,
		Workspace: a.workspaceMgr,
		Tools:     a.toolRegistry,
	}

	// Inicializa o Settings Service (config CRUD e reset de dados)
	a.settingsSvc = config.NewSettingsService(config.SettingsServiceConfig{
		Emitter:        a.emitter,
		CredCleaner:    a.credMgr,
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
		SkillMgr:      a.skillMgr,
		PromptBuilder: a.promptBuilder,
	})

	// Inicializa hotkeys globais
	a.initGlobalHotkeys()

	// Registra hotkeys do perfil ativo
	a.registerActiveProfileHotkeys()

	// Inicializa o sistema de jobs (event-driven automation)
	a.initJobs()

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
		CredMgr:    a.credMgr,
		ProfileMgr: a.profileManager,
		SkillMgr:   a.skillMgr,
		Emitter:    a.emitter,
		RestartChannel: func(channelName string) error {
			return a.RestartChannel(channelName)
		},
		GetModels: func() ([]string, error) {
			return a.GetModels()
		},
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
	a.workspaceCtrl = controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{
		WorkspaceMgr: a.workspaceMgr,
		Emitter:      a.emitter,
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

	// Verifica atualizações no startup (não bloqueante)
	go a.checkForUpdatesOnStartup()

	// Restaura foco da janela no startup (resolve bug do Wails no Windows)
	// Deixa 400ms para garantir que a janela está completamente pronta
	go func() {
		time.Sleep(400 * time.Millisecond)
		a.windowPort.Show()
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
