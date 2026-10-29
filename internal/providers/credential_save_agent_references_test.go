package providers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"errors"
	"testing"
)

// Apenas metadados persistidos: não cria sessão ACP nem inicia processos.
func TestNewProviderCredentialPreservesAgentReference(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	if err := mgr.RegisterPatternWithContext(ctx, "provider:api", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "original"}); err != nil {
		t.Fatal(err)
	}
	consumer := &database.LLMProvider{ID: "agent", Name: "Agent", Type: "custom", APIFormat: "acp", ACPCredentialEnv: `{"API_TOKEN":"provider:api"}`}
	if err := database.NewProviderRepository(database.DB()).SaveLLMProvider(ctx, consumer); err != nil {
		t.Fatal(err)
	}
	_, err := service.Create(ctx, vaultProviderRequest(&credentials.AuthConfig{Source: "static", Type: "bearer", Token: "replacement"}))
	if !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("expected credential collision: %v", err)
	}
	auth, err := mgr.GetConfigByPatternWithContext(ctx, "provider:api")
	if err != nil || auth == nil || auth.Token != "original" {
		t.Fatalf("agent credential changed: %v", err)
	}
	if service.registry.Get("api") != nil {
		t.Fatal("rejected connection published")
	}
	if _, err := service.store.Get(ctx, "api"); err == nil {
		t.Fatal("rejected connection persisted")
	}
}
