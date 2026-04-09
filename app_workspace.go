package main

import (
	"assistente/internal/configdir"
	"assistente/internal/workspace"
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
}

func (a *App) GetActiveWorkspace() *workspace.Workspace { return a.workspaceCtrl.GetActiveWorkspace() }
func (a *App) ListWorkspaces() ([]workspace.WorkspaceInfo, error) {
	return a.workspaceCtrl.ListWorkspaces()
}
func (a *App) CreateWorkspace(name string) (*workspace.Workspace, error) {
	return a.workspaceCtrl.CreateWorkspace(name)
}
func (a *App) SwitchWorkspace(workspaceID string) (*workspace.Workspace, error) {
	return a.workspaceCtrl.SwitchWorkspace(workspaceID)
}
func (a *App) RenameWorkspace(newName string) error { return a.workspaceCtrl.RenameWorkspace(newName) }
func (a *App) DeleteWorkspace(workspaceID string) error {
	return a.workspaceCtrl.DeleteWorkspace(workspaceID)
}
func (a *App) SetWorkspaceProfile(profileSlug string) error {
	return a.workspaceCtrl.SetWorkspaceProfile(profileSlug)
}
func (a *App) SaveWorkspace() error { return a.workspaceCtrl.SaveWorkspace() }

func (a *App) AddWorkspaceTab(tab workspace.Tab) (*workspace.Workspace, error) {
	return a.workspaceCtrl.AddWorkspaceTab(tab)
}
func (a *App) RemoveWorkspaceTab(tabID string) (*workspace.Workspace, error) {
	return a.workspaceCtrl.RemoveWorkspaceTab(tabID)
}
func (a *App) SetActiveWorkspaceTab(tabID string) error {
	return a.workspaceCtrl.SetActiveWorkspaceTab(tabID)
}
func (a *App) UpdateWorkspaceTab(tabID string, updates map[string]any) error {
	return a.workspaceCtrl.UpdateWorkspaceTab(tabID, updates)
}
func (a *App) ReorderWorkspaceTabs(orderedIDs []string) error {
	return a.workspaceCtrl.ReorderWorkspaceTabs(orderedIDs)
}
func (a *App) MoveWorkspaceTabTo(tabID, targetWorkspaceID string) (*workspace.Workspace, error) {
	return a.workspaceCtrl.MoveWorkspaceTabTo(tabID, targetWorkspaceID)
}
func (a *App) ExportWorkspace() (string, error) { return a.workspaceCtrl.ExportWorkspace() }
func (a *App) ImportWorkspace(yamlData string) (*workspace.Workspace, error) {
	return a.workspaceCtrl.ImportWorkspace(yamlData)
}
