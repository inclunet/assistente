package providers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
)

type failingProviderTemplateStore struct {
	ProviderStore
	err error
}

func (s failingProviderTemplateStore) Save(context.Context, []*llm.ProviderConfig) error {
	return s.err
}

type failingCredentialListStore struct {
	credentials.Store
	stored   []credentials.StoredCredential
	failList bool
}

func (s *failingCredentialListStore) SaveCredential(_ context.Context, cred credentials.StoredCredential) error {
	for i := range s.stored {
		if cred.ID != "" && s.stored[i].ID == cred.ID {
			s.stored[i] = cred
			return nil
		}
	}
	if cred.ID == "" {
		cred.ID = fmt.Sprintf("credential-%d", len(s.stored)+1)
	}
	s.stored = append(s.stored, cred)
	return nil
}

func (s *failingCredentialListStore) ListCredentials(context.Context) ([]credentials.StoredCredential, error) {
	if s.failList {
		return nil, errors.New("leitura de credenciais indisponível")
	}
	return append([]credentials.StoredCredential(nil), s.stored...), nil
}

func (s *failingCredentialListStore) DeleteCredential(_ context.Context, pattern string) error {
	filtered := s.stored[:0]
	for _, cred := range s.stored {
		if cred.Pattern != pattern {
			filtered = append(filtered, cred)
		}
	}
	s.stored = filtered
	return nil
}

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

func TestTemplateFromCatalogServerUsesDeclaredFieldPolicyDefaults(t *testing.T) {
	server := llmcatalog.Server{
		ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
		APIFormat: "openai", BaseURL: "https://stale.example/v1",
		Auth: llmcatalog.Auth{Mode: "none", Required: false}, DefaultModelID: "stale-model",
		Fields: map[string]llmcatalog.FieldPolicy{
			"api_format":       {Policy: "fixed", Default: "openai_responses"},
			"base_url":         {Policy: "fixed", Default: "https://api.openai.com/v1"},
			"auth.mode":        {Policy: "fixed", Default: "optional_api_key"},
			"default_model_id": {Policy: "editable", Default: "model-from-field-policy"},
		},
	}
	provider, err := TemplateFromCatalogServer(server)
	if err != nil {
		t.Fatal(err)
	}
	if provider.BaseURL != "https://api.openai.com/v1" || provider.CredentialPattern != "api.openai.com" {
		t.Fatalf("BaseURL/CredentialPattern não vieram do default da política: %+v", provider)
	}
	if provider.DefaultModel != "stale-model" || provider.Model != "stale-model" {
		t.Fatalf("modelo editable não preservou o valor top-level: %+v", provider)
	}
	if provider.EffectiveAuthMode() != llm.AuthModeOptional {
		t.Fatalf("AuthMode = %q, esperado o default optional_api_key da política", provider.EffectiveAuthMode())
	}

	server.DefaultModelID = ""
	provider, err = TemplateFromCatalogServer(server)
	if err != nil {
		t.Fatal(err)
	}
	if provider.DefaultModel != "model-from-field-policy" {
		t.Fatalf("modelo editable ausente não usou o default da política: %+v", provider)
	}
}

func TestTemplateFromCatalogServerDerivesAuthModeFromEffectivePolicy(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		required bool
		want     llm.AuthMode
	}{
		{name: "bearer sem required top-level", mode: "bearer_api_key", required: false, want: llm.AuthModeRequired},
		{name: "header api key sem required top-level", mode: "api_key_header", required: false, want: llm.AuthModeRequired},
		{name: "optional com required top-level obsoleto", mode: "optional_api_key", required: true, want: llm.AuthModeOptional},
		{name: "none com required top-level obsoleto", mode: "none", required: true, want: llm.AuthModeNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := llmcatalog.Server{
				ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
				APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1",
				Auth: llmcatalog.Auth{Mode: "none", Required: tt.required},
				Fields: map[string]llmcatalog.FieldPolicy{
					"auth.mode": {Policy: "fixed", Default: tt.mode},
				},
			}
			provider, err := TemplateFromCatalogServer(server)
			if err != nil {
				t.Fatal(err)
			}
			if got := provider.EffectiveAuthMode(); got != tt.want {
				t.Fatalf("AuthMode = %q, esperado %q", got, tt.want)
			}
		})
	}
}

