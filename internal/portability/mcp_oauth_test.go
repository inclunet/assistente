package portability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func TestExternalMCPOAuthUsesAtomicPendingAuthorization(t *testing.T) {
	for _, generic := range []bool{false, true} {
		t.Run(map[bool]string{false: "mcp", true: "data"}[generic], func(t *testing.T) {
			setupPortabilityTestDB(t)
			ctx := portabilityTestCtx()
			key := []byte("01234567890123456789012345678901")
			manager := credentials.NewManagerWithStoreAndPersistence(key, credentials.NewDBStore(), true)
			payload := `{"mcpServers":{"remote":{"url":"https://unreachable.invalid/mcp"}}}`
			importOne := func() {
				t.Helper()
				if generic {
					result, err := ImportConversationsWithContext(ctx, payload, manager, "")
					if err != nil || !result.Success {
						t.Fatalf("import: %v %+v", err, result)
					}
				} else {
					result, err := ImportMCPServersJSONWithContext(ctx, []byte(payload), manager)
					if err != nil || result.Failed != 0 {
						t.Fatalf("import: %v %+v", err, result)
					}
				}
			}
			importOne()
			importOne()
			var row database.MCPServer
			if err := database.DB().Where("user_id = ? AND slug = ?", portabilityTestUserID, "remote").First(&row).Error; err != nil {
				t.Fatal(err)
			}
			if !row.OAuthManaged || row.OAuthAuthorizationID == "" || row.AutoConnect || row.OAuth2Scopes != "" {
				t.Fatal("import did not persist an exclusively managed, non-autoconnecting consumer")
			}
			// A new manager simulates a restart and must decrypt the same record.
			restarted := credentials.NewManagerWithStoreAndPersistence(key, credentials.NewDBStore(), true)
			store, err := restarted.OAuthStore(ctx)
			if err != nil {
				t.Fatal(err)
			}
			r, err := store.Load(ctx, row.OAuthAuthorizationID)
			if err != nil {
				t.Fatal(err)
			}
			if r.ConsumerID != row.ID || r.UserID != row.UserID || r.Resource != row.URL || r.State != "pending" || r.Tokens != (oauthflow.Tokens{}) || r.Client.ID != "" {
				t.Fatal("invalid pending authorization")
			}
			if _, err := oauthflow.NewConfigured(r, nil); err != nil {
				t.Fatal(err)
			}
			var entries []database.CredentialEntry
			if err := database.DB().Find(&entries).Error; err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Source != "oauth" || entries[0].OAuthEnc == "" || strings.Contains(entries[0].OAuthEnc, row.URL) {
				t.Fatal("expected one encrypted OAuth entry")
			}
			other, err := restarted.OAuthStore(database.WithUserID(context.Background(), "other-user"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := other.Load(database.WithUserID(context.Background(), "other-user"), r.ID); err == nil {
				t.Fatal("cross-user authorization read")
			}
		})
	}
}

func TestExternalMCPOAuthFailureLeavesNoPartialImport(t *testing.T) {
	for _, failure := range []string{"missing_vault", "locked_vault", "consumer_write"} {
		t.Run(failure, func(t *testing.T) {
			setupPortabilityTestDB(t)
			var manager *credentials.Manager
			switch failure {
			case "locked_vault":
				manager = credentials.NewManager(nil)
			case "consumer_write":
				manager = credentials.NewManagerWithStoreAndPersistence([]byte("01234567890123456789012345678901"), credentials.NewDBStore(), true)
				if err := database.DB().Callback().Create().Before("gorm:create").Register("fail_mcp_import", func(tx *gorm.DB) {
					if tx.Statement.Table == "mcp_servers" {
						_ = tx.AddError(errors.New("injected"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = database.DB().Callback().Create().Remove("fail_mcp_import") })
			}
			result, err := ImportMCPServersJSONWithContext(portabilityTestCtx(), []byte(`{"mcpServers":{"remote":{"url":"https://resource.example/mcp"}}}`), manager)
			if err != nil || result.Failed != 1 || result.Imported != 0 {
				t.Fatalf("failure not reported: %v %+v", err, result)
			}
			for _, model := range []any{&database.MCPServer{}, &database.CredentialEntry{}} {
				var count int64
				if err := database.DB().Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("partial import: %d %v", count, err)
				}
			}
		})
	}
}

func TestExternalMCPOAuthSkipsExistingLegacyWithoutVault(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	old := MCPServerExport{Slug: "remote", URL: "https://old.example/mcp", Transport: "streamable", AuthType: "oauth2_pkce", OAuth2ClientID: "old-client"}
	if _, err := ImportMCPServerWithContext(ctx, old); err != nil {
		t.Fatal(err)
	}
	result, err := ImportMCPServersJSONWithContext(ctx, []byte(`{"mcpServers":{"remote":{"url":"https://new.example/mcp"}}}`), nil)
	if err != nil || result.Skipped != 1 || result.Failed != 0 {
		t.Fatalf("existing import: %v %+v", err, result)
	}
	var row database.MCPServer
	if err := database.DB().Where("slug = ?", "remote").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.OAuthManaged || row.OAuthAuthorizationID != "" || row.OAuth2ClientID != old.OAuth2ClientID || row.URL != old.URL {
		t.Fatal("existing legacy configuration modified")
	}
}

func TestExternalMCPOAuthVaultErrorIsLocalized(t *testing.T) {
	setupPortabilityTestDB(t)
	for _, manager := range []*credentials.Manager{nil, credentials.NewManager(nil)} {
		result, err := ImportConversationsWithContext(portabilityTestCtx(), `{"mcpServers":{"remote":{"url":"https://resource.example/mcp"}}}`, manager, "")
		if err != nil || len(result.Errors) != 1 || result.Errors[0].Code != CodeCredentialVaultUnavailableImport {
			t.Fatalf("untranslated vault error: %v %+v", err, result)
		}
	}
}
