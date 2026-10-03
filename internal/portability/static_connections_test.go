package portability

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

func TestStaticConnectionPasswordBackupRoundTripAndConflict(t *testing.T) {
	setupPortabilityTestDB(t)
	if err := database.DB().AutoMigrate(&database.Channel{}); err != nil {
		t.Fatal(err)
	}
	mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "source-owner")
	input := CredentialExport{Source: "static", AuthType: credentials.StaticConnectionType, Pattern: slackConnectionBackupPattern, StaticComponents: map[credentials.SecretRole]string{credentials.RoleBotToken: "synthetic-bot-token", credentials.RoleAppToken: "synthetic-app-token"}}
	if err := importStaticConnection(ctx, mgr, input, false); err != nil {
		t.Fatal(err)
	}
	without, err := BuildExportFileWithContext(ctx, nil, nil, nil, mgr, ExportRequest{}, "test")
	if err != nil || without.Resources.Credentials != nil {
		t.Fatal("credentials exported without opt-in", err)
	}
	if _, err := BuildExportFileWithContext(ctx, nil, nil, nil, mgr, ExportRequest{IncludeCredentials: true}, "test"); err == nil {
		t.Fatal("passwordless export allowed")
	}
	with, err := BuildExportFileWithContext(ctx, nil, nil, nil, mgr, ExportRequest{IncludeCredentials: true, CredentialExportPassword: "synthetic-backup-password"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(with)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "synthetic-bot-token") || strings.Contains(string(wire), "synthetic-app-token") || strings.Contains(string(wire), "staticComponents") {
		t.Fatal("plaintext components leaked")
	}
	if _, err := decodeCredentialExports(with.Resources.Credentials, "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	items, err := decodeCredentialExports(with.Resources.Credentials, "synthetic-backup-password")
	if err != nil || len(items) != 1 || items[0].StaticComponents[credentials.RoleAppToken] != input.StaticComponents[credentials.RoleAppToken] {
		t.Fatal("backup lost composed record", err)
	}
	destination := database.WithUserID(context.Background(), "destination-owner")
	n, skipped, err := importCredentials(destination, mgr, with.Resources.Credentials, "synthetic-backup-password", nil, nil)
	if err != nil || n != 1 || skipped != 0 {
		t.Fatal("restore failed", err)
	}
	var row database.Channel
	if err := database.DB().Where("user_id = ?", "destination-owner").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Enabled || row.CredentialID == "" {
		t.Fatal("restore activated channel or missed binding")
	}
	resolved, err := mgr.ResolveStaticComponents(destination, row.CredentialID, "slack", row.ID)
	if err != nil || resolved[credentials.RoleBotToken] != input.StaticComponents[credentials.RoleBotToken] {
		t.Fatal("restored secret unavailable", err)
	}
	if _, err := mgr.ResolveStaticComponents(ctx, row.CredentialID, "slack", row.ID); err == nil {
		t.Fatal("restored record retained source owner")
	}
	_, patterns, err := loadExistingCredentialIdentifiers(destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := patterns[slackConnectionBackupPattern]; !found {
		t.Fatal("existing connection not detected")
	}
	n, skipped, err = importCredentials(destination, mgr, with.Resources.Credentials, "synthetic-backup-password", patterns, nil)
	if err != nil || n != 0 || skipped != 1 {
		t.Fatal("conflict not skipped by default", err)
	}
	resolutions := buildImportResolutionMap([]ImportResolution{{ResourceType: "credential", Identifier: slackConnectionBackupPattern, Strategy: ConflictResolutionOverwrite}})
	n, _, err = importCredentials(destination, mgr, with.Resources.Credentials, "synthetic-backup-password", patterns, resolutions)
	if err != nil || n != 1 {
		t.Fatal("explicit overwrite failed", err)
	}
	for _, missing := range []bool{false, true} {
		if missing {
			if err := database.DB().Delete(&database.CredentialEntry{}, "id = ?", row.CredentialID).Error; err != nil {
				t.Fatal(err)
			}
		} else if err := database.DB().Model(&database.CredentialEntry{}).Where("id = ?", row.CredentialID).Update("token_enc", "unreadable").Error; err != nil {
			t.Fatal(err)
		}
		n, _, err = importCredentials(destination, mgr, with.Resources.Credentials, "synthetic-backup-password", patterns, resolutions)
		if err != nil || n != 1 {
			t.Fatal("backup could not reconstruct missing/unreadable connection", err)
		}
		pair, err := mgr.ResolveStaticComponents(destination, row.CredentialID, "slack", row.ID)
		if err != nil || pair[credentials.RoleAppToken] != "synthetic-app-token" {
			t.Fatal("reconstructed backup lost token", err)
		}
	}
	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Where("user_id = ?", "destination-owner").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("restore created split/duplicate entries", err)
	}
}

func TestHistoricalSlackBackupRestoresOverComposedConnection(t *testing.T) {
	setupPortabilityTestDB(t)
	if err := database.DB().AutoMigrate(&database.Channel{}); err != nil {
		t.Fatal(err)
	}
	mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	current := CredentialExport{Pattern: slackConnectionBackupPattern, Source: "static", AuthType: credentials.StaticConnectionType, StaticComponents: map[credentials.SecretRole]string{credentials.RoleBotToken: "current-bot", credentials.RoleAppToken: "current-app"}}
	if err := importStaticConnection(ctx, mgr, current, false); err != nil {
		t.Fatal(err)
	}
	var row database.Channel
	if err := database.DB().Where("user_id = ?", "owner").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	legacy := []CredentialExport{{ID: row.CredentialID, Pattern: "channel:slack:bot_token", AuthType: "secret", Token: "historical-bot"}, {Pattern: "channel:slack:app_token", Source: "static", AuthType: "secret", Token: "historical-app"}}
	blob, err := EncryptCredentialsPayload("synthetic-password", legacy)
	if err != nil {
		t.Fatal(err)
	}
	file := &ExportFile{Version: ExportVersion, Options: ExportOptions{IncludeCredentials: true}, Resources: ExportResources{Credentials: blob}}
	analysis, err := analyzeImportFile(ctx, file, mgr, "synthetic-password")
	if err != nil || analysis.CredentialCount != 1 || len(analysis.CredentialConflicts) != 1 || analysis.CredentialConflicts[0].Identifier != slackConnectionBackupPattern {
		t.Fatal("historical conflict missing", err)
	}
	_, patterns, err := loadExistingCredentialIdentifiers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n, skipped, err := importCredentials(ctx, mgr, blob, "synthetic-password", patterns, nil); err != nil || n != 0 || skipped != 1 {
		t.Fatal("historical restore bypassed conflict", err)
	}
	resolutions := buildImportResolutionMap([]ImportResolution{{ResourceType: "credential", Identifier: slackConnectionBackupPattern, Strategy: ConflictResolutionOverwrite}})
	if n, _, err := importCredentials(ctx, mgr, blob, "synthetic-password", patterns, resolutions); err != nil || n != 1 {
		t.Fatal("historical restore failed", err)
	}
	pair, err := mgr.ResolveStaticComponents(ctx, row.CredentialID, "slack", row.ID)
	if err != nil || pair[credentials.RoleBotToken] != "historical-bot" || pair[credentials.RoleAppToken] != "historical-app" {
		t.Fatal("historical tokens not restored", err)
	}
	partial, err := EncryptCredentialsPayload("synthetic-password", legacy[:1])
	if err != nil {
		t.Fatal(err)
	}
	if n, _, err := importCredentials(ctx, mgr, partial, "synthetic-password", patterns, resolutions); err != nil || n != 1 {
		t.Fatal("partial historical restore failed", err)
	}
	pair, err = mgr.ResolveStaticComponents(ctx, row.CredentialID, "slack", row.ID)
	if err != nil || pair[credentials.RoleAppToken] != "historical-app" {
		t.Fatal("partial backup discarded omitted role", err)
	}
	if err := mgr.RegisterPatternWithContext(ctx, "channel:slack:bot_token", &credentials.AuthConfig{Source: "static", Type: "secret", Token: "late"}); err == nil {
		t.Fatal("generic barrier removed")
	}
	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("legacy pair recreated", err)
	}
}

