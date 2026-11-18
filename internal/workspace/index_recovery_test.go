package workspace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestInitializePreservesDocumentWithoutWorkspaceIdentity(t *testing.T) {
	for _, content := range []string{"note: keep-me\n", "id: workspace\n", "name: Projeto\n", ""} {
		t.Run(content, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), assistenteDir)
			m := newTestManager(t, home)
			path := filepath.Join(home, workspaceFile)
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.Initialize(""); err == nil {
				t.Fatal("invalid identity accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != content || m.Active() != nil {
				t.Fatalf("document modified: %q %v", after, err)
			}
		})
	}
}

func TestRecreatedWorkspaceCannotBeDeletedThroughOldReference(t *testing.T) {
	home := filepath.Join(t.TempDir(), assistenteDir)
	m := newTestManager(t, home)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	oldID := m.ActiveID()
	path := filepath.Join(home, workspaceFile)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	if m.ActiveID() == oldID {
		t.Fatal("workspace not recreated")
	}
	if err := m.Delete(oldID); err == nil {
		t.Fatal("stale identity accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("active workspace removed", err)
	}
	index, err := m.loadIndex()
	if err != nil || index.LastOpened != m.ActiveID() || len(index.Workspaces) != 1 {
		t.Fatalf("index=%+v err=%v", index, err)
	}
}

func TestUnavailableWorkspaceKeepsLastOpenedAcrossFallback(t *testing.T) {
	home := filepath.Join(t.TempDir(), assistenteDir)
	m := newTestManager(t, home)
	directory := filepath.Join(t.TempDir(), "project")
	if err := m.Initialize(directory); err != nil {
		t.Fatal(err)
	}
	id := m.ActiveID()
	away := directory + "-offline"
	if err := os.Rename(directory, away); err != nil {
		t.Fatal(err)
	}
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	index, err := m.loadIndex()
	if err != nil || index.LastOpened != id {
		t.Fatalf("lost last opened: %+v %v", index, err)
	}
	if err := os.Rename(away, directory); err != nil {
		t.Fatal(err)
	}
	if err := m.Initialize(""); err != nil || m.ActiveID() != id {
		t.Fatalf("did not restore returned volume: %v", err)
	}
}

func TestWorkspaceIndexUsesReopenedDatabaseConnection(t *testing.T) {
	home := filepath.Join(t.TempDir(), assistenteDir)
	fixture := newTestManager(t, home)
	current := fixture.indexDB()
	m := NewManager(home, func() *gorm.DB { return current })
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := current.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	current = newTestManager(t, home).indexDB()
	list, err := m.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("reopened index: %v %v", list, err)
	}
	if err := m.Rename("Após reabertura"); err != nil {
		t.Fatal(err)
	}
}

func TestRenameIndexFailureRestoresDocumentAndMemory(t *testing.T) {
	m := newTestManager(t, filepath.Join(t.TempDir(), assistenteDir))
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.activePath, assistenteDir, workspaceFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	name := m.Active().Name
	if err := m.indexDB().Exec("CREATE TRIGGER deny_workspace_update BEFORE UPDATE ON desktop_workspace_states BEGIN SELECT RAISE(ABORT, 'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("Should fail"); err == nil {
		t.Fatal("write failure hidden")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || m.Active().Name != name {
		t.Fatalf("failed rename leaked: %v", err)
	}
}

func TestConcurrentWorkspaceDocumentCreationPublishesOnlyCompleteWinner(t *testing.T) {
	m := newTestManager(t, t.TempDir())
	directory := t.TempDir()
	var workers sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- m.createWorkspaceDocument(m.newWorkspace("Projeto"), directory)
		}()
	}
	workers.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !os.IsExist(err) {
			t.Fatal(err)
		}
	}
	ws, err := m.loadWorkspaceFile(filepath.Join(directory, assistenteDir, workspaceFile))
	if err != nil || winners != 1 || ws == nil || ws.ID == "" {
		t.Fatalf("winner=%d ws=%+v err=%v", winners, ws, err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, assistenteDir))
	if err != nil || len(entries) != 1 {
		t.Fatalf("left temporary files: %v %v", entries, err)
	}
}

func TestLegacyIndexCleanupPreservesDirectorySymlink(t *testing.T) {
	home := filepath.Join(t.TempDir(), assistenteDir)
	m := newTestManager(t, home)
	target := t.TempDir()
	link := filepath.Join(home, workspacesDir)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, indexFile), []byte("workspaces: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.importLegacyIndex(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("legacy link removed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(link, "keep")); err != nil || string(data) != "legacy" {
		t.Fatalf("legacy content inaccessible: %v", err)
	}
}

func TestWorkspaceRejectsBlankNameBeforePersisting(t *testing.T) {
	m := newTestManager(t, filepath.Join(t.TempDir(), assistenteDir))
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	before, _ := m.List()
	if _, err := m.Create("   "); err == nil {
		t.Fatal("blank create accepted")
	}
	if _, err := m.ImportWorkspace([]byte("{}")); err == nil {
		t.Fatal("nameless import accepted")
	}
	if err := m.Rename("   "); err == nil {
		t.Fatal("blank rename accepted")
	}
	after, err := m.List()
	if err != nil || len(before) != len(after) {
		t.Fatalf("index changed: %v", err)
	}
}

func TestDeleteRollbackPreservesConcurrentDocument(t *testing.T) {
	m := newTestManager(t, filepath.Join(t.TempDir(), assistenteDir))
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	ws, err := m.Create("Remover")
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := m.List()
	var path string
	for _, entry := range entries {
		if entry.ID == ws.ID {
			path = filepath.Join(entry.Path, assistenteDir, workspaceFile)
		}
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	db := m.indexDB()
	const callback = "test:concurrent_workspace_publication"
	if err := db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "desktop_workspace_states" {
			return
		}
		if err := os.WriteFile(path, []byte("concurrent document"), 0600); err != nil {
			t.Error(err)
		}
		_ = tx.AddError(errors.New("injected index failure"))
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	err = m.Delete(ws.ID)
	if err == nil || !strings.Contains(err.Error(), "backup preserved at") {
		t.Fatalf("delete=%v", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil || string(content) != "concurrent document" {
		t.Fatalf("concurrent content lost: %v", readErr)
	}
	backups, _ := filepath.Glob(path + ".removing-*")
	if len(backups) != 1 {
		t.Fatalf("backups=%v", backups)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatalf("backup lost: %v", err)
	}
}
