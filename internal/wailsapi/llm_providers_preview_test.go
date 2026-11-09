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
	"net/url"
	"os"
	"testing"
	"time"
)

func TestWailsPreviewCommandHelper(t *testing.T) {
	if os.Getenv("ASSISTENTE_WAILS_PREVIEW_HELPER") != "1" {
		return
	}
	fmt.Print("preview-token")
	os.Exit(0)
}

func TestWailsProviderPreviewPreservesCommandBudget(t *testing.T) {
	for _, tc := range []struct {
		name         string
		seconds      int
		budget       time.Duration
		parentBudget time.Duration
	}{
		{name: "default", budget: 60 * time.Second},
		{name: "configured", seconds: 90, budget: 120 * time.Second},
		{name: "parent_deadline", seconds: 90, budget: 20 * time.Second, parentBudget: 20 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ASSISTENTE_WAILS_PREVIEW_HELPER", "1")
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			original := http.DefaultTransport
			transport := &wailsPreviewBudgetTransport{t: t, budget: tc.budget}
			base := http.DefaultTransport.(*http.Transport).Clone()
			base.Proxy = transport.observe
			http.DefaultTransport = base
			t.Cleanup(func() { http.DefaultTransport = original; base.CloseIdleConnections() })
			ctx := database.WithUserID(context.Background(), "preview-user")
			if tc.parentBudget != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.parentBudget)
				defer cancel()
				transport.parentDeadline, _ = ctx.Deadline()
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer preview-token" {
					t.Error("unexpected upstream credential")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
			}))
			defer server.Close()
			baseURL := server.URL
			registry := llm.NewProviderRegistry()
			service := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credentials.NewManager(nil)})
			ctrl := controllers.NewLLMController(controllers.LLMControllerConfig{LLMRegistry: registry, ProviderSvc: service})
			api := NewLLMProviders()
			AttachLLMProviders(api, stubSession{ctx: ctx}, ctrl, LLMProvidersHooks{})
			pattern, _ := providers.ExtractHostname(baseURL)
			models, err := api.ListModelsRaw(apidto.TestLLMProviderRequest{Type: "custom", BaseURL: baseURL, APIFormat: "openai", AuthMode: "required", Credential: &apidto.CredentialInput{
				Pattern: pattern, Type: "bearer", Source: "command", SourceConfig: &credentials.SourceConfig{Command: exe, Args: []string{"-test.run=^TestWailsPreviewCommandHelper$"}, TimeoutSeconds: tc.seconds},
			}})
			if err != nil || len(models) != 1 || models[0] != "model" {
				t.Fatalf("Wails command preview: %v %v", models, err)
			}
			if transport.calls != 1 {
				t.Fatalf("requests: got %d, want 1", transport.calls)
			}
		})
	}
}

// O serviço mantém a prova do comando real de 16 s. Na borda, observar o
// contexto que chega ao HTTP detecta um teto indevido sem repetir a espera.
type wailsPreviewBudgetTransport struct {
	t              *testing.T
	calls          int
	budget         time.Duration
	parentDeadline time.Time
}

func (tr *wailsPreviewBudgetTransport) observe(req *http.Request) (*url.URL, error) {
	tr.calls++
	if req.Header.Get("Authorization") != "Bearer preview-token" {
		tr.t.Error("unexpected credential")
	}
	deadline, ok := req.Context().Deadline()
	remaining := time.Until(deadline)
	if !ok || remaining <= tr.budget-10*time.Second || remaining > tr.budget {
		tr.t.Errorf("command preview budget lost: deadline=%v remaining=%v", ok, remaining)
	}
	if !tr.parentDeadline.IsZero() && !deadline.Equal(tr.parentDeadline) {
		tr.t.Errorf("parent deadline lost: got %v want %v", deadline, tr.parentDeadline)
	}
	return nil, nil // conexão direta ao servidor local, sem proxy
}
