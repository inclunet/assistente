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

func (a *App) initWorkspace() error {
	homeDir := configdir.GetHomeDir()
	a.workspaceMgr = workspace.NewManager(homeDir)

	if err := a.workspaceMgr.Initialize(a.workspaceStartupDir()); err != nil {
		logging.Errorf(context.Background(), "app.app-workspace", "Erro ao inicializar workspace: %v", err)
		return err
	} else if ws := a.workspaceMgr.Active(); ws != nil {
		logging.Infof(context.Background(), "app.app-workspace", "Workspace ativo: %s (%s)", ws.Name, ws.ID)
	}

	a.configureWorkspaceController()
	return nil
}

func (a *App) workspaceStartupDir() string {
	if a.desktopWorkspaceSelection {
		return a.startupWorkspacePath
	}
	workDir, _ := os.Getwd()
	return workDir
}

// SetStartupWorkspacePath define a pasta escolhida antes do startup desktop.
func SetStartupWorkspacePath(a *App, path string) {
	a.startupWorkspacePath = path
	a.desktopWorkspaceSelection = true
}

func (a *App) configureWorkspaceController() {
	a.workspaceCtrl = controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{
		WorkspaceMgr:        a.workspaceMgr,
		Emitter:             a.emitter,
		OnWorkspaceSwitched: a.reloadCommandsAfterWorkspaceSwitch,
	})
}