func TestTemplateFromCatalogServerRejectsUnknownRemoteValues(t *testing.T) {
	base := llmcatalog.Server{
		ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
		APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
		Fields: map[string]llmcatalog.FieldPolicy{
			"api_format": {Policy: "fixed", Default: "openai_responses"},
		},
	}
	tests := []struct {
		name   string
		change func(*llmcatalog.Server)
	}{
		{name: "família desconhecida", change: func(s *llmcatalog.Server) { s.ProviderFamily = "run-this-adapter" }},
		{name: "formato desconhecido", change: func(s *llmcatalog.Server) {
			s.Fields["api_format"] = llmcatalog.FieldPolicy{Policy: "fixed", Default: "custom_protocol"}
		}},
		{name: "combinação de formato e família não allowlisted", change: func(s *llmcatalog.Server) {
			s.Fields["api_format"] = llmcatalog.FieldPolicy{Policy: "fixed", Default: "anthropic"}
		}},
		{name: "identidade de outro perfil com família/formato incompatíveis", change: func(s *llmcatalog.Server) { s.ID = "xai" }},
		{name: "default api_format da política fora do perfil", change: func(s *llmcatalog.Server) {
			s.Fields["api_format"] = llmcatalog.FieldPolicy{Policy: "fixed", Default: "openai"}
		}},
		{name: "autenticação desconhecida", change: func(s *llmcatalog.Server) { s.Auth.Mode = "execute-command" }},
		{name: "ollama sem formato runtime", change: func(s *llmcatalog.Server) { s.ID, s.ProviderFamily, s.APIFormat = "ollama", "ollama", "ollama" }},
		{name: "endpoint Cohere sem suporte", change: func(s *llmcatalog.Server) { s.ID, s.ProviderFamily, s.APIFormat = "cohere", "cohere", "openai" }},
		{name: "perfil desativado", change: func(s *llmcatalog.Server) { s.Status = "deprecated" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := base
			server.Fields = make(map[string]llmcatalog.FieldPolicy, len(base.Fields))
			for field, policy := range base.Fields {
				server.Fields[field] = policy
			}
			tt.change(&server)
			if _, err := TemplateFromCatalogServer(server); err == nil {
				t.Fatal("esperava rejeição do valor fora da allowlist")
			}
		})
	}
}

func TestTemplateFromCatalogServerRejectsCatalogControlledCredentialDestination(t *testing.T) {
	tests := []struct {
		name   string
		server llmcatalog.Server
	}{
		{
			name: "provedor hospedado",
			server: llmcatalog.Server{
				ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
				APIFormat: "openai_responses", BaseURL: "https://collector.example/v1",
				Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
			},
		},
		{
			name: "proxy LiteLLM",
			server: llmcatalog.Server{
				ID: "litellm-openai-proxy", Name: "LiteLLM", ProviderFamily: "litellm", Status: "active",
				APIFormat: "openai", BaseURL: "https://collector.example/v1",
				Auth: llmcatalog.Auth{Mode: "optional_api_key"},
			},
		},
		{
			name: "servidor local",
			server: llmcatalog.Server{
				ID: "localai-openai-compatible", Name: "LocalAI", ProviderFamily: "localai", Status: "active",
				APIFormat: "openai", BaseURL: "https://collector.example/v1",
				Auth: llmcatalog.Auth{Mode: "optional_api_key"},
			},
		},
		{
			name: "proxy local com porta arbitrária",
			server: llmcatalog.Server{
				ID: "litellm-openai-proxy", Name: "LiteLLM", ProviderFamily: "litellm", Status: "active",
				APIFormat: "openai", BaseURL: "http://localhost:49152/v1",
				Auth: llmcatalog.Auth{Mode: "optional_api_key"},
			},
		},
		{
			name: "provedor hospedado com porta não padronizada",
			server: llmcatalog.Server{
				ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
				APIFormat: "openai_responses", BaseURL: "https://api.openai.com:444/v1",
				Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := TemplateFromCatalogServer(tt.server); err == nil {
				t.Fatal("esperava rejeição de host de credencial escolhido pelo catálogo")
			}
		})
	}
}

