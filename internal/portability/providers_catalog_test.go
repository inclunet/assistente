package portability

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
)

func TestExportProviderPreservaPerfilDeServidorExplicito(t *testing.T) {
	exported, err := exportProvider(&database.LLMProvider{
		ID: "openrouter-connection", Name: "OpenRouter", Type: "openrouter",
		APIFormat: "openai", ServerProfileID: "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
	})
	if err != nil {
		t.Fatalf("exportProvider() error = %v", err)
	}
	if exported.ServerProfileID != "openrouter" {
		t.Fatalf("ServerProfileID = %q, esperado openrouter", exported.ServerProfileID)
	}

	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded ProviderExport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.ServerProfileID != "openrouter" {
		t.Fatalf("perfil após round-trip JSON = %q, esperado openrouter", decoded.ServerProfileID)
	}
}

func TestImportProviderPersistePerfilNovoESobrescrito(t *testing.T) {
	setupPortabilityTestDB(t)
	ctx := portabilityTestCtx()
	incoming := ProviderExport{
		ID: "catalog-connection", Name: "Gateway", Type: "custom",
		APIFormat: "openai", BaseURL: "https://gateway.example/v1",
		ServerProfileID: " openrouter ",
	}
	if _, err := importProvider(ctx, incoming); err != nil {
		t.Fatalf("importProvider() novo error = %v", err)
	}
	assertImportedProfileID(t, ctx, incoming.ID, "openrouter")

	incoming.Name = "Gateway atualizado"
	incoming.ServerProfileID = " litellm-openai-proxy "
	if _, err := overwriteProvider(ctx, incoming); err != nil {
		t.Fatalf("overwriteProvider() error = %v", err)
	}
	assertImportedProfileID(t, ctx, incoming.ID, "litellm-openai-proxy")
}

func TestValidateProviderExportOmitsServerProfileForChatGPT(t *testing.T) {
	validated, err := validateProviderExport(ProviderExport{
		ID: "chatgpt-oauth", Name: "ChatGPT", Type: "chatgpt",
		ServerProfileID: "openai", BaseURL: "https://invalid.example",
	})
	if err != nil {
		t.Fatalf("validateProviderExport() error = %v", err)
	}
	if validated.ServerProfileID != "" {
		t.Fatalf("perfil de servidor permaneceu na conexão OAuth: %q", validated.ServerProfileID)
	}
}

func assertImportedProfileID(t *testing.T, ctx context.Context, id, want string) {
	t.Helper()
	provider, err := database.GetLLMProviderWithContext(ctx, id)
	if err != nil {
		t.Fatalf("buscar provider importado: %v", err)
	}
	if provider.ServerProfileID != want {
		t.Fatalf("server profile id = %q, esperado %q", provider.ServerProfileID, want)
	}
}
