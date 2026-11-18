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
	created, err := m.CreateForCommand(context.Background(), expected, "Novo workspace", t.TempDir())
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
	if _, err := manager.CreateForCommand(nil, expected, "nil context", t.TempDir()); !errors.Is(err, ErrCommandCreateWorkspaceNilContext) { //nolint:staticcheck // Verifica a rejeição explícita de contexto nil.
		t.Fatalf("nil context error: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.CreateForCommand(canceled, expected, "canceled", t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error: %v", err)
	}
	if err := manager.AddTab(Tab{ID: "stale-tab", Type: TabTypeChat}); err != nil {
		t.Fatalf("AddTab: %v", err)
	}
	if _, err := manager.CreateForCommand(context.Background(), expected, "stale", t.TempDir()); !errors.Is(err, ErrCommandCreateWorkspaceStale) {
		t.Fatalf("stale snapshot error: %v", err)
	}
}

func TestCreateForCommandIndexReadFailureDoesNotPublish(t *testing.T) {
	m, _ := newCommandCreateWorkspaceManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	sqlDB, err := m.indexDB().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateForCommand(context.Background(), expected, "failure", directory); err == nil {
		t.Fatal("closed database accepted write")
	}
	if _, err := os.Stat(filepath.Join(directory, assistenteDir, workspaceFile)); !os.IsNotExist(err) {
		t.Fatalf("document published despite failed admission: %v", err)
	}
	if m.ActiveID() != expected.WorkspaceID {
		t.Fatal("failed creation changed workspace")
	}
}
