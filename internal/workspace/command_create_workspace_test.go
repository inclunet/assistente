package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newCommandCreateWorkspaceManager(t *testing.T) (*Manager, string) {
	t.Helper()
	home := t.TempDir()
	m := newTestManager(t, home)
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return m, home
}

func commandWorkspaceDirCount(t *testing.T, home string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, workspacesDir))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
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
	m, home := newCommandCreateWorkspaceManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before, err := m.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	created, err := m.CreateForCommand(context.Background(), expected, "Novo workspace")
	if err != nil {
		t.Fatal(err)
	}
	reloaded := newTestManager(t, home)
	if err := reloaded.Initialize(""); err != nil {
		t.Fatal(err)
	}
	if reloaded.ActiveID() != expected.WorkspaceID {
		t.Fatal("creation changed active workspace")
	}
	after, err := reloaded.loadIndex()
	if err != nil || after.LastOpened != before.LastOpened || len(after.Workspaces) != len(before.Workspaces)+1 {
		t.Fatalf("index=%+v err=%v", after, err)
	}
	found := false
	for _, entry := range after.Workspaces {
		if entry.ID == created.ID && entry.Name == "Novo workspace" {
			found = true
		}
	}
	if !found {
		t.Fatal("created workspace not persisted")
	}
	if _, err := os.Stat(filepath.Join(home, workspacesDir, indexFile)); !os.IsNotExist(err) {
		t.Fatal("legacy index was recreated")
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

func TestCreateForCommandIndexReadFailureCompensates(t *testing.T) {
	m, home := newCommandCreateWorkspaceManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before := commandWorkspaceDirCount(t, home)
	sqlDB, err := m.indexDB().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateForCommand(context.Background(), expected, "failure"); err == nil {
		t.Fatal("closed database accepted write")
	}
	if got := commandWorkspaceDirCount(t, home); got != before {
		t.Fatalf("orphan directories: %d -> %d", before, got)
	}
	if m.ActiveID() != expected.WorkspaceID {
		t.Fatal("failed creation changed workspace")
	}
}
