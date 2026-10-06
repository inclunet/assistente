package providers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/oauthflow"
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
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

type pausedCredentialStore struct {
	inner     credentialConsumerStore
	committed chan struct{}
	release   chan struct{}
}

func (s *pausedCredentialStore) SaveWithConsumer(ctx context.Context, pattern string, auth *credentials.AuthConfig, retire *oauthflow.Record, update func(*gorm.DB) error, publish func()) error {
	return s.inner.SaveWithConsumer(ctx, pattern, auth, retire, update, func() {
		close(s.committed)
		<-s.release
		publish()
	})
}

func TestProviderCredentialCommitPublicationSerializesMutations(t *testing.T) {
	for _, mutation := range []string{"clear", "update", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			service, mgr, ctx := chatGPTTestService(t)
			req := vaultProviderRequest(&credentials.AuthConfig{Source: "static", Type: "bearer", Token: "old"})
			if _, err := service.Create(ctx, req); err != nil {
				t.Fatal(err)
			}
			expected := service.registry.Get("api")
			next := *expected
			next.Name = "Committed"
			draft := &CredentialSpec{Pattern: "example.com", Auth: &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "new"}}
			scope, err := service.captureCredentialSave(ctx, draft)
			if err != nil {
				t.Fatal(err)
			}
			paused := &pausedCredentialStore{inner: scope.store, committed: make(chan struct{}), release: make(chan struct{})}
			scope.store = paused
			var once sync.Once
			release := func() { once.Do(func() { close(paused.release) }) }
			defer release()
			saved := make(chan error, 1)
			go func() { saved <- service.saveWithCredential(ctx, &next, expected, draft, scope) }()
			select {
			case <-paused.committed:
			case <-time.After(10 * time.Second):
				t.Fatal("commit not reached")
			}
			changed := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				switch mutation {
				case "clear":
					service.registry.Clear()
					changed <- nil
				case "update":
					_, err := service.Update(ctx, "api", UpdateRequest{DefaultModel: "concurrent-model"})
					changed <- err
				case "delete":
					changed <- service.Delete(ctx, "api")
				}
			}()
			<-started
			select {
			case err := <-changed:
				t.Fatalf("mutation crossed commit/publication boundary: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			release()
			if err := <-saved; err != nil {
				t.Fatalf("save reported error after commit: %v", err)
			}
			if err := <-changed; err != nil {
				t.Fatal(err)
			}
			auth, err := mgr.GetByPatternWithContext(ctx, "example.com")
			if err != nil || auth.Token != "new" {
				t.Fatal("credential not committed")
			}
			stored, err := service.store.Get(ctx, "api")
			switch mutation {
			case "clear":
				if err != nil || stored.Name != "Committed" || service.registry.Get("api") != nil {
					t.Fatal("late publication into cleared registry")
				}
			case "update":
				published := service.registry.Get("api")
				if err != nil || published == nil || stored.Name != "Committed" || stored.DefaultModel != "concurrent-model" || published.DefaultModel != stored.DefaultModel {
					t.Fatal("lost newer state")
				}
			case "delete":
				if !errors.Is(err, gorm.ErrRecordNotFound) || service.registry.Get("api") != nil {
					t.Fatal("resurrected deleted consumer")
				}
			}
		})
	}
}

func TestProviderCredentialGoogleRejectsNonRequiredMode(t *testing.T) {
	for _, mode := range []string{"none", "optional"} {
		t.Run(mode, func(t *testing.T) {
			service, _, ctx := chatGPTTestService(t)
			req := vaultProviderRequest(&credentials.AuthConfig{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "MUST_NOT_RESOLVE_GOOGLE"}})
			req.APIFormat = "google"
			if _, err := service.Create(ctx, req); err != nil {
				t.Fatal(err)
			}
			req.ID = "invalid"
			req.Credential = nil
			req.AuthMode = mode
			if _, err := service.Create(ctx, req); !errors.Is(err, credentials.ErrCredentialResolution) {
				t.Fatalf("create: %v", err)
			}
			if _, err := service.Update(ctx, "api", UpdateRequest{AuthMode: mode}); !errors.Is(err, credentials.ErrCredentialResolution) {
				t.Fatalf("update: %v", err)
			}
			if _, err := service.ListModelsRaw(ctx, ListModelsRawRequest{ProviderID: "api", Type: "custom", BaseURL: req.BaseURL, APIFormat: "google", AuthMode: mode}); !errors.Is(err, credentials.ErrCredentialResolution) {
				t.Fatalf("preview resolved source: %v", err)
			}
			if service.registry.Get("api").AuthMode != llm.AuthModeRequired {
				t.Fatal("changed saved mode")
			}
		})
	}
}