func TestCreateFromTemplateRequiresCatalogProfile(t *testing.T) {
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
	if err := svc.CreateFromTemplate(ctx, "openai", ""); err == nil {
		t.Fatal("bootstrap sem catálogo deve exigir configuração manual")
	}
	created = registry.Get("openai-default")
	if created != nil {
		t.Fatalf("não deve registrar preset sem catálogo: %+v", created)
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
	if err := svc.CreateFromTemplate(ctx, "ollama", ""); err == nil {
		t.Fatal("perfil Ollama sem suporte de runtime deve exigir configuração manual")
	}
	created = registry.Get("ollama-local")
	if created != nil {
		t.Fatalf("não deve registrar fallback Ollama incompatível: %+v", created)
	}
}

func TestCreateFromTemplateRollsBackRegistryWhenPersistenceFails(t *testing.T) {
	registry := llm.NewProviderRegistry()
	server := llmcatalog.Server{
		ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
		APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
	}
	snapshot := llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{server}}}
	svc := NewService(ServiceConfig{
		Registry:        registry,
		Store:           failingProviderTemplateStore{ProviderStore: NewMemoryStore(), err: errors.New("persistência indisponível")},
		CatalogSnapshot: func() llmcatalog.Snapshot { return snapshot },
	})

	if err := svc.CreateFromTemplate(context.Background(), "openai", ""); err == nil {
		t.Fatal("esperava falha de persistência")
	}
	if created := registry.Get("openai-default"); created != nil {
		t.Fatalf("provedor permaneceu no registry após falha de persistência: %+v", created)
	}
}

func TestCreateFromTemplateDoesNotPersistCredentialsForAuthNoneProfile(t *testing.T) {
	ctx := context.Background()
	registry := llm.NewProviderRegistry()
	credMgr := credentials.NewManager([]byte("01234567890123456789012345678901"))
	server := llmcatalog.Server{
		ID: "llamacpp", Name: "llama.cpp", ProviderFamily: "llamacpp", Status: "active",
		APIFormat: "openai", BaseURL: "http://localhost:8080/v1",
		Auth: llmcatalog.Auth{Mode: "none", Required: false},
	}
	snapshot := llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{server}}}
	svc := NewService(ServiceConfig{
		Registry: registry, Store: NewMemoryStore(), CredMgr: credMgr,
		CatalogSnapshot: func() llmcatalog.Snapshot { return snapshot },
	})

	if err := svc.CreateFromTemplate(ctx, "llamacpp", "accidental-secret"); err != nil {
		t.Fatal(err)
	}
	created := registry.Get("llamacpp-local")
	if created == nil || created.CredentialPattern != "" || created.EffectiveAuthMode() != llm.AuthModeNone {
		t.Fatalf("perfil sem autenticação recebeu padrão de credencial: %+v", created)
	}
	auth, err := credMgr.GetByPatternWithContext(ctx, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if auth != nil {
		t.Fatalf("chave acidental foi salva para localhost: %+v", auth)
	}
}

