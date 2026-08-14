package app

import (
	"assistente/controllers"
	"assistente/internal/logging"
	"assistente/internal/wailsapi"
)

// wireTokens monta o TokensController e associa o bind Wails (AEP-0088 Fase 4).
func (a *App) wireTokens() {
	a.tokensCtrl = controllers.NewTokensController(controllers.TokensControllerConfig{
		ProfileMgr: a.profileManager,
		TokenSvc:   a.tokenSvc,
	})
	if a.tokensAPI != nil {
		wailsapi.AttachTokens(a.tokensAPI, wailsSession{app: a}, a.tokensCtrl)
	}
}

// wireAllowlist monta o AllowlistController e associa o bind Wails (AEP-0088).
func (a *App) wireAllowlist() {
	a.allowlistCtrl = controllers.NewAllowlistController(controllers.AllowlistControllerConfig{
		AllowlistMgr:     a.allowlistMgr,
		QuestionnaireMgr: a.questionnaireMgr,
	})
	if a.allowlistsAPI != nil {
		wailsapi.AttachAllowlists(a.allowlistsAPI, wailsSession{app: a}, a.allowlistCtrl)
	}
}

// wireSkills monta o SkillsController e associa o bind Wails (AEP-0088).
func (a *App) wireSkills() {
	a.skillsCtrl = controllers.NewSkillsController(controllers.SkillsControllerConfig{
		SkillMgr:   a.skillMgr,
		ProfileMgr: a.profileManager,
		Emitter:    a.emitter,
	})
	if a.skillsAPI != nil {
		wailsapi.AttachSkills(a.skillsAPI, wailsSession{app: a}, a.skillsCtrl)
	}
}

// wireTools monta o ToolsController e associa o bind Wails (AEP-0088).
func (a *App) wireTools() {
	a.toolsCtrl = controllers.NewToolsController(controllers.ToolsControllerConfig{
		ToolRegistry: a.toolRegistry,
		MCPMgr:       a.mcpMgr,
	})
	if a.toolsAPI != nil {
		wailsapi.AttachTools(a.toolsAPI, wailsSession{app: a}, a.toolsCtrl)
	}
}

// wireUpdater monta o UpdaterController e associa o bind Wails (AEP-0088).
func (a *App) wireUpdater() {
	a.updaterCtrl = controllers.NewUpdaterController(controllers.UpdaterControllerConfig{
		Updater:          a.updater,
		Emitter:          a.emitter,
		QuestionnaireMgr: a.questionnaireMgr,
		ProviderSvc:      a.providerSvc,
		AppVersion:       AppVersion,
	})
	if a.updaterAPI != nil {
		wailsapi.AttachUpdater(a.updaterAPI, wailsSession{app: a}, a.updaterCtrl)
	}
}

// wireProfiles monta o ProfilesController e associa o bind Wails (AEP-0088).
func (a *App) wireProfiles() {
	a.profilesCtrl = controllers.NewProfilesController(controllers.ProfilesControllerConfig{
		ProfileMgr:       a.profileManager,
		Emitter:          a.emitter,
		ContextProviders: a.contextProviders,
		OnProfileChanged: func(slug string) {
			a.initLLMClient()
			if err := a.InitSpeechManagerFromProfile(); err != nil {
				logging.Errorf(a.ctx, "app.app_wire", "[Profile] Erro ao inicializar speech manager para perfil %s: %v", slug, err)
			}
			a.registerActiveProfileHotkeys()
		},
	})
	if a.profilesAPI != nil {
		wailsapi.AttachProfiles(a.profilesAPI, wailsSession{app: a}, a.profilesCtrl)
	}
}

// wireHotkeys monta o HotkeysController e associa o bind Wails (AEP-0088).
func (a *App) wireHotkeys() {
	a.hotkeyCtrl = controllers.NewHotkeysController(controllers.HotkeysControllerConfig{
		ProfileMgr: a.profileManager,
		Emitter:    a.emitter,
		WindowPort: a.windowPort,
	})
	if a.hotkeysAPI != nil {
		wailsapi.AttachHotkeys(a.hotkeysAPI, wailsSession{app: a}, a.hotkeyCtrl)
	}
}

// wireNetTrust monta o NetTrustController e associa o bind Wails (AEP-0088).
func (a *App) wireNetTrust() {
	a.netTrustCtrl = controllers.NewNetTrustController(controllers.NetTrustControllerConfig{
		NetTrustMgr: a.netTrustMgr,
		ProfileMgr:  a.profileManager,
	})
	if a.netTrustAPI != nil {
		wailsapi.AttachNetTrust(a.netTrustAPI, wailsSession{app: a}, a.netTrustCtrl)
	}
}

