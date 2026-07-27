package channels

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestAdoptOrphans_WithDBSkipsFilesystemWrites: com UseDatabase ativo,
// AdoptOrphans não deve escrever OwnerUserID em JSON legado (AEP-0083 fail-closed).
func TestAdoptOrphans_WithDBSkipsFilesystemWrites(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	if err := Save("signal", &ChannelConfig{Enabled: true, Type: "signal"}); err != nil {
		t.Fatalf("save DB orphan: %v", err)
	}

	dir := filepath.Join(configdir.GetHomeDir(), "channels")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fsPath := filepath.Join(dir, "telegram.json")
	payload, err := json.Marshal(ChannelConfig{Enabled: true, Type: "telegram", BotToken: "legacy"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fsPath, payload, 0600); err != nil {
		t.Fatalf("write fs: %v", err)
	}

	migrated, err := AdoptOrphans("user-ana")
	if err != nil {
		t.Fatalf("AdoptOrphans: %v", err)
	}
	if len(migrated) != 1 || migrated[0] != "signal" {
		t.Fatalf("esperava só órfão DB signal, got %v", migrated)
	}

	raw, err := os.ReadFile(fsPath)
	if err != nil {
		t.Fatalf("read fs: %v", err)
	}
	var fsCfg ChannelConfig
	if err := json.Unmarshal(raw, &fsCfg); err != nil {
		t.Fatalf("unmarshal fs: %v", err)
	}
	if strings.TrimSpace(fsCfg.OwnerUserID) != "" {
		t.Fatalf("AdoptOrphans com DB não deveria escrever FS; OwnerUserID=%q", fsCfg.OwnerUserID)
	}
}

func TestImportLegacyChannels_Idempotent(t *testing.T) {
	setupTempHome(t)
	db := setupChannelsDB(t)

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
	contactsPayload := []byte(`{"telegram":[{"id":"42","display_name":"Ana","username":"ana","authorized_at":"2024-01-02T03:04:05Z"}]}`)
	if err := os.WriteFile(filepath.Join(configdir.GetHomeDir(), "contacts.json"), contactsPayload, 0600); err != nil {
		t.Fatalf("write contacts: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-ana")
	first, err := ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("import1: %v", err)
	}
	if first.Imported < 2 {
		t.Fatalf("esperava importar canal+contato (>=2), got %+v", first)
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

	channelID, _, err := ChannelIDBySlug("telegram")
	if err != nil || channelID == "" {
		t.Fatalf("ChannelIDBySlug: %v id=%q", err, channelID)
	}
	var contactCount int64
	if err := db.Model(&database.ChannelContact{}).
		Where("channel_id = ? AND external_id = ?", channelID, "42").
		Count(&contactCount).Error; err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	if contactCount != 1 {
		t.Fatalf("contacts.json não persistiu em channel_contacts: count=%d", contactCount)
	}
	var row database.ChannelContact
	if err := db.Where("channel_id = ? AND external_id = ?", channelID, "42").First(&row).Error; err != nil {
		t.Fatalf("load contact: %v", err)
	}
	if row.DisplayName != "Ana" || row.Username != "ana" || row.UserID != "user-ana" {
		t.Fatalf("contact row=%+v", row)
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

func TestSave_PreservesConversationsAndReplyChatIDsWhenNil(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	if err := Save("slack", &ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-1",
		Type:        "slack",
		Conversations: map[string]string{
			"U1": "conv-1",
		},
		ReplyChatIDs: map[string]string{
			"U1": "C1",
		},
	}); err != nil {
		t.Fatalf("Save inicial: %v", err)
	}

	// Partial save da UI (sem conversations / reply_chat_ids).
	if err := Save("slack", &ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-1",
		Type:        "slack",
		Profile:     "canais-comunicacao",
		MaxHistory:  40,
	}); err != nil {
		t.Fatalf("Save parcial: %v", err)
	}

	loaded, err := Load("slack")
	if err != nil || loaded == nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Conversations["U1"] != "conv-1" {
		t.Fatalf("conversations perdidas: %v", loaded.Conversations)
	}
	if loaded.ReplyChatIDs["U1"] != "C1" {
		t.Fatalf("reply_chat_ids perdidos: %v", loaded.ReplyChatIDs)
	}
	if loaded.MaxHistory != 40 {
		t.Fatalf("MaxHistory=%d", loaded.MaxHistory)
	}
}

func TestSave_AdoptsOrphanSameSlug(t *testing.T) {
	setupTempHome(t)
	db := setupChannelsDB(t)

	if err := Save("telegram", &ChannelConfig{Enabled: true, Type: "telegram"}); err != nil {
		t.Fatalf("orphan: %v", err)
	}
	if err := Save("telegram", &ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-1",
		Type:        "telegram",
		Profile:     "p",
	}); err != nil {
		t.Fatalf("adopt save: %v", err)
	}

	var count int64
	if err := db.Model(&database.Channel{}).Where("slug = ?", "telegram").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("esperava 1 row (adotada), got %d", count)
	}
	loaded, _ := Load("telegram")
	if loaded == nil || loaded.OwnerUserID != "user-1" || loaded.Profile != "p" {
		t.Fatalf("loaded=%+v", loaded)
	}
}
