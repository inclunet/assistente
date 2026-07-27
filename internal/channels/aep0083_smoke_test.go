package channels

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
)

// TestAEP0083_LegacySmokeAutomatic cobre o checklist de smoke da AEP-0083
// que é viável sem tokens reais nem rede Telegram/Signal/Slack:
// HOME com JSON legado → UseDatabase → import (+idempotência) → Load /
// LoadEnabledForUser / contatos no DB → cleanup dry-run + confirm →
// AdoptOrphans só DB.
//
// Contatos são assertados via GORM (não via pacote contacts) para evitar
// ciclo de import channels↔contacts nos testes.
func TestAEP0083_LegacySmokeAutomatic(t *testing.T) {
	setupTempHome(t)
	db := setupChannelsDB(t)

	home := configdir.GetHomeDir()
	channelsDir := filepath.Join(home, channelsSubdir)
	if err := os.MkdirAll(channelsDir, 0700); err != nil {
		t.Fatalf("mkdir channels: %v", err)
	}

	tgPayload, err := json.Marshal(ChannelConfig{
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
	if err := Save("signal", &ChannelConfig{Enabled: true, Type: "signal"}); err != nil {
		t.Fatalf("seed DB orphan signal: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-smoke")

	first, err := ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("import1: %v", err)
	}
	if first.Imported < 2 {
		t.Fatalf("esperava canal+contato importados (>=2), got %+v", first)
	}

	second, err := ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("import2: %v", err)
	}
	if second.Imported != 0 {
		t.Fatalf("segunda importação deveria ser idempotente (imported=0), got %+v", second)
	}

	loaded, err := Load("telegram")
	if err != nil || loaded == nil {
		t.Fatalf("Load telegram: %v cfg=%v", err, loaded)
	}
	if loaded.OwnerUserID != "user-smoke" {
		t.Fatalf("owner=%q", loaded.OwnerUserID)
	}
	if loaded.Conversations["42"] != "conv-legacy" {
		t.Fatalf("conversations=%v", loaded.Conversations)
	}

	enabled, err := LoadEnabledForUser("user-smoke")
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

	channelID, _, err := ChannelIDBySlug("telegram")
	if err != nil || channelID == "" {
		t.Fatalf("ChannelIDBySlug: %v id=%q", err, channelID)
	}
	var contact database.ChannelContact
	if err := db.Where("channel_id = ? AND external_id = ?", channelID, "42").First(&contact).Error; err != nil {
		t.Fatalf("contato no DB: %v", err)
	}
	if contact.DisplayName != "Ana" || contact.Username != "ana" || contact.UserID != "user-smoke" {
		t.Fatalf("contact row=%+v", contact)
	}

	dry, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{ContactsUsingDB: true})
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if !dry.DryRun || len(dry.Removed) != 0 {
		t.Fatalf("dry-run inesperado: %+v", dry)
	}
	if _, err := os.Stat(tgPath); err != nil {
		t.Fatalf("dry-run não deve apagar telegram.json: %v", err)
	}

	confirmed, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{
		Confirm:         true,
		NoBackup:        true,
		ContactsUsingDB: true,
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
	loaded, err = Load("telegram")
	if err != nil || loaded == nil || loaded.OwnerUserID != "user-smoke" {
		t.Fatalf("DB telegram após cleanup: err=%v cfg=%+v", err, loaded)
	}
	var contactCount int64
	if err := db.Model(&database.ChannelContact{}).
		Where("channel_id = ? AND external_id = ?", channelID, "42").
		Count(&contactCount).Error; err != nil || contactCount != 1 {
		t.Fatalf("contatos DB após cleanup: err=%v count=%d", err, contactCount)
	}

	migrated, err := AdoptOrphans("user-smoke")
	if err != nil {
		t.Fatalf("AdoptOrphans: %v", err)
	}
	if len(migrated) != 1 || migrated[0] != "signal" {
		t.Fatalf("AdoptOrphans deveria adotar só signal do DB, got %v", migrated)
	}
	sg, err := Load("signal")
	if err != nil || sg == nil || sg.OwnerUserID != "user-smoke" {
		t.Fatalf("signal após adopt: err=%v cfg=%+v", err, sg)
	}
	// JSON já removido — AdoptOrphans não recria FS.
	if entries, _ := os.ReadDir(channelsDir); len(entries) != 0 {
		t.Fatalf("AdoptOrphans não deve reescrever JSON; entries=%v", entries)
	}
}
