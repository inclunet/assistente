package workspace

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrCommandCreateWorkspaceNilContext = errors.New("workspace command create workspace: context nil")
	ErrCommandCreateWorkspaceStale      = errors.New("workspace command create workspace: snapshot stale")
)

// CreateForCommand cria um workspace persistido sem trocar o workspace ativo.
// O snapshot autoriza somente o estado observado do workspace atualmente ativo;
// a nova entrada é publicada no índice sem alterar last_opened.
func (m *Manager) CreateForCommand(ctx context.Context, expected CommandSnapshot, name, directory string) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandCreateWorkspaceNilContext
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
	current, err := m.commandSnapshotLocked()
	if err != nil {
		return nil, err
	}
	if current.WorkspaceID != expected.WorkspaceID ||
		current.Version != expected.Version ||
		current.ActiveTabID != expected.ActiveTabID {
		return nil, ErrCommandCreateWorkspaceStale
	}

	return m.createInDirectoryLocked(ctx, m.newWorkspace(name), directory)
}
