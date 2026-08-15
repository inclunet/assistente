package app

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"assistente/controllers"
	"assistente/internal/acp"
	"assistente/internal/acpregistry"
	"assistente/internal/acptrust"
	"assistente/internal/agent"
	"assistente/internal/allowlist"
	"assistente/internal/apidto"
	"assistente/internal/auth"
	"assistente/internal/chat"
	"assistente/internal/connstatus"
	"assistente/internal/contextprovider"
	"assistente/internal/conversation"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/deeplinkprotocol"
	"assistente/internal/events"
	"assistente/internal/jobs"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/memory"
	"assistente/internal/messaging"
	"assistente/internal/nettrust"
	"assistente/internal/profiles"
	"assistente/internal/prompt"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"assistente/internal/skills"
	"assistente/internal/slashskill"
	"assistente/internal/speech"
	"assistente/internal/subagent"
	"assistente/internal/summarization"
	"assistente/internal/tasklist"
	"assistente/internal/terminal"
	"assistente/internal/toolinvocations"
	"assistente/internal/toolprotocol"
	"assistente/internal/tools"
	"assistente/internal/updater"
	"assistente/internal/wailsapi"
	"assistente/internal/workspace"
)

// subagentReconcileTimeout é o teto de tempo da reconciliação de runs órfãos de
// sub-agente no startup. Operação de manutenção que roda em goroutine; o deadline
// impede que um DB travado (lock/I/O lento) deixe a goroutine pendurada
// indefinidamente. 30s é consistente com o teto usado em operações de jobs
// (internal/jobs/manager.go).
const subagentReconcileTimeout = 30 * time.Second

// Request structs for LLM Provider Management — type aliases para apidto.
// Mantém compatibilidade com código e testes existentes durante a migração.
type CreateLLMProviderRequest = apidto.CreateLLMProviderRequest
type TestLLMProviderRequest = apidto.TestLLMProviderRequest
type UpdateLLMProviderRequest = apidto.UpdateLLMProviderRequest

// ChannelInfo — type alias para controllers.
type ChannelInfo = controllers.ChannelInfo

