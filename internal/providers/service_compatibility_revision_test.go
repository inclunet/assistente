package providers

import (
	"context"
	"testing"

	"assistente/internal/llm"
)

type revisionAwareProviderStore struct {
	ProviderStore
	bumps []string
}

func (s *revisionAwareProviderStore) BumpCompatibilityRevision(_ context.Context, id string) error {
	s.bumps = append(s.bumps, id)
	return nil
}

func TestUpdateAPIKeyInvalidatesConnectionCompatibilityRevision(t *testing.T) {
	store := &revisionAwareProviderStore{ProviderStore: NewMemoryStore()}
	credentials := &credSpy{}
	service := NewService(ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  credentials,
		Store:    store,
	})
	ctx := context.Background()
	if _, err := service.Create(ctx, CreateRequest{
		ID: "custom-provider", Name: "Custom", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	}); err != nil {
		t.Fatalf("criar provedor: %v", err)
	}
	if _, err := service.Update(ctx, "custom-provider", UpdateRequest{APIKey: "new-secret"}); err != nil {
		t.Fatalf("atualizar chave: %v", err)
	}
	if len(store.bumps) != 1 || store.bumps[0] != "custom-provider" {
		t.Fatalf("troca da chave não invalidou a identidade da conexão: %v", store.bumps)
	}
	if len(credentials.registrados) != 1 || credentials.registrados[0] != "api.example.test" {
		t.Fatalf("a credencial não foi registrada no pattern esperado: %v", credentials.registrados)
	}
}
