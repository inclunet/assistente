package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/llmcapabilities"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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
	if _, err := api.GetProfileChatParameterContract("provider", "model"); !errors.Is(err, ErrLLMModelsNotWired) {
		t.Fatalf("GetProfileChatParameterContract: got %v", err)
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

func TestGetProfileChatParameterContractResolvesDefaultSelections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.LLMProvider{}, &database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateLLMModelCapabilities(db); err != nil {
		t.Fatal(err)
	}

	ctx := database.WithUserID(context.Background(), "owner")
	providerConfigs := []*llm.ProviderConfig{
		{ID: "default-provider", Name: "Default", Type: llm.ProviderOpenAI, BaseURL: "https://api.openai.com/v1", APIFormat: llm.APIFormatOpenAI, DefaultModel: "default-model", CompatibilityRevision: 1},
		{ID: "selected-provider", Name: "Selected", Type: llm.ProviderOpenAI, BaseURL: "https://selected.example/v1", APIFormat: llm.APIFormatOpenAI, DefaultModel: "selected-model", CompatibilityRevision: 1},
	}
	registry := llm.NewProviderRegistry()
	store := providers.NewMemoryStore()
	if err := store.Save(ctx, providerConfigs); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDefault(ctx, "default-provider"); err != nil {
		t.Fatal(err)
	}
	for _, config := range providerConfigs {
		if err := registry.Register(config); err != nil {
			t.Fatal(err)
		}
		provider := &database.LLMProvider{
			ID: config.ID, UserID: "owner", Name: config.Name, Type: string(config.Type),
			BaseURL: config.BaseURL, APIFormat: string(config.APIFormat), DefaultModel: config.DefaultModel,
			CompatibilityRevision: 1,
		}
		if err := database.NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
			t.Fatal(err)
		}
	}

	capabilities := database.NewLLMModelCapabilitiesRepository(db)
	for _, item := range []struct{ providerID, modelID string }{
		{"default-provider", "default-model"},
		{"selected-provider", "selected-model"},
	} {
		model, err := capabilities.SaveModel(ctx, item.providerID, item.modelID, item.modelID)
		if err != nil {
			t.Fatal(err)
		}
		if err := capabilities.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, 1, "openai.chat.invalid_request:v1"); err != nil {
			t.Fatal(err)
		}
	}

	api := NewLLMModels()
	AttachLLMModels(api, stubSession{ctx: ctx}, providers.NewService(providers.ServiceConfig{Registry: registry, Store: store}), profiles.NewManager(), LLMModelsHooks{}, capabilities)
	for _, selection := range []struct{ providerID, modelID string }{
		{profiles.DefaultProviderSentinel, "default-model"},
		{"selected-provider", profiles.DefaultProviderSentinel},
		{profiles.DefaultProviderSentinel, profiles.DefaultProviderSentinel},
	} {
		fields, err := api.GetProfileChatParameterContract(selection.providerID, selection.modelID)
		if err != nil {
			t.Fatalf("seleção (%q, %q): %v", selection.providerID, selection.modelID, err)
		}
		if fields[string(llmcapabilities.FieldTemperature)].Support != "unsupported" {
			t.Fatalf("seleção (%q, %q): campos = %v; esperado temperature", selection.providerID, selection.modelID, fields)
		}
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
		nil,
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
		}}, nil)
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
		}, nil)
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
		database.NewLLMModelCapabilitiesRepository(nil),
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
		{"GetProfileChatParameterContract", func() error {
			_, err := api.GetProfileChatParameterContract("provider", "model")
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