// wireCredentials monta o CredentialsController e associa o bind Wails (AEP-0088).
func (a *App) wireCredentials() {
	a.credentialsCtrl = controllers.NewCredentialsController(controllers.CredentialsControllerConfig{
		CredMgr: a.credMgr,
	})
	if a.credentialsAPI != nil {
		wailsapi.AttachCredentials(a.credentialsAPI, wailsSession{app: a}, a.credentialsCtrl)
	}
}

// wireSettings monta o SettingsController e associa o bind Wails (AEP-0088).
func (a *App) wireSettings() {
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
	if a.settingsAPI != nil {
		wailsapi.AttachSettings(a.settingsAPI, wailsSession{app: a}, a.settingsCtrl)
	}
}

// wireDatabase associa o bind Wails de database/manutenção (AEP-0088).
// Reusa settingsCtrl (montado em wireSettings) e callbacks ACP do App.
func (a *App) wireDatabase() {
	if a.databaseAPI != nil {
		wailsapi.AttachDatabase(
			a.databaseAPI,
			wailsSession{app: a},
			a.settingsCtrl,
			a.resetACPRuntime,
			a.closeAllACPSessions,
		)
	}
}

// wireMCP monta o MCPController e associa o bind Wails (AEP-0088).
func (a *App) wireMCP() {
	a.mcpCtrl = controllers.NewMCPController(a.mcpMgr, a.jobMgr, a.emitter)
	if a.mcpAPI != nil {
		wailsapi.AttachMCP(a.mcpAPI, wailsSession{app: a}, a.mcpCtrl)
	}
}

// wireSignal monta o SignalController e associa o bind Wails (AEP-0088).
func (a *App) wireSignal() {
	a.signalCtrl = controllers.NewSignalController()
	if a.signalAPI != nil {
		wailsapi.AttachSignal(a.signalAPI, wailsSession{app: a}, a.signalCtrl)
	}
}

// wireTerminal monta o TerminalController e associa o bind Wails (AEP-0088).
// initTerminalAndAllowlists (managers) e Shutdown/CloseAll permanecem no App.
func (a *App) wireTerminal() {
	a.terminalCtrl = controllers.NewTerminalController(controllers.TerminalControllerConfig{
		TerminalMgr: a.terminalMgr,
	})
	if a.terminalAPI != nil {
		wailsapi.AttachTerminal(a.terminalAPI, wailsSession{app: a}, a.terminalCtrl)
	}
}

// wireMemory monta o MemoryController e associa o bind Wails (AEP-0088).
func (a *App) wireMemory() {
	a.memoryCtrl = controllers.NewMemoryController(controllers.MemoryControllerConfig{
		MemorySvc: a.memorySvc,
	})
	if a.memoryAPI != nil {
		wailsapi.AttachMemory(a.memoryAPI, wailsSession{app: a}, a.memoryCtrl)
	}
}

// wireWelcome monta o WelcomeController e associa o bind Wails (AEP-0088).
func (a *App) wireWelcome() {
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
	if a.welcomeAPI != nil {
		wailsapi.AttachWelcome(a.welcomeAPI, wailsSession{app: a}, a.welcomeCtrl, welcomeRuntime{app: a})
	}
}

// wireLegacyCleanup associa o bind Wails de cleanup de JSON legado (AEP-0088).
// Sem controller: o bind chama channels.CleanupLegacyJSONFiles diretamente.
func (a *App) wireLegacyCleanup() {
	if a.legacyCleanupAPI != nil {
		wailsapi.AttachLegacyCleanup(a.legacyCleanupAPI, wailsSession{app: a})
	}
}

// wireTasklistActions associa o bind Wails de custom actions (AEP-0088).
// Reusa taskListCtrl + jobMgr já montados; CRUD geral de tasklist permanece no App.
func (a *App) wireTasklistActions() {
	if a.tasklistActionsAPI != nil {
		wailsapi.AttachTasklistActions(a.tasklistActionsAPI, wailsSession{app: a}, a.taskListCtrl, a.jobMgr)
	}
}

// wireSubagent associa o bind Wails ao Manager já criado no startup (AEP-0088).
// subagentParentDelivery e a criação do Manager permanecem no App.
func (a *App) wireSubagent() {
	if a.subagentAPI != nil {
		wailsapi.AttachSubagent(a.subagentAPI, wailsSession{app: a}, a.subagentMgr)
	}
}
