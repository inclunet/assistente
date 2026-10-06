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
	if modelChanged.CompatibilityRevision != connectionChanged.CompatibilityRevision {
		t.Fatalf("model-only import changed compatibility revision from %d to %d", connectionChanged.CompatibilityRevision, modelChanged.CompatibilityRevision)
	}
}
