package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/profiles"
	"assistente/internal/providers"
)

func TestLLMModelsNotWired(t *testing.T) {
	t.Parallel()
	api := NewLLMModels()

	if _, err := api.GetModels(); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("GetModels: got %v", err)
	}
	if _, err := api.GetModelsByProvider("x"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("GetModelsByProvider: got %v", err)
	}
	if _, err := api.RefreshModels(); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("RefreshModels: got %v", err)
	}
	if _, err := api.RefreshModelsByProvider("x"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("RefreshModelsByProvider: got %v", err)
	}
	if _, err := api.GetModelCatalogByProvider("x"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("GetModelCatalogByProvider: got %v", err)
	}
	if _, err := api.RefreshModelCatalogByProvider("x"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("RefreshModelCatalogByProvider: got %v", err)
	}
	if err := api.CancelStreamingForConversation("c1"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("CancelStreamingForConversation: got %v", err)
	}
}

func TestLLMModelsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "llm_models.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("llm_models.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("llm_models.go deve chamar WithUser(session,")
	}
}

func TestLLMModelsAuthRejectsWhenSessionFails(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewLLMModels()
	AttachLLMModels(
		api,
		stubSession{err: semAuth},
		providers.NewService(providers.ServiceConfig{}),
		profiles.NewManager(),
		LLMModelsHooks{CancelStreaming: func(string) {}},
	)

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"GetModels", func() error {
			_, err := api.GetModels()
			return err
		}},
		{"GetModelsByProvider", func() error {
			_, err := api.GetModelsByProvider("x")
			return err
		}},
		{"RefreshModels", func() error {
			_, err := api.RefreshModels()
			return err
		}},
		{"RefreshModelsByProvider", func() error {
			_, err := api.RefreshModelsByProvider("x")
			return err
		}},
		{"GetModelCatalogByProvider", func() error {
			_, err := api.GetModelCatalogByProvider("x")
			return err
		}},
		{"RefreshModelCatalogByProvider", func() error {
			_, err := api.RefreshModelCatalogByProvider("x")
			return err
		}},
		{"CancelStreamingForConversation", func() error {
			return api.CancelStreamingForConversation("c1")
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}