// App struct
type App struct {
	ctx               context.Context
	cancel            context.CancelFunc    // cancela o ctx raiz no Shutdown
	bgWG              sync.WaitGroup        // join das goroutines de background no Shutdown
	llmRegistry       *llm.ProviderRegistry // Registro de provedores LLM
	profileManager    *profiles.Manager
	toolRegistry      *tools.Registry          // Registro de ferramentas disponíveis
	toolExecutor      *tools.Executor          // Executor de ferramentas com paralelismo e timeout
	toolInvocationSvc *toolinvocations.Service // Persistência e execução comum de tool calls
	terminalMgr       *terminal.Manager        // Gerenciador de sessões PTY (pool compartilhado LLM + usuário)
	questionnaireMgr  *questionnaire.Manager   // Gerenciador de questionários (coleta estruturada)
	allowlistMgr      *allowlist.Manager       // Gerenciador de allowlists de comandos
	netTrustMgr       *nettrust.Manager        // Allowlist de rede escopável (anti-SSRF override)
	mcpMgr            *mcpmgr.Manager          // Gerenciador de servidores MCP
	acpMgr            *acp.Manager             // Processos e sessões dos agentes ACP (AEP-0084)
	acpTrust          *acptrust.Store          // Permissões que o perfil concedeu ao agente para sempre (AEP-0084 D9)
	acpRegistry       *acpregistry.Service     // Catálogo de agentes do registro oficial do ACP (AEP-0086 D2)
	skillMgr          *skills.Manager          // Gerenciador de skills
	// acpCatalogSvc é o catálogo do registro ACP: o serviço acima e o instalador
	// de agentes (AEP-0086). Montado na primeira chamada que precisa dele — ver
	// acpCatalogServices em app_acp_install.go —, porque o instalador só existe
	// para quem for instalar, e nada no startup depende dele.
	acpCatalogSvc    *acpCatalog
	acpCatalogOnce   sync.Once
	responseNotifier *messaging.ResponseNotifier // Notificador de respostas para mensageiros
	msgGateway       *messaging.Gateway          // Gateway de mensageria (Telegram, etc.)
	updater          *updater.Updater            // Gerenciador de atualizações automáticas

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
	editorWatchMu             sync.Mutex
	editorDirWatches          map[string]*editorDirWatch
	editorAssistedWriteByPath map[string][]editorAssistedWrite
	editorAssistedWriteSeq    int64

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

	legacyImportSummaryMu            sync.Mutex
	legacyImportSkippedSummaryUserID string

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
	taskListCtrl      *controllers.TaskListController
	conversationsCtrl *controllers.ConversationsController
	memoryCtrl        *controllers.MemoryController
	speechCtrl        *controllers.SpeechController
	jobsCtrl          *controllers.JobsController
	workspaceCtrl     *controllers.WorkspaceController
	tokensCtrl        *controllers.TokensController
	toolsCtrl         *controllers.ToolsController
	updaterCtrl       *controllers.UpdaterController
	credentialsCtrl   *controllers.CredentialsController
	welcomeCtrl       *controllers.WelcomeController
	terminalCtrl      *controllers.TerminalController
	allowlistCtrl     *controllers.AllowlistController
	signalCtrl        *controllers.SignalController
	hotkeyCtrl        *controllers.HotkeysController
	netTrustCtrl      *controllers.NetTrustController

	// tokensAPI é o bind Wails do domínio tokens (AEP-0088). Criado em main e
	// wired após NewTokensController.
	tokensAPI *wailsapi.Tokens

	// allowlistsAPI é o bind Wails do domínio allowlists (AEP-0088). Criado em
	// main e wired após NewAllowlistController.
	allowlistsAPI *wailsapi.Allowlists

	// skillsAPI é o bind Wails do domínio skills (AEP-0088). Criado em main e
	// wired após NewSkillsController.
	skillsAPI *wailsapi.Skills

	// toolsAPI é o bind Wails do domínio tools (AEP-0088). Criado em main e
	// wired após NewToolsController.
	toolsAPI *wailsapi.Tools

	// updaterAPI é o bind Wails do domínio updater (AEP-0088). Criado em main e
	// wired após NewUpdaterController.
	updaterAPI *wailsapi.Updater

	// profilesAPI é o bind Wails do domínio profiles (AEP-0088). Criado em main e
	// wired após NewProfilesController.
	profilesAPI *wailsapi.Profiles

	// hotkeysAPI é o bind Wails do domínio hotkeys (AEP-0088). Criado em main e
	// wired após NewHotkeysController.
	hotkeysAPI *wailsapi.Hotkeys

	// netTrustAPI é o bind Wails do domínio nettrust (AEP-0088). Criado em main e
	// wired após NewNetTrustController.
	netTrustAPI *wailsapi.NetTrust

	// credentialsAPI é o bind Wails do domínio credentials (AEP-0088). Criado em
	// main e wired após NewCredentialsController.
	credentialsAPI *wailsapi.Credentials

	// settingsAPI é o bind Wails do domínio settings (AEP-0088). Criado em main e
	// wired após NewSettingsController.
	settingsAPI *wailsapi.Settings

	// mcpAPI é o bind Wails do domínio MCP (AEP-0088). Criado em main e
	// wired após NewMCPController.
	mcpAPI *wailsapi.MCP

	// signalAPI é o bind Wails do domínio Signal (AEP-0088). Criado em main e
	// wired após NewSignalController.
	signalAPI *wailsapi.Signal

	// terminalAPI é o bind Wails do domínio terminal (AEP-0088). Criado em
	// main e wired após NewTerminalController.
	terminalAPI *wailsapi.Terminal

	// memoryAPI é o bind Wails do domínio memory (AEP-0088). Criado em main e
	// wired após NewMemoryController.
	memoryAPI *wailsapi.Memory

	// welcomeAPI é o bind Wails do domínio welcome (AEP-0088). Criado em main e
	// wired após NewWelcomeController.
	welcomeAPI *wailsapi.Welcome

	// legacyCleanupAPI é o bind Wails do cleanup de JSON legado (AEP-0088).
	// Criado em main e wired sem controller (chama channels diretamente).
	legacyCleanupAPI *wailsapi.LegacyCleanup

	// databaseAPI é o bind Wails do domínio database/manutenção (AEP-0088).
	// Criado em main e wired após wireSettings (reusa settingsCtrl).
	databaseAPI *wailsapi.Database

	// subagentAPI é o bind Wails do domínio subagent (AEP-0088). Criado em main
	// e wired após a criação do subagentMgr.
	subagentAPI *wailsapi.Subagent

	// tasklistActionsAPI é o bind Wails do domínio tasklist_actions / custom
	// actions (AEP-0088). Criado em main e wired após NewTaskListController.
	tasklistActionsAPI *wailsapi.TasklistActions

	// tasklistAPI é o bind Wails do domínio tasklist CRUD (AEP-0088). Criado em
	// main e wired após NewTaskListController.
	tasklistAPI *wailsapi.Tasklist

	// conversationsAPI é o bind Wails do domínio conversations/persistência
	// (AEP-0088). Criado em main e wired após NewConversationsController.
	conversationsAPI *wailsapi.Conversations

	// jobsAPI é o bind Wails do domínio jobs (AEP-0088). Criado em main e
	// wired após NewJobsController.
	jobsAPI *wailsapi.Jobs

	// llmProvidersAPI é o bind Wails do domínio llm_providers (AEP-0088).
	// Criado em main e wired após NewLLMController.
	llmProvidersAPI *wailsapi.LLMProviders

	// acpCommandsAPI é o bind Wails do domínio acp_commands (AEP-0088). Criado
	// em main e wired após initACP (reusa acpMgr).
	acpCommandsAPI *wailsapi.ACPCommands

	// acpProvidersAPI é o bind Wails de detect/test de agentes ACP (AEP-0088).
	// Criado em main e wired após initACP.
	acpProvidersAPI *wailsapi.ACPProviders

	// acpOptionsAPI é o bind Wails do domínio acp_options (AEP-0088). Criado
	// em main e wired após initACP (reusa acpMgr). Eventos lowercase permanecem no *App.
	acpOptionsAPI *wailsapi.ACPOptions

	// acpRegistryAPI é o bind Wails do catálogo do registro ACP (AEP-0088).
	// Criado em main e wired após initACP. Helpers de montagem permanecem no *App.
	acpRegistryAPI *wailsapi.ACPRegistry

	// acpWorkDirAPI é o bind Wails do domínio acp_workdir (AEP-0088). Criado em
	// main e wired após initACP. Helpers ConversationDir permanecem no *App.
	acpWorkDirAPI *wailsapi.ACPWorkDir

	// acpInstallAPI é o bind Wails de install/update/remove de agentes ACP
	// (AEP-0088). Criado em main; helpers de handshake/progresso/repontar
	// permanecem no *App e entram via hooks.
	acpInstallAPI *wailsapi.ACPInstall

	// acpTrustAPI é o bind Wails de autorizações permanentes ACP (AEP-0088).
	// Criado em main e wired após initACP (reusa acpTrust). Handlers de
	// permissão em tempo de turno permanecem no *App.
	acpTrustAPI *wailsapi.ACPTrust
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

// SetTokensAPI registra o bind Wails de tokens antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetTokensAPI(a *App, api *wailsapi.Tokens) {
	if a == nil {
		return
	}
	a.tokensAPI = api
}

