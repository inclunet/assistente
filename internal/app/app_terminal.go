package app

import (
	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/configdir"
	"assistente/internal/fstrust"
	"assistente/internal/logging"
	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
	"assistente/internal/shelltrust"
	"assistente/internal/terminal"
	"assistente/internal/trustscope"
	"context"
	"path/filepath"
)

// ============================================================================
// Terminal Management API — migrada para wailsapi.Terminal (AEP-0088).
//
// Os 8 métodos Wails (List/Create/Close/GetHistory/Run/SendInput/Interrupt/
// GetStats) saíram do *App; ver internal/wailsapi/terminal.go. Este arquivo
// mantém só o wiring de managers que permanece no App (ciclo de vida).
// ============================================================================

// initTerminalAndAllowlists inicializa os managers de terminal, questionário e allowlists.
func (a *App) initTerminalAndAllowlists() {
	emitEvent := func(event string, data any) {
		a.emitter.Emit(event, data)
	}

	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), emitEvent)
	a.questionnaireMgr = questionnaire.NewManager(emitEvent)
	a.allowlistMgr = allowlist.NewManager(allowlist.Options{MayImport: authorization.LocalLegacyOwner})

	a.netTrustMgr = nettrust.NewManager(trustscope.Options{MayImport: authorization.LocalLegacyOwner})
	a.fsTrustMgr = fstrust.NewManager(trustscope.Options{MayImport: authorization.LocalLegacyOwner})
	a.shellTrustMgr = shelltrust.NewManager(trustscope.Options{})
	workspaceDir := func() string {
		if a.workspaceMgr != nil {
			if root := a.workspaceMgr.ActivePath(); root != "" {
				return filepath.Join(root, ".assistente")
			}
		}
		return configdir.GetWorkDir()
	}
	profileSlug := func() string {
		if a.profileManager != nil {
			return a.profileManager.GetActiveSlug()
		}
		return ""
	}
	// Os callbacks só resolvem contexto de gestão. Operações capturam a origem
	// no invocationctx antes de serem enfileiradas e mantêm essa identidade.
	for _, manager := range []interface {
		SetWorkspaceDirFunc(func() string)
		SetActiveProfileSlugFunc(func() string)
	}{a.netTrustMgr, a.fsTrustMgr, a.shellTrustMgr} {
		manager.SetWorkspaceDirFunc(workspaceDir)
		manager.SetActiveProfileSlugFunc(profileSlug)
	}
	logging.Infof(context.Background(), "app.app-terminal", "[Terminal] Managers de terminal, questionário e allowlist inicializados")
}
