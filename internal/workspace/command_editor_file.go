package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	EditorFileOperationOpen = "open"
	EditorFileOperationSave = "save"
	EditorFileOperationCopy = "save_copy"
)

var (
	ErrCommandEditorFileNilContext       = errors.New("workspace command editor file: context nil")
	ErrCommandEditorFileInvalidOperation = errors.New("workspace command editor file: invalid operation")
	ErrCommandEditorFileInvalidPath      = errors.New("workspace command editor file: invalid path")
	ErrCommandEditorFileNilWrite         = errors.New("workspace command editor file: write callback required")
	ErrCommandEditorFileStale            = errors.New("workspace command editor file: snapshot stale")
	ErrCommandEditorFileUnsupportedType  = errors.New("workspace command editor file: active tab is not an editor")
	ErrCommandEditorFileDuplicateID      = errors.New("workspace command editor file: duplicate tab id")
)

// CommitEditorFileForCommand coordena a fronteira não atômica entre o efeito
// no arquivo e os metadados do workspace. O callback roda depois do CAS exato,
// mantendo m.mu até o efeito, os metadados e a persistência terminarem. Um
// callback bem-sucedido é reportado por written mesmo quando a persistência
// posterior falha; nesse caso somente os metadados em memória são revertidos.
func (m *Manager) CommitEditorFileForCommand(
	ctx context.Context,
	expected CommandSnapshot,
	operation, path, newTabID string,
	write func(context.Context) error,
) (*Workspace, string, bool, error) {
	if m == nil {
		return nil, "", false, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, "", false, ErrCommandEditorFileNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, "", false, err
	}
	if operation != EditorFileOperationOpen && operation != EditorFileOperationSave && operation != EditorFileOperationCopy {
		return nil, "", false, ErrCommandEditorFileInvalidOperation
	}
	if operation != EditorFileOperationOpen && write == nil {
		return nil, "", false, ErrCommandEditorFileNilWrite
	}
	canonicalPath, err := canonicalEditorFilePath(path)
	if err != nil {
		return nil, "", false, err
	}
	if operation == EditorFileOperationOpen && strings.TrimSpace(newTabID) == "" {
		return nil, "", false, ErrCommandEditorFileInvalidPath
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, expected.ActiveTabID, false, err
	}
	if err := m.validateEditorFileSnapshotLocked(expected); err != nil {
		return nil, expected.ActiveTabID, false, err
	}
	if operation == EditorFileOperationOpen {
		result, err := m.openEditorFileLocked(ctx, canonicalPath, newTabID)
		if err != nil {
			return nil, expected.ActiveTabID, false, err
		}
		return result, result.Tabs.Active, false, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, expected.ActiveTabID, false, err
	}
	if err := write(ctx); err != nil {
		return nil, expected.ActiveTabID, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, expected.ActiveTabID, true, err
	}
	if err := m.validateEditorFileSnapshotLocked(expected); err != nil {
		return nil, expected.ActiveTabID, true, err
	}
	if operation == EditorFileOperationCopy {
		return m.cloneActiveSnapshotLocked(m.active), expected.ActiveTabID, true, nil
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	updated := cloneWorkspace(previousActive)
	tab := updated.FindTab(expected.ActiveTabID)
	if tab == nil || tab.Type != TabTypeEditor {
		return nil, expected.ActiveTabID, true, ErrCommandEditorFileStale
	}
	if tab.State == nil {
		tab.State = make(map[string]any)
	}
	tab.State["filePath"] = canonicalPath
	tab.Title = filepath.Base(canonicalPath)
	m.active = updated
	m.commandMutationEpochLocked()
	if err := ctx.Err(); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, expected.ActiveTabID, true, err
	}
	if err := m.saveWorkspaceForCommand(ctx, m.active, m.activePath); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, expected.ActiveTabID, true, err
	}
	return m.cloneActiveSnapshotLocked(m.active), expected.ActiveTabID, true, nil
}

func (m *Manager) validateEditorFileSnapshotLocked(expected CommandSnapshot) error {
	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}
	current, err := m.commandSnapshotLocked()
	if err != nil {
		return err
	}
	if current.WorkspaceID != expected.WorkspaceID || current.Version != expected.Version || current.ActiveTabID != expected.ActiveTabID {
		return ErrCommandEditorFileStale
	}
	if expected.Tab.Type != TabTypeEditor || current.Tab.Type != TabTypeEditor {
		return ErrCommandEditorFileUnsupportedType
	}
	return nil
}

func (m *Manager) openEditorFileLocked(ctx context.Context, path, newTabID string) (*Workspace, error) {
	previousActive := m.active
	previousEpoch := m.commandEpoch
	updated := cloneWorkspace(previousActive)
	var target *Tab
	for i := range updated.Tabs.Items {
		candidate := &updated.Tabs.Items[i]
		if candidate.Type != TabTypeEditor {
			continue
		}
		candidatePath, ok := editorTabFilePath(candidate)
		if ok && sameEditorFilePath(candidatePath, path) {
			target = candidate
			break
		}
	}
	if target == nil {
		if updated.FindTab(newTabID) != nil {
			return nil, ErrCommandEditorFileDuplicateID
		}
		target = &Tab{ID: newTabID, Type: TabTypeEditor, Title: filepath.Base(path), Position: len(updated.Tabs.Items), State: map[string]any{"filePath": path}}
		updated.Tabs.Items = append(updated.Tabs.Items, *target)
		target = &updated.Tabs.Items[len(updated.Tabs.Items)-1]
	} else {
		if target.State == nil {
			target.State = make(map[string]any)
		}
		target.State["filePath"] = path
		target.Title = filepath.Base(path)
	}
	updated.Tabs.Active = target.ID
	m.active = updated
	if previousActive.Tabs.Active == updated.Tabs.Active && previousActive.FindTab(target.ID) != nil && sameEditorFilePathFromTab(previousActive.FindTab(target.ID), path) {
		m.active = previousActive
		return m.cloneActiveSnapshotLocked(m.active), nil
	}
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

func canonicalEditorFilePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrCommandEditorFileInvalidPath
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCommandEditorFileInvalidPath, err)
	}
	return filepath.Clean(absolute), nil
}

func editorTabFilePath(tab *Tab) (string, bool) {
	if tab == nil || tab.State == nil {
		return "", false
	}
	path, ok := tab.State["filePath"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", false
	}
	canonical, err := canonicalEditorFilePath(path)
	return canonical, err == nil
}

func sameEditorFilePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func sameEditorFilePathFromTab(tab *Tab, path string) bool {
	candidate, ok := editorTabFilePath(tab)
	return ok && sameEditorFilePath(candidate, path)
}