func TestProviderCredentialCreateUsesEffectiveReference(t *testing.T) {
	for _, key := range []string{"", "explicit-key"} {
		t.Run(key, func(t *testing.T) {
			service, mgr, ctx := chatGPTTestService(t)
			if err := mgr.RegisterPatternWithContext(ctx, "*.example.com", &credentials.AuthConfig{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "MUST_NOT_RESOLVE_WILDCARD"}}); err != nil {
				t.Fatal(err)
			}
			got, err := service.Create(ctx, CreateRequest{ID: "api", Name: "API", Type: "custom", BaseURL: "https://api.example.com/v1", APIKey: key})
			if err != nil {
				t.Fatal(err)
			}
			want := "*.example.com"
			if key != "" {
				want = "api.example.com"
			}
			if got.CredentialPattern != want || !got.CredentialConfigured {
				t.Fatalf("reference/status: %+v", got)
			}
			if key != "" {
				auth, err := mgr.GetByPatternWithContext(ctx, want)
				if err != nil || auth.Token != key {
					t.Fatal("did not bind the explicit key")
				}
			}
		})
	}
}

func TestProviderCredentialDefaultPortsPreserveAlias(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer alias-token" {
			t.Error("lost alias credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
	}))
	defer server.Close()
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original; transport.CloseIdleConnections() })
	for _, scheme := range []string{"http", "https"} {
		id := scheme
		port := "80"
		if scheme == "https" {
			port = "443"
		}
		p := &llm.ProviderConfig{ID: id, Name: id, Type: llm.ProviderCustom, APIFormat: llm.APIFormatOpenAI, BaseURL: scheme + "://example.com/v1", CredentialPattern: "alias-" + id, AuthMode: llm.AuthModeRequired}
		if err := service.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
			t.Fatal(err)
		}
		if err := service.registry.Register(p); err != nil {
			t.Fatal(err)
		}
		if err := mgr.RegisterPatternWithContext(ctx, p.CredentialPattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "alias-token"}); err != nil {
			t.Fatal(err)
		}
		target := scheme + "://example.com:" + port + "/v1"
		if scheme == "http" {
			models, err := service.ListModelsRaw(ctx, ListModelsRawRequest{ProviderID: id, Type: "custom", BaseURL: target})
			if err != nil || len(models) != 1 {
				t.Fatalf("preview: %v %v", models, err)
			}
		}
		got, err := service.Update(ctx, id, UpdateRequest{BaseURL: target, Credential: &CredentialSpec{Pattern: p.CredentialPattern, Auth: &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "alias-token"}}})
		if err != nil || got.Provider.CredentialPattern != p.CredentialPattern {
			t.Fatalf("save: %v", err)
		}
	}
	for _, other := range []string{"http://example.com:443", "https://example.com:444", "http://example.com"} {
		if sameCredentialOrigin("https://example.com", other) {
			t.Fatalf("different origin accepted: %s", other)
		}
	}
}

func TestProviderPreviewCommandHelper(t *testing.T) {
	if os.Getenv("ASSISTENTE_PROVIDER_PREVIEW_HELPER") != "1" {
		return
	}
	time.Sleep(16 * time.Second)
	fmt.Print("preview-command-token")
	os.Exit(0)
}
func TestProviderCredentialPreviewAllowsSlowCommand(t *testing.T) {
	service, _, ctx := chatGPTTestService(t)
	t.Setenv("ASSISTENTE_PROVIDER_PREVIEW_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer preview-command-token" {
			t.Error("unexpected command token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
	}))
	defer server.Close()
	pattern, _ := ExtractHostname(server.URL)
	models, err := service.ListModelsRaw(ctx, ListModelsRawRequest{Type: "custom", BaseURL: server.URL, APIFormat: "openai", AuthMode: "required",
		Credential: &CredentialSpec{Pattern: pattern, Auth: &credentials.AuthConfig{Type: "bearer", Source: "command", SourceConfig: &credentials.SourceConfig{
			Command: exe, Args: []string{"-test.run=^TestProviderPreviewCommandHelper$"}, TimeoutSeconds: 30,
		}}}})
	if err != nil || len(models) != 1 {
		t.Fatalf("slow command preview: %v %v", models, err)
	}
}

