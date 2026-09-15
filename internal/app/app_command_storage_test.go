package app

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCommandStoragePreparesWithoutPublishingExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "storage.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.CredentialKeyWrap{}); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	defer database.SetDB(previous)
	dek := bytes.Repeat([]byte{17}, 32)
	manager := credentials.NewManagerWithStore(dek, credentials.NewDBStore(), true)
	if err := manager.LoadInstanceSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	sessions := &auth.SessionService{}
	a := &App{credMgr: manager, sessionSvc: sessions}
	for range 2 {
		if err := a.prepareCommandStorage(ctx, db, manager); err != nil {
			t.Fatal(err)
		}
		if a.commandStorageVersion != "v1" || a.commandStorageErr != nil || a.commandHost != nil || a.sessionSvc != sessions {
			t.Fatal("prontidão de armazenamento alterou execução/autenticação")
		}
	}
	manager.Reset(dek, false)
	if err := a.prepareCommandStorage(ctx, db, manager); err == nil {
		t.Fatal("cofre efêmero aceito")
	}
	if a.commandStorageVersion != "" || a.commandStorageErr == nil || a.commandHost != nil || a.sessionSvc != sessions {
		t.Fatal("falha não fechou prontidão ou alterou login")
	}
	manager.Reset(dek, true)
	if err := manager.LoadInstanceSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.prepareCommandStorage(ctx, db, manager); err != nil {
		t.Fatal("retomada", err)
	}
	var count int64
	if err := db.Table("credential_entries").Where("pattern = ?", "internal-auth:command-request-hmac:v1").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("chave duplicada", count, err)
	}
	if err := a.prepareCommandStorage(ctx, db, credentials.NewManager(dek)); err == nil || a.commandStorageVersion != "" {
		t.Fatal("manager diferente publicou prontidão")
	}
}

func TestCommandStorageRejectsInvalidDependencies(t *testing.T) {
	a := &App{}
	if err := a.prepareCommandStorage(nil, nil, nil); err == nil || a.commandStorageVersion != "" || a.commandStorageErr == nil {
		t.Fatal("dependências ausentes aceitas")
	}
}
