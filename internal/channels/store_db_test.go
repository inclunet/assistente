package channels

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func resetStoreForTests() {
	mu.Lock()
	defer mu.Unlock()
	storeDB = nil
	knownOwners = sync.Map{}
}

func setupChannelsDB(t *testing.T) *gorm.DB {
	t.Helper()
	resetStoreForTests()
	path := filepath.Join(t.TempDir(), "channels.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&database.Channel{},
		&database.ChannelContact{},
		&database.ChannelContactConversation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	UseDatabase(db)
	t.Cleanup(func() {
		resetStoreForTests()
		_ = sqlDB.Close()
	})
	return db
}

func TestSaveLoadDelete_DB(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	cfg := &ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-ana",
		Type:        "telegram",
		DisplayName: "Telegram",
		BotTokenRef: "channel:telegram:bot_token",
		Profile:     "canais-comunicacao",
		MaxHistory:  50,
		MaxContacts: 1,
		Conversations: map[string]string{
			"42": "conv-uuid",
		},
	}
	if err := Save("telegram", cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("telegram")
	if err != nil || loaded == nil {
		t.Fatalf("Load: %v cfg=%v", err, loaded)
	}
	if !loaded.Enabled || loaded.OwnerUserID != "user-ana" {
		t.Fatalf("loaded=%+v", loaded)
	}
	if loaded.Conversations["42"] != "conv-uuid" {
		t.Fatalf("conversations=%v", loaded.Conversations)
	}
	if loaded.BotToken != "" {
		t.Fatal("plaintext não deve existir no Load DB")
	}

	all, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if all["telegram"] == nil {
		t.Fatal("ListAll sem telegram")
	}

	enabled, err := LoadEnabled()
	if err != nil || enabled["telegram"] == nil {
		t.Fatalf("LoadEnabled: %v %v", err, enabled)
	}

	if err := SaveConversationID("telegram", "99", "conv-2"); err != nil {
		t.Fatalf("SaveConversationID: %v", err)
	}
	loaded, _ = Load("telegram")
	if loaded.Conversations["99"] != "conv-2" {
		t.Fatalf("after SaveConversationID: %v", loaded.Conversations)
	}

	if err := Delete("telegram"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	loaded, err = Load("telegram")
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if loaded != nil {
		t.Fatalf("esperava nil após Delete, got %+v", loaded)
	}
}

func TestAdoptOrphans_DB(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	if err := Save("telegram", &ChannelConfig{Enabled: true, Type: "telegram"}); err != nil {
		t.Fatalf("save orphan: %v", err)
	}
	if err := Save("signal", &ChannelConfig{Enabled: true, Type: "signal", OwnerUserID: "user-leo"}); err != nil {
		t.Fatalf("save owned: %v", err)
	}

	migrated, err := AdoptOrphans("user-ana")
	if err != nil {
		t.Fatalf("AdoptOrphans: %v", err)
	}
	if len(migrated) != 1 || migrated[0] != "telegram" {
		t.Fatalf("migrated=%v", migrated)
	}
	tg, _ := Load("telegram")
	if tg.OwnerUserID != "user-ana" {
		t.Fatalf("telegram owner=%q", tg.OwnerUserID)
	}
	sg, _ := Load("signal")
	if sg.OwnerUserID != "user-leo" {
		t.Fatalf("signal owner sobrescrito: %q", sg.OwnerUserID)
	}
}

func TestImportLegacyChannels_Idempotent(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	dir := filepath.Join(configdir.GetHomeDir(), "channels")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	payload, _ := json.Marshal(ChannelConfig{
		Enabled:     true,
		BotToken:    "plain-secret-token",
		MaxContacts: 1,
		Conversations: map[string]string{
			"1": "c-1",
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "telegram.json"), payload, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-ana")
	first, err := ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("import1: %v", err)
	}
	if first.Imported < 1 {
		t.Fatalf("esperava importar >=1, got %+v", first)
	}

	loaded, err := Load("telegram")
	if err != nil || loaded == nil {
		t.Fatalf("load after import: %v", err)
	}
	if loaded.OwnerUserID != "user-ana" {
		t.Fatalf("owner=%q", loaded.OwnerUserID)
	}
	// Sem CredManager: plaintext permanece no DTO em memória pós-Save; o importante
	// é que a row não tenha coluna de secret (só refs). BotTokenRef pode ficar vazio
	// sem credMgr — e o arquivo legado permanece.
	if _, err := os.Stat(filepath.Join(dir, "telegram.json")); err != nil {
		t.Fatalf("arquivo legado foi removido: %v", err)
	}

	second, err := ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("import2: %v", err)
	}
	if second.Imported != 0 {
		t.Fatalf("segunda importação deveria ser skip, got imported=%d", second.Imported)
	}
	if second.Skipped < 1 {
		t.Fatalf("esperava skipped>=1, got %+v", second)
	}
}
