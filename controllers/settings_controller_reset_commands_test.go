package controllers

import (
	"errors"
	"path/filepath"
	"testing"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSettingsControllerResetDatabaseAbortsBeforeResolvingOrClosing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commands.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
		_ = sqlDB.Close()
	})

	want := errors.New("commands still running")
	called := 0
	controller := NewSettingsController(SettingsControllerConfig{
		BeforeDatabaseReset: func() error {
			called++
			return want
		},
	})

	err = controller.ResetDatabase()
	if !errors.Is(err, want) {
		t.Fatalf("ResetDatabase error = %v, want %v", err, want)
	}
	if called != 1 {
		t.Fatalf("BeforeDatabaseReset calls = %d, want 1", called)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("o banco temporário foi fechado apesar da recusa: %v", err)
	}
}
