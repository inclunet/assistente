package channels

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
)

func TestCleanupLegacyJSON_DryRunListsEligible(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	home := configdir.GetHomeDir()
	channelsDir := filepath.Join(home, channelsSubdir)
	if err := os.MkdirAll(channelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	tgPath := filepath.Join(channelsDir, "telegram.json")
	orphanPath := filepath.Join(channelsDir, "orphan.json")
	if err := os.WriteFile(tgPath, []byte(`{"enabled":true,"type":"telegram"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte(`{"enabled":false,"type":"orphan"}`), 0600); err != nil {
		t.Fatal(err)
	}
	contactsPath := filepath.Join(home, "contacts.json")
	if err := os.WriteFile(contactsPath, []byte(`{"telegram":[{"id":"1"}]}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Save("telegram", &ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
		Type:        "telegram",
		DisplayName: "Telegram",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-a")
	result, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{ContactsUsingDB: true})
	if err != nil {
		t.Fatalf("CleanupLegacyJSONFiles: %v", err)
	}
	if !result.DryRun {
		t.Fatal("esperava dry-run")
	}
	if len(result.Removed) != 0 {
		t.Fatalf("dry-run não deve remover: %v", result.Removed)
	}
	if _, err := os.Stat(tgPath); err != nil {
		t.Fatalf("arquivo não deveria ser apagado no dry-run: %v", err)
	}

	var foundTG, foundContacts, foundOrphan bool
	for _, item := range result.Eligible {
		switch {
		case item.Kind == "channel" && item.Slug == "telegram":
			foundTG = true
		case item.Kind == "contacts":
			foundContacts = true
		case item.Kind == "channel" && item.Slug == "orphan":
			foundOrphan = true
		}
	}
	if !foundTG || !foundContacts {
		t.Fatalf("elegíveis incompletos: %+v", result.Eligible)
	}
	if foundOrphan {
		t.Fatal("orphan não deveria ser elegível")
	}
	var orphanSkipped bool
	for _, item := range result.Skipped {
		if item.Slug == "orphan" {
			orphanSkipped = true
		}
	}
	if !orphanSkipped {
		t.Fatalf("orphan deveria estar em skipped: %+v", result.Skipped)
	}
}

func TestCleanupLegacyJSON_ConfirmBackupAndRemove(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	home := configdir.GetHomeDir()
	channelsDir := filepath.Join(home, channelsSubdir)
	if err := os.MkdirAll(channelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	tgPath := filepath.Join(channelsDir, "telegram.json")
	payload := []byte(`{"enabled":true,"type":"telegram","bot_token":"secret"}`)
	if err := os.WriteFile(tgPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	contactsPath := filepath.Join(home, "contacts.json")
	if err := os.WriteFile(contactsPath, []byte(`{"telegram":[{"id":"42"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Save("telegram", &ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-a",
		Type:        "telegram",
		DisplayName: "Telegram",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-a")
	result, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{
		Confirm:         true,
		ContactsUsingDB: true,
	})
	if err != nil {
		t.Fatalf("CleanupLegacyJSONFiles: %v", err)
	}
	if result.DryRun {
		t.Fatal("não deveria ser dry-run")
	}
	if len(result.Removed) != 2 {
		t.Fatalf("removed=%v", result.Removed)
	}
	if result.BackedUpTo == "" {
		t.Fatal("esperava BackedUpTo")
	}
	if _, err := os.Stat(tgPath); !os.IsNotExist(err) {
		t.Fatalf("telegram.json deveria ter sido removido: %v", err)
	}
	if _, err := os.Stat(contactsPath); !os.IsNotExist(err) {
		t.Fatalf("contacts.json deveria ter sido removido: %v", err)
	}
	backupTG := filepath.Join(result.BackedUpTo, channelsSubdir, "telegram.json")
	data, err := os.ReadFile(backupTG)
	if err != nil {
		t.Fatalf("backup telegram: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("conteúdo do backup diverge")
	}
	if !strings.Contains(result.BackedUpTo, legacyBackupDirName) {
		t.Fatalf("backup path inesperado: %s", result.BackedUpTo)
	}
}

func TestCleanupLegacyJSON_DBOff(t *testing.T) {
	setupTempHome(t)
	resetStoreForTests()

	ctx := database.WithUserID(context.Background(), "user-a")
	_, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{ContactsUsingDB: true})
	if err == nil || !strings.Contains(err.Error(), "channels DB não habilitado") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestCleanupLegacyJSON_ContactsDBOff(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	ctx := database.WithUserID(context.Background(), "user-a")
	_, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{ContactsUsingDB: false})
	if err == nil || !strings.Contains(err.Error(), "contacts DB não habilitado") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestCleanupLegacyJSON_DoesNotDeleteWhenChannelMissing(t *testing.T) {
	setupTempHome(t)
	setupChannelsDB(t)

	home := configdir.GetHomeDir()
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "contacts.json"), []byte(`{"telegram":[{"id":"1"}]}`), 0600); err != nil {
		t.Fatal(err)
	}

	ctx := database.WithUserID(context.Background(), "user-a")
	result, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{
		Confirm:         true,
		ContactsUsingDB: true,
	})
	if err != nil {
		t.Fatalf("CleanupLegacyJSONFiles: %v", err)
	}
	if len(result.Removed) != 0 {
		t.Fatalf("não deveria remover: %v", result.Removed)
	}
	var skippedContacts bool
	for _, item := range result.Skipped {
		if item.Kind == "contacts" {
			skippedContacts = true
		}
	}
	if !skippedContacts {
		t.Fatalf("contacts deveria ser skipped: %+v", result.Skipped)
	}
	if _, err := os.Stat(filepath.Join(home, "contacts.json")); err != nil {
		t.Fatalf("contacts.json deve permanecer: %v", err)
	}
}

func TestCleanupLegacyJSON_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup varia no Windows sem privilégio de desenvolvedor")
	}
	setupTempHome(t)
	setupChannelsDB(t)

	home := configdir.GetHomeDir()
	channelsDir := filepath.Join(home, channelsSubdir)
	if err := os.MkdirAll(channelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(realFile, []byte(`{"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(channelsDir, "telegram.json")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := Save("telegram", &ChannelConfig{
		Enabled: true, OwnerUserID: "user-a", Type: "telegram", DisplayName: "Telegram",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ctx := database.WithUserID(context.Background(), "user-a")
	result, err := CleanupLegacyJSONFiles(ctx, LegacyCleanupOptions{
		Confirm: true, ContactsUsingDB: true,
	})
	if err != nil {
		t.Fatalf("CleanupLegacyJSONFiles: %v", err)
	}
	if len(result.Removed) != 0 {
		t.Fatalf("symlink não deveria ser removido: %v", result.Removed)
	}
	var skippedSymlink bool
	for _, item := range result.Skipped {
		if item.Slug == "telegram" && strings.Contains(item.Reason, "symlink") {
			skippedSymlink = true
		}
	}
	if !skippedSymlink {
		t.Fatalf("esperava symlink em skipped: %+v errors=%v", result.Skipped, result.Errors)
	}
}