func TestProviderUpdateExplicitKeyPreservesSharedWildcard(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	_, err := service.Create(ctx, CreateRequest{ID: "api", Name: "API", Type: "custom", BaseURL: "https://old.example.net/v1", APIKey: "old-key"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.RegisterPatternWithContext(ctx, "*.example.com", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "shared-key"}); err != nil {
		t.Fatal(err)
	}
	got, err := service.Update(ctx, "api", UpdateRequest{BaseURL: "https://new.example.com/v1", APIKey: "new-key"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider.CredentialPattern != "new.example.com" {
		t.Fatalf("wrong reference: %s", got.Provider.CredentialPattern)
	}
	for pattern, token := range map[string]string{"*.example.com": "shared-key", "new.example.com": "new-key", "old.example.net": "old-key"} {
		auth, err := mgr.GetByPatternWithContext(ctx, pattern)
		if err != nil || auth.Token != token {
			t.Fatalf("changed credential %s: %v", pattern, err)
		}
	}
}

func TestGoogleRejectsIncompatibleReferencedCredentialWithoutResolution(t *testing.T) {
	for _, scheme := range []string{"basic", "custom"} {
		t.Run(scheme, func(t *testing.T) {
			service, mgr, ctx := chatGPTTestService(t)
			auth := &credentials.AuthConfig{Source: "env", Type: scheme, Username: "user", Headers: map[string]string{"X-Key": "placeholder"}, SourceConfig: &credentials.SourceConfig{Env: "UNSET_GOOGLE_TEST_TOKEN"}}
			if err := mgr.RegisterPatternWithContext(ctx, "*.googleapis.com", auth); err != nil {
				t.Fatal(err)
			}
			req := CreateRequest{ID: "google", Name: "Google", Type: "google", BaseURL: "https://generativelanguage.googleapis.com", APIFormat: "google", AuthMode: "required"}
			if _, err := service.Create(ctx, req); !errors.Is(err, credentials.ErrCredentialResolution) {
				t.Fatalf("incompatible reference accepted: %v", err)
			}
			req.APIKey = "explicit-key"
			if _, err := service.Create(ctx, req); err != nil {
				t.Fatalf("explicit replacement rejected: %v", err)
			}
			req.ID = "google-draft"
			req.APIKey = ""
			req.Credential = &CredentialSpec{Pattern: "*.googleapis.com", Auth: &credentials.AuthConfig{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "UNSET_GOOGLE_TEST_TOKEN"}}}
			if _, err := service.Create(ctx, req); err != nil {
				t.Fatalf("compatible draft rejected: %v", err)
			}
		})
	}
}

func TestProviderDuplicatePreservesScopedAliasWithoutRewritingCredential(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	source := &llm.ProviderConfig{ID: "original", Name: "Original", Type: llm.ProviderCustom, APIFormat: llm.APIFormatOpenAI, BaseURL: "https://example.com/v1", AuthMode: llm.AuthModeRequired, CredentialPattern: "shared-alias"}
	if err := service.store.Save(ctx, []*llm.ProviderConfig{source}); err != nil {
		t.Fatal(err)
	}
	if err := service.registry.Register(source); err != nil {
		t.Fatal(err)
	}
	auth := &credentials.AuthConfig{Source: "command", Type: "bearer", SourceConfig: &credentials.SourceConfig{Command: "must-not-execute", TimeoutSeconds: 30}}
	if err := mgr.RegisterPatternWithContext(ctx, source.CredentialPattern, auth); err != nil {
		t.Fatal(err)
	}
	req := CreateRequest{ID: "copy", Name: "Copy", Type: "custom", APIFormat: "openai", BaseURL: source.BaseURL, AuthMode: "required", CredentialFromProviderID: source.ID}
	got, err := service.Create(ctx, req)
	if err != nil || got.Provider.CredentialPattern != "shared-alias" || !got.CredentialConfigured {
		t.Fatalf("duplicate: %v %v", got, err)
	}
	config, err := mgr.GetConfigByPatternWithContext(ctx, "shared-alias")
	if err != nil || config.Source != "command" || config.SourceConfig.Command != "must-not-execute" {
		t.Fatalf("credential changed: %v", err)
	}
	if _, err := service.Create(database.WithUserID(ctx, "another-user"), CreateRequest{ID: "foreign", Name: "Foreign", Type: "custom", APIFormat: "openai", BaseURL: source.BaseURL, CredentialFromProviderID: source.ID}); err == nil {
		t.Fatal("foreign source accepted")
	}
	for _, change := range []func(*CreateRequest){
		func(r *CreateRequest) { r.BaseURL = "https://other.example/v1" },
		func(r *CreateRequest) { r.Type = "google" },
		func(r *CreateRequest) { r.APIFormat = "anthropic" },
		func(r *CreateRequest) { r.APIKey = "must-not-write" },
		func(r *CreateRequest) {
			r.Credential = &CredentialSpec{Pattern: "shared-alias", Auth: &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "must-not-write"}}
		},
	} {
		attempt := req
		attempt.ID = "invalid"
		change(&attempt)
		if _, err := service.Create(ctx, attempt); err == nil {
			t.Fatal("invalid credential source accepted")
		}
	}
}

func TestProviderCredentialCanonicalHostnamePreviewAndSave(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing draft token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
	}))
	defer server.Close()
	base := strings.Replace(server.URL, "127.0.0.1", "LOCALHOST", 1)
	draft := &CredentialSpec{Pattern: "localhost", Auth: &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "token"}}
	models, err := service.ListModelsRaw(ctx, ListModelsRawRequest{Type: "custom", APIFormat: "openai", BaseURL: base, Credential: draft})
	if err != nil || len(models) != 1 {
		t.Fatalf("preview: %v %v", models, err)
	}
	got, err := service.Create(ctx, CreateRequest{ID: "mixed-case", Name: "Mixed", Type: "custom", APIFormat: "openai", BaseURL: base, Credential: draft})
	if err != nil || got.CredentialPattern != "localhost" {
		t.Fatalf("save: %v %v", got, err)
	}
	stored, err := mgr.GetConfigByPatternWithContext(ctx, "localhost")
	if err != nil || stored == nil {
		t.Fatalf("missing canonical credential: %v", err)
	}
}

