package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
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

	indexPath := filepath.Join(m.homeDir, workspacesDir, indexFile)
	index, err := readCommandWorkspaceIndex(indexPath)
	if err != nil {
		return rollback(err)
	}
	index.Workspaces = append(index.Workspaces, IndexEntry{
		ID:       ws.ID,
		Name:     ws.Name,
		Path:     workspaceDir,
		LastUsed: time.Now(),
	})
	if err := publishCommandWorkspaceIndex(ctx, indexPath, index); err != nil {
		return rollback(err)
	}

	// Cancellation after the index publication cannot undo a successful durable
	// publication without risking removal of a workspace the caller now owns.
	return ws, nil
}

func readCommandWorkspaceIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workspace index: %w", err)
	}
	var index Index
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("parse workspace index: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse workspace index: multiple YAML documents")
		}
		return nil, fmt.Errorf("parse workspace index: %w", err)
	}
	return &index, nil
}

func publishCommandWorkspaceIndex(ctx context.Context, path string, index *Index) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := yaml.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshal workspace index: %w", err)
	}
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".index.yaml-command-*")
	if err != nil {
		return fmt.Errorf("create workspace index temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write workspace index temporary file: %w", err)
	}
	if existing, statErr := os.Stat(path); statErr == nil {
		if err := temporary.Chmod(existing.Mode().Perm()); err != nil {
			return fmt.Errorf("set workspace index temporary file permissions: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat workspace index: %w", statErr)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close workspace index temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if renameErr = os.Rename(temporaryName, path); renameErr == nil {
			return nil
		}
		if attempt == 4 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("publish workspace index after retries: %w", renameErr)
}
