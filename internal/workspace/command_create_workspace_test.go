package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newCommandCreateWorkspaceManager(t *testing.T) (*Manager, string) {
	t.Helper()
	homeDir := t.TempDir()
	workDir := t.TempDir()
	manager := NewManager(homeDir)
	if err := manager.Initialize(workDir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return manager, homeDir
}

func commandWorkspaceIndexPath(homeDir string) string {
	return filepath.Join(homeDir, workspacesDir, indexFile)
}

func commandWorkspaceDirCount(t *testing.T, homeDir string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(homeDir, workspacesDir))
	if err != nil {
		t.Fatalf("ReadDir workspaces: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func TestCreateForCommandPersistsWithoutChangingActiveWorkspace(t *testing.T) {
	manager, homeDir := newCommandCreateWorkspaceManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	activeBefore := manager.Active()
	indexBefore, err := os.ReadFile(commandWorkspaceIndexPath(homeDir))
	if err != nil {
		t.Fatalf("Read index before: %v", err)
	}
	lastOpenedBefore := ""
	index, err := readCommandWorkspaceIndex(commandWorkspaceIndexPath(homeDir))
	if err != nil {
		t.Fatalf("read index before: %v", err)
	}
	lastOpenedBefore = index.LastOpened

	created, err := manager.CreateForCommand(context.Background(), expected, "Novo workspace")
	if err != nil {
		t.Fatalf("CreateForCommand: %v", err)
	}
	if created.ID == expected.WorkspaceID || created.Name != "Novo workspace" {
		t.Fatalf("unexpected created workspace: %#v", created)
	}
	activeAfter := manager.Active()
	if activeAfter.ID != activeBefore.ID || activeAfter.LastUsed != activeBefore.LastUsed {
		t.Fatalf("active workspace changed: before=%#v after=%#v", activeBefore, activeAfter)
	}

	indexAfter, err := readCommandWorkspaceIndex(commandWorkspaceIndexPath(homeDir))
	if err != nil {
		t.Fatalf("read index after: %v", err)
	}
	if indexAfter.LastOpened != lastOpenedBefore {
		t.Fatalf("last_opened changed from %q to %q", lastOpenedBefore, indexAfter.LastOpened)
	}
	reloaded := NewManager(homeDir)
	if err := reloaded.Initialize(""); err != nil {
		t.Fatalf("reload Initialize: %v", err)
	}
	if active := reloaded.Active(); active == nil || active.ID != activeBefore.ID {
		t.Fatalf("reload switched the active workspace: got %#v, want %s", active, activeBefore.ID)
	}
	infos, err := reloaded.List()
	if err != nil {
		t.Fatalf("reload List: %v", err)
	}
	found := false
	for _, info := range infos {
		if info.ID == created.ID && info.Name == created.Name {
			found = true
		}
	}
	if !found {
		t.Fatalf("created workspace not found after reload: %#v", infos)
	}
	if afterBytes, err := os.ReadFile(commandWorkspaceIndexPath(homeDir)); err != nil {
		t.Fatalf("Read index after: %v", err)
	} else if bytes.Equal(indexBefore, afterBytes) {
		t.Fatal("index was not updated")
	}
}

func TestCreateForCommandRejectsNilCanceledAndStale(t *testing.T) {
	manager, _ := newCommandCreateWorkspaceManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	if _, err := manager.CreateForCommand(nil, expected, "nil context"); !errors.Is(err, ErrCommandCreateWorkspaceNilContext) { //nolint:staticcheck // Verifica a rejeição explícita de contexto nil.
		t.Fatalf("nil context error: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.CreateForCommand(canceled, expected, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error: %v", err)
	}
	if err := manager.AddTab(Tab{ID: "stale-tab", Type: TabTypeChat}); err != nil {
		t.Fatalf("AddTab: %v", err)
	}
	if _, err := manager.CreateForCommand(context.Background(), expected, "stale"); !errors.Is(err, ErrCommandCreateWorkspaceStale) {
		t.Fatalf("stale snapshot error: %v", err)
	}
}

func TestCreateForCommandRejectsMalformedIndexWithoutOrphan(t *testing.T) {
	manager, homeDir := newCommandCreateWorkspaceManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	indexPath := commandWorkspaceIndexPath(homeDir)
	if err := os.WriteFile(indexPath, []byte("workspaces: [unterminated\n"), 0644); err != nil {
		t.Fatalf("write malformed index: %v", err)
	}
	dirsBefore := commandWorkspaceDirCount(t, homeDir)
	if _, err := manager.CreateForCommand(context.Background(), expected, "malformed"); err == nil {
		t.Fatal("expected malformed index error")
	}
	if dirsAfter := commandWorkspaceDirCount(t, homeDir); dirsAfter != dirsBefore {
		t.Fatalf("orphan workspace directory remains: before=%d after=%d", dirsBefore, dirsAfter)
	}
}

func TestCreateForCommandRejectsMissingIndexWithoutMutation(t *testing.T) {
	manager, homeDir := newCommandCreateWorkspaceManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	activeBefore := manager.Active()
	indexPath := commandWorkspaceIndexPath(homeDir)
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	dirsBefore := commandWorkspaceDirCount(t, homeDir)

	if _, err := manager.CreateForCommand(context.Background(), expected, "missing index"); err == nil || !strings.Contains(err.Error(), "workspace index") {
		t.Fatalf("expected missing index rejection, got %v", err)
	}
	activeAfter := manager.Active()
	if activeAfter.ID != activeBefore.ID || activeAfter.LastUsed != activeBefore.LastUsed {
		t.Fatalf("active workspace changed: before=%#v after=%#v", activeBefore, activeAfter)
	}
	if dirsAfter := commandWorkspaceDirCount(t, homeDir); dirsAfter != dirsBefore {
		t.Fatalf("new workspace directory remains: before=%d after=%d", dirsBefore, dirsAfter)
	}
}

func TestCreateForCommandIndexReadFailureCompensates(t *testing.T) {
	manager, homeDir := newCommandCreateWorkspaceManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	indexPath := commandWorkspaceIndexPath(homeDir)
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove index fixture: %v", err)
	}
	if err := os.Mkdir(indexPath, 0755); err != nil {
		t.Fatalf("create index directory fixture: %v", err)
	}
	markerPath := filepath.Join(indexPath, "marker")
	marker := []byte("old-index-marker")
	if err := os.WriteFile(markerPath, marker, 0644); err != nil {
		t.Fatalf("write index marker: %v", err)
	}
	dirsBefore := commandWorkspaceDirCount(t, homeDir)
	_, err = manager.CreateForCommand(context.Background(), expected, "write failure")
	if err == nil || !strings.Contains(err.Error(), "workspace index") {
		t.Fatalf("expected index read failure, got %v", err)
	}
	if got, readErr := os.ReadFile(markerPath); readErr != nil {
		t.Fatalf("read preserved index marker: %v", readErr)
	} else if !bytes.Equal(got, marker) {
		t.Fatalf("index marker changed after failed creation")
	}
	if dirsAfter := commandWorkspaceDirCount(t, homeDir); dirsAfter != dirsBefore {
		t.Fatalf("orphan workspace directory remains: before=%d after=%d", dirsBefore, dirsAfter)
	}
}

func TestPublishCommandWorkspaceIndexPreservesNonEmptyTargetOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, indexFile)
	if err := os.Mkdir(indexPath, 0755); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	markerPath := filepath.Join(indexPath, "marker")
	marker := []byte("old-index-marker")
	if err := os.WriteFile(markerPath, marker, 0644); err != nil {
		t.Fatalf("write target marker: %v", err)
	}

	err := publishCommandWorkspaceIndex(context.Background(), indexPath, &Index{})
	if err == nil {
		t.Fatal("expected rename failure for non-empty target directory")
	}
	if got, readErr := os.ReadFile(markerPath); readErr != nil {
		t.Fatalf("read preserved target marker: %v", readErr)
	} else if !bytes.Equal(got, marker) {
		t.Fatalf("target marker changed after failed rename")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read publication directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".index.yaml-command-") {
			t.Fatalf("temporary index artifact remains: %s", entry.Name())
		}
	}
}
