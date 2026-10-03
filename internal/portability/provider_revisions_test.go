package portability

import (
	"testing"

	"assistente/internal/database"
)

func TestProviderImportAdvancesOnlyRelevantRevisions(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	provider := &database.LLMProvider{
		ID:                "import-revisions",
		Name:              "Imported provider",
		Type:              "openai",
		APIFormat:         "openai",
		BaseURL:           "https://api.example/v1",
		Model:             "model-a",
		CredentialPattern: "api-key",
	}
	if err := database.SaveLLMProviderWithContext(ctx, provider); err != nil {
		t.Fatal(err)
	}
	initial, err := database.GetLLMProviderWithContext(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := overwriteProvider(ctx, ProviderExport{
		ID: provider.ID, Name: provider.Name, Type: provider.Type,
		APIFormat: provider.APIFormat, BaseURL: "https://other.example/v1",
		Model: provider.Model, CredentialPattern: provider.CredentialPattern,
	}); err != nil {
		t.Fatal(err)
	}
	connectionChanged, err := database.GetLLMProviderWithContext(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if connectionChanged.ConfigRevision != initial.ConfigRevision+1 {
		t.Fatalf("config revision after endpoint import = %d, want %d", connectionChanged.ConfigRevision, initial.ConfigRevision+1)
	}
	if connectionChanged.CompatibilityRevision != initial.CompatibilityRevision+1 {
		t.Fatalf("compatibility revision after endpoint import = %d, want %d", connectionChanged.CompatibilityRevision, initial.CompatibilityRevision+1)
	}

	if _, err := overwriteProvider(ctx, ProviderExport{
		ID: provider.ID, Name: provider.Name, Type: provider.Type,
		APIFormat: provider.APIFormat, BaseURL: connectionChanged.BaseURL,
		Model: "model-b", CredentialPattern: provider.CredentialPattern,
	}); err != nil {
		t.Fatal(err)
	}
	modelChanged, err := database.GetLLMProviderWithContext(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if modelChanged.ConfigRevision != connectionChanged.ConfigRevision+1 {
		t.Fatalf("config revision after model import = %d, want %d", modelChanged.ConfigRevision, connectionChanged.ConfigRevision+1)
	}
	if modelChanged.CompatibilityRevision != connectionChanged.CompatibilityRevision {
		t.Fatalf("model-only import changed compatibility revision from %d to %d", connectionChanged.CompatibilityRevision, modelChanged.CompatibilityRevision)
	}
}

func TestProviderImportDefaultSwitchAdvancesPreviousConfigRevision(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	previous := &database.LLMProvider{
		ID: "previous-default", Name: "Previous", Type: "openai", APIFormat: "openai",
		BaseURL: "https://previous.example/v1", IsDefault: true,
	}
	if err := database.SaveLLMProviderWithContext(ctx, previous); err != nil {
		t.Fatal(err)
	}
	initial, err := database.GetLLMProviderWithContext(ctx, previous.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := importProvider(ctx, ProviderExport{
		ID: "new-default", Name: "New", Type: "openai", APIFormat: "openai",
		BaseURL: "https://new.example/v1", IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}
	cleared, err := database.GetLLMProviderWithContext(ctx, previous.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.IsDefault || cleared.ConfigRevision != initial.ConfigRevision+1 {
		t.Fatalf("previous default after import: %+v; initial=%+v", cleared, initial)
	}
	current, err := database.GetLLMProviderWithContext(ctx, "new-default")
	if err != nil || !current.IsDefault {
		t.Fatalf("new default after import: provider=%+v err=%v", current, err)
	}
}
