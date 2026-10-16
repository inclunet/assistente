package mcp

import (
	"bytes"
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/tools"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLegacyConfigWriterPreventsStaleWALSnapshot(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "writer.db") + "?_pragma=busy_timeout(1)&_pragma=journal_mode(WAL)"
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
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })
	ctx := database.WithUserID(context.Background(), "owner")
	m := NewManager(tools.NewRegistry(), credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true), func(string, any) {})
	repo := NewDBRepository(db)
	m.SetRepository(repo)
	m.SetAuthContextProvider(func() context.Context { return ctx })
	t.Cleanup(m.CloseAll)
	cfg := managedConfig("https://resource.example")
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	original, err := m.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	_, store, candidate := loadManaged(t, m, ctx, "legacy")
	atomicStore, ok := store.(mcpAtomicOAuthStore)
	if !ok {
		t.Fatal("shared vault does not support consumer transaction")
	}
	var attempted atomic.Bool
	var competingErr error
	if err := db.Callback().Query().After("gorm:query").Register("concurrent_unrelated_write", func(tx *gorm.DB) {
		if tx.Statement.Table == "mcp_servers" && attempted.CompareAndSwap(false, true) {
			competingErr = other.Exec("UPDATE unrelated SET value = value + 1").Error
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("concurrent_unrelated_write") })
	candidate.Callback.Port, candidate.Callback.PortPolicy = 34567, "fixed"
	candidate.Client.ID = "registered"
	candidate.Revision++
	if err := atomicStore.CompareAndSwapWithConsumer(ctx, candidate, candidate.Revision-1, func(tx *gorm.DB) error {
		var consumer database.MCPServer
		return tx.Where("id = ? AND user_id = ?", original.ID, original.UserID).First(&consumer).Error
	}); err != nil {
		t.Fatalf("lost DCR metadata under contention: %v", err)
	}
	if !attempted.Load() || !database.IsSQLiteBusyError(competingErr) {
		t.Fatalf("missing writer lock: attempted=%v err=%v", attempted.Load(), competingErr)
	}
	if err := other.Exec("UPDATE unrelated SET value = value + 1").Error; err != nil {
		t.Fatalf("writer lock leaked: %v", err)
	}
	stored, err := repo.GetServer(ctx, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	cached, err := m.GetConfig("legacy")
	if err != nil {
		t.Fatal(err)
	}
	_, _, persisted := loadManaged(t, m, ctx, "legacy")
	if stored.OAuthAuthorizationID != persisted.ID || persisted.Callback.Port != 34567 || persisted.Client.ID != "registered" || cached.OAuth2CallbackPort != 34567 || cached.OAuth2ClientID != "registered" {
		t.Fatal("DCR metadata diverges in database/cache")
	}
}
