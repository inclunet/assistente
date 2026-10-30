package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
)

func TestModelCatalogEnrichesOnlyRemoteModelsAndRefreshesMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"openai/alias"},{"id":"private-alias"}]}`)
	}))
	defer server.Close()
	registry := llm.NewProviderRegistry()
	config := &llm.ProviderConfig{ID: "connection", Name: "Proxy", Type: llm.ProviderCustom, APIFormat: llm.APIFormatOpenAI, BaseURL: server.URL, AuthMode: llm.AuthModeNone, ServerProfileID: "litellm", DefaultModel: "private-alias"}
	if err := registry.Register(config); err != nil {
		t.Fatal(err)
	}
	doc := llmcatalog.Document{
		Models: []llmcatalog.Model{{ID: "canonical", Name: "Friendly", Capabilities: map[string]string{"tools.function_calling": "supported"}, Limits: map[string]int64{"context_tokens": 1000}}},
		Exposures: []llmcatalog.Exposure{
			{ServerProfileID: "litellm", APIFormat: "openai", ExposedModelID: "openai/alias", CanonicalModelID: "canonical"},
			{ServerProfileID: "litellm", APIFormat: "openai", ExposedModelID: "not-accessible", CanonicalModelID: "canonical"},
		},
	}
	service := NewService(ServiceConfig{Registry: registry, Store: NewMemoryStore(), CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} }})
	for _, refresh := range []bool{false, true} {
		var result llm.ModelCatalog
		var err error
		if refresh {
			doc.Models[0].Limits["context_tokens"] = 2000
			result, err = service.RefreshModelCatalogByProvider(context.Background(), "connection")
		} else {
			result, err = service.GetModelCatalogByProvider(context.Background(), "connection")
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Models) != 2 || result.Models[0].Value != "openai/alias" || result.Models[1].Value != "private-alias" {
			t.Fatalf("IDs changed: %+v", result)
		}
		known, unknown := result.Models[0], result.Models[1]
		if known.Catalog == nil || known.Catalog.ID != "canonical" || known.Catalog.Limits["context_tokens"] != doc.Models[0].Limits["context_tokens"] || known.Label != "Friendly (openai/alias)" {
			t.Fatalf("metadata missing: %+v", known)
		}
		if unknown.Catalog != nil || unknown.Label != unknown.Value {
			t.Fatal("unknown alias inferred")
		}
	}
	if registry.Get("connection").DefaultModel != "private-alias" {
		t.Fatal("saved model replaced")
	}
	config.ModelAliases = map[string]string{"private-alias": "canonical", "not-accessible": "canonical"}
	if err := registry.Register(config); err != nil {
		t.Fatal(err)
	}
	bound, err := service.GetModelCatalogByProvider(context.Background(), "connection")
	if err != nil || len(bound.Models) != 2 || bound.Models[1].Value != "private-alias" || bound.Models[1].Catalog == nil || bound.Models[1].Catalog.ID != "canonical" {
		t.Fatalf("private binding not applied to offered model: %+v %v", bound, err)
	}
	config.ModelAliases = nil
	config.ServerProfileID = ""
	if err := registry.Register(config); err != nil {
		t.Fatal(err)
	}
	result, err := service.GetModelCatalogByProvider(context.Background(), "connection")
	if err != nil || result.Models[0].Catalog != nil {
		t.Fatalf("legacy connection inferred: %+v %v", result, err)
	}
}

func TestModelCatalogUsesConfigurationThatListedModels(t *testing.T) {
	registry := llm.NewProviderRegistry()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		next := registry.GetSnapshot("connection")
		next.ServerProfileID = "changed"
		if err := registry.Register(next); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"alias"}]}`)
	}))
	defer server.Close()
	if err := registry.Register(&llm.ProviderConfig{ID: "connection", Name: "Proxy", Type: llm.ProviderCustom, APIFormat: llm.APIFormatOpenAI, BaseURL: server.URL, AuthMode: llm.AuthModeNone, ServerProfileID: "original"}); err != nil {
		t.Fatal(err)
	}
	doc := llmcatalog.Document{
		Models:    []llmcatalog.Model{{ID: "canonical", Name: "Original"}},
		Exposures: []llmcatalog.Exposure{{ServerProfileID: "original", APIFormat: "openai", ExposedModelID: "alias", CanonicalModelID: "canonical"}},
	}
	service := NewService(ServiceConfig{Registry: registry, Store: NewMemoryStore(), CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} }})
	result, err := service.GetModelCatalogByProvider(context.Background(), "connection")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 1 || result.Models[0].Catalog == nil || result.Models[0].Catalog.ID != "canonical" {
		t.Fatalf("mixed connection snapshots: %+v", result)
	}
}
