package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrCommandCreateWorkspaceNilContext = errors.New("workspace command create workspace: context nil")
	ErrCommandCreateWorkspaceStale      = errors.New("workspace command create workspace: snapshot stale")
)

// CreateForCommand cria um workspace persistido sem trocar o workspace ativo.
// O snapshot autoriza somente o estado observado do workspace atualmente ativo;
// a nova entrada é publicada no índice sem alterar last_opened.
func (m *Manager) CreateForCommand(ctx context.Context, expected CommandSnapshot, name string) (*Workspace, error) {
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

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}
	ws := m.newWorkspace(name)
	workspaceDir := filepath.Join(m.homeDir, workspacesDir, ws.ID)
	if err := os.MkdirAll(filepath.Join(m.homeDir, workspacesDir), 0755); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}
	if err := os.Mkdir(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("create workspace directory: %w", err)
	}

	rollback := func(primary error) (*Workspace, error) {
		if cleanupErr := os.RemoveAll(workspaceDir); cleanupErr != nil {
			return nil, errors.Join(primary, fmt.Errorf("rollback created workspace %s: %w", ws.ID, cleanupErr))
		}
		return nil, primary
	}

	if err := m.saveWorkspaceForCommand(ctx, ws, workspaceDir); err != nil {
		return rollback(err)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}

	index, err := m.loadIndex()
	if err != nil {
		return rollback(err)
	}
	index.Workspaces = append(index.Workspaces, IndexEntry{
		ID:       ws.ID,
		Name:     ws.Name,
		Path:     workspaceDir,
		LastUsed: time.Now(),
	})
	if err := m.saveIndexContext(ctx, index); err != nil {
		return rollback(err)
	}

	// Cancellation after the index publication cannot undo a successful durable
	// publication without risking removal of a workspace the caller now owns.
	return ws, nil
}
