package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/events"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestWireSettingsResetUsesDesktopDatabasePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Chdir(root)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	if err := os.MkdirAll(filepath.Join(root, ".assistente"), 0700); err != nil {
		t.Fatal(err)
	}
	// A resolução legada aponta para uma sentinela, diferente do DB reservado.
	sentinelPath := filepath.Join(root, ".assistente", "conversations.db")
	sentinel := []byte("do-not-reset-the-resolver-database")
	if err := os.WriteFile(sentinelPath, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "reserved.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	restoreDB := database.SetDB(db)
	t.Cleanup(func() {
		_ = database.Close()
		restoreDB()
	})
	if err := db.Exec("CREATE TABLE reset_sentinel (value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.emitter = events.NoopEmitter{}
	SetDesktopDatabasePath(a, dbPath)
	a.wireSettings()
	if err := a.settingsCtrl.ResetDatabase(); err != nil {
		t.Fatal(err)
	}
	var attached []struct{ Name, File string }
	if err := database.DB().Raw("PRAGMA database_list").Scan(&attached).Error; err != nil {
		t.Fatal(err)
	}
	var openedPath string
	for _, db := range attached {
		if db.Name == "main" {
			openedPath = db.File
		}
	}
	if filepath.Clean(openedPath) != filepath.Clean(dbPath) {
		t.Fatalf("wireSettings reabriu %q; esperado %q", openedPath, dbPath)
	}
	if database.DB().Migrator().HasTable("reset_sentinel") {
		t.Fatal("reset não removeu tabela do banco reservado")
	}
	if got, err := os.ReadFile(sentinelPath); err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("reset alterou banco da resolução legada: bytes=%q err=%v", got, err)
	}
}
