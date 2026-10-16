package wailsapi

import (
	"context"
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
	if err := api.CancelStreamingExecution("c1", "execution-1"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("CancelStreamingExecution: got %v", err)
	}
}

func TestLLMModelsCancelStreamingRequiresHook(t *testing.T) {
	t.Parallel()
	api := NewLLMModels()
	AttachLLMModels(
		api,
		stubSession{},
		providers.NewService(providers.ServiceConfig{}),
		profiles.NewManager(),
		LLMModelsHooks{}, // CancelStreaming nil
	)
	if err := api.CancelStreamingForConversation("c1"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("CancelStreaming sem hook: got %v, quer ErrLLMModelsNotWired", err)
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

func TestLLMModelsCancelExecutionRequiresHook(t *testing.T) {
	t.Parallel()
	api := NewLLMModels()
	AttachLLMModels(api, stubSession{ctx: context.Background()},
		providers.NewService(providers.ServiceConfig{}), profiles.NewManager(),
		LLMModelsHooks{CancelStreaming: func(string) {
			t.Fatal("cancelamento identificado não deve usar o hook da conversa")
		}})
	if err := api.CancelStreamingExecution("c1", "execution-1"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("CancelExecution sem hook: got %v, quer ErrLLMModelsNotWired", err)
	}
}

func TestLLMModelsCancelExecutionForwardsIdentity(t *testing.T) {
	t.Parallel()
	api := NewLLMModels()
	calls := 0
	AttachLLMModels(api, stubSession{ctx: context.Background()},
		providers.NewService(providers.ServiceConfig{}), profiles.NewManager(),
		LLMModelsHooks{
			CancelStreaming: func(string) { t.Fatal("não deve cancelar a conversa inteira") },
			CancelExecution: func(conversationID, executionID string) {
				calls++
				if conversationID != "c1" || executionID != "execution-2" {
					t.Fatalf("identidade recebida = (%q, %q)", conversationID, executionID)
				}
			},
		})
	if err := api.CancelStreamingExecution("c1", "execution-2"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("hook chamado %d vezes; esperado 1", calls)
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
		LLMModelsHooks{
			CancelStreaming: func(string) { t.Error("hook executado sem autenticação") },
			CancelExecution: func(string, string) { t.Error("hook identificado executado sem autenticação") },
		},
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
		{"CancelStreamingExecution", func() error {
			return api.CancelStreamingExecution("c1", "execution-1")
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
