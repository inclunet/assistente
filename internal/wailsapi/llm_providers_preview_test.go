package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/providers"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestWailsPreviewCommandHelper(t *testing.T) {
	if os.Getenv("ASSISTENTE_WAILS_PREVIEW_HELPER") != "1" {
		return
	}
	time.Sleep(16 * time.Second)
	fmt.Print("preview-token")
	os.Exit(0)
}

func TestWailsProviderPreviewAllowsSlowCommand(t *testing.T) {
	t.Setenv("ASSISTENTE_WAILS_PREVIEW_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer preview-token" {
			t.Error("unexpected credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
	}))
	defer server.Close()
	registry := llm.NewProviderRegistry()
	service := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credentials.NewManager(nil)})
	ctrl := controllers.NewLLMController(controllers.LLMControllerConfig{LLMRegistry: registry, ProviderSvc: service})
	api := NewLLMProviders()
	AttachLLMProviders(api, stubSession{ctx: database.WithUserID(context.Background(), "preview-user")}, ctrl, LLMProvidersHooks{})
	pattern, _ := providers.ExtractHostname(server.URL)
	models, err := api.ListModelsRaw(apidto.TestLLMProviderRequest{Type: "custom", BaseURL: server.URL, APIFormat: "openai", AuthMode: "required", Credential: &apidto.CredentialInput{
		Pattern: pattern, Type: "bearer", Source: "command", SourceConfig: &credentials.SourceConfig{Command: exe, Args: []string{"-test.run=^TestWailsPreviewCommandHelper$"}, TimeoutSeconds: 30},
	}})
	if err != nil || len(models) != 1 || models[0] != "model" {
		t.Fatalf("Wails slow preview: %v %v", models, err)
	}
}
