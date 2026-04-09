package controllers

import (
	"fmt"

	"assistente/internal/core/ports"
	"assistente/internal/workspace"
)

// WorkspaceControllerConfig agrupa as dependências do WorkspaceController.
type WorkspaceControllerConfig struct {
	WorkspaceMgr *workspace.Manager
	Emitter      ports.Emitter
}

// WorkspaceController é o Inbound Adapter para gerenciamento de workspaces e abas.
type WorkspaceController struct {
	workspaceMgr *workspace.Manager
	emitter      ports.Emitter
}

// NewWorkspaceController cria um WorkspaceController com as dependências injetadas.
func NewWorkspaceController(cfg WorkspaceControllerConfig) *WorkspaceController {
	return &WorkspaceController{
		workspaceMgr: cfg.WorkspaceMgr,
		emitter:      cfg.Emitter,
	}
}

func (c *WorkspaceController) GetActiveWorkspace() *workspace.Workspace {
	if c.workspaceMgr == nil {
		return nil
	}
	return c.workspaceMgr.Active()
}

func (c *WorkspaceController) ListWorkspaces() ([]workspace.WorkspaceInfo, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	return c.workspaceMgr.List()
}

func (c *WorkspaceController) CreateWorkspace(name string) (*workspace.Workspace, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := c.workspaceMgr.Create(name)
	if err != nil {
		return nil, err
	}
	c.emitter.Emit("workspace:created", ws)
	return ws, nil
}

func (c *WorkspaceController) SwitchWorkspace(workspaceID string) (*workspace.Workspace, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := c.workspaceMgr.Switch(workspaceID)
	if err != nil {
		return nil, err
	}
	c.emitter.Emit("workspace:switched", ws)
	return ws, nil
}

func (c *WorkspaceController) RenameWorkspace(newName string) error {
	if c.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := c.workspaceMgr.Rename(newName); err != nil {
		return err
	}
	c.emitter.Emit("workspace:renamed", c.workspaceMgr.Active())
	return nil
}

func (c *WorkspaceController) DeleteWorkspace(workspaceID string) error {
	if c.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := c.workspaceMgr.Delete(workspaceID); err != nil {
		return err
	}
	c.emitter.Emit("workspace:deleted", workspaceID)
	return nil
}

func (c *WorkspaceController) SetWorkspaceProfile(profileSlug string) error {
	if c.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return c.workspaceMgr.SetProfile(profileSlug)
}

func (c *WorkspaceController) SaveWorkspace() error {
	if c.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return c.workspaceMgr.Save()
}

func (c *WorkspaceController) AddWorkspaceTab(tab workspace.Tab) (*workspace.Workspace, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := c.workspaceMgr.AddTab(tab); err != nil {
		return nil, err
	}
	ws := c.workspaceMgr.Active()
	c.emitter.Emit("workspace:tab_added", ws)
	return ws, nil
}

func (c *WorkspaceController) RemoveWorkspaceTab(tabID string) (*workspace.Workspace, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := c.workspaceMgr.RemoveTab(tabID); err != nil {
		return nil, err
	}
	ws := c.workspaceMgr.Active()
	c.emitter.Emit("workspace:tab_removed", ws)
	return ws, nil
}

func (c *WorkspaceController) SetActiveWorkspaceTab(tabID string) error {
	if c.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := c.workspaceMgr.SetActiveTab(tabID); err != nil {
		return err
	}
	c.emitter.Emit("workspace:tab_activated", tabID)
	return nil
}

func (c *WorkspaceController) UpdateWorkspaceTab(tabID string, updates map[string]any) error {
	if c.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return c.workspaceMgr.UpdateTab(tabID, updates)
}

func (c *WorkspaceController) ReorderWorkspaceTabs(orderedIDs []string) error {
	if c.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return c.workspaceMgr.ReorderTabs(orderedIDs)
}

func (c *WorkspaceController) MoveWorkspaceTabTo(tabID, targetWorkspaceID string) (*workspace.Workspace, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := c.workspaceMgr.MoveTabToWorkspace(tabID, targetWorkspaceID); err != nil {
		return nil, err
	}
	ws := c.workspaceMgr.Active()
	c.emitter.Emit("workspace:tab_removed", ws)
	return ws, nil
}

func (c *WorkspaceController) ExportWorkspace() (string, error) {
	if c.workspaceMgr == nil {
		return "", fmt.Errorf("workspace manager not initialized")
	}
	data, err := c.workspaceMgr.ExportWorkspace()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *WorkspaceController) ImportWorkspace(yamlData string) (*workspace.Workspace, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := c.workspaceMgr.ImportWorkspace([]byte(yamlData))
	if err != nil {
		return nil, err
	}
	c.emitter.Emit("workspace:created", ws)
	return ws, nil
}
