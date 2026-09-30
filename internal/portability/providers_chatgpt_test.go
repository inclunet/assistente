package portability

import (
	"assistente/internal/database"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestChatGPTProviderRoundTripWithoutOAuthSecrets(t *testing.T) {
	setupPortabilityTestDB(t)
	exported, err := exportProvider(&database.LLMProvider{ID: "chatgpt-import", Name: "Personal", Type: "chatgpt", APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1", CredentialPattern: "oauth:source-host"})
	if err != nil {
		t.Fatal(err)
	}
	file := ExportFile{Version: ExportVersion, ExportedAt: time.Now(), Resources: ExportResources{Providers: []ProviderExport{exported}}}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, ""); err != nil {
		t.Fatal(err)
	}
	provider, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "chatgpt-import")
	if err != nil || provider == nil || provider.Type != "chatgpt" {
		t.Fatalf("provider: %v", err)
	}
	if provider.CredentialPattern == exported.CredentialPattern || !strings.HasPrefix(provider.CredentialPattern, "oauth:") {
		t.Fatal("import retained foreign authorization reference")
	}
	var count int64
	if err = database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("import manufactured OAuth tokens")
	}
}

func TestOAuthImportCannotBindAnotherLocalAuthorization(t *testing.T) {
	for _, providerType := range []string{"chatgpt", "openai"} {
		t.Run(providerType, func(t *testing.T) {
			setupPortabilityTestDB(t)
			ctx := portabilityTestCtx()
			user, _ := database.RequireUserID(ctx)
			entry := database.CredentialEntry{UUIDModel: database.UUIDModel{ID: "local-grant"}, UserID: user, Source: "oauth", Pattern: "oauth:local-grant", OAuthEnc: "existing-envelope"}
			if err := database.DB().Create(&entry).Error; err != nil {
				t.Fatal(err)
			}
			incoming := ProviderExport{ID: "new-consumer", Name: "Imported", Type: providerType, APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1", CredentialPattern: "oauth:local-grant"}
			if _, err := importProvider(ctx, incoming); err != nil {
				t.Fatal(err)
			}
			first, err := database.GetLLMProviderWithContext(ctx, incoming.ID)
			if err != nil {
				t.Fatal(err)
			}
			if first.CredentialPattern == incoming.CredentialPattern {
				t.Fatal("bound to existing grant")
			}
			incoming.ID = "copy"
			incoming.CredentialPattern = first.CredentialPattern
			if _, err = importProvider(ctx, incoming); err != nil {
				t.Fatal(err)
			}
			second, _ := database.GetLLMProviderWithContext(ctx, incoming.ID)
			if second.CredentialPattern == first.CredentialPattern {
				t.Fatal("copies share authorization reference")
			}
			var count int64
			if err = database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("credential count=%d err=%v", count, err)
			}
		})
	}
}
func TestOAuthOverwritePreservesOnlyTrustedLocalAssociation(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	local := &database.LLMProvider{ID: "existing-consumer", Name: "Local", Type: "chatgpt", APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1", CredentialPattern: "oauth:owned"}
	if err := database.SaveLLMProviderWithContext(ctx, local); err != nil {
		t.Fatal(err)
	}
	incoming := ProviderExport{ID: local.ID, Name: "Renamed", Type: local.Type, APIFormat: local.APIFormat, BaseURL: local.BaseURL, CredentialPattern: "oauth:someone-else"}
	if _, err := overwriteProvider(ctx, incoming); err != nil {
		t.Fatal(err)
	}
	saved, err := database.GetLLMProviderWithContext(ctx, local.ID)
	if err != nil || saved.CredentialPattern != local.CredentialPattern || saved.Name != incoming.Name {
		t.Fatalf("association replaced: %+v %v", saved, err)
	}
}

func TestChatGPTImportNormalizesFixedConnection(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		for _, format := range []string{"", "openai", "acp"} {
			t.Run(fmt.Sprintf("overwrite=%v/format=%s", overwrite, format), func(t *testing.T) {
				setupPortabilityTestDB(t)
				ctx := portabilityTestCtx()
				if overwrite {
					local := &database.LLMProvider{ID: "connection", Name: "Local", Type: "chatgpt", APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1", CredentialPattern: "oauth:owned"}
					if err := database.SaveLLMProviderWithContext(ctx, local); err != nil {
						t.Fatal(err)
					}
				}
				incoming := ProviderExport{ID: "connection", Name: "Imported", Type: "chatgpt", APIFormat: format, BaseURL: "https://invalid.example/v1", CredentialPattern: "oauth:foreign"}
				if _, err := importProvider(ctx, incoming); err != nil {
					t.Fatal(err)
				}
				saved, err := database.GetLLMProviderWithContext(ctx, incoming.ID)
				if err != nil {
					t.Fatal(err)
				}
				if saved.Type != "chatgpt" || saved.BaseURL != "https://api.openai.com/v1" || saved.APIFormat != "openai_responses" {
					t.Fatal("invalid configuration persisted")
				}
				if overwrite && saved.CredentialPattern != "oauth:owned" {
					t.Fatal("local connection replaced")
				}
				if !overwrite && saved.CredentialPattern == incoming.CredentialPattern {
					t.Fatal("foreign grant retained")
				}
			})
		}
	}
}