// SetAllowlistsAPI registra o bind Wails de allowlists antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetAllowlistsAPI(a *App, api *wailsapi.Allowlists) {
	if a == nil {
		return
	}
	a.allowlistsAPI = api
}

// SetSkillsAPI registra o bind Wails de skills antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetSkillsAPI(a *App, api *wailsapi.Skills) {
	if a == nil {
		return
	}
	a.skillsAPI = api
}

// SetToolsAPI registra o bind Wails de tools antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetToolsAPI(a *App, api *wailsapi.Tools) {
	if a == nil {
		return
	}
	a.toolsAPI = api
}

// ListAvailableTools expõe o catálogo runtime para o CLI (não entra no Bind Wails).
func ListAvailableTools(a *App) []controllers.ToolInfo {
	if a == nil || a.toolsCtrl == nil {
		return nil
	}
	return a.toolsCtrl.GetAvailableTools()
}

// SetUpdaterAPI registra o bind Wails de updater antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetUpdaterAPI(a *App, api *wailsapi.Updater) {
	if a == nil {
		return
	}
	a.updaterAPI = api
}

// SetProfilesAPI registra o bind Wails de profiles antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetProfilesAPI(a *App, api *wailsapi.Profiles) {
	if a == nil {
		return
	}
	a.profilesAPI = api
}

// SetHotkeysAPI registra o bind Wails de hotkeys antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetHotkeysAPI(a *App, api *wailsapi.Hotkeys) {
	if a == nil {
		return
	}
	a.hotkeysAPI = api
}

// SetNetTrustAPI registra o bind Wails de nettrust antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetNetTrustAPI(a *App, api *wailsapi.NetTrust) {
	if a == nil {
		return
	}
	a.netTrustAPI = api
}

