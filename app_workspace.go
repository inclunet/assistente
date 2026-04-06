package main

import (
	"fmt"
	"log"
	"os"

	"assistente/internal/configdir"
	"assistente/internal/workspace"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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

// GetActiveWorkspace retorna o workspace ativo.
func (a *App) GetActiveWorkspace() *workspace.Workspace {
	if a.workspaceMgr == nil {
		return nil
	}
	return a.workspaceMgr.Active()
}

// ListWorkspaces retorna todos os workspaces conhecidos.
func (a *App) ListWorkspaces() ([]workspace.WorkspaceInfo, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.List()
}

// CreateWorkspace cria um novo workspace avulso.
func (a *App) CreateWorkspace(name string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.Create(name)
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:created", ws)
	return ws, nil
}

// SwitchWorkspace alterna para outro workspace.
func (a *App) SwitchWorkspace(workspaceID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.Switch(workspaceID)
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:switched", ws)
	return ws, nil
}

// RenameWorkspace renomeia o workspace ativo.
func (a *App) RenameWorkspace(newName string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.Rename(newName); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:renamed", a.workspaceMgr.Active())
	return nil
}

// DeleteWorkspace remove um workspace (não pode ser o ativo).
func (a *App) DeleteWorkspace(workspaceID string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.Delete(workspaceID); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:deleted", workspaceID)
	return nil
}

// SetWorkspaceProfile define o perfil base do workspace ativo.
func (a *App) SetWorkspaceProfile(profileSlug string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.SetProfile(profileSlug)
}

// SaveWorkspace persiste o estado do workspace ativo.
func (a *App) SaveWorkspace() error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.Save()
}

// --- Workspace Tab APIs ---

// AddWorkspaceTab adiciona uma aba ao workspace ativo.
func (a *App) AddWorkspaceTab(tab workspace.Tab) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.AddTab(tab); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_added", ws)
	return ws, nil
}

// RemoveWorkspaceTab remove uma aba do workspace ativo.
func (a *App) RemoveWorkspaceTab(tabID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.RemoveTab(tabID); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_removed", ws)
	return ws, nil
}

// SetActiveWorkspaceTab define a aba ativa no workspace.
func (a *App) SetActiveWorkspaceTab(tabID string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "workspace:tab_activated", tabID)
	return nil
}

// UpdateWorkspaceTab atualiza campos de uma aba.
func (a *App) UpdateWorkspaceTab(tabID string, updates map[string]any) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.UpdateTab(tabID, updates)
}

// ReorderWorkspaceTabs reordena as abas do workspace.
func (a *App) ReorderWorkspaceTabs(orderedIDs []string) error {
	if a.workspaceMgr == nil {
		return fmt.Errorf("workspace manager not initialized")
	}
	return a.workspaceMgr.ReorderTabs(orderedIDs)
}

// MoveWorkspaceTabTo move uma aba do workspace ativo para outro workspace.
func (a *App) MoveWorkspaceTabTo(tabID, targetWorkspaceID string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	if err := a.workspaceMgr.MoveTabToWorkspace(tabID, targetWorkspaceID); err != nil {
		return nil, err
	}
	ws := a.workspaceMgr.Active()
	runtime.EventsEmit(a.ctx, "workspace:tab_removed", ws)
	return ws, nil
}

// ExportWorkspace exporta o workspace ativo como YAML.
func (a *App) ExportWorkspace() (string, error) {
	if a.workspaceMgr == nil {
		return "", fmt.Errorf("workspace manager not initialized")
	}
	data, err := a.workspaceMgr.ExportWorkspace()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ImportWorkspace importa um workspace a partir de YAML.
func (a *App) ImportWorkspace(yamlData string) (*workspace.Workspace, error) {
	if a.workspaceMgr == nil {
		return nil, fmt.Errorf("workspace manager not initialized")
	}
	ws, err := a.workspaceMgr.ImportWorkspace([]byte(yamlData))
	if err != nil {
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "workspace:created", ws)
	return ws, nil
}
