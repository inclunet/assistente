package controllers

import (
	"context"
	"fmt"
	"sync"

	"assistente/internal/core/ports"
	"assistente/internal/workspace"
)

// WorkspaceControllerConfig agrupa as dependências do WorkspaceController.
type WorkspaceControllerConfig struct {
	WorkspaceMgr *workspace.Manager
	Emitter      ports.Emitter
	// OnWorkspaceSwitched é chamado após a troca persistida e antes do evento
	// público, para o bootstrap interno reconstruir a geração dependente.
	OnWorkspaceSwitched func()
}

// WorkspaceController é o Inbound Adapter para gerenciamento de workspaces e abas.
type WorkspaceController struct {
	workspaceMgr        *workspace.Manager
	emitter             ports.Emitter
	switchMu            sync.Mutex
	onWorkspaceSwitched func()
}

// NewWorkspaceController cria um WorkspaceController com as dependências injetadas.
func NewWorkspaceController(cfg WorkspaceControllerConfig) *WorkspaceController {
	return &WorkspaceController{
		workspaceMgr:        cfg.WorkspaceMgr,
		emitter:             cfg.Emitter,
		onWorkspaceSwitched: cfg.OnWorkspaceSwitched,
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
	c.switchMu.Lock()
	defer c.switchMu.Unlock()
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := c.workspaceMgr.Switch(workspaceID)
	if err != nil {
		return nil, err
	}
	if c.onWorkspaceSwitched != nil {
		c.onWorkspaceSwitched()
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
	// O evento carrega uma reconciliação do estado atual, já carimbada pelo
	// Manager. Não é uma auditoria da operação nem depende do tabID isolado.
	c.emitter.Emit("workspace:tab_activated", c.workspaceMgr.Active())
	return nil
}

// SetActiveWorkspaceTabForWorkspace seleciona uma aba somente no workspace
// explicitamente capturado pelo caller e publica o snapshot resultante.
func (c *WorkspaceController) SetActiveWorkspaceTabForWorkspace(ctx context.Context, workspaceID, tabID string) (*workspace.Workspace, error) {
	if c.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := c.workspaceMgr.SetActiveWorkspaceTabForWorkspace(ctx, workspaceID, tabID)
	if err != nil {
		return nil, err
	}
	c.emitter.Emit("workspace:tab_activated", ws)
	return ws, nil
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