// SetCredentialsAPI registra o bind Wails de credentials antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetCredentialsAPI(a *App, api *wailsapi.Credentials) {
	if a == nil {
		return
	}
	a.credentialsAPI = api
}

// SetSettingsAPI registra o bind Wails de settings antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetSettingsAPI(a *App, api *wailsapi.Settings) {
	if a == nil {
		return
	}
	a.settingsAPI = api
}

// SetMCPAPI registra o bind Wails de MCP antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetMCPAPI(a *App, api *wailsapi.MCP) {
	if a == nil {
		return
	}
	a.mcpAPI = api
}

// SetSignalAPI registra o bind Wails de Signal antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetSignalAPI(a *App, api *wailsapi.Signal) {
	if a == nil {
		return
	}
	a.signalAPI = api
}

// SetTerminalAPI registra o bind Wails de terminal antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetTerminalAPI(a *App, api *wailsapi.Terminal) {
	if a == nil {
		return
	}
	a.terminalAPI = api
}

// SetMemoryAPI registra o bind Wails de memory antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetMemoryAPI(a *App, api *wailsapi.Memory) {
	if a == nil {
		return
	}
	a.memoryAPI = api
}

// SetWelcomeAPI registra o bind Wails de welcome antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetWelcomeAPI(a *App, api *wailsapi.Welcome) {
	if a == nil {
		return
	}
	a.welcomeAPI = api
}

// SetLegacyCleanupAPI registra o bind Wails de legacy cleanup antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetLegacyCleanupAPI(a *App, api *wailsapi.LegacyCleanup) {
	if a == nil {
		return
	}
	a.legacyCleanupAPI = api
}

// SetDatabaseAPI registra o bind Wails de database antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetDatabaseAPI(a *App, api *wailsapi.Database) {
	if a == nil {
		return
	}
	a.databaseAPI = api
}

// SetSubagentAPI registra o bind Wails de subagent antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetSubagentAPI(a *App, api *wailsapi.Subagent) {
	if a == nil {
		return
	}
	a.subagentAPI = api
}

// SetTasklistActionsAPI registra o bind Wails de tasklist_actions antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetTasklistActionsAPI(a *App, api *wailsapi.TasklistActions) {
	if a == nil {
		return
	}
	a.tasklistActionsAPI = api
}

// SetTasklistAPI registra o bind Wails de tasklist CRUD antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetTasklistAPI(a *App, api *wailsapi.Tasklist) {
	if a == nil {
		return
	}
	a.tasklistAPI = api
}

// SetConversationsAPI registra o bind Wails de conversations antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetConversationsAPI(a *App, api *wailsapi.Conversations) {
	if a == nil {
		return
	}
	a.conversationsAPI = api
}

// SetJobsAPI registra o bind Wails de jobs antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetJobsAPI(a *App, api *wailsapi.Jobs) {
	if a == nil {
		return
	}
	a.jobsAPI = api
}

// SetLLMProvidersAPI registra o bind Wails de llm_providers antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetLLMProvidersAPI(a *App, api *wailsapi.LLMProviders) {
	if a == nil {
		return
	}
	a.llmProvidersAPI = api
}

// SetACPCommandsAPI registra o bind Wails de acp_commands antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetACPCommandsAPI(a *App, api *wailsapi.ACPCommands) {
	if a == nil {
		return
	}
	a.acpCommandsAPI = api
}

// SetACPProvidersAPI registra o bind Wails de acp_providers antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetACPProvidersAPI(a *App, api *wailsapi.ACPProviders) {
	if a == nil {
		return
	}
	a.acpProvidersAPI = api
}

// SetACPOptionsAPI registra o bind Wails de acp_options antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetACPOptionsAPI(a *App, api *wailsapi.ACPOptions) {
	if a == nil {
		return
	}
	a.acpOptionsAPI = api
}

// SetACPRegistryAPI registra o bind Wails de acp_registry antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetACPRegistryAPI(a *App, api *wailsapi.ACPRegistry) {
	if a == nil {
		return
	}
	a.acpRegistryAPI = api
}

// SetACPWorkDirAPI registra o bind Wails de acp_workdir antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetACPWorkDirAPI(a *App, api *wailsapi.ACPWorkDir) {
	if a == nil {
		return
	}
	a.acpWorkDirAPI = api
}

// SetACPInstallAPI registra o bind Wails de acp_install antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetACPInstallAPI(a *App, api *wailsapi.ACPInstall) {
	if a == nil {
		return
	}
	a.acpInstallAPI = api
}

