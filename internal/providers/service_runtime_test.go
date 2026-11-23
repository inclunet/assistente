package providers

import (
	"assistente/internal/llm"
	"testing"
)

func TestLoadForChatAcceptsEmptyStore(t *testing.T) {
	svc := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), Store: NewMemoryStore()})
	if err := svc.LoadForChat(t.Context()); err != nil {
		t.Fatalf("restauração com catálogo de provedores vazio: %v", err)
	}
	if err := svc.Load(t.Context()); err == nil {
		t.Fatal("bootstrap de configuração perdeu a indicação de cadastro vazio")
	}
}
