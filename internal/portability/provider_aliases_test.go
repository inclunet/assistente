package portability

import (
	"assistente/internal/database"
	"testing"
)

func TestPrivateAliasesImportExportAndClear(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	incoming := ProviderExport{ID: "proxy-alias", Name: "Proxy", Type: "custom", APIFormat: "openai", BaseURL: "https://example.com/v1", Model: " private ", DefaultModel: " private ", ModelAliases: map[string]string{" private ": "vendor/model"}}
	if _, err := importProvider(ctx, incoming); err != nil {
		t.Fatal(err)
	}
	row, err := database.GetLLMProviderWithContext(ctx, incoming.ID)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := exportProvider(row)
	if err != nil || exported.ModelAliases[" private "] != "vendor/model" || exported.Model != " private " || exported.DefaultModel != " private " {
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

func TestPrivateAliasesImportErrorIsLocalized(t *testing.T) {
	for _, aliases := range []map[string]string{{" ": "model"}, {"remote": ""}} {
		_, err := validateProviderExport(ProviderExport{ID: "proxy", ModelAliases: aliases})
		if err == nil {
			t.Fatal("empty identifier accepted")
		}
		message := messageFromError(err)
		if message.Code != CodeProviderInvalidModelAliases || message.Params["providerId"] != "proxy" {
			t.Fatalf("untranslated error: %+v", message)
		}
	}
}