// SetACPTrustAPI registra o bind Wails de acp_trust antes do Run (main.go).
// Função de pacote (não método) para não entrar na superfície Bind do Wails.
func SetACPTrustAPI(a *App, api *wailsapi.ACPTrust) {
	if a == nil {
		return
	}
	a.acpTrustAPI = api
}

// ProfilesCtrl expõe o ProfilesController para a CLI (não entra no Bind Wails).
func ProfilesCtrl(a *App) *controllers.ProfilesController {
	if a == nil {
		return nil
	}
	return a.profilesCtrl
}

// LLMCtrl expõe o LLMController para a CLI (não entra no Bind Wails).
func LLMCtrl(a *App) *controllers.LLMController {
	if a == nil {
		return nil
	}
	return a.llmCtrl
}

// ApplyInstalledBinaryEnv expõe applyInstalledBinaryEnv para a CLI (não entra no Bind Wails).
func ApplyInstalledBinaryEnv(a *App, ctx context.Context, providerID, agentID string) {
	if a == nil {
		return
	}
	a.applyInstalledBinaryEnv(ctx, providerID, agentID)
}

// PersistLLMProviderDelete remove o provedor do store (side effect de Delete; não entra no Bind).
func PersistLLMProviderDelete(a *App, ctx context.Context, id string) error {
	if a == nil {
		return fmt.Errorf("app não inicializado")
	}
	return database.DeleteLLMProviderWithContext(ctx, id)
}

// CredentialsCtrl expõe o CredentialsController para a CLI (não entra no Bind Wails).
func CredentialsCtrl(a *App) *controllers.CredentialsController {
	if a == nil {
		return nil
	}
	return a.credentialsCtrl
}

// MCPCtrl expõe o MCPController para a CLI (não entra no Bind Wails).
func MCPCtrl(a *App) *controllers.MCPController {
	if a == nil {
		return nil
	}
	return a.mcpCtrl
}

// TaskListCtrl expõe o TaskListController para a CLI (não entra no Bind Wails).
func TaskListCtrl(a *App) *controllers.TaskListController {
	if a == nil {
		return nil
	}
	return a.taskListCtrl
}

// ConversationsCtrl expõe o ConversationsController para a CLI (não entra no Bind Wails).
func ConversationsCtrl(a *App) *controllers.ConversationsController {
	if a == nil {
		return nil
	}
	return a.conversationsCtrl
}

