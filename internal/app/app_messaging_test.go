package app

import (
	"strings"
	"testing"

	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestBindMessagingDatabase_FailClosedWhenDBNil garante o cutover AEP-0083:
// sem database.DB() no boot, não omitir UseDatabase silenciosamente.
func TestBindMessagingDatabase_FailClosedWhenDBNil(t *testing.T) {
	prev := database.DB()
	database.SetDB(nil)
	t.Cleanup(func() { database.SetDB(prev) })

	err := bindMessagingDatabase()
	if err == nil {
		t.Fatal("esperava erro fail-closed quando database.DB() == nil")
	}
	if !strings.Contains(err.Error(), "AEP-0083") {
		t.Fatalf("erro deveria citar AEP-0083: %v", err)
	}
	if !strings.Contains(err.Error(), "filesystem") {
		t.Fatalf("erro deveria mencionar fallback filesystem: %v", err)
	}
}

// TestBindMessagingDatabase_EnablesStoresWhenDBAvailable cobre o caminho feliz do boot.
func TestBindMessagingDatabase_EnablesStoresWhenDBAvailable(t *testing.T) {
	prev := database.DB()
	channels.UseDatabase(nil)
	contacts.UseDatabase(nil)
	t.Cleanup(func() {
		database.SetDB(prev)
		channels.UseDatabase(nil)
		contacts.UseDatabase(nil)
	})

	db, err := gorm.Open(sqlite.Open("file:bind_messaging?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	database.SetDB(db)

	if err := bindMessagingDatabase(); err != nil {
		t.Fatalf("bindMessagingDatabase: %v", err)
	}
	if channels.DB() == nil {
		t.Fatal("channels.UseDatabase não foi chamado")
	}
}
