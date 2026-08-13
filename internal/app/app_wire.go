package app

import (
	"assistente/controllers"
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
