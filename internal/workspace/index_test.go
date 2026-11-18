package workspace

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"assistente/internal/database"
	"assistente/internal/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestManager(t *testing.T, home string) *Manager {
	t.Helper()
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(home, "workspace-index-test.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.DesktopWorkspaceEntry{}, &database.DesktopWorkspaceState{}); err != nil {
		t.Fatal(err)
	}
	return NewManager(home, db)
}

func TestIndexMigrationPreservesLegacyWorkspacesAndDoesNotReimport(t *testing.T) {
	home := t.TempDir()
	legacyRoot := filepath.Join(home, workspacesDir)
	if err := os.MkdirAll(filepath.Join(legacyRoot, "old", assistenteDir), 0700); err != nil {
		t.Fatal(err)
	}
	document := filepath.Join(legacyRoot, "old", assistenteDir, workspaceFile)
	if err := os.WriteFile(document, []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, home)
	legacy := "last_opened: old\nworkspaces:\n  - id: old\n    name: Antigo\n    path: " + filepath.ToSlash(filepath.Dir(filepath.Dir(document))) + "\n"
	indexPath := filepath.Join(legacyRoot, indexFile)
	if err := os.WriteFile(indexPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.importLegacyIndex(); err != nil {
		t.Fatal(err)
	}
	index, err := m.loadIndex()
	if err != nil || index.LastOpened != "old" || len(index.Workspaces) != 1 {
		t.Fatalf("index=%+v err=%v", index, err)
	}
	if _, err := os.Stat(indexPath); !os.IsNotExist(err) {
		t.Fatalf("legacy index still exists: %v", err)
	}
	if data, err := os.ReadFile(document); err != nil || string(data) != "preserved" {
		t.Fatalf("workspace changed: %s %v", data, err)
	}
	index.Workspaces = nil
	index.LastOpened = ""
	if err := m.saveIndex(index); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.importLegacyIndex(); err != nil {
		t.Fatal(err)
	}
	index, err = m.loadIndex()
	if err != nil || len(index.Workspaces) != 0 {
		t.Fatalf("reimported old state: %+v %v", index, err)
	}
}

func TestIndexRejectsStaleWriter(t *testing.T) {
	m := newTestManager(t, t.TempDir())
	if err := m.importLegacyIndex(); err != nil {
		t.Fatal(err)
	}
	a, _ := m.loadIndex()
	b, _ := m.loadIndex()
	a.LastOpened = "winner"
	if err := m.saveIndex(a); err != nil {
		t.Fatal(err)
	}
	b.LastOpened = "stale"
	if err := m.saveIndex(b); err != ErrIndexChanged {
		t.Fatalf("stale write: %v", err)
	}
	current, err := m.loadIndex()
	if err != nil || current.LastOpened != "winner" {
		t.Fatalf("lost update: %+v %v", current, err)
	}
}

func TestIndexMigrationPreservesChangedSource(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home)
	if err := m.importLegacyIndex(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, workspacesDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, indexFile)
	data := []byte("workspaces: []\nlast_opened: changed\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.importLegacyIndex(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
		t.Fatalf("source changed: %q %v", got, err)
	}
}

func TestInitializePreservesInvalidHomeWorkspace(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, assistenteDir)
	m := newTestManager(t, home)
	path := filepath.Join(home, workspaceFile)
	data := []byte("tabs: [invalid\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Initialize(""); err == nil {
		t.Fatal("invalid workspace accepted")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
		t.Fatalf("source overwritten: %q %v", got, err)
	}
	if m.Active() != nil {
		t.Fatal("published invalid workspace")
	}
}

