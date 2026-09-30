package portability

import (
	"assistente/internal/database"
	"encoding/json"
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
	var count int64
	if err = database.DB().Model(&database.CredentialEntry{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("import manufactured OAuth tokens")
	}
}
