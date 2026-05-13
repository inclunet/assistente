package app

import (
	"assistente/controllers"
	"assistente/internal/configdir"
	"assistente/internal/workspace"
	"fmt"
	"log"
	"os"
)

// ============================================================================
// Workspace Management API
// ============================================================================

func (a *App) initWorkspace() {
	homeDir := configdir.GetHomeDir()
	a.workspaceMgr = workspace.NewManager(homeDir)

	workDir := ""
	if wd, err := os.Getwd(); err == nil {
		workDir = wd
	}

	if err := a.workspaceMgr.Initialize(workDir); err != nil {
		log.Printf("Erro ao inicializar workspace: %v", err)
	} else if ws := a.workspaceMgr.Active(); ws != nil {
		log.Printf("Workspace ativo: %s (%s)", ws.Name, ws.ID)
	}

	a.workspaceCtrl = controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{
		WorkspaceMgr: a.workspaceMgr,
		Emitter:      a.emitter,
	})
}

func (a *App) workspaceController() (*controllers.WorkspaceController, error) {
	if a.workspaceCtrl == nil {
		return nil, fmt.Errorf("workspace controller not initialized")
	}
	return a.workspaceCtrl, nil
}

func (a *App) GetActiveWorkspace() *workspace.Workspace {
	if a.workspaceCtrl == nil {
		return nil
	}
	return a.workspaceCtrl.GetActiveWorkspace()
}
func (a *App) ListWorkspaces() ([]workspace.WorkspaceInfo, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return nil, err
	}
	return ctrl.ListWorkspaces()
}
func (a *App) CreateWorkspace(name string) (*workspace.Workspace, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return nil, err
	}
	return ctrl.CreateWorkspace(name)
}
func (a *App) SwitchWorkspace(workspaceID string) (*workspace.Workspace, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return nil, err
	}
	return ctrl.SwitchWorkspace(workspaceID)
}
func (a *App) RenameWorkspace(newName string) error {
	ctrl, err := a.workspaceController()
	if err != nil {
		return err
	}
	return ctrl.RenameWorkspace(newName)
}
func (a *App) DeleteWorkspace(workspaceID string) error {
	ctrl, err := a.workspaceController()
	if err != nil {
		return err
	}
	return ctrl.DeleteWorkspace(workspaceID)
}
func (a *App) SetWorkspaceProfile(profileSlug string) error {
	ctrl, err := a.workspaceController()
	if err != nil {
		return err
	}
	return ctrl.SetWorkspaceProfile(profileSlug)
}
func (a *App) SaveWorkspace() error {
	ctrl, err := a.workspaceController()
	if err != nil {
		return err
	}
	return ctrl.SaveWorkspace()
}

func (a *App) AddWorkspaceTab(tab workspace.Tab) (*workspace.Workspace, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return nil, err
	}
	return ctrl.AddWorkspaceTab(tab)
}
func (a *App) RemoveWorkspaceTab(tabID string) (*workspace.Workspace, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return nil, err
	}
	return ctrl.RemoveWorkspaceTab(tabID)
}
func (a *App) SetActiveWorkspaceTab(tabID string) error {
	ctrl, err := a.workspaceController()
	if err != nil {
		return err
	}
	return ctrl.SetActiveWorkspaceTab(tabID)
}
func (a *App) UpdateWorkspaceTab(tabID string, updates map[string]any) error {
	ctrl, err := a.workspaceController()
	if err != nil {
		return err
	}
	return ctrl.UpdateWorkspaceTab(tabID, updates)
}
func (a *App) ReorderWorkspaceTabs(orderedIDs []string) error {
	ctrl, err := a.workspaceController()
	if err != nil {
		return err
	}
	return ctrl.ReorderWorkspaceTabs(orderedIDs)
}
func (a *App) MoveWorkspaceTabTo(tabID, targetWorkspaceID string) (*workspace.Workspace, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return nil, err
	}
	return ctrl.MoveWorkspaceTabTo(tabID, targetWorkspaceID)
}
func (a *App) ExportWorkspace() (string, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return "", err
	}
	return ctrl.ExportWorkspace()
}
func (a *App) ImportWorkspace(yamlData string) (*workspace.Workspace, error) {
	ctrl, err := a.workspaceController()
	if err != nil {
		return nil, err
	}
	return ctrl.ImportWorkspace(yamlData)
}
