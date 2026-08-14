package app

import (
	"errors"
	"testing"

	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/memory"
	"assistente/internal/profiles"
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