// AuthenticatedContext expõe o contexto autenticado para a CLI (não entra no Bind Wails).
func AuthenticatedContext(a *App) (context.Context, error) {
	if a == nil {
		return nil, database.ErrUserScopeRequired
	}
	return a.requireAuthenticatedContext()
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
		logging.Errorf(ctx, "app.app", "Erro ao limpar drafts órfãos do editor no startup: %v", err)
	}

	// Instala/atualiza perfis embutidos em ~/.assistente/profiles/
	a.installBuiltinProfiles()

	// Garante que o diretório de perfis existe
	if err := a.profileManager.EnsureDefaults(); err != nil {
		logging.Errorf(ctx, "app.app", "Erro ao garantir diretório de perfis: %v", err)
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
			logging.Infof(ctx, "app.app", "[llm/ratelimit] usuário %s próximo do limite de chamadas LLM (%.0f tokens restantes)", key, remaining)
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

	// Inicializa o serviço dos agentes ACP (processos e sessões). Nada sobe
	// aqui; ele precisa existir antes do provider service porque é dele que um
	// provedor de agente empresta a sessão da conversa (AEP-0084 D3).
	a.initACP()

	// Inicializa o Provider Service (camada de negócio para provedores LLM)
	a.providerSvc = providers.NewService(providers.ServiceConfig{
		Registry:         a.llmRegistry,
		CredMgr:          a.credMgr,
		Store:            providers.NewDBStore(),
		RateLimiter:      llmRateLimiter,
		RateLimitKeyFunc: llmRateLimitKeyFunc,
		ACPManager:       a.acpMgr,
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

	// Inicializa o gateway de mensageria (Telegram, etc.).
	// Fail-closed (AEP-0083): sem DB não há fallback silencioso para filesystem.
	if err := a.initMessaging(); err != nil {
		return err
	}

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
			logging.Warnf(ctx, "app.app", "[Speech] WARN: dispatchSpeechEvent falhou (conv=%s msg=%s): %v", conversationID, messageID, err)
		}
	}

	// Inicializa o Agent Service (agentic loop desacoplado do Wails)
	a.agentSvc = agent.NewService(agent.ServiceConfig{
		Emitter:          a.emitter,
		MsgRepo:          a.msgRepo,
		ToolExecutor:     a.toolExecutor,
		ToolInvocations:  a.toolInvocationSvc,
		ResponseNotifier: a.responseNotifier,
		GetTokenStats: func(conversationID string) (*chat.TokenStats, error) {
			ctx, err := a.requireAuthenticatedContext()
			if err != nil {
				return nil, err
			}
			if a.tokensCtrl == nil {
				return nil, fmt.Errorf("controller de tokens ainda não está pronto")
			}
			return a.tokensCtrl.GetConversationTokenStats(ctx, conversationID)
		},
		TriggerSummarize: a.summarySvc.CheckAndTriggerSummarization,
		OnSpeechRequest:  speechDispatcher,
		// O interactor nasce depois deste ponto, então ele é resolvido na hora
		// do uso: guardá-lo agora congelaria um nulo.
		RenameFromAgent: func(ctx context.Context, conversationID, turnMessageID, title string) error {
			if a.chatInteractor == nil {
				return nil
			}
			return a.chatInteractor.RenameFromAgent(ctx, conversationID, turnMessageID, title)
		},
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
		conversation.NewContextProvider(),
		a.memorySvc,
		skills.NewContextProvider(a.skillMgr),
		slashskill.NewContextProvider(),
		tasklist.NewContextProvider(),
		toolprotocol.NewContextProvider(),
		deeplinkprotocol.NewContextProvider(),
		workspace.NewContextProvider(),
		workspace.NewSurfaceContextProvider(),
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
	a.wireHotkeys()
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
	a.wireMCP()
	a.wireProfiles()
	a.wireLLMProviders()
	a.wireSettings()
	a.wireDatabase()

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
	// Conecta adapters de canal só agora — SendMessageFromChannel precisa de chatCtrl.
	if a.msgCtrl != nil {
		if a.msgGateway != nil {
			a.msgGateway.SetCancelStream(a.streamMgr.Cancel)
		}
		a.msgCtrl.StartAdapters("")
	}
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
		EmitEvent: func(event string, data any) {
			a.emitter.Emit(event, data)
		},
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
			logging.Errorf(ctx, "app.app", "[Subagent] erro ao reconciliar runs órfãos: %v", err)
		} else if n > 0 {
			logging.Infof(ctx, "app.app", "[Subagent] %d run(s) órfão(s) de sub-agente reconciliado(s) como failed", n)
		}
	}()

	a.taskListCtrl = controllers.NewTaskListController(controllers.TaskListControllerConfig{
		TaskSvc: a.taskSvc,
	})
	a.wireTasklist()
	a.wireTasklistActions()
	a.conversationsCtrl = controllers.NewConversationsController(controllers.ConversationsControllerConfig{
		MsgRepo:               a.msgRepo,
		Emitter:               a.emitter,
		ResetScopedState:      a.resetConversationScopedState,
		ConfirmDeleteMessage:  a.confirmDeleteMessageQuestionnaire,
		GetEffectiveModelFunc: a.effectiveModelFromActiveProfile,
	})
	a.wireConversations()
	a.speechCtrl = controllers.NewSpeechController(controllers.SpeechControllerConfig{
		SpeechSvc: a.speechSvc,
	})
	a.jobsCtrl = controllers.NewJobsController(controllers.JobsControllerConfig{
		JobMgr: a.jobMgr,
	})
	a.wireJobs()
	a.wireTokens()
	a.wireSkills()
	a.wireAllowlist()
	a.wireTools()
	a.wireUpdater()
	a.wireNetTrust()
	a.wireCredentials()
	a.wireMemory()
	a.wireWelcome()
	a.wireLegacyCleanup()
	a.wireSubagent()
	a.wireACPCommands()
	a.wireACPProviders()
	a.wireACPOptions()
	a.wireACPRegistry()
	a.wireACPWorkDir()
	a.wireACPInstall()
	a.wireACPTrust()
	a.wireSignal()
	a.wireTerminal()

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
		logging.Warnf(context.Background(), "app.app", "[App] Timeout aguardando goroutines de background no Shutdown")
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

	// Derruba os processos dos agentes ACP. As sessões ficam registradas: o
	// agente sobrevive ao app e, na volta, a conversa é retomada de onde parou.
	if a.acpMgr != nil {
		a.acpMgr.Shutdown()
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
