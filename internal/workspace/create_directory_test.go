package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateRequiresChosenDirectoryAndPreservesExistingDocument(t *testing.T) {
	m, _ := newCommandCreateWorkspaceManager(t)
	for _, directory := range []string{"", ".", filepath.Join(t.TempDir(), "missing")} {
		if _, err := m.createForTest("Novo", directory); err == nil {
			t.Fatalf("accepted destination %q", directory)
		}
	}
	directory := t.TempDir()
	ws, err := m.createForTest("  Projeto  ", directory)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, assistenteDir, workspaceFile)
	before, err := os.ReadFile(path)
	if err != nil || ws.Name != "Projeto" {
		t.Fatalf("workspace=%+v err=%v", ws, err)
	}
	if _, err := m.createForTest("Outro", directory); err == nil {
		t.Fatal("overwrote existing workspace")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("existing workspace changed: %v", err)
	}
	index, err := m.loadIndex()
	found := false
	for _, entry := range index.Workspaces {
		if entry.ID == ws.ID && entry.Path == directory {
			found = true
		}
	}
	if err != nil || !found {
		t.Fatalf("index=%+v err=%v", index, err)
	}
}

func TestImportRequiresDestinationAndDoesNotOverwrite(t *testing.T) {
	m, _ := newCommandCreateWorkspaceManager(t)
	directory := t.TempDir()
	data := []byte("name: Importado\ntabs:\n  items: []\n")
	ws, err := m.ImportWorkspace(data, directory)
	if err != nil || ws == nil || len(ws.Tabs.Items) != 1 {
		t.Fatalf("workspace=%+v err=%v", ws, err)
	}
	if _, err := m.ImportWorkspace(data, directory); err == nil {
		t.Fatal("import overwrote existing document")
	}
}

func (m *Manager) createForTest(name, directory string) (*Workspace, error) {
	snapshot, err := m.CommandSnapshot()
	if err != nil {
		return nil, err
	}
	return m.CreateForCommand(context.Background(), snapshot, name, directory)
}

func TestCreateAndImportRejectActiveDirectoryWithMissingDocument(t *testing.T) {
	m, _ := newCommandCreateWorkspaceManager(t)
	directory := m.activePath
	before, err := m.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, assistenteDir, workspaceFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.createForTest("Novo", directory); err == nil {
		t.Fatal("replaced active workspace")
	}
	if _, err := m.ImportWorkspace([]byte("name: Importado\ntabs:\n  items: []\n"), directory); err == nil {
		t.Fatal("import replaced active workspace")
	}
	after, err := m.loadIndex()
	if err != nil || len(after.Workspaces) != len(before.Workspaces) || after.LastOpened != before.LastOpened {
		t.Fatalf("index changed: %+v %v", after, err)
	}
	if _, err := os.Stat(filepath.Join(directory, assistenteDir, workspaceFile)); !os.IsNotExist(err) {
		t.Fatalf("document recreated: %v", err)
	}
}

func TestRenameInactivePreservesActiveAndPersistsSelectedDocument(t *testing.T) {
	m, _ := newCommandCreateWorkspaceManager(t)
	active := m.Active()
	directory := t.TempDir()
	ws, err := m.createForTest("Antes", directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Rename(ws.ID, " Depois "); err != nil {
		t.Fatal(err)
	}
	saved, err := m.loadWorkspaceFile(filepath.Join(directory, assistenteDir, workspaceFile))
	if err != nil || saved.Name != "Depois" {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	if m.Active().ID != active.ID || m.Active().Name != active.Name {
		t.Fatal("renamed active workspace instead")
	}
	index, err := m.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range index.Workspaces {
		if entry.ID == ws.ID && entry.Name == "Depois" {
			return
		}
	}
	t.Fatal("index not renamed")
}
