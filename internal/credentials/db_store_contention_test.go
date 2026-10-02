package credentials

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLegacyCredentialWritePreventsStaleWALSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contention.db")
	dsn := path + "?_pragma=busy_timeout(1)&_pragma=journal_mode(WAL)"
	open := func() *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(4)
		t.Cleanup(func() { _ = pool.Close() })
		return db
	}
	db, other := open(), open()
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE unrelated (value INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO unrelated VALUES (0)").Error; err != nil {
		t.Fatal(err)
	}
	store := &DBStore{db: db}
	ctx := database.WithUserID(context.Background(), "owner")
	var attempted atomic.Bool
	var competingErr error
	if err := db.Callback().Query().After("gorm:query").Register("concurrent_unrelated_write", func(tx *gorm.DB) {
		if tx.Statement.Table == "mcp_servers" && attempted.CompareAndSwap(false, true) {
			// Force the competing connection between ownership validation and upsert.
			// A deferred transaction allows this commit, invalidating its snapshot.
			competingErr = other.Exec("UPDATE unrelated SET value = value + 1").Error
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("concurrent_unrelated_write") })
	err := store.SaveCredential(ctx, StoredCredential{Pattern: "mcp-tokens:legacy", Auth: &AuthConfig{Type: "oauth2", Token: "fresh", RefreshURL: "rotated"}})
	if err != nil {
		t.Fatalf("local contention lost newly obtained tokens: %v", err)
	}
	if !attempted.Load() || !database.IsSQLiteBusyError(competingErr) {
		t.Fatalf("writer lock not held before guard: attempted=%v err=%v", attempted.Load(), competingErr)
	}
	if err := other.Exec("UPDATE unrelated SET value = value + 1").Error; err != nil {
		t.Fatalf("writer lock leaked after save: %v", err)
	}
	entries, err := store.ListCredentials(ctx)
	if err != nil || len(entries) != 1 || entries[0].Auth.Token != "fresh" || entries[0].Auth.RefreshURL != "rotated" {
		t.Fatalf("new token pair not persisted: %v", err)
	}
}
