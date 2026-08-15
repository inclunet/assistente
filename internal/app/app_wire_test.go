package app

import (
	"context"
	"errors"
	"testing"

	"assistente/controllers"
	"assistente/internal/acp"
	"assistente/internal/acpregistry"
	"assistente/internal/acptrust"
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/memory"
	"assistente/internal/portability"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/speech"
	"assistente/internal/subagent"
	"assistente/internal/terminal"
	"assistente/internal/wailsapi"
)

func TestWireTokensAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
		tokenSvc:       chat.NewTokenService(chat.NewDBMessageStore()),
	}
	api := wailsapi.NewTokens()
	SetTokensAPI(a, api)

	a.wireTokens()

	if a.tokensCtrl == nil {
		t.Fatal("tokensCtrl deve ser criado")
	}
	_, err := api.GetConversationTokenStats("c1")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireAllowlistAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewAllowlists()
	SetAllowlistsAPI(a, api)

	a.wireAllowlist()

	if a.allowlistCtrl == nil {
		t.Fatal("allowlistCtrl deve ser criado")
	}
	_, err := api.GetAllowlists()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireSkillsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewSkills()
	SetSkillsAPI(a, api)

	a.wireSkills()

	if a.skillsCtrl == nil {
		t.Fatal("skillsCtrl deve ser criado")
	}
	_, err := api.GetSkills()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireToolsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewTools()
	SetToolsAPI(a, api)

	a.wireTools()

	if a.toolsCtrl == nil {
		t.Fatal("toolsCtrl deve ser criado")
	}
	_, err := api.GetAvailableTools()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireUpdaterAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewUpdater()
	SetUpdaterAPI(a, api)

	a.wireUpdater()

	if a.updaterCtrl == nil {
		t.Fatal("updaterCtrl deve ser criado")
	}
	_, err := api.GetAppVersion()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireProfilesAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewProfiles()
	SetProfilesAPI(a, api)

	a.wireProfiles()

	if a.profilesCtrl == nil {
		t.Fatal("profilesCtrl deve ser criado")
	}
	_, err := api.GetProfiles()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireHotkeysAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewHotkeys()
	SetHotkeysAPI(a, api)

	a.wireHotkeys()

	if a.hotkeyCtrl == nil {
		t.Fatal("hotkeyCtrl deve ser criado")
	}
	_, err := api.IsGlobalHotkeySupported()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireNetTrustAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewNetTrust()
	SetNetTrustAPI(a, api)

	a.wireNetTrust()

	if a.netTrustCtrl == nil {
		t.Fatal("netTrustCtrl deve ser criado")
	}
	_, err := api.GetNetworkAllowlist()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireCredentialsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewCredentials()
	SetCredentialsAPI(a, api)

	a.wireCredentials()

	if a.credentialsCtrl == nil {
		t.Fatal("credentialsCtrl deve ser criado")
	}
	_, err := api.ListCredentials()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireSettingsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewSettings()
	SetSettingsAPI(a, api)

	a.wireSettings()

	if a.settingsCtrl == nil {
		t.Fatal("settingsCtrl deve ser criado")
	}
	_, err := api.GetNativeTTSProviders()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireMCPAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewMCP()
	SetMCPAPI(a, api)

	a.wireMCP()

	if a.mcpCtrl == nil {
		t.Fatal("mcpCtrl deve ser criado")
	}
	_, err := api.ListMCPServers()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireSignalAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewSignal()
	SetSignalAPI(a, api)

	a.wireSignal()

	if a.signalCtrl == nil {
		t.Fatal("signalCtrl deve ser criado")
	}
	_, err := api.SignalListAccounts("http://x", "")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireTerminalAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		terminalMgr: terminal.NewManager(terminal.DefaultManagerConfig(), func(string, any) {}),
	}
	api := wailsapi.NewTerminal()
	SetTerminalAPI(a, api)

	a.wireTerminal()

	if a.terminalCtrl == nil {
		t.Fatal("terminalCtrl deve ser criado")
	}
	_, err := api.ListTerminalSessions()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireMemoryAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		memorySvc: memory.NewService(nil),
	}
	api := wailsapi.NewMemory()
	SetMemoryAPI(a, api)

	a.wireMemory()

	if a.memoryCtrl == nil {
		t.Fatal("memoryCtrl deve ser criado")
	}
	_, err := api.ListMemoryRecords(memory.Filter{})
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireDatabaseAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewDatabase()
	SetDatabaseAPI(a, api)

	a.wireSettings()
	a.wireDatabase()

	if a.settingsCtrl == nil {
		t.Fatal("settingsCtrl deve existir para o bind Database")
	}
	_, err := api.GetMaintenanceSettings()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireWelcomeAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewWelcome()
	SetWelcomeAPI(a, api)

	a.wireWelcome()

	if a.welcomeCtrl == nil {
		t.Fatal("welcomeCtrl deve ser criado")
	}
	// Sem master key/db: fail-safe true (Attach não exige sessão).
	if !api.NeedsWelcomeWizard() {
		t.Fatal("NeedsWelcomeWizard após Attach parcial deve permanecer fail-safe true sem master key/db")
	}
}

func TestWireLegacyCleanupAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewLegacyCleanup()
	SetLegacyCleanupAPI(a, api)

	a.wireLegacyCleanup()

	_, err := api.CleanupLegacyChannelJSON(apidto.CleanupLegacyChannelJSONOptions{})
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireTasklistAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		taskListCtrl: controllers.NewTaskListController(controllers.TaskListControllerConfig{}),
	}
	api := wailsapi.NewTasklist()
	SetTasklistAPI(a, api)

	a.wireTasklist()

	_, err := api.GetAllTaskLists()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireConversationsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		conversationsCtrl: controllers.NewConversationsController(controllers.ConversationsControllerConfig{}),
	}
	api := wailsapi.NewConversations()
	SetConversationsAPI(a, api)

	a.wireConversations()

	_, err := api.GetConversations()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireSpeechAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		speechCtrl: controllers.NewSpeechController(controllers.SpeechControllerConfig{}),
	}
	api := wailsapi.NewSpeech()
	SetSpeechAPI(a, api)

	a.wireSpeech()

	_, err := api.GetSpeechProviders()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireWorkspaceAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		workspaceCtrl: controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{}),
	}
	api := wailsapi.NewWorkspace()
	SetWorkspaceAPI(a, api)

	a.wireWorkspace()

	_, err := api.ListWorkspaces()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireMessagingAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		msgCtrl: controllers.NewMessagingController(controllers.MessagingControllerConfig{}),
	}
	api := wailsapi.NewMessaging()
	SetMessagingAPI(a, api)

	a.wireMessaging()

	_, err := api.GetMessagingStatus()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireEditorAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewEditor()
	SetEditorAPI(a, api)

	a.wireEditor()

	_, err := api.EditorLoadState()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireExportImportAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewExportImport()
	SetExportImportAPI(a, api)

	a.wireExportImport()

	_, err := api.ExportData(portability.ExportRequest{})
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

type fatalSpeechProfileProvider struct {
	t *testing.T
}

func (p fatalSpeechProfileProvider) GetActive() (*profiles.Profile, error) {
	p.t.Fatal("InitFromProfile não deve rodar sem userID no contexto")
	return nil, errors.New("unreachable")
}

func (p fatalSpeechProfileProvider) ResolveDefaults(ctx context.Context, profile *profiles.Profile) *profiles.Profile {
	p.t.Fatal("ResolveDefaults não deve rodar sem userID no contexto")
	return profile
}