func TestStaticConnectionRestoreConsolidatesUnreferencedLegacyPair(t *testing.T) {
	setupPortabilityTestDB(t)
	if err := database.DB().AutoMigrate(&database.Channel{}); err != nil {
		t.Fatal(err)
	}
	mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	for _, role := range []string{"bot_token", "app_token"} {
		if err := mgr.RegisterPatternWithContext(ctx, "channel:slack:"+role, &credentials.AuthConfig{Source: "static", Type: "secret", Token: "old-" + role}); err != nil {
			t.Fatal(err)
		}
	}
	input := CredentialExport{Pattern: slackConnectionBackupPattern, Source: "static", AuthType: credentials.StaticConnectionType, StaticComponents: map[credentials.SecretRole]string{credentials.RoleBotToken: "new-bot", credentials.RoleAppToken: "new-app"}}
	_, patterns, err := loadExistingCredentialIdentifiers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := patterns[slackConnectionBackupPattern]; !exists {
		t.Fatal("orphan pair omitted from conflicts")
	}
	if err := importStaticConnection(ctx, mgr, input, false); err == nil {
		t.Fatal("orphan pair overwritten without conflict decision")
	}
	if err := importStaticConnection(ctx, mgr, input, true); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("restore left orphan pair", err)
	}
	items, err := exportCredentials(ctx, mgr)
	if err != nil || len(items) != 1 || items[0].Pattern != slackConnectionBackupPattern {
		t.Fatal("export includes stale entries", err)
	}
}

func TestStaticConnectionBackupRejectsAmbiguousPayload(t *testing.T) {
	if got := messageFromError(credentials.ErrStaticConnection); got.Code != CodeCredentialConnectionUnavailable {
		t.Fatal("connection failure is not localized")
	}
	for _, item := range []CredentialExport{
		{Pattern: slackConnectionBackupPattern, Source: "command", AuthType: credentials.StaticConnectionType},
		{Pattern: slackConnectionBackupPattern, Source: "static", AuthType: credentials.StaticConnectionType, Token: "unexpected"},
		{Pattern: "connection:foreign", Source: "static", AuthType: credentials.StaticConnectionType},
		{Pattern: slackConnectionBackupPattern, Source: "static", AuthType: credentials.StaticConnectionType, StaticComponents: map[credentials.SecretRole]string{"unknown": "secret"}},
	} {
		if err := validatePortableCredentialExport(item); err == nil {
			t.Fatal("ambiguous backup accepted")
		}
	}
}
