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
	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).Where("user_id = ?", "destination-owner").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("restore created split/duplicate entries", err)
	}
}

func TestStaticConnectionBackupRejectsAmbiguousPayload(t *testing.T) {
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
