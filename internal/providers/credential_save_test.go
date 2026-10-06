package providers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"context"
	"errors"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func vaultProviderRequest(auth *credentials.AuthConfig) CreateRequest {
	return CreateRequest{ID: "api", Name: "API", Type: "custom", BaseURL: "https://example.com/v1", APIFormat: "openai", AuthMode: "required", Credential: &CredentialSpec{Pattern: "example.com", Auth: auth}}
}
func TestProviderCredentialSourcesSaveWithoutResolving(t *testing.T) {
	for _, auth := range []*credentials.AuthConfig{
		{Source: "static", Type: "bearer", Token: "private-token"},
		{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "UNSET_PROVIDER_TOKEN"}},
		{Source: "keyring", Type: "bearer", SourceConfig: &credentials.SourceConfig{KeyringService: "unavailable", KeyringUser: "user"}},
		{Source: "command", Type: "bearer", SourceConfig: &credentials.SourceConfig{Command: "unavailable-program", Args: []string{"literal"}, TimeoutSeconds: 30}},
	} {
		t.Run(auth.Source, func(t *testing.T) {
			service, mgr, ctx := chatGPTTestService(t)
			result, err := service.Create(ctx, vaultProviderRequest(auth))
			if err != nil {
				t.Fatal(err)
			}
			stored, err := service.store.Get(ctx, "api")
			if err != nil || stored.CredentialPattern != "example.com" || !stored.IsDefault || !result.CredentialConfigured {
				t.Fatalf("provider: %+v %v", stored, err)
			}
			config, err := mgr.GetConfigByPatternWithContext(ctx, "example.com")
			if err != nil || config.Source != auth.Source {
				t.Fatalf("credential: %v", err)
			}
			if strings.Contains(toDBModel(stored).BaseURL, "private-token") {
				t.Fatal("secret in provider")
			}
		})
	}
}
func TestProviderCredentialConsumerFailureRollsBackVaultAndRegistry(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	old := &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "old"}
	if err := mgr.RegisterPatternWithContext(ctx, "example.com", old); err != nil {
		t.Fatal(err)
	}
	db := database.DB()
	if err := db.Callback().Create().Before("gorm:create").Register("test:provider-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "llm_providers" {
			_ = tx.AddError(errors.New("provider write failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:provider-failure") })
	_, err := service.Create(ctx, vaultProviderRequest(&credentials.AuthConfig{Source: "static", Type: "bearer", Token: "new"}))
	if err == nil {
		t.Fatal("expected rollback")
	}
	got, err := mgr.GetByPatternWithContext(ctx, "example.com")
	if err != nil || got.Token != "old" {
		t.Fatalf("vault changed: %v", err)
	}
	if service.registry.Get("api") != nil {
		t.Fatal("published failed consumer")
	}
	if _, err := service.store.Get(ctx, "api"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
}
func TestProviderCredentialPreservesExplicitReferenceAndDefault(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	p := &llm.ProviderConfig{ID: "api", Name: "Saved", Type: llm.ProviderCustom, BaseURL: "https://example.com/v1", APIFormat: llm.APIFormatOpenAI, CredentialPattern: "shared-alias", AuthMode: llm.AuthModeOptional, IsDefault: true}
	if err := service.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
		t.Fatal(err)
	}
	if err := service.registry.Register(p); err != nil {
		t.Fatal(err)
	}
	result, err := service.Update(ctx, "api", UpdateRequest{Name: "Edited", Type: "custom", BaseURL: p.BaseURL, Credential: &CredentialSpec{Pattern: "shared-alias", Auth: &credentials.AuthConfig{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "TOKEN"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider.CredentialPattern != "shared-alias" || result.Provider.AuthMode != llm.AuthModeOptional || !result.Provider.IsDefault {
		t.Fatal("lost consumer metadata")
	}
	if _, err := mgr.GetConfigByPatternWithContext(ctx, "shared-alias"); err != nil {
		t.Fatal(err)
	}
}
func TestProviderCredentialRejectsInvalidTargetsAndCrossUser(t *testing.T) {
	for _, scenario := range []string{"unrelated-pattern", "managed", "none", "acp", "google-basic", "duplicate-other-user"} {
		t.Run(scenario, func(t *testing.T) {
			service, mgr, ctx := chatGPTTestService(t)
			req := vaultProviderRequest(&credentials.AuthConfig{Source: "static", Type: "bearer", Token: "new"})
			switch scenario {
			case "unrelated-pattern":
				req.Credential.Pattern = "unrelated.example"
			case "managed":
				req.Credential.Pattern = "oauth:grant"
			case "none":
				req.AuthMode = "none"
			case "acp":
				req.APIFormat = "acp"
				req.ACPCommand = "agent"
			case "google-basic":
				req.APIFormat = "google"
				req.Credential.Auth.Type = "basic"
				req.Credential.Auth.Username = "user"
				req.Credential.Auth.Password = "secret"
			case "duplicate-other-user":
				other := database.WithUserID(context.Background(), "other")
				if err := service.store.Save(other, []*llm.ProviderConfig{{ID: "api", Name: "Other", Type: llm.ProviderCustom, BaseURL: req.BaseURL}}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.Create(ctx, req); err == nil {
				t.Fatal("accepted invalid draft")
			}
			if auth, err := mgr.GetConfigByPatternWithContext(ctx, "example.com"); err == nil && auth != nil {
				t.Fatal("left a credential behind")
			}
		})
	}
}
func TestProviderCredentialPreviewUsesEphemeralManager(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	t.Setenv("PROVIDER_PREVIEW_TOKEN", "preview-only")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer preview-only" {
			t.Errorf("wrong auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	}))
	defer server.Close()
	pattern, _ := ExtractHostname(server.URL)
	if err := mgr.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "persisted"}); err != nil {
		t.Fatal(err)
	}
	models, err := service.ListModelsRaw(ctx, ListModelsRawRequest{Type: "custom", BaseURL: server.URL, APIFormat: "openai", AuthMode: "required", Credential: &CredentialSpec{Pattern: pattern, Auth: &credentials.AuthConfig{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "PROVIDER_PREVIEW_TOKEN"}}}})
	if err != nil || len(models) != 1 || models[0] != "test-model" {
		t.Fatalf("preview: %v %v", models, err)
	}
	got, err := mgr.GetByPatternWithContext(ctx, pattern)
	if err != nil || got.Token != "persisted" {
		t.Fatalf("preview changed vault: %v", err)
	}
}

func TestProviderCredentialRejectsStaleConsumerSnapshot(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	if _, err := service.Create(ctx, vaultProviderRequest(&credentials.AuthConfig{Source: "static", Type: "bearer", Token: "old"})); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Model(&database.LLMProvider{}).Where("id = ?", "api").Updates(map[string]any{"name": "Changed elsewhere", "default_model": "new-model"}).Error; err != nil {
		t.Fatal(err)
	}
	_, err := service.Update(ctx, "api", UpdateRequest{Credential: &CredentialSpec{Pattern: "example.com", Auth: &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "new"}}})
	if err == nil {
		t.Fatal("stale snapshot accepted")
	}
	saved, err := service.store.Get(ctx, "api")
	if err != nil || saved.Name != "Changed elsewhere" || saved.DefaultModel != "new-model" {
		t.Fatal("lost persisted preferences")
	}
	auth, err := mgr.GetByPatternWithContext(ctx, "example.com")
	if err != nil || auth.Token != "old" {
		t.Fatal("vault changed on conflict")
	}
}
func TestProviderCredentialSessionChangeDuringPreflightDoesNotPublish(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	db := database.DB()
	switched := false
	if err := db.Callback().Query().After("gorm:query").Register("test:session-change", func(tx *gorm.DB) {
		if !switched && tx.Statement.Table == "llm_providers" {
			switched = true
			service.registry.Clear()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:session-change") })
	_, err := service.Create(ctx, vaultProviderRequest(&credentials.AuthConfig{Source: "static", Type: "bearer", Token: "new"}))
	if !switched || err == nil {
		t.Fatal("session transition was not rejected")
	}
	if service.registry.Get("api") != nil {
		t.Fatal("published into new session")
	}
	if _, err := service.store.Get(ctx, "api"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("saved consumer after session transition")
	}
	if auth, err := mgr.GetConfigByPatternWithContext(ctx, "example.com"); err == nil && auth != nil {
		t.Fatal("saved credential after session transition")
	}
}
func TestProviderCredentialVaultFailureDoesNotSaveConsumer(t *testing.T) {
	service, _, ctx := chatGPTTestService(t)
	db := database.DB()
	if err := db.Callback().Create().Before("gorm:create").Register("test:vault-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "credential_entries" {
			_ = tx.AddError(errors.New("vault failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:vault-failure") })
	if _, err := service.Create(ctx, vaultProviderRequest(&credentials.AuthConfig{Source: "static", Type: "bearer", Token: "new"})); err == nil {
		t.Fatal("expected failure")
	}
	if service.registry.Get("api") != nil {
		t.Fatal("published before vault commit")
	}
	if _, err := service.store.Get(ctx, "api"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("saved without credential")
	}
}

func TestProviderCredentialPreviewChangedOriginWithoutAuth(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"public-model"}]}`))
	}))
	defer server.Close()
	p := &llm.ProviderConfig{ID: "api", Name: "Existing", Type: llm.ProviderCustom, APIFormat: llm.APIFormatOpenAI, BaseURL: "https://old.example/v1", CredentialPattern: "old.example", AuthMode: llm.AuthModeRequired}
	if err := service.registry.Register(p); err != nil {
		t.Fatal(err)
	}
	// A missing environment source proves preview never resolves the old credential.
	if err := mgr.RegisterPatternWithContext(ctx, "old.example", &credentials.AuthConfig{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "PROVIDER_PREVIEW_MUST_NOT_RESOLVE"}}); err != nil {
		t.Fatal(err)
	}
	models, err := service.ListModelsRaw(ctx, ListModelsRawRequest{ProviderID: "api", Type: "custom", BaseURL: server.URL, APIFormat: "openai", AuthMode: "none"})
	if err != nil || len(models) != 1 || models[0] != "public-model" {
		t.Fatalf("preview: %v %v", models, err)
	}
	if service.registry.Get("api").AuthMode != llm.AuthModeRequired {
		t.Fatal("preview mutated saved mode")
	}
}

func TestProviderCredentialLegacyAPIKeyPreviewWithAliasOrWildcard(t *testing.T) {
	for _, pattern := range []string{"shared-alias", "*"} {
		t.Run(pattern, func(t *testing.T) {
			service, mgr, ctx := chatGPTTestService(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer preview-key" {
					t.Error("preview did not use ad-hoc key")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
			}))
			defer server.Close()
			p := &llm.ProviderConfig{ID: "api", Name: "Existing", Type: llm.ProviderCustom, APIFormat: llm.APIFormatOpenAI, BaseURL: server.URL, CredentialPattern: pattern, AuthMode: llm.AuthModeRequired}
			if err := service.registry.Register(p); err != nil {
				t.Fatal(err)
			}
			if err := mgr.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "persisted"}); err != nil {
				t.Fatal(err)
			}
			models, err := service.ListModelsRaw(ctx, ListModelsRawRequest{ProviderID: "api", Type: "custom", BaseURL: server.URL, APIKey: "preview-key"})
			if err != nil || len(models) != 1 {
				t.Fatalf("preview: %v %v", models, err)
			}
			auth, err := mgr.GetByPatternWithContext(ctx, pattern)
			if err != nil || auth.Token != "persisted" {
				t.Fatal("preview changed vault")
			}
			if service.registry.Get("api").CredentialPattern != pattern {
				t.Fatal("preview changed reference")
			}
		})
	}
}