type pausedProviderStore struct {
	ProviderStore
	committed chan struct{}
	release   chan struct{}
}

func (s *pausedProviderStore) Save(ctx context.Context, p []*llm.ProviderConfig) error {
	if err := s.ProviderStore.Save(ctx, p); err != nil {
		return err
	}
	close(s.committed)
	<-s.release
	return nil
}
func TestProviderExistingCredentialPublicationReservesSession(t *testing.T) {
	for _, action := range []string{"create", "update"} {
		t.Run(action, func(t *testing.T) {
			service, _, ctx := chatGPTTestService(t)
			req := CreateRequest{ID: "api", Name: "API", Type: "custom", APIFormat: "openai", BaseURL: "https://example.com"}
			if action == "update" {
				if _, err := service.Create(ctx, req); err != nil {
					t.Fatal(err)
				}
			}
			paused := &pausedProviderStore{ProviderStore: service.store, committed: make(chan struct{}), release: make(chan struct{})}
			service.store = paused
			var once sync.Once
			release := func() { once.Do(func() { close(paused.release) }) }
			defer release()
			saved := make(chan error, 1)
			go func() {
				var err error
				if action == "create" {
					_, err = service.Create(ctx, req)
				} else {
					_, err = service.Update(ctx, "api", UpdateRequest{Name: "Updated"})
				}
				saved <- err
			}()
			select {
			case <-paused.committed:
			case <-time.After(10 * time.Second):
				t.Fatal("save not reached")
			}
			cleared := make(chan struct{})
			started := make(chan struct{})
			go func() { close(started); service.registry.Clear(); close(cleared) }()
			<-started
			select {
			case <-cleared:
				t.Fatal("clear crossed save/publication")
			case <-time.After(30 * time.Millisecond):
			}
			release()
			if err := <-saved; err != nil {
				t.Fatalf("error after save: %v", err)
			}
			<-cleared
			if service.registry.Get("api") != nil {
				t.Fatal("old session published into cleared registry")
			}
			if _, err := paused.Get(ctx, "api"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type sessionChangingCredentialManager struct {
	CredentialManager
	clear func()
}

func (m *sessionChangingCredentialManager) GetConfigByPatternWithContext(ctx context.Context, pattern string) (*credentials.AuthConfig, error) {
	m.clear()
	return m.CredentialManager.GetConfigByPatternWithContext(ctx, pattern)
}
func TestProviderExistingCredentialRejectsSessionChangeBeforeSave(t *testing.T) {
	for _, action := range []string{"create", "update"} {
		t.Run(action, func(t *testing.T) {
			service, mgr, ctx := chatGPTTestService(t)
			req := CreateRequest{ID: "api", Name: "Original", Type: "custom", BaseURL: "https://example.com"}
			if action == "update" {
				if _, err := service.Create(ctx, req); err != nil {
					t.Fatal(err)
				}
			}
			service.credMgr = &sessionChangingCredentialManager{CredentialManager: mgr, clear: service.registry.Clear}
			var err error
			if action == "create" {
				_, err = service.Create(ctx, req)
			} else {
				_, err = service.Update(ctx, "api", UpdateRequest{Name: "Stale"})
			}
			if err == nil {
				t.Fatal("stale session accepted")
			}
			if service.registry.Get("api") != nil {
				t.Fatal("stale publication")
			}
			got, err := service.store.Get(ctx, "api")
			if action == "create" {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					t.Fatal("stale create persisted")
				}
			} else if err != nil || got.Name != "Original" {
				t.Fatal("stale update persisted")
			}
		})
	}
}
func TestProviderLocalPresetCanConfigureFirstCredential(t *testing.T) {
	for _, sendURL := range []bool{false, true} {
		t.Run(fmt.Sprint(sendURL), func(t *testing.T) {
			service, _, ctx := chatGPTTestService(t)
			p := &llm.ProviderConfig{ID: "local", Name: "Local", Type: llm.ProviderType("localai"), APIFormat: llm.APIFormatOpenAI, BaseURL: "http://localhost:8080", AuthMode: llm.AuthModeNone}
			if err := service.store.Save(ctx, []*llm.ProviderConfig{p}); err != nil {
				t.Fatal(err)
			}
			if err := service.registry.Register(p); err != nil {
				t.Fatal(err)
			}
			req := UpdateRequest{AuthMode: "required", Credential: &CredentialSpec{Pattern: "localhost", Auth: &credentials.AuthConfig{Source: "env", Type: "bearer", SourceConfig: &credentials.SourceConfig{Env: "UNSET_LOCAL_API_TOKEN"}}}}
			if sendURL {
				req.BaseURL = p.BaseURL
			}
			got, err := service.Update(ctx, "local", req)
			if err != nil || got.Provider.CredentialPattern != "localhost" {
				t.Fatalf("local update: %v %v", got, err)
			}
		})
	}
}
func TestProviderLocalPresetPreviewKeepsEffectiveCredential(t *testing.T) {
	service, mgr, ctx := chatGPTTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved" {
			t.Error("preview lost effective credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
	}))
	defer server.Close()
	p := &llm.ProviderConfig{ID: "local", Name: "Local", Type: llm.ProviderType("localai"), APIFormat: llm.APIFormatOpenAI, BaseURL: server.URL, AuthMode: llm.AuthModeNone}
	if err := service.registry.Register(p); err != nil {
		t.Fatal(err)
	}
	pattern, _ := ExtractHostname(server.URL)
	if err := mgr.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "saved"}); err != nil {
		t.Fatal(err)
	}
	models, err := service.ListModelsRaw(ctx, ListModelsRawRequest{ProviderID: p.ID, Type: "localai", APIFormat: "openai", BaseURL: server.URL, AuthMode: "required"})
	if err != nil || len(models) != 1 {
		t.Fatalf("preview: %v %v", models, err)
	}
	if service.registry.Get(p.ID).CredentialPattern != "" {
		t.Fatal("preview modified original")
	}
}
