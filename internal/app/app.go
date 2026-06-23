package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"assistente/controllers"
	"assistente/internal/agent"
	"assistente/internal/allowlist"
	"assistente/internal/auth"
	"assistente/internal/chat"
	"assistente/internal/connstatus"
	"assistente/internal/contextprovider"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/jobs"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/memory"
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

// subagentReconcileTimeout é o teto de tempo da reconciliação de runs órfãos de
// sub-agente no startup. Operação de manutenção que roda em goroutine; o deadline
// impede que um DB travado (lock/I/O lento) deixe a goroutine pendurada
// indefinidamente. 30s é consistente com o teto usado em operações de jobs
// (internal/jobs/manager.go).
const subagentReconcileTimeout = 30 * time.Second

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
	cancel            context.CancelFunc    // cancela o ctx raiz no Shutdown
	bgWG              sync.WaitGroup        // join das goroutines de background no Shutdown
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

	// Memory service (Context Provider de memória)
	memorySvc *memory.Service

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

	// Context Providers (AEP-0075): blocos dinâmicos separados de skills.
	contextProviders *contextprovider.Registry

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
	memoryCtrl      *controllers.MemoryController
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
	// Deriva um contexto cancelável: Shutdown chama a.cancel() para sinalizar o
	// encerramento às goroutines de background. WithCancel preserva os values do
	// ctx pai (ex.: userID), então Context() continua válido para os consumidores.
	a.ctx, a.cancel = context.WithCancel(ctx)
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

	// Inicializa o Memory Service (Context Provider de memória)
	a.memorySvc = memory.NewService(memory.NewDBStore(database.DB()))

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
		Skills:    a.skillMgr,
		Workspace: a.workspaceMgr,
		Tools:     a.toolRegistry,
	}
	a.contextProviders = contextprovider.NewRegistry(
		a.memorySvc,
		tasklist.NewContextProvider(),
		workspace.NewContextProvider(),
	)

	// Inicializa o ChatInteractor (após skillMgr e promptBuilder estarem prontos)
	a.chatInteractor = chat.NewInteractor(chat.InteractorConfig{
		Emitter:          a.emitter,
		Repo:             a.msgRepo,
		ConvRepo:         a.convSvc,
		ProviderSvc:      a.providerSvc,
		ProfileMgr:       a.profileManager,
		Workspace:        a.workspaceMgr,
		SkillMgr:         a.skillMgr,
		PromptBuilder:    a.promptBuilder,
		ContextProviders: a.contextProviders,
		LinkedTaskLists:  a.linkedTaskListsForConversation,
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
		SkillMgr:   a.skillMgr,
		ProfileMgr: a.profileManager,
		Emitter:    a.emitter,
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

	// Reconciliação de runs órfãos (AEP-0068 F4): runs deixados em queued/running
	// por um encerramento abrupto do app são marcados como failed no startup
	// (espelha a reconciliação de jobs). Não bloqueia o startup.
	//
	// cutoff capturado AQUI (após criar o manager, antes de servir requests):
	// como a reconciliação roda em goroutine enquanto o app já pode aceitar
	// chamadas, só reconciliamos runs criados antes deste instante — um run
	// legítimo criado em paralelo (created_at >= cutoff) não é marcado como
	// órfão.
	reconcileCutoff := time.Now()
	go func() {
		// Teto de tempo: a reconciliação roda em goroutine de startup e não pode
		// pendurar o processo indefinidamente se o DB travar (lock/I/O lento).
		// WithoutCancel é mantido (não deve ser cancelada por cancelamento normal
		// do ctx do app durante uso), mas WithTimeout adiciona o deadline —
		// consistente com o teto de operações de jobs (internal/jobs/manager.go).
		ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), subagentReconcileTimeout)
		defer cancel()
		n, err := a.subagentMgr.ReconcileOrphans(ctx, reconcileCutoff)
		if err != nil {
			log.Printf("[Subagent] erro ao reconciliar runs órfãos: %v", err)
		} else if n > 0 {
			log.Printf("[Subagent] %d run(s) órfão(s) de sub-agente reconciliado(s) como failed", n)
		}
	}()

	a.taskListCtrl = controllers.NewTaskListController(controllers.TaskListControllerConfig{
		TaskSvc: a.taskSvc,
	})
	a.memoryCtrl = controllers.NewMemoryController(controllers.MemoryControllerConfig{
		MemorySvc: a.memorySvc,
	})
	a.speechCtrl = controllers.NewSpeechController(controllers.SpeechControllerConfig{
		SpeechSvc: a.speechSvc,
	})
	a.jobsCtrl = controllers.NewJobsController(controllers.JobsControllerConfig{
		JobMgr: a.jobMgr,
	})
	a.tokensCtrl = controllers.NewTokensController(controllers.TokensControllerConfig{
		ProfileMgr: a.profileManager,
		TokenSvc:   a.tokenSvc,
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

	// Verifica atualizações no startup (não bloqueante). Rastreada em bgWG para
	// que o Shutdown faça join e não deixe a goroutine órfã.
	a.bgWG.Add(1)
	go func() {
		defer a.bgWG.Done()
		a.checkForUpdatesOnStartup()
	}()

	return nil
}

// ShowWindow torna a janela visível (delegado ao WindowPort).
func (a *App) ShowWindow() {
	a.windowPort.Show()
}

// shutdownBackgroundTimeout é o teto de espera pelo join das goroutines de
// background no Shutdown. Defensivo: evita travar o encerramento caso alguma
// goroutine não respeite o cancelamento do contexto a tempo.
const shutdownBackgroundTimeout = 10 * time.Second

// waitBackground aguarda o término das goroutines rastreadas em bgWG, com
// timeout defensivo para não bloquear o shutdown indefinidamente.
func (a *App) waitBackground(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		a.bgWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("[App] Timeout aguardando goroutines de background no Shutdown")
	}
}

// Shutdown encerra todos os serviços do app.
func (a *App) Shutdown() {
	// Sinaliza o cancelamento às goroutines de background e aguarda o join
	// antes de derrubar os managers, evitando loops órfãos no encerramento.
	if a.cancel != nil {
		a.cancel()
	}
	a.waitBackground(shutdownBackgroundTimeout)

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
