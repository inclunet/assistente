package providers

import (
	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
	"context"
	"testing"
)

func TestPrivateAliasesCRUDAndOfflineRuntime(t *testing.T) {
	ctx := context.Background()
	doc := llmcatalog.Document{Models: []llmcatalog.Model{{ID: "canonical", Parameters: map[string]llmcatalog.Parameter{"temperature": {Support: "supported", Default: 0.7}}}}}
	registry := llm.NewProviderRegistry()
	service := NewService(ServiceConfig{Registry: registry, Store: NewMemoryStore(), CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} }})
	aliases := map[string]string{"private": "canonical"}
	_, err := service.Create(ctx, CreateRequest{ID: "proxy", Name: "Proxy", Type: "custom", APIFormat: "openai", BaseURL: "https://example.com/v1", AuthMode: "none", DefaultModel: "private", ModelAliases: aliases})
	if err != nil {
		t.Fatal(err)
	}
	aliases["private"] = "mutated"
	saved := registry.GetSnapshot("proxy")
	if saved.ModelAliases["private"] != "canonical" {
		t.Fatal("input map retained")
	}
	resolved, err := service.ApplyChatParameterContract(saved, llm.ChatParams{Model: "private"})
	if err != nil || resolved.Model != "private" || resolved.Temperature == nil || *resolved.Temperature != 0.7 {
		t.Fatalf("alias contract: %+v %v", resolved, err)
	}
	doc = llmcatalog.Document{}
	if _, _, err := service.GetChatProviderWithConfigSnapshot(ctx, "proxy"); err != nil {
		t.Fatalf("offline use blocked: %v", err)
	}
	if _, err := service.Update(ctx, "proxy", UpdateRequest{Name: "Renamed"}); err != nil {
		t.Fatalf("offline edit blocked: %v", err)
	}
	preserved := registry.GetSnapshot("proxy").ModelAliases
	if preserved["private"] != "canonical" {
		t.Fatal("offline edit lost binding")
	}
	unknown := map[string]string{"private": "unknown"}
	if _, err := service.Update(ctx, "proxy", UpdateRequest{ModelAliases: &unknown}); err == nil {
		t.Fatal("unknown target accepted")
	}
	empty := map[string]string{}
	if _, err := service.Update(ctx, "proxy", UpdateRequest{ModelAliases: &empty}); err != nil {
		t.Fatal(err)
	}
	if len(registry.GetSnapshot("proxy").ModelAliases) != 0 {
		t.Fatal("binding not removed")
	}
}

func TestPrivateAliasesStoreConversionIsolated(t *testing.T) {
	original := &llm.ProviderConfig{ModelAliases: map[string]string{"alias": "canonical"}}
	row := toDBModel(original)
	restored, err := fromDBModel(row)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ModelAliases["alias"] != "canonical" {
		t.Fatal("lost binding")
	}
	restored.ModelAliases["alias"] = "changed"
	if row.ModelAliases["alias"] != "canonical" || original.ModelAliases["alias"] != "canonical" {
		t.Fatal("shared mutable map")
	}
}
