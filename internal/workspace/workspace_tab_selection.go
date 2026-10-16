package workspace

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrWorkspaceSelectionNilContext     = errors.New("workspace selection: context nil")
	ErrWorkspaceSelectionWorkspaceStale = errors.New("workspace selection: workspace is not active")
	ErrWorkspaceSelectionTabNotFound    = errors.New("workspace selection: tab not found")
)

// SetActiveWorkspaceTabForWorkspace seleciona uma aba somente se o workspace
// identificado pelo caller for o workspace ativo no instante da operação. O
// workspaceID bloqueia chamadas enquanto outro workspace estiver ativo; não é
// uma revisão nem detecta por si só um retorno ABA ao mesmo ID.
func (m *Manager) SetActiveWorkspaceTabForWorkspace(ctx context.Context, workspaceID, tabID string) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrWorkspaceSelectionNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.active == nil {
		return nil, fmt.Errorf("no active workspace")
	}
	if m.active.ID != workspaceID {
		return nil, ErrWorkspaceSelectionWorkspaceStale
	}
	if m.active.FindTab(tabID) == nil {
		return nil, ErrWorkspaceSelectionTabNotFound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.active.Tabs.Active == tabID {
		return m.cloneActiveSnapshotLocked(m.active), nil
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	m.active = cloneWorkspace(previousActive)
	m.active.Tabs.Active = tabID
	m.commandMutationEpochLocked()
	if err := ctx.Err(); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	if err := m.saveWorkspaceForCommand(ctx, m.active, m.activePath); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	return m.cloneActiveSnapshotLocked(m.active), nil
}
