package app

import (
	"assistente/controllers"
	"assistente/internal/configdir"
	"assistente/internal/logging"
	"assistente/internal/workspace"
	"context"
	"os"
)

// ============================================================================
// Workspace — ciclo de vida interno (AEP-0088)
// ============================================================================
// A superfície Wails pública vive em wailsapi.Workspace.
// Aqui ficam init do manager/controller e o uso interno de workspaceMgr.

func (a *App) initWorkspace() {
	homeDir := configdir.GetHomeDir()
	a.workspaceMgr = workspace.NewManager(homeDir)

	workDir := ""
	if wd, err := os.Getwd(); err == nil {
		workDir = wd
	}

	if err := a.workspaceMgr.Initialize(workDir); err != nil {
		logging.Errorf(context.Background(), "app.app-workspace", "Erro ao inicializar workspace: %v", err)
	} else if ws := a.workspaceMgr.Active(); ws != nil {
		logging.Infof(context.Background(), "app.app-workspace", "Workspace ativo: %s (%s)", ws.Name, ws.ID)
	}

	a.workspaceCtrl = controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{
		WorkspaceMgr: a.workspaceMgr,
		Emitter:      a.emitter,
	})
}