func TestCreateFromTemplateRollsBackCredentialWhenCredentialStoreFailsAfterSave(t *testing.T) {
	ctx := database.WithUserID(context.Background(), "test-user")
	registry := llm.NewProviderRegistry()
	credentialStore := &failingCredentialListStore{failList: true}
	credMgr := credentials.NewManagerWithStoreAndPersistence(
		[]byte("01234567890123456789012345678901"), credentialStore, true,
	)
	server := llmcatalog.Server{
		ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
		APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
	}
	snapshot := llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{server}}}
	svc := NewService(ServiceConfig{
		Registry: registry, Store: NewMemoryStore(), CredMgr: credMgr,
		CatalogSnapshot: func() llmcatalog.Snapshot { return snapshot },
	})

	if err := svc.CreateFromTemplate(ctx, "openai", "secret"); err == nil {
		t.Fatal("esperava falha ao consultar credenciais persistidas")
	}
	if len(credentialStore.stored) != 0 {
		t.Fatalf("credencial ficou persistida após erro: %+v", credentialStore.stored)
	}
	if created := registry.Get("openai-default"); created != nil {
		t.Fatalf("provedor ficou ativo após erro de credencial: %+v", created)
	}
}

func TestCreateFromTemplateRestoresPreviousCredentialWhenCredentialStoreFailsAfterSave(t *testing.T) {
	ctx := database.WithUserID(context.Background(), "test-user")
	registry := llm.NewProviderRegistry()
	credentialStore := &failingCredentialListStore{}
	credMgr := credentials.NewManagerWithStoreAndPersistence(
		[]byte("01234567890123456789012345678901"), credentialStore, true,
	)
	if err := credMgr.RegisterPatternWithContext(ctx, "api.openai.com", &credentials.AuthConfig{
		Source: "static", Type: "bearer", Token: "previous-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if len(credentialStore.stored) != 1 {
		t.Fatalf("credencial inicial não foi persistida: %+v", credentialStore.stored)
	}
	previousID := credentialStore.stored[0].ID
	credentialStore.failList = true

	server := llmcatalog.Server{
		ID: "openai", Name: "OpenAI", ProviderFamily: "openai", Status: "active",
		APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true},
	}
	snapshot := llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{server}}}
	svc := NewService(ServiceConfig{
		Registry: registry, Store: NewMemoryStore(), CredMgr: credMgr,
		CatalogSnapshot: func() llmcatalog.Snapshot { return snapshot },
	})
	if err := svc.CreateFromTemplate(ctx, "openai", "replacement-secret"); err == nil {
		t.Fatal("esperava falha ao consultar credenciais persistidas")
	}
	auth, err := credMgr.GetByPatternWithContext(ctx, "api.openai.com")
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil || auth.Token != "previous-secret" {
		t.Fatalf("credencial anterior não foi restaurada: %+v", auth)
	}
	if len(credentialStore.stored) != 1 || credentialStore.stored[0].ID != previousID {
		t.Fatalf("registro/ID de credencial anterior não foi restaurado: %+v", credentialStore.stored)
	}
	if created := registry.Get("openai-default"); created != nil {
		t.Fatalf("provedor ficou ativo após erro de credencial: %+v", created)
	}
}

func TestCreateFromTemplateStoresAnthropicKeyInExpectedHeader(t *testing.T) {
	ctx := context.Background()
	registry := llm.NewProviderRegistry()
	credMgr := credentials.NewManager([]byte("01234567890123456789012345678901"))
	server := llmcatalog.Server{
		ID: "anthropic", Name: "Anthropic", ProviderFamily: "anthropic",
		Status: "active", APIFormat: "anthropic", BaseURL: "https://api.anthropic.com",
		Auth: llmcatalog.Auth{Mode: "api_key_header", Required: true},
	}
	snapshot := llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{server}}}
	svc := NewService(ServiceConfig{
		Registry: registry, Store: NewMemoryStore(), CredMgr: credMgr,
		CatalogSnapshot: func() llmcatalog.Snapshot { return snapshot },
	})
	if err := svc.CreateFromTemplate(ctx, "claude", "anthropic-secret"); err != nil {
		t.Fatal(err)
	}

	auth, err := credMgr.GetByPatternWithContext(ctx, "api.anthropic.com")
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil || auth.Type != "custom" || auth.Headers["x-api-key"] != "anthropic-secret" {
		t.Fatalf("credencial Anthropic não foi salva no header exigido pela API: %+v", auth)
	}
}
