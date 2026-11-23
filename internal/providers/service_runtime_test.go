package providers

import (
	"context"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/profiles"
)

func TestChatStartupAcceptsEmptyStore(t *testing.T) {
	cfg := ServiceConfig{Registry: llm.NewProviderRegistry(), Store: NewMemoryStore(), SkipBootRepairs: true}
	svc := NewService(cfg)
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("restauração com catálogo de provedores vazio: %v", err)
	}
	cfg.SkipBootRepairs = false
	if err := NewService(cfg).Load(t.Context()); err == nil {
		t.Fatal("bootstrap de configuração perdeu a indicação de cadastro vazio")
	}
}

// Reproduz a escolha de um padrão no desktop após a primeira leitura da CLI.
type defaultChangedStore struct {
	*MemoryStore
	selected string
}

func (s defaultChangedStore) GetDefault(ctx context.Context) (*llm.ProviderConfig, error) {
	current, err := s.MemoryStore.GetDefault(ctx)
	if err != nil {
		return nil, err
	}
	if s.selected != "" {
		err = s.SetDefault(ctx, s.selected)
	}
	return current, err
}

func TestProfileDefaultWithStartupRepairsDisabled(t *testing.T) {
	for _, tc := range []struct {
		name               string
		registered, stored []string
		selected, want     string
	}{
		{name: "padrão escolhido entre leituras", registered: []string{"a", "b"}, stored: []string{"a", "b"}, selected: "b", want: "b"},
		{name: "registro e cadastro divergem", registered: []string{"a", "c"}, stored: []string{"b", "c"}, want: "c"},
		{name: "cadastro vazio", registered: []string{"a"}, want: profiles.DefaultProviderSentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			store := defaultChangedStore{MemoryStore: NewMemoryStore(), selected: tc.selected}
			registry := llm.NewProviderRegistry()
			makeProvider := func(id string) *llm.ProviderConfig {
				return &llm.ProviderConfig{ID: id, Name: id, Type: llm.ProviderOpenAI, BaseURL: "https://example.org/v1"}
			}
			for _, id := range tc.stored {
				if err := store.Save(ctx, []*llm.ProviderConfig{makeProvider(id)}); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range tc.registered {
				if err := registry.Register(makeProvider(id)); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewService(ServiceConfig{Registry: registry, Store: store, SkipBootRepairs: true})
			profile := &profiles.Profile{Chat: profiles.ChatConfig{LLMProvider: profiles.DefaultProviderSentinel}}
			resolved := svc.ResolveProfileDefaults(ctx, profile)
			if resolved.Chat.LLMProvider != tc.want {
				t.Fatalf("provedor selecionado=%q, esperado=%q", resolved.Chat.LLMProvider, tc.want)
			}
		})
	}
}
