package channels_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/channels"
	"assistente/internal/configdir"
	"assistente/internal/contacts"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupSmokeEnv prepara HOME temporário + SQLite com channels.UseDatabase e
// contacts.UseDatabase (mesmo par que bindMessagingDatabase liga no boot).
//
// Este arquivo vive no pacote de teste externo (channels_test) justamente
// para poder importar internal/contacts sem ciclo (contacts → channels).
func setupSmokeEnv(t *testing.T) *gorm.DB {
	t.Helper()

	tmp := t.TempDir()
	configdir.ResetForTests()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Cleanup(configdir.ResetForTests)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "smoke.db")), &gorm.Config{})
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

	channels.UseDatabase(db)
	contacts.UseDatabase(db)
	t.Cleanup(func() {
		channels.UseDatabase(nil)
		contacts.UseDatabase(nil)
		_ = sqlDB.Close()
	})
	return db
}

// TestAEP0083_LegacySmokeAutomatic cobre o checklist de smoke da AEP-0083
// que é viável sem tokens reais nem rede Telegram/Signal/Slack:
// HOME com JSON legado → UseDatabase (channels + contacts) → import
// (+idempotência) → Load / LoadEnabledForUser / contatos pela fachada
// contacts → cleanup dry-run + confirm → AdoptOrphans só DB.
func TestAEP0083_LegacySmokeAutomatic(t *testing.T) {
	db := setupSmokeEnv(t)

	home := configdir.GetHomeDir()
	channelsDir := filepath.Join(home, "channels")
	if err := os.MkdirAll(channelsDir, 0700); err != nil {
		t.Fatalf("mkdir channels: %v", err)
	}

	tgPayload, err := json.Marshal(channels.ChannelConfig{
		Enabled:     true,
		BotToken:    "plain-legacy-token",
		MaxContacts: 2,
		Conversations: map[string]string{
			"42": "conv-legacy",
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tgPath := filepath.Join(channelsDir, "telegram.json")
	if err := os.WriteFile(tgPath, tgPayload, 0600); err != nil {
		t.Fatalf("write telegram.json: %v", err)
	}
	contactsPath := filepath.Join(home, "contacts.json")
	contactsPayload := []byte(`{"telegram":[{"id":"42","display_name":"Ana","username":"ana","authorized_at":"2024-01-02T03:04:05Z"}]}`)
	if err := os.WriteFile(contactsPath, contactsPayload, 0600); err != nil {
		t.Fatalf("write contacts.json: %v", err)
	}

	// Órfão só no DB (fora do JSON) para AdoptOrphans no fim.
	if err := channels.Save("signal", &channels.ChannelConfig{Enabled: true, Type: "signal"}); err != nil {
		t.Fatalf("seed DB orphan signal: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-smoke")

	first, err := channels.ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("import1: %v", err)
	}
	if first.Imported < 2 {
		t.Fatalf("esperava canal+contato importados (>=2), got %+v", first)
	}

	second, err := channels.ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("import2: %v", err)
	}
	if second.Imported != 0 {
		t.Fatalf("segunda importação deveria ser idempotente (imported=0), got %+v", second)
	}

	loaded, err := channels.Load("telegram")
	if err != nil || loaded == nil {
		t.Fatalf("Load telegram: %v cfg=%v", err, loaded)
	}
	if loaded.OwnerUserID != "user-smoke" {
		t.Fatalf("owner=%q", loaded.OwnerUserID)
	}
	if loaded.Conversations["42"] != "conv-legacy" {
		t.Fatalf("conversations=%v", loaded.Conversations)
	}

	enabled, err := channels.LoadEnabledForUser("user-smoke")
	if err != nil {
		t.Fatalf("LoadEnabledForUser: %v", err)
	}
	if enabled["telegram"] == nil {
		t.Fatalf("telegram enabled ausente: %v", enabled)
	}
	// signal órfão (user_id="") também entra em ListForUser/LoadEnabledForUser.
	if enabled["signal"] == nil {
		t.Fatalf("signal órfão enabled ausente: %v", enabled)
	}

	// Contatos pela fachada (garante que contacts.UseDatabase está ligado).
	list, err := contacts.GetForChannel("telegram")
	if err != nil {
		t.Fatalf("contacts.GetForChannel: %v", err)
	}
	if len(list) != 1 || list[0].ID != "42" || list[0].DisplayName != "Ana" || list[0].Username != "ana" {
		t.Fatalf("contatos inesperados: %+v", list)
	}
	allContacts, err := contacts.Load()
	if err != nil {
		t.Fatalf("contacts.Load: %v", err)
	}
	if len(allContacts["telegram"]) != 1 {
		t.Fatalf("contacts.Load telegram=%v", allContacts["telegram"])
	}
	if has, allowed := contacts.IsAuthorized("telegram", 2, "42"); !has || !allowed {
		t.Fatalf("contato importado deveria estar autorizado; got (%v,%v)", has, allowed)
	}

	// Confirma o vínculo user_id na row de contato (AEP-0052).
	channelID, _, err := channels.ChannelIDBySlug("telegram")
	if err != nil || channelID == "" {
		t.Fatalf("ChannelIDBySlug: %v id=%q", err, channelID)
	}
	var contactRow database.ChannelContact
	if err := db.Where("channel_id = ? AND external_id = ?", channelID, "42").First(&contactRow).Error; err != nil {
		t.Fatalf("contato no DB: %v", err)
	}
	if contactRow.UserID != "user-smoke" {
		t.Fatalf("contact row user_id=%q", contactRow.UserID)
	}

	dry, err := channels.CleanupLegacyJSONFiles(ctx, channels.LegacyCleanupOptions{
		ContactsUsingDB: contacts.UsingDatabase(),
	})
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if !dry.DryRun || len(dry.Removed) != 0 {
		t.Fatalf("dry-run inesperado: %+v", dry)
	}
	if _, err := os.Stat(tgPath); err != nil {
		t.Fatalf("dry-run não deve apagar telegram.json: %v", err)
	}

	confirmed, err := channels.CleanupLegacyJSONFiles(ctx, channels.LegacyCleanupOptions{
		Confirm:         true,
		NoBackup:        true,
		ContactsUsingDB: contacts.UsingDatabase(),
	})
	if err != nil {
		t.Fatalf("cleanup confirm: %v", err)
	}
	if confirmed.DryRun {
		t.Fatal("confirm não deveria ser dry-run")
	}
	if len(confirmed.Removed) < 2 {
		t.Fatalf("esperava remover channel+contacts, got %+v", confirmed)
	}
	if _, err := os.Stat(tgPath); !os.IsNotExist(err) {
		t.Fatalf("telegram.json deveria ter sumido: %v", err)
	}
	if _, err := os.Stat(contactsPath); !os.IsNotExist(err) {
		t.Fatalf("contacts.json deveria ter sumido: %v", err)
	}

	// DB intacto após cleanup.
	loaded, err = channels.Load("telegram")
	if err != nil || loaded == nil || loaded.OwnerUserID != "user-smoke" {
		t.Fatalf("DB telegram após cleanup: err=%v cfg=%+v", err, loaded)
	}
	if list, err = contacts.GetForChannel("telegram"); err != nil || len(list) != 1 {
		t.Fatalf("contatos após cleanup: err=%v list=%v", err, list)
	}

	migrated, err := channels.AdoptOrphans("user-smoke")
	if err != nil {
		t.Fatalf("AdoptOrphans: %v", err)
	}
	if len(migrated) != 1 || migrated[0] != "signal" {
		t.Fatalf("AdoptOrphans deveria adotar só signal do DB, got %v", migrated)
	}
	sg, err := channels.Load("signal")
	if err != nil || sg == nil || sg.OwnerUserID != "user-smoke" {
		t.Fatalf("signal após adopt: err=%v cfg=%+v", err, sg)
	}
	// JSON já removido — AdoptOrphans não recria FS.
	if entries, _ := os.ReadDir(channelsDir); len(entries) != 0 {
		t.Fatalf("AdoptOrphans não deve reescrever JSON; entries=%v", entries)
	}
}
