package providers

import (
	"context"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
)

func TestTemplateFromCatalogServerUsesAllowlistsAndPersistsProfile(t *testing.T) {
	server := llmcatalog.Server{
		ID: "openai", Name: "Nome vindo do catálogo", ProviderFamily: "openai",
		Status: "active", APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true}, DefaultModelID: "model-from-catalog",
	}
	provider, err := TemplateFromCatalogServer(server)
	if err != nil {
		t.Fatal(err)
	}
	if provider.ServerProfileID != server.ID || provider.Name != server.Name || provider.Model != server.DefaultModelID {
		t.Fatalf("campos do perfil não foram preservados: %+v", provider)
	}
	if provider.Type != llm.ProviderOpenAI || provider.GetAPIFormat() != llm.APIFormatOpenAIResponses || provider.EffectiveAuthMode() != llm.AuthModeRequired {
		t.Fatalf("mapeamento de runtime inesperado: %+v", provider)
	}
	if provider.ErrorFamily != llm.ProviderErrorFamilyOpenAI {
		t.Fatalf("ErrorFamily = %q, esperado openai", provider.ErrorFamily)
	}

	deepseek := llmcatalog.Server{
		ID: "deepseek", Name: "DeepSeek", ProviderFamily: "deepseek", Status: "active",
		APIFormat: "openai", BaseURL: "https://api.deepseek.com/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
	}
	provider, err = TemplateFromCatalogServer(deepseek)
	if err != nil {
		t.Fatal(err)
	}
	if provider.ReasoningContentMode != llm.ReasoningContentReplayWithTools {
		t.Fatalf("ReasoningContentMode = %q, esperado replay_with_tools", provider.ReasoningContentMode)
	}
}

func TestTemplateFromCatalogServerRejectsUnknownRemoteValues(t *testing.T) {
	base := llmcatalog.Server{
		ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
		APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
	}
	tests := []struct {
		name   string
		change func(*llmcatalog.Server)
	}{
		{name: "família desconhecida", change: func(s *llmcatalog.Server) { s.ProviderFamily = "run-this-adapter" }},
		{name: "formato desconhecido", change: func(s *llmcatalog.Server) { s.APIFormat = "custom_protocol" }},
		{name: "combinação de formato e família não allowlisted", change: func(s *llmcatalog.Server) { s.APIFormat = "anthropic" }},
		{name: "identidade de outro perfil com família/formato incompatíveis", change: func(s *llmcatalog.Server) { s.ID = "xai" }},
		{name: "autenticação desconhecida", change: func(s *llmcatalog.Server) { s.Auth.Mode = "execute-command" }},
		{name: "ollama sem formato runtime", change: func(s *llmcatalog.Server) { s.ID, s.ProviderFamily, s.APIFormat = "ollama", "ollama", "ollama" }},
		{name: "endpoint Cohere sem suporte", change: func(s *llmcatalog.Server) { s.ID, s.ProviderFamily, s.APIFormat = "cohere", "cohere", "openai" }},
		{name: "perfil desativado", change: func(s *llmcatalog.Server) { s.Status = "deprecated" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := base
			tt.change(&server)
			if _, err := TemplateFromCatalogServer(server); err == nil {
				t.Fatal("esperava rejeição do valor fora da allowlist")
			}
		})
	}
}

func TestCreateFromTemplateUsesCatalogAndFallsBackWhenSnapshotIsEmpty(t *testing.T) {
	ctx := context.Background()
	registry := llm.NewProviderRegistry()
	store := NewMemoryStore()
	server := llmcatalog.Server{
		ID: "openrouter", Name: "OpenRouter atualizado", ProviderFamily: "openrouter",
		Status: "active", APIFormat: "openai", BaseURL: "https://openrouter.ai/api/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true}, DefaultModelID: "model-catalogado",
	}
	snapshot := llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{server}}}
	svc := NewService(ServiceConfig{Registry: registry, Store: store, CatalogSnapshot: func() llmcatalog.Snapshot { return snapshot }})
	if err := svc.CreateFromTemplate(ctx, "openrouter", ""); err != nil {
		t.Fatal(err)
	}
	created := registry.Get("openrouter-default")
	if created == nil {
		t.Fatal("provedor do catálogo não foi registrado")
	}
	if created.ServerProfileID != "openrouter" || created.Name != server.Name || created.DefaultModel != server.DefaultModelID || created.ErrorFamily != llm.ProviderErrorFamilyOpenRouter {
		t.Fatalf("configuração do catálogo não foi aplicada: %+v", created)
	}

	registry = llm.NewProviderRegistry()
	store = NewMemoryStore()
	svc = NewService(ServiceConfig{Registry: registry, Store: store, CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{} }})
	if err := svc.CreateFromTemplate(ctx, "openai", ""); err != nil {
		t.Fatalf("bootstrap sem catálogo deve manter fallback: %v", err)
	}
	created = registry.Get("openai-default")
	if created == nil || created.ServerProfileID != "" {
		t.Fatalf("fallback local inesperado: %+v", created)
	}

	registry = llm.NewProviderRegistry()
	store = NewMemoryStore()
	ollama := llmcatalog.Server{
		ID: "ollama", Name: "Ollama — servidor local", ProviderFamily: "ollama",
		Status: "active", APIFormat: "ollama", BaseURL: "http://localhost:11434",
		Auth: llmcatalog.Auth{Mode: "none", Required: false},
	}
	snapshot = llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{ollama}}}
	svc = NewService(ServiceConfig{Registry: registry, Store: store, CatalogSnapshot: func() llmcatalog.Snapshot { return snapshot }})
	if err := svc.CreateFromTemplate(ctx, "ollama", ""); err != nil {
		t.Fatalf("perfil Ollama incompatível deve preservar fallback de setup: %v", err)
	}
	created = registry.Get("ollama-local")
	if created == nil || created.ServerProfileID != "" || created.BaseURL != "http://localhost:11434/api" {
		t.Fatalf("fallback Ollama inesperado: %+v", created)
	}
}
