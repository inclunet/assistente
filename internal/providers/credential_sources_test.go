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
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProviderCredentialCommandHelper(t *testing.T) {
	if os.Getenv("PROVIDER_CREDENTIAL_HELPER") != "1" {
		return
	}
	if path := os.Getenv("PROVIDER_COMMAND_VALUE_FILE"); path != "" {
		value, err := os.ReadFile(path)
		if err != nil {
			os.Exit(3)
		}
		f, err := os.OpenFile(os.Getenv("PROVIDER_COMMAND_CALLS_FILE"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(3)
		}
		if _, err := fmt.Fprintln(f, "call"); err != nil {
			os.Exit(3)
		}
		if err := f.Close(); err != nil {
			os.Exit(3)
		}
		fmt.Print(string(value))
		os.Exit(0)
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

func TestProbesAuthNoneIgnoraChaveInformada(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("AuthNone sent Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"data":[{"id":"model"}]}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	registry := llm.NewProviderRegistry()
	if err := registry.Register(&llm.ProviderConfig{ID: "none", Name: "none", Type: llm.ProviderOpenAI, BaseURL: server.URL, CredentialPattern: "ignored", AuthMode: llm.AuthModeNone}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(ServiceConfig{Registry: registry, CredMgr: credentials.NewManager(nil), Store: NewMemoryStore()})
	ctx := context.Background()
	req := TestRequest{BaseURL: server.URL, ProviderID: "none", APIKey: "must-not-be-sent"}
	if _, err := svc.TestConnection(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListModels(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListModelsRaw(ctx, ListModelsRawRequest{Type: "openai", BaseURL: server.URL, ProviderID: "none", APIKey: req.APIKey}); err != nil {
		t.Fatal(err)
	}
}

type metadataOnlyCredentials struct{ credSpy }

func (*metadataOnlyCredentials) GetByPatternWithContext(context.Context, string) (*credentials.AuthConfig, error) {
	panic("metadata query materialized source")
}
func (*metadataOnlyCredentials) GetConfigByPatternWithContext(context.Context, string) (*credentials.AuthConfig, error) {
	return &credentials.AuthConfig{Source: "command", Type: "bearer", SourceConfig: &credentials.SourceConfig{Command: "never-execute"}}, nil
}
func TestProviderMetadataRequiresNonMaterializingReader(t *testing.T) {
	svc := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), CredMgr: &metadataOnlyCredentials{}, Store: NewMemoryStore()})
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateRequest{ID: "metadata", Name: "Metadata", Type: "openai", BaseURL: "https://example.com"})
	if err != nil || !created.CredentialConfigured {
		t.Fatalf("create metadata: %v", err)
	}
	updated, err := svc.Update(ctx, "metadata", UpdateRequest{Name: "Updated"})
	if err != nil || !updated.CredentialConfigured {
		t.Fatalf("update metadata: %v", err)
	}
	status := svc.ListWithStatus(ctx)
	if len(status) != 1 || !status[0].CredentialConfigured {
		t.Fatal("listing lost configuration")
	}
}

func TestProviderCommandCacheSharedByChatProbesAndHealth(t *testing.T) {
	t.Setenv("PROVIDER_CREDENTIAL_HELPER", "1")
	dir := t.TempDir()
	valueFile := filepath.Join(dir, "value")
	callsFile := filepath.Join(dir, "calls")
	t.Setenv("PROVIDER_COMMAND_VALUE_FILE", valueFile)
	t.Setenv("PROVIDER_COMMAND_CALLS_FILE", callsFile)
	set := func(v string) {
		t.Helper()
		if e := os.WriteFile(valueFile, []byte(v), 0600); e != nil {
			t.Fatal(e)
		}
	}
	count := func() int { v, _ := os.ReadFile(callsFile); return strings.Count(string(v), "call\n") }
	set("first")
	var accepted atomic.Value
	accepted.Store("first")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+accepted.Load().(string) {
			w.WriteHeader(401)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"model"}]}`)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	mgr := credentials.NewManager(nil)
	if e := mgr.RegisterPattern(u.Hostname(), &credentials.AuthConfig{Source: "command", Type: "bearer", SourceConfig: &credentials.SourceConfig{Command: exe, Args: []string{"-test.run=^TestProviderCredentialCommandHelper$"}}}); e != nil {
		t.Fatal(e)
	}
	registry := llm.NewProviderRegistry()
	if e := registry.Register(&llm.ProviderConfig{ID: "saved", Name: "saved", Type: llm.ProviderOpenAI, BaseURL: server.URL, CredentialPattern: u.Hostname()}); e != nil {
		t.Fatal(e)
	}
	svc := NewService(ServiceConfig{Registry: registry, CredMgr: mgr, Store: NewMemoryStore()})
	ctx := context.Background()
	req := TestRequest{BaseURL: server.URL, ProviderID: "saved"}
	client := credentials.NewHTTPClient(mgr, u.Hostname(), 0)
	response, e := client.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	_ = response.Body.Close()
	if ok, e := svc.TestConnection(ctx, req); e != nil || !ok {
		t.Fatal(e)
	}
	if _, e := svc.ListModels(ctx, req); e != nil {
		t.Fatal(e)
	}
	if health := svc.CheckHealth(ctx, profileForProvider("saved")); health.State != HealthOnline {
		t.Fatalf("health=%+v", health)
	}
	if count() != 1 {
		t.Fatalf("cache não compartilhado: %d", count())
	}
	accepted.Store("second")
	set("second")
	if health := svc.CheckHealth(ctx, profileForProvider("saved")); health.State != HealthOnline {
		t.Fatalf("health renovado=%+v", health)
	}
	if count() != 2 {
		t.Fatalf("renovação=%d", count())
	}
	accepted.Store("third")
	set("third")
	if _, e := svc.ListModels(ctx, req); e != nil {
		t.Fatal(e)
	}
	accepted.Store("fourth")
	set("fourth")
	if ok, e := svc.TestConnection(ctx, req); e != nil || !ok {
		t.Fatal(e)
	}
	if count() != 4 {
		t.Fatalf("probes sem renovação: %d", count())
	}
}

func TestCommandProbeRejectsCrossOriginRedirect(t *testing.T) {
	for _, source := range []string{"command", "env", "static"} {
		for _, scheme := range []string{"bearer", "custom"} {
			t.Run(source+"/"+scheme, func(t *testing.T) {
				t.Setenv("PROVIDER_CREDENTIAL_HELPER", "1")
				var reached atomic.Int32
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1); w.WriteHeader(200) }))
				defer destination.Close()
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
				defer origin.Close()
				u, _ := url.Parse(origin.URL)
				exe, e := os.Executable()
				if e != nil {
					t.Fatal(e)
				}
				mgr := credentials.NewManager(nil)
				config := &credentials.AuthConfig{Source: source, Type: scheme}
				switch source {
				case "command":
					config.SourceConfig = &credentials.SourceConfig{Command: exe, Args: []string{"-test.run=^TestProviderCredentialCommandHelper$"}}
				case "env":
					t.Setenv("PROBE_REDIRECT_TOKEN", "source-token")
					config.SourceConfig = &credentials.SourceConfig{Env: "PROBE_REDIRECT_TOKEN"}
				case "static":
					config.Token = "source-token"
				}
				if scheme == "custom" {
					config.Headers = map[string]string{"X-Credential": ""}
					if source == "static" {
						config.Headers["X-Credential"] = "source-token"
						config.Token = ""
					}
				}
				if e := mgr.RegisterPattern(u.Hostname(), config); e != nil {
					t.Fatal(e)
				}
				registry := llm.NewProviderRegistry()
				if e := registry.Register(&llm.ProviderConfig{ID: "redirect", Name: "redirect", Type: llm.ProviderOpenAI, BaseURL: origin.URL, CredentialPattern: u.Hostname()}); e != nil {
					t.Fatal(e)
				}
				svc := NewService(ServiceConfig{Registry: registry, CredMgr: mgr, Store: NewMemoryStore()})
				req := TestRequest{BaseURL: origin.URL, ProviderID: "redirect"}
				if ok, e := svc.TestConnection(context.Background(), req); e == nil || ok {
					t.Fatal("redirect externo aceito")
				}
				if _, e := svc.ListModels(context.Background(), req); e == nil {
					t.Fatal("modelos seguiram redirect externo")
				}
				if health := svc.CheckHealth(context.Background(), profileForProvider("redirect")); health.State == HealthOnline {
					t.Fatal("health seguiu redirect externo")
				}
				if reached.Load() != 0 {
					t.Fatal("credencial foi enviada a outra origem")
				}

			})
		}
	}
}
