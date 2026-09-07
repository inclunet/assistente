package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/apidto"
)

func TestLLMProvidersNotWired(t *testing.T) {
	t.Parallel()
	api := NewLLMProviders()

	if _, err := api.GetLLMProviders(); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("GetLLMProviders: got %v", err)
	}
	if _, err := api.GetLLMProvider("x"); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("GetLLMProvider: got %v", err)
	}
	if _, err := api.GetActiveProviderInfo(); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("GetActiveProviderInfo: got %v", err)
	}
	if _, err := api.GetLLMProvidersWithStatus(); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("GetLLMProvidersWithStatus: got %v", err)
	}
	if _, err := api.TestLLMProvider(apidto.TestLLMProviderRequest{}); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("TestLLMProvider: got %v", err)
	}
	if _, err := api.ListModelsRaw(apidto.TestLLMProviderRequest{}); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("ListModelsRaw: got %v", err)
	}
	if _, err := api.CreateLLMProvider(apidto.CreateLLMProviderRequest{}); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("CreateLLMProvider: got %v", err)
	}
	if _, err := api.UpdateLLMProvider("id", apidto.UpdateLLMProviderRequest{}); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("UpdateLLMProvider: got %v", err)
	}
	if err := api.SetDefaultProvider("id"); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("SetDefaultProvider: got %v", err)
	}
	if err := api.DeleteLLMProvider("id"); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("DeleteLLMProvider: got %v", err)
	}
	if err := api.ReloadLLMClient(); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("ReloadLLMClient: got %v", err)
	}
	if err := api.CreateDefaultLLMProvider("openai", "sk"); !errors.Is(err, ErrLLMProvidersNotWired) {
		t.Fatalf("CreateDefaultLLMProvider: got %v", err)
	}
}

func TestLLMProvidersUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "llm_providers.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("llm_providers.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("llm_providers.go deve chamar WithUser(session,")
	}
	// CreateDefaultLLMProvider é bootstrap pré-login — não deve usar WithUser.
	createDefaultIdx := strings.Index(body, "func (p *LLMProviders) CreateDefaultLLMProvider")
	if createDefaultIdx < 0 {
		t.Fatal("CreateDefaultLLMProvider não encontrado")
	}
	rest := body[createDefaultIdx:]
	end := strings.Index(rest, "\nfunc ")
	if end > 0 {
		rest = rest[:end]
	}
	if strings.Contains(rest, "WithUser(") {
		t.Fatal("CreateDefaultLLMProvider não deve usar WithUser (bootstrap pré-login)")
	}
}
