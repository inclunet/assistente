package controllers

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/desktopinstance"
	"assistente/internal/events"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSettingsControllerResetDatabaseUsesFixedDatabasePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("XDG_CACHE_HOME", root)
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
	aliasPath := filepath.Join(databaseDir, "hardlink.db")
	if err := os.Link(dbPath, aliasPath); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	activated := make(chan struct{}, 1)
	guard, err := desktopinstance.Acquire(dbPath, func() { activated <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Close() })
	if err := controller.ResetDatabase(); err != nil {
		t.Fatalf("ResetDatabase: %v", err)
	}
	after, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := os.Stat(aliasPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !os.SameFile(after, alias) {
		t.Fatal("reset substituiu a identidade física do banco/hardlink")
	}
	second, err := desktopinstance.Acquire(aliasPath, func() {})
	if second != nil {
		_ = second.Close()
	}
	if !errors.Is(err, desktopinstance.ErrAlreadyRunning) {
		t.Fatalf("alias após reset não ativou a instância reservada: %v", err)
	}
	select {
	case <-activated:
	case <-time.After(3 * time.Second):
		t.Fatal("ativação via hardlink não chegou à primeira instância")
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

func TestSettingsControllerResetDatabaseRejectsUnsafePathBeforeClose(t *testing.T) {
	for _, kind := range []string{"symlink", "directory", "missing"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("USERPROFILE", root)
			t.Chdir(root)
			configdir.ResetForTests()
			t.Cleanup(configdir.ResetForTests)
			sentinelPath := filepath.Join(root, "outside.db")
			sentinel := []byte("outside-file-must-not-be-truncated")
			if err := os.WriteFile(sentinelPath, sentinel, 0600); err != nil {
				t.Fatal(err)
			}
			resetPath := filepath.Join(root, "reserved.db")
			switch kind {
			case "symlink":
				if err := os.Symlink(sentinelPath, resetPath); err != nil {
					if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
						t.Skip("Windows sem privilégio para criar symlink")
					}
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(resetPath, 0700); err != nil {
					t.Fatal(err)
				}
			}
			db, err := gorm.Open(sqlite.Open(filepath.Join(root, "open.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			restoreDB := database.SetDB(db)
			t.Cleanup(func() { restoreDB(); _ = pool.Close() })
			controller := NewSettingsController(SettingsControllerConfig{DatabasePath: resetPath, Emitter: events.NoopEmitter{}})
			if err := controller.ResetDatabase(); err == nil {
				t.Fatal("reset aceitou caminho inseguro")
			}
			if err := pool.Ping(); err != nil {
				t.Fatalf("reset fechou DB antes de rejeitar caminho: %v", err)
			}
			if got, err := os.ReadFile(sentinelPath); err != nil || !bytes.Equal(got, sentinel) {
				t.Fatalf("reset alterou sentinela externa: %q, %v", got, err)
			}
		})
	}
}
