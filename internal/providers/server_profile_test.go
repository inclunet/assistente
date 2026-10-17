package providers

import (
	"context"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
)

func TestProviderProfileIDCreatePreserveAndClear(t *testing.T) {
	registry := llm.NewProviderRegistry()
	service := NewService(ServiceConfig{
		Registry: registry,
		CredMgr:  credentials.NewManager(nil),
		Store:    NewMemoryStore(),
	})

	created, err := service.Create(context.Background(), CreateRequest{
		ID: "openrouter-connection", Name: "OpenRouter", Type: "custom",
		APIFormat: "openai", BaseURL: "https://openrouter.ai/api/v1",
		AuthMode: "none", ServerProfileID: " openrouter ",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Provider.ServerProfileID != "openrouter" {
		t.Fatalf("perfil após criação = %q, esperado openrouter", created.Provider.ServerProfileID)
	}

	updated, err := service.Update(context.Background(), created.Provider.ID, UpdateRequest{Name: "OpenRouter editado"})
	if err != nil {
		t.Fatalf("Update() parcial error = %v", err)
	}
	if updated.Provider.ServerProfileID != "openrouter" {
		t.Fatalf("edição parcial perdeu perfil = %q", updated.Provider.ServerProfileID)
	}

	clear := ""
	updated, err = service.Update(context.Background(), created.Provider.ID, UpdateRequest{ServerProfileID: &clear})
	if err != nil {
		t.Fatalf("Update() para limpar perfil error = %v", err)
	}
	if updated.Provider.ServerProfileID != "" {
		t.Fatalf("perfil após limpeza = %q, esperado vazio", updated.Provider.ServerProfileID)
	}
}
