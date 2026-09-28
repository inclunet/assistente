package controllers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/events"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSettingsControllerResetDatabaseUsesFixedDatabasePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Chdir(root)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	configDir := filepath.Join(root, ".assistente")
	databaseDir := filepath.Join(root, "profile-data")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(databaseDir, 0700); err != nil {
		t.Fatal(err)
	}
	configSentinel := []byte("config-directory-must-not-be-treated-as-database")
	configDBPath := filepath.Join(configDir, "conversations.db")
	if err := os.WriteFile(configDBPath, configSentinel, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"maintenance":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := config.GetConfigPath(); err != nil || filepath.Clean(got) != filepath.Join(configDir, "config.json") {
		t.Fatalf("resolver fora da fixture: path=%q err=%v", got, err)
	}

	dbPath := filepath.Join(databaseDir, "conversations.db")
	initialDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := initialDB.Exec("CREATE TABLE reset_sentinel (value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := initialDB.Exec("INSERT INTO reset_sentinel(value) VALUES ('old-data')").Error; err != nil {
		t.Fatal(err)
	}
	restoreDB := database.SetDB(initialDB)
	t.Cleanup(func() {
		if current := database.DB(); current != nil {
			if sqlDB, err := current.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		restoreDB()
	})

	controller := NewSettingsController(SettingsControllerConfig{
		DatabasePath: dbPath,
		Emitter:      events.NoopEmitter{},
	})
	if err := controller.ResetDatabase(); err != nil {
		t.Fatalf("ResetDatabase: %v", err)
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
		t.Fatalf("banco reaberto=%q, esperado caminho reservado=%q", openedPath, dbPath)
	}

	gotConfigSentinel, err := os.ReadFile(configDBPath)
	if err != nil || !bytes.Equal(gotConfigSentinel, configSentinel) {
		t.Fatalf("arquivo sentinela na pasta de config alterado: bytes=%q err=%v", gotConfigSentinel, err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "config.json")); err != nil {
		t.Fatalf("config.json removido pelo reset: %v", err)
	}
	var count int64
	if err := database.DB().Table("reset_sentinel").Count(&count).Error; err == nil {
		t.Fatalf("tabela antiga ainda existe após reset (count query=%d)", count)
	}
	if err := database.DB().Exec("CREATE TABLE post_reset_check (value TEXT)").Error; err != nil {
		t.Fatalf("InitPath não reabriu o banco reservado: %v", err)
	}
}

func TestSettingsControllerResetDatabaseRejectsRelativeConfiguredPathBeforeClose(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Chdir(root)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "db.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	restoreDB := database.SetDB(db)
	t.Cleanup(func() {
		restoreDB()
		_ = sqlDB.Close()
	})

	controller := NewSettingsController(SettingsControllerConfig{DatabasePath: "relative/conversations.db"})
	if err := controller.ResetDatabase(); err == nil {
		t.Fatal("caminho relativo deveria ser recusado")
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("DB foi fechado antes da validação do caminho: %v", err)
	}
}