func TestReinitSpeechFromActiveProfileSemSessaoNaoTocaNoCofre(t *testing.T) {
	t.Parallel()
	a := &App{
		ctx: context.Background(),
		speechSvc: speech.NewService(speech.ServiceConfig{
			ProfileProvider: fatalSpeechProfileProvider{t: t},
		}),
	}

	a.reinitSpeechFromActiveProfile("qualquer")
}

func TestWireTasklistActionsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		taskListCtrl: controllers.NewTaskListController(controllers.TaskListControllerConfig{}),
	}
	api := wailsapi.NewTasklistActions()
	SetTasklistActionsAPI(a, api)

	a.wireTasklistActions()

	_, err := api.GetTaskListCustomActions("list")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireSubagentAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		subagentMgr: subagent.NewManager(subagent.ManagerConfig{}),
	}
	api := wailsapi.NewSubagent()
	SetSubagentAPI(a, api)

	a.wireSubagent()

	_, err := api.ListSubAgentRuns(10)
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireACPProvidersAttachesBind(t *testing.T) {
	t.Parallel()
	mgr := acp.NewManager(acp.ManagerConfig{})
	t.Cleanup(mgr.Shutdown)
	a := &App{
		acpMgr: mgr,
	}
	api := wailsapi.NewACPProviders()
	SetACPProvidersAPI(a, api)

	a.wireACPProviders()

	_, err := api.DetectACPAgent("cursor")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireACPRegistryAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		acpRegistry: acpregistry.New(acpregistry.Config{}),
	}
	api := wailsapi.NewACPRegistry()
	SetACPRegistryAPI(a, api)

	a.wireACPRegistry()

	_, err := api.GetACPCatalog()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireACPInstallAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewACPInstall()
	SetACPInstallAPI(a, api)

	a.wireACPInstall()

	_, err := api.ACPAgentInstallPlan("codex-acp")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireACPCommandsAttachesBind(t *testing.T) {
	t.Parallel()
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	a := &App{acpMgr: mgr}
	api := wailsapi.NewACPCommands()
	SetACPCommandsAPI(a, api)

	a.wireACPCommands()

	_, err := api.GetAgentSessionCommands("conversa-1")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireACPOptionsAttachesBind(t *testing.T) {
	t.Parallel()
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	a := &App{acpMgr: mgr}
	api := wailsapi.NewACPOptions()
	SetACPOptionsAPI(a, api)

	a.wireACPOptions()

	_, err := api.GetAgentSessionOptions("conversa-1")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireACPWorkDirAttachesBind(t *testing.T) {
	t.Parallel()
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	a := &App{acpMgr: mgr}
	api := wailsapi.NewACPWorkDir()
	SetACPWorkDirAPI(a, api)

	a.wireACPWorkDir()

	_, err := api.GetAgentConversationWorkDir("conversa-1")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireACPTrustAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{acpTrust: acptrust.NewStoreWithDir(t.TempDir())}
	api := wailsapi.NewACPTrust()
	SetACPTrustAPI(a, api)

	a.wireACPTrust()

	_, err := api.GetAgentPermissions()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireLLMProvidersAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{}
	api := wailsapi.NewLLMProviders()
	SetLLMProvidersAPI(a, api)

	a.wireLLMProviders()

	if a.llmCtrl == nil {
		t.Fatal("llmCtrl deve ser criado")
	}
	_, err := api.GetLLMProviders()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireLLMModelsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		providerSvc:    providers.NewService(providers.ServiceConfig{}),
		profileManager: profiles.NewManager(),
	}
	api := wailsapi.NewLLMModels()
	SetLLMModelsAPI(a, api)

	a.wireLLMModels()

	_, err := api.GetModels()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}

func TestWireJobsAttachesBind(t *testing.T) {
	t.Parallel()
	a := &App{
		jobsCtrl: controllers.NewJobsController(controllers.JobsControllerConfig{}),
	}
	api := wailsapi.NewJobs()
	SetJobsAPI(a, api)

	a.wireJobs()

	_, err := api.GetJobs()
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem sessão: want ErrUserScopeRequired, got %v", err)
	}
}
