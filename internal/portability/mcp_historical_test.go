package portability

import (
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestHistoricalMCPOAuthImportsForRecoveryWithoutLosingSecrets(t *testing.T) {
	for _, authType := range []string{"oauth2_pkce", "oauth2_client_credentials"} {
		t.Run(authType, func(t *testing.T) {
			setupPortabilityTestDB(t)
			ctx := portabilityTestCtx()
			manager := credentials.NewManagerWithStoreAndPersistence([]byte("01234567890123456789012345678901"), credentials.NewDBStore(), true)
			if err := manager.RegisterPatternWithContext(ctx, "mcp-client:historical", &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "original", ClientSecret: "retained-secret"}); err != nil {
				t.Fatal(err)
			}
			var before database.CredentialEntry
			if err := database.DB().Where("pattern = ?", "mcp-client:historical").First(&before).Error; err != nil {
				t.Fatal(err)
			}
			blob, err := EncryptCredentialsPayload("backup-password", []CredentialExport{{Pattern: "resource.example", Source: "static", AuthType: "bearer", Token: "preserved-backup-token"}})
			if err != nil {
				t.Fatal(err)
			}
			server := MCPServerExport{Slug: "historical", Name: "Historical", Transport: "streamable", URL: "https://resource.example/mcp", AuthType: authType, OAuth2ClientID: "original", OAuth2AuthURL: "https://auth.example/authorize", OAuth2TokenURL: "https://auth.example/token", OAuth2CallbackPort: 8765, OAuth2CallbackHost: "127.0.0.1", OAuth2Scopes: []string{"read"}, Enabled: true, AutoConnect: true}
			file := ExportFile{Version: ExportVersion, Options: ExportOptions{IncludeCredentials: true}, Resources: ExportResources{MCPServers: []MCPServerExport{server}, Credentials: blob}}
			raw, err := json.Marshal(file)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ImportConversationsWithContext(ctx, string(raw), manager, "backup-password")
			if err != nil || !result.Success || len(result.Warnings) != 1 || result.Warnings[0].Code != "mcpServer.oauthRecoveryRequired" {
				t.Fatalf("import: %v %+v", err, result)
			}
			var row database.MCPServer
			if err := database.DB().Where("slug = ?", server.Slug).First(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.Enabled || row.AutoConnect || row.OAuthManaged || row.OAuthAuthorizationID != "" || row.OAuth2ClientID != server.OAuth2ClientID || row.OAuth2TokenURL != server.OAuth2TokenURL || row.OAuth2AuthURL != server.OAuth2AuthURL || row.OAuth2CallbackPort != 8765 || row.OAuth2CallbackHost != "127.0.0.1" || row.OAuth2Scopes != `["read"]` {
				t.Fatal("recovery configuration lost metadata or became active")
			}
			var after database.CredentialEntry
			if err := database.DB().First(&after, "id = ?", before.ID).Error; err != nil {
				t.Fatal(err)
			}
			if after.ClientSecretEnc != before.ClientSecretEnc {
				t.Fatal("existing secret changed")
			}
			backup, err := manager.GetByPatternWithContext(ctx, "resource.example")
			if err != nil || backup.Token != "preserved-backup-token" {
				t.Fatal("backup token lost", err)
			}
			// Explicit later activation must not be undone by an idempotent import.
			if err := database.DB().Model(&row).Updates(map[string]any{"enabled": true, "auto_connect": true}).Error; err != nil {
				t.Fatal(err)
			}
			result, err = ImportConversationsWithContext(ctx, string(raw), manager, "backup-password")
			if err != nil || len(result.Warnings) != 0 || result.SkippedMCPServerConflict != 1 {
				t.Fatalf("reimport: %v %+v", err, result)
			}
			if err := database.DB().First(&row, "id = ?", row.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !row.Enabled || !row.AutoConnect {
				t.Fatal("reimport modified existing settings")
			}
		})
	}
}

func TestHistoricalMCPOAuthDisableFailureRollsBackImport(t *testing.T) {
	setupPortabilityTestDB(t)
	if err := database.DB().Callback().Update().Before("gorm:update").Register("reject_recovery", func(tx *gorm.DB) {
		if tx.Statement.Table == "mcp_servers" {
			_ = tx.AddError(errors.New("injected"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.DB().Callback().Update().Remove("reject_recovery") })
	imported, err := ImportMCPServerWithContext(portabilityTestCtx(), MCPServerExport{Slug: "historical", Transport: "streamable", URL: "https://resource.example/mcp", AuthType: "oauth2_pkce", Enabled: true, AutoConnect: true})
	if err == nil || imported {
		t.Fatal("failed recovery import accepted")
	}
	var count int64
	if err := database.DB().Model(&database.MCPServer{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial active server persisted", err)
	}
}
