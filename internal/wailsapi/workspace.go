package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/workspace"
	"context"
	"sync"
)

// Workspace é o bind Wails do domínio workspace / tabs (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
// Workspace é sensível: nenhum método aqui roda sem sessão autenticada, mesmo
// que a versão anterior no *App não autenticasse (fail-closed corrigido na
// borda).
//
// initWorkspace (managers/controller) e o protocolo de eventos workspace:*
// continuam no *App — fora do escopo desta migração de superfície Wails.
type Workspace struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.WorkspaceController
}

// NewWorkspace cria o bind vazio; AttachWorkspace preenche session + controller no startup.
func NewWorkspace() *Workspace {
	return &Workspace{}
}

// AttachWorkspace associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachWorkspace(api *Workspace, session Session, ctrl *controllers.WorkspaceController) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
}

func (api *Workspace) deps() (Session, *controllers.WorkspaceController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, ErrWorkspaceNotWired
	}
	return api.session, api.ctrl, nil
}

// GetActiveWorkspace retorna o workspace ativo.
func (api *Workspace) GetActiveWorkspace() (*workspace.Workspace, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*workspace.Workspace, error) {
		return ctrl.GetActiveWorkspace(), nil
	})
}

// ListWorkspaces lista workspaces conhecidos.
func (api *Workspace) ListWorkspaces() ([]workspace.WorkspaceInfo, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]workspace.WorkspaceInfo, error) {
		return ctrl.ListWorkspaces()
	})
}

// CreateWorkspace cria um workspace.
func (api *Workspace) CreateWorkspace(name string) (*workspace.Workspace, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*workspace.Workspace, error) {
		return ctrl.CreateWorkspace(name)
	})
}

// SwitchWorkspace troca o workspace ativo.
func (api *Workspace) SwitchWorkspace(workspaceID string) (*workspace.Workspace, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*workspace.Workspace, error) {
		return ctrl.SwitchWorkspace(workspaceID)
	})
}

// RenameWorkspace renomeia o workspace ativo.
func (api *Workspace) RenameWorkspace(newName string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RenameWorkspace(newName)
	})
	return err
}

// DeleteWorkspace remove um workspace.
func (api *Workspace) DeleteWorkspace(workspaceID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteWorkspace(workspaceID)
	})
	return err
}

// SetWorkspaceProfile associa um perfil ao workspace ativo.
func (api *Workspace) SetWorkspaceProfile(profileSlug string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetWorkspaceProfile(profileSlug)
	})
	return err
}

// SaveWorkspace persiste o workspace ativo.
func (api *Workspace) SaveWorkspace() error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SaveWorkspace()
	})
	return err
}

// AddWorkspaceTab adiciona uma aba ao workspace ativo.
func (api *Workspace) AddWorkspaceTab(tab workspace.Tab) (*workspace.Workspace, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*workspace.Workspace, error) {
		return ctrl.AddWorkspaceTab(tab)
	})
}

// RemoveWorkspaceTab remove uma aba do workspace ativo.
func (api *Workspace) RemoveWorkspaceTab(tabID string) (*workspace.Workspace, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*workspace.Workspace, error) {
		return ctrl.RemoveWorkspaceTab(tabID)
	})
}

// SetActiveWorkspaceTab define a aba ativa.
func (api *Workspace) SetActiveWorkspaceTab(tabID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetActiveWorkspaceTab(tabID)
	})
	return err
}

// UpdateWorkspaceTab atualiza campos de uma aba.
func (api *Workspace) UpdateWorkspaceTab(tabID string, updates map[string]any) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateWorkspaceTab(tabID, updates)
	})
	return err
}

// ReorderWorkspaceTabs reordena abas do workspace ativo.
func (api *Workspace) ReorderWorkspaceTabs(orderedIDs []string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ReorderWorkspaceTabs(orderedIDs)
	})
	return err
}

// MoveWorkspaceTabTo move uma aba para outro workspace.
func (api *Workspace) MoveWorkspaceTabTo(tabID, targetWorkspaceID string) (*workspace.Workspace, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*workspace.Workspace, error) {
		return ctrl.MoveWorkspaceTabTo(tabID, targetWorkspaceID)
	})
}

// ExportWorkspace exporta o workspace ativo como YAML.
func (api *Workspace) ExportWorkspace() (string, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.ExportWorkspace()
	})
}

// ImportWorkspace importa um workspace a partir de YAML.
func (api *Workspace) ImportWorkspace(yamlData string) (*workspace.Workspace, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*workspace.Workspace, error) {
		return ctrl.ImportWorkspace(yamlData)
	})
}
