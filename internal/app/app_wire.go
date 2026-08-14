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
