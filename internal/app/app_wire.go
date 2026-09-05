package app

import (
	"assistente/controllers"
	"assistente/internal/acpinstall"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/wailsapi"
	"context"
	"fmt"
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
			a.reinitSpeechFromActiveProfile(slug)
			a.registerActiveProfileHotkeys()
		},
	})
	if a.profilesAPI != nil {
		wailsapi.AttachProfiles(a.profilesAPI, wailsSession{app: a}, a.profilesCtrl)
	}
}

// reinitSpeechFromActiveProfile reinicia o speech manager após troca de perfil.
// Precisa do userID: InitFromProfile resolve API keys no cofre com
// GetByPatternWithContext. Sem sessão autenticada não tenta — a.ctx puro
// deixaria TTS/STT sem credencial mesmo com chave cadastrada (comportamento
// que InitSpeechManagerFromProfile no App já garantia).
func (a *App) reinitSpeechFromActiveProfile(slug string) {
	if a == nil || a.speechSvc == nil {
		return
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		logging.Errorf(a.ctx, "app.app_wire", "[Profile] Sem sessão para reinicializar speech no perfil %s: %v", slug, err)
		return
	}
	if err := a.speechSvc.InitFromProfile(ctx); err != nil {
		logging.Errorf(ctx, "app.app_wire", "[Profile] Erro ao inicializar speech manager para perfil %s: %v", slug, err)
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

// wireFSTrust monta o FSTrustController e associa o bind Wails (AEP-0092).
func (a *App) wireFSTrust() {
	a.fsTrustCtrl = controllers.NewFSTrustController(controllers.FSTrustControllerConfig{
		FSTrustMgr: a.fsTrustMgr,
		ProfileMgr: a.profileManager,
	})
	if a.fsTrustAPI != nil {
		wailsapi.AttachFSTrust(a.fsTrustAPI, wailsSession{app: a}, a.fsTrustCtrl)
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
		CredMgr:    a.credMgr,
		ProfileMgr: a.profileManager,
		SkillMgr:   a.skillMgr,
		Emitter:    a.emitter,
		RestartChannel: func(channelName string) error {
			// Via Messaging bind: WithUser + SetCredentialUserID + ownership
			// (não chamar msgCtrl.RestartChannel direto — perde escopo de credenciais).
			if a.messagingAPI == nil {
				return fmt.Errorf("messaging API não inicializado")
			}
			return a.messagingAPI.RestartChannel(channelName)
		},
		GetModels: func() ([]string, error) {
			if a.llmModelsAPI != nil {
				return a.llmModelsAPI.GetModels()
			}
			ctx, err := a.requireAuthenticatedContext()
			if err != nil {
				return nil, err
			}
			activeProfile, _ := a.profileManager.GetActive()
			return a.providerSvc.GetModels(ctx, activeProfile)
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
		UpdaterCtrl:                a.updaterCtrl,
		ConfigureCredentialManager: a.configureCredentialManager,
		InitLLMClient:              a.initLLMClient,
		SaveLLMProviders:           a.saveLLMProviders,
	})
	if a.welcomeAPI != nil {
		wailsapi.AttachWelcome(a.welcomeAPI, wailsSession{app: a}, a.welcomeCtrl, welcomeRuntime{app: a})
	}
}

// wireWorkspace associa o bind Wails de workspace/tabs (AEP-0088).
// Reusa workspaceCtrl criado em initWorkspace; eventos workspace:* permanecem no App.
func (a *App) wireWorkspace() {
	if a.workspaceAPI != nil {
		wailsapi.AttachWorkspace(a.workspaceAPI, wailsSession{app: a}, a.workspaceCtrl)
	}
}

// wireMessaging associa o bind Wails de messaging/canais/contatos (AEP-0088).
// Reusa msgCtrl criado em initMessaging; StartAdapters permanece no App.
func (a *App) wireMessaging() {
	if a.messagingAPI != nil {
		wailsapi.AttachMessaging(a.messagingAPI, wailsSession{app: a}, a.msgCtrl)
	}
}

// wireEditor associa o bind Wails do domínio editor (AEP-0088).
// Watcher, eventos editor:fileChanged e assisted writes permanecem no App via hooks.
func (a *App) wireEditor() {
	if a.editorAPI != nil {
		wailsapi.AttachEditor(a.editorAPI, wailsSession{app: a}, wailsapi.EditorHooks{
			AppContext:    func() context.Context { return a.ctx },
			Dialog:        func() ports.SystemDialogPort { return a.dialogPort },
			MarkSelfWrite: a.markEditorSelfWrite,
			WatchFile:     a.editorWatchFile,
			UnwatchFile:   a.editorUnwatchFile,
		})
	}
}

// wireExportImport associa o bind Wails de export/import (AEP-0088).
// Na CLI o bind é criado aqui (main GUI já registra via SetExportImportAPI).
func (a *App) wireExportImport() {
	if a.exportImportAPI == nil {
		a.exportImportAPI = wailsapi.NewExportImport()
	}
	wailsapi.AttachExportImport(
		a.exportImportAPI,
		wailsSession{app: a},
		a.credMgr,
		func() ports.SystemDialogPort { return a.dialogPort },
		AppVersion,
	)
}

// wireLegacyCleanup associa o bind Wails de cleanup de JSON legado (AEP-0088).
// Sem controller: o bind chama channels.CleanupLegacyJSONFiles diretamente.
func (a *App) wireLegacyCleanup() {
	if a.legacyCleanupAPI != nil {
		wailsapi.AttachLegacyCleanup(a.legacyCleanupAPI, wailsSession{app: a})
	}
}

// wireTasklist associa o bind Wails de CRUD de tasklist (AEP-0088).
// Reusa taskListCtrl já montado; helpers internos permanecem no App.
func (a *App) wireTasklist() {
	if a.tasklistAPI != nil {
		wailsapi.AttachTasklist(a.tasklistAPI, wailsSession{app: a}, a.taskListCtrl)
	}
}

// wireConversations associa o bind Wails de conversations/persistência (AEP-0088).
// Reusa conversationsCtrl; helpers reset* permanecem no App.
func (a *App) wireConversations() {
	if a.conversationsAPI != nil {
		wailsapi.AttachConversations(a.conversationsAPI, wailsSession{app: a}, a.conversationsCtrl)
	}
}

// wireSpeech associa o bind Wails de speech/TTS/STT (AEP-0088).
// Reusa speechCtrl; helpers dispatchSpeechEvent/resolveSpeechProfile permanecem no App.
func (a *App) wireSpeech() {
	if a.speechAPI != nil {
		wailsapi.AttachSpeech(a.speechAPI, wailsSession{app: a}, a.speechCtrl, speechDispatcher{app: a})
	}
}

// wireTasklistActions associa o bind Wails de custom actions (AEP-0088).
// Reusa taskListCtrl + jobMgr já montados; domínio separado do CRUD de tasklist.
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

// wireJobs associa o bind Wails de jobs após NewJobsController (AEP-0088).
// initJobs (Manager) permanece no App; dry-run MCP e custom action events via hooks.
func (a *App) wireJobs() {
	if a.jobsAPI != nil {
		wailsapi.AttachJobs(
			a.jobsAPI,
			wailsSession{app: a},
			a.jobsCtrl,
			a.mcpMgr,
			a.customActionEventNames,
		)
	}
}

// wireLLMProviders monta o LLMController e associa o bind Wails (AEP-0088).
func (a *App) wireLLMProviders() {
	a.llmCtrl = controllers.NewLLMController(controllers.LLMControllerConfig{
		LLMRegistry:      a.llmRegistry,
		ProfileMgr:       a.profileManager,
		ProviderSvc:      a.providerSvc,
		Emitter:          a.emitter,
		OnProviderChange: a.initLLMClient,
	})
	if a.llmProvidersAPI != nil {
		wailsapi.AttachLLMProviders(a.llmProvidersAPI, wailsSession{app: a}, a.llmCtrl, wailsapi.LLMProvidersHooks{
			ApplyInstalledBinaryEnv: a.applyInstalledBinaryEnv,
			ReloadClient:            a.initLLMClient,
			PersistDelete: func(ctx context.Context, id string) error {
				return database.DeleteLLMProviderWithContext(ctx, id)
			},
			CreateDefault: a.createDefaultLLMProvider,
		})
	}
}

// wireLLMModels associa o bind Wails de catálogo/refresh/cancel (AEP-0088).
// streamMgr permanece no App; cancel entra via hook (sem duplicar estado).
// Na CLI o bind é criado aqui (main GUI já registra via SetLLMModelsAPI).
func (a *App) wireLLMModels() {
	if a.llmModelsAPI == nil {
		a.llmModelsAPI = wailsapi.NewLLMModels()
	}
	wailsapi.AttachLLMModels(
		a.llmModelsAPI,
		wailsSession{app: a},
		a.providerSvc,
		a.profileManager,
		wailsapi.LLMModelsHooks{
			CancelStreaming: func(conversationID string) {
				CancelStreamingForConversation(a, conversationID)
			},
		},
	)
}

// wireChat associa o bind Wails de SendMessage/RetryMessage (AEP-0040, AEP-0088).
// ChatController interno, streamMgr e sendMessageFromChannel permanecem no App.
// Na CLI o bind é criado aqui (main GUI já registra via SetChatAPI).
func (a *App) wireChat() {
	if a.chatAPI == nil {
		a.chatAPI = wailsapi.NewChat()
	}
	wailsapi.AttachChat(a.chatAPI, wailsSession{app: a}, a.chatCtrl)
}

// wireACPCommands associa o bind Wails ao Manager ACP já criado em initACP (AEP-0088).
// agentSessionCommandsChanged permanece no App.
func (a *App) wireACPCommands() {
	if a.acpCommandsAPI != nil {
		wailsapi.AttachACPCommands(a.acpCommandsAPI, wailsSession{app: a}, a.acpMgr)
	}
}

// wireACPProviders associa o bind Wails de detect/test de agentes ACP (AEP-0088).
// initACP (Manager) permanece no App.
func (a *App) wireACPProviders() {
	if a.acpProvidersAPI != nil {
		wailsapi.AttachACPProviders(a.acpProvidersAPI, wailsSession{app: a}, a.acpMgr, a.acpWorkDir)
	}
}

// wireACPOptions associa o bind Wails de opções de sessão ACP (AEP-0088).
// agentSessionOptionsChanged e noticePermissionBarrier permanecem no App.
func (a *App) wireACPOptions() {
	if a.acpOptionsAPI != nil {
		wailsapi.AttachACPOptions(a.acpOptionsAPI, wailsSession{app: a}, a.acpMgr, a.noticePermissionBarrier)
	}
}

// wireACPRegistry associa o bind Wails do catálogo ACP (AEP-0088).
// Helpers de montagem (acpCatalogOf) permanecem no App.
func (a *App) wireACPRegistry() {
	if a.acpRegistryAPI != nil {
		wailsapi.AttachACPRegistry(a.acpRegistryAPI, wailsSession{app: a}, a.acpRegistry, a.acpCatalogOf)
	}
}

// wireACPWorkDir associa o bind Wails de diretório por conversa (AEP-0088).
// agentConversationDir / conversationAgentDir / acpWorkDir permanecem no App.
func (a *App) wireACPWorkDir() {
	if a.acpWorkDirAPI != nil {
		wailsapi.AttachACPWorkDir(a.acpWorkDirAPI, wailsSession{app: a}, a.acpMgr)
	}
}

// wireACPInstall associa o bind Wails de install/update/remove de agentes ACP
// (AEP-0088). Handshake, progresso e repontar provedores permanecem no App.
func (a *App) wireACPInstall() {
	if a.acpInstallAPI != nil {
		wailsapi.AttachACPInstall(a.acpInstallAPI, wailsSession{app: a}, wailsapi.ACPInstallHooks{
			Installer: func() *acpinstall.Installer {
				return a.acpCatalogServices().installer
			},
			ProvidersFrom:            a.acpProvidersFrom,
			RefuseUpdateDuringTurn:   a.refuseUpdateDuringTurn,
			RepointProviders:         a.repointACPProviders,
			RemoveSupersededVersions: a.removeSupersededVersions,
		})
	}
}

// wireACPTrust associa o bind Wails de autorizações permanentes ACP (AEP-0088).
// Handlers de permissão em tempo de turno (app_acp_permissions.go) permanecem no App.
func (a *App) wireACPTrust() {
	if a.acpTrustAPI != nil {
		wailsapi.AttachACPTrust(a.acpTrustAPI, wailsSession{app: a}, a.acpTrust, a.profileNames)
	}
}