func TestListPrunesRemovedDocumentButKeepsUnavailableDirectory(t *testing.T) {
	m := newTestManager(t, t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	removed, err := m.Create("removed")
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range index.Workspaces {
		if entry.ID == removed.ID {
			if err := os.Remove(filepath.Join(entry.Path, assistenteDir, workspaceFile)); err != nil {
				t.Fatal(err)
			}
		}
	}
	index.Workspaces = append(index.Workspaces, IndexEntry{ID: "unavailable", Path: filepath.Join(t.TempDir(), "missing")})
	if err := m.saveIndex(index); err != nil {
		t.Fatal(err)
	}
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	foundUnavailable := false
	for _, entry := range list {
		if entry.ID == removed.ID {
			t.Fatal("removed workspace still listed")
		}
		if entry.ID == "unavailable" {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatal("unavailable directory was treated as deleted")
	}
}

func TestIndexClientIdentitySeparatesSamePathOnSharedDatabase(t *testing.T) {
	home := t.TempDir()
	first := newTestManager(t, home)
	if err := first.importLegacyIndex(); err != nil {
		t.Fatal(err)
	}
	index, err := first.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	index.LastOpened = "first-client"
	if err := first.saveIndex(index); err != nil {
		t.Fatal(err)
	}
	// Simula outra instalação usando a mesma grafia de home no mesmo banco.
	if err := os.WriteFile(filepath.Join(home, "desktop-client-id"), []byte(uuid.NewString()), 0600); err != nil {
		t.Fatal(err)
	}
	second := NewManager(home, first.indexDB)
	other, err := second.loadIndex()
	if err != nil || other.LastOpened != "" {
		t.Fatalf("client leaked: %+v %v", other, err)
	}
	current, err := first.loadIndex()
	if err != nil || current.LastOpened != "first-client" {
		t.Fatalf("live client scope changed: %+v %v", current, err)
	}
}

func TestIndexWriteFailureRollsBackStateAndEntries(t *testing.T) {
	m := newTestManager(t, t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	before, err := m.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.indexDB.Exec("CREATE TRIGGER reject_workspace_insert BEFORE INSERT ON desktop_workspace_entries BEGIN SELECT RAISE(ABORT, 'injected write failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	next, err := m.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	next.LastOpened = "rejected"
	if err := m.saveIndex(next); err == nil {
		t.Fatal("write should fail")
	}
	after, err := m.loadIndex()
	if err != nil || after.LastOpened != before.LastOpened || after.revision != before.revision || len(after.Workspaces) != len(before.Workspaces) {
		t.Fatalf("partial write: %+v %v", after, err)
	}
}

func TestIndexMigrationRejectsConflictingPathsWithoutDeletingSource(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home)
	dir := filepath.Join(home, workspacesDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, indexFile)
	data := []byte("workspaces:\n  - id: same\n    path: " + filepath.ToSlash(t.TempDir()) + "\n  - id: same\n    path: " + filepath.ToSlash(t.TempDir()) + "\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.importLegacyIndex(); err == nil {
		t.Fatal("conflicting references silently accepted")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
		t.Fatalf("source lost: %q %v", got, err)
	}
	index, err := m.loadIndex()
	if err != nil || index.legacyImported || len(index.Workspaces) != 0 {
		t.Fatalf("partial migration: %+v %v", index, err)
	}
}

func TestDesktopClientIdentityPublicationIsSharedAndComplete(t *testing.T) {
	home := t.TempDir()
	const clients = 8
	var group sync.WaitGroup
	keys := make([]string, clients)
	errors := make([]error, clients)
	for i := range clients {
		group.Add(1)
		go func() {
			defer group.Done()
			manager := NewManager(home, nil)
			keys[i], errors[i] = manager.indexScope()
		}()
	}
	group.Wait()
	for i := range clients {
		if errors[i] != nil || keys[i] == "" || keys[i] != keys[0] {
			t.Fatalf("inconsistent identity: %q %v", keys, errors)
		}
	}
	data, err := os.ReadFile(filepath.Join(home, "desktop-client-id"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(string(data)); err != nil {
		t.Fatal(err)
	}
	temporary, err := filepath.Glob(filepath.Join(home, ".desktop-client-id-*"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary files: %v %v", temporary, err)
	}
}
