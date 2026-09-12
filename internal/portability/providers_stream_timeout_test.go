package portability

import (
	"testing"

	"assistente/internal/database"
)

func TestProviderExportImportPreservaStreamIdleTimeout(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	input := ProviderExport{
		ID:                       "provider-timeout",
		Name:                     "Provider Timeout",
		Type:                     "openai",
		APIFormat:                "openai",
		BaseURL:                  "https://api.example/v1",
		StreamIdleTimeoutSeconds: 75,
	}

	created, err := importProvider(ctx, input)
	if err != nil || !created {
		t.Fatalf("importProvider: created=%v err=%v", created, err)
	}
	persisted, err := database.GetLLMProviderWithContext(ctx, input.ID)
	if err != nil {
		t.Fatalf("GetLLMProviderWithContext: %v", err)
	}
	if persisted.StreamIdleTimeoutSeconds != 75 {
		t.Fatalf("timeout persistido=%d, esperado 75", persisted.StreamIdleTimeoutSeconds)
	}
	exported, err := exportProvider(persisted)
	if err != nil {
		t.Fatalf("exportProvider: %v", err)
	}
	if exported.StreamIdleTimeoutSeconds != 75 {
		t.Fatalf("timeout exportado=%d, esperado 75", exported.StreamIdleTimeoutSeconds)
	}
}
