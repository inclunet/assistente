package portability

import (
	"assistente/internal/database"
	"testing"
)

func TestPrivateAliasesImportExportAndClear(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	incoming := ProviderExport{ID: "proxy-alias", Name: "Proxy", Type: "custom", APIFormat: "openai", BaseURL: "https://example.com/v1", ModelAliases: map[string]string{"private": "vendor/model"}}
	if _, err := importProvider(ctx, incoming); err != nil {
		t.Fatal(err)
	}
	row, err := database.GetLLMProviderWithContext(ctx, incoming.ID)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := exportProvider(row)
	if err != nil || exported.ModelAliases["private"] != "vendor/model" {
		t.Fatalf("binding lost: %+v %v", exported, err)
	}
	incoming.ModelAliases = nil
	if _, err := overwriteProvider(ctx, incoming); err != nil {
		t.Fatal(err)
	}
	row, err = database.GetLLMProviderWithContext(ctx, incoming.ID)
	if err != nil || len(row.ModelAliases) != 0 {
		t.Fatalf("binding not cleared: %+v %v", row, err)
	}
}
