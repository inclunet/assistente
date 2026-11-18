package app

import (
	"assistente/controllers"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/workspace"
	"context"
	"fmt"
	"os"
)

// ============================================================================
// Workspace — ciclo de vida interno (AEP-0088)
// ============================================================================
// A superfície Wails pública vive em wailsapi.Workspace.
// Aqui ficam init do manager/controller e o uso interno de workspaceMgr.

// SetStartupWorkspaceDirectory define a pasta solicitada pelo desktop.
// Vazio restaura o último workspace; não altera cwd, configuração nem banco.
func SetStartupWorkspaceDirectory(a *App, directory string) {
	a.startupWorkspaceDir = &directory
}

// initialToolWorkDir fornece a pasta inicial às tools sem mudar o cwd global.
func (a *App) initialToolWorkDir() (string, error) {
	if a.workspaceMgr != nil {
		if path := a.workspaceMgr.ActivePath(); path != "" {
			return path, nil
		}
	}
	return os.Getwd()
}

func (a *App) initWorkspace() error {
	homeDir := configdir.GetHomeDir()
	a.workspaceMgr = workspace.NewManager(homeDir, database.DB)

	workDir := ""
	if a.startupWorkspaceDir != nil {
		workDir = *a.startupWorkspaceDir
	} else if wd, err := os.Getwd(); err == nil {
		workDir = wd
	}

	if err := a.workspaceMgr.Initialize(workDir); err != nil {
		if a.startupWorkspaceDir != nil {
			return fmt.Errorf("workspace: %w", err)
		}
		// Consumidores não desktop conservam o startup tolerante a falhas.
		logging.Errorf(context.Background(), "app.app-workspace", "Erro ao inicializar workspace: %v", err)
	} else if ws := a.workspaceMgr.Active(); ws != nil {
		logging.Infof(context.Background(), "app.app-workspace", "Workspace ativo: %s (%s)", ws.Name, ws.ID)
	}

	a.configureWorkspaceController()
	return nil
}

func (a *App) configureWorkspaceController() {
	a.workspaceCtrl = controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{
		WorkspaceMgr:        a.workspaceMgr,
		Emitter:             a.emitter,
		OnWorkspaceSwitched: a.reloadCommandsAfterWorkspaceSwitch,
	})
}
