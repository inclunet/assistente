package providers

import (
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestProviderCredentialCommandHelper(t *testing.T) {
	if os.Getenv("PROVIDER_CREDENTIAL_HELPER") != "1" {
		return
	}
	fmt.Print("source-token")
	os.Exit(0)
}

func TestProviderProbesUseSourcesAndPreserveCredentials(t *testing.T) {
	t.Setenv("PROVIDER_CREDENTIAL_HELPER", "1")
	t.Setenv("PROVIDER_SOURCE_TOKEN", "source-token")
	for _, source := range []string{"env", "command"} {
		for _, scheme := range []string{"bearer", "custom"} {
			t.Run(source+scheme, func(t *testing.T) {
				var seen string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					seen = r.Header.Get("Authorization")
					if scheme == "custom" {
						seen = r.Header.Get("X-Credential")
					}
					w.Header().Set("Content-Type", "application/json")
					if _, err := fmt.Fprint(w, `{"data":[{"id":"model"}]}`); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				u, _ := url.Parse(server.URL)
				ctx := context.Background()
				mgr := credentials.NewManager(nil)
				cfg := &credentials.SourceConfig{Env: "PROVIDER_SOURCE_TOKEN"}
				if source == "command" {
					exe, err := os.Executable()
					if err != nil {
						t.Fatal(err)
					}
					cfg = &credentials.SourceConfig{Command: exe, Args: []string{"-test.run=^TestProviderCredentialCommandHelper$"}}
				}
				auth := &credentials.AuthConfig{Source: source, SourceConfig: cfg, Type: scheme}
				if scheme == "custom" {
					auth.Headers = map[string]string{"X-Credential": ""}
				}
				if err := mgr.RegisterPattern(u.Hostname(), auth); err != nil {
					t.Fatal(err)
				}
				registry := llm.NewProviderRegistry()
				if err := registry.Register(&llm.ProviderConfig{ID: "saved", Name: "saved", Type: llm.ProviderOpenAI, BaseURL: server.URL, CredentialPattern: u.Hostname()}); err != nil {
					t.Fatal(err)
				}
				svc := NewService(ServiceConfig{Registry: registry, CredMgr: mgr, Store: NewMemoryStore()})
				probe := TestRequest{BaseURL: server.URL, ProviderID: "saved"}
				if ok, err := svc.TestConnection(ctx, probe); err != nil || !ok {
					t.Fatal(err)
				}
				expected := "source-token"
				if scheme == "bearer" {
					expected = "Bearer " + expected
				}
				if seen != expected {
					t.Fatalf("header=%q", seen)
				}
				if _, err := svc.ListModels(ctx, probe); err != nil {
					t.Fatal(err)
				}
				if seen != expected {
					t.Fatalf("header=%q", seen)
				}
				if _, err := svc.ListModelsRaw(ctx, ListModelsRawRequest{Type: "openai", BaseURL: server.URL, ProviderID: "saved"}); err != nil {
					t.Fatal(err)
				}
				if seen != expected {
					t.Fatalf("header=%q", seen)
				}
				if _, err := svc.ListModelsRaw(ctx, ListModelsRawRequest{Type: "openai", BaseURL: server.URL, APIKey: "temporary"}); err != nil {
					t.Fatal(err)
				}
				original, err := mgr.GetConfigByPatternWithContext(ctx, u.Hostname())
				if err != nil || original.Source != source || original.Type != scheme {
					t.Fatalf("stored credential changed: %v", err)
				}
				if _, err := svc.ListModelsRaw(ctx, ListModelsRawRequest{Type: "openai", BaseURL: "http://example.invalid", ProviderID: "saved"}); err == nil {
					t.Fatal("changed origin accepted")
				}
			})
		}
	}
}

func TestProbeAuthModesAgreeWithRuntime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected auth")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"data":[{"id":"model"}]}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	for _, source := range []string{"env", "static"} {
		for _, mode := range []llm.AuthMode{llm.AuthModeOptional, llm.AuthModeNone} {
			t.Run(source+string(mode), func(t *testing.T) {
				mgr := credentials.NewManager(nil)
				t.Setenv("PROVIDER_MISSING_ENV", "")
				auth := &credentials.AuthConfig{Source: source, Type: "bearer"}
				if source == "env" {
					auth.SourceConfig = &credentials.SourceConfig{Env: "PROVIDER_MISSING_ENV"}
				}
				if err := mgr.RegisterPattern("source", auth); err != nil {
					t.Fatal(err)
				}
				registry := llm.NewProviderRegistry()
				base := server.URL
				if mode == llm.AuthModeNone {
					base = "http://old.invalid"
				}
				if err := registry.Register(&llm.ProviderConfig{ID: "modes", Name: "modes", Type: llm.ProviderOpenAI, BaseURL: base, CredentialPattern: "source", AuthMode: mode}); err != nil {
					t.Fatal(err)
				}
				svc := NewService(ServiceConfig{Registry: registry, CredMgr: mgr, Store: NewMemoryStore()})
				req := TestRequest{BaseURL: server.URL, ProviderID: "modes"}
				if _, err := svc.TestConnection(context.Background(), req); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.ListModels(context.Background(), req); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.ListModelsRaw(context.Background(), ListModelsRawRequest{Type: "openai", BaseURL: server.URL, ProviderID: "modes"}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}

}

type legacyCredentialReader struct{ credSpy }

func (*legacyCredentialReader) GetConfigByPatternWithContext(context.Context, string) (*credentials.AuthConfig, error) {
	return &credentials.AuthConfig{Type: "bearer", Token: "legacy"}, nil
}
func TestLegacyCredentialNotConfigured(t *testing.T) {
	svc := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), CredMgr: &legacyCredentialReader{}, Store: NewMemoryStore()})
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateRequest{ID: "legacy", Name: "Legacy", Type: "openai", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if created.CredentialConfigured {
		t.Fatal("creation marked legacy credential configured")
	}
	updated, err := svc.Update(ctx, "legacy", UpdateRequest{Name: "Updated"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CredentialConfigured {
		t.Fatal("update marked legacy credential configured")
	}
	status := svc.ListWithStatus(ctx)
	if len(status) != 1 || status[0].CredentialConfigured {
		t.Fatal("listing marked legacy credential configured")
	}
}
