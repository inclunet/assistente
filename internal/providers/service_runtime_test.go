package providers

import (
	"testing"

	"assistente/internal/llm"
)

func TestReadOnlyStartupAcceptsEmptyStore(t *testing.T) {
	cfg := ServiceConfig{Registry: llm.NewProviderRegistry(), Store: NewMemoryStore(), ReadOnlyStartup: true}
	svc := NewService(cfg)
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("restauração com catálogo de provedores vazio: %v", err)
	}
	cfg.ReadOnlyStartup = false
	if err := NewService(cfg).Load(t.Context()); err == nil {
		t.Fatal("bootstrap de configuração perdeu a indicação de cadastro vazio")
	}
}
