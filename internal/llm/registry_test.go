package llm

import "testing"

func TestProviderRegistryRegisterGetListRemove(t *testing.T) {
	registry := NewProviderRegistry()

	p1 := &ProviderConfig{
		ID:      "openai-default",
		Name:    "OpenAI Default",
		Type:    ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
	}
	p2 := &ProviderConfig{
		ID:      "claude-prod",
		Name:    "Claude Prod",
		Type:    ProviderClaude,
		BaseURL: "https://api.anthropic.com/v1",
	}

	if err := registry.Register(p1); err != nil {
		t.Fatalf("Register p1: %v", err)
	}
	if err := registry.Register(p2); err != nil {
		t.Fatalf("Register p2: %v", err)
	}

	if got := registry.Get("openai-default"); got == nil {
		t.Fatalf("Get openai-default: nil")
	}
	if got := registry.Get("missing"); got != nil {
		t.Fatalf("Get missing: expected nil")
	}

	list := registry.List()
	if len(list) != 2 {
		t.Fatalf("List len: got %d", len(list))
	}
	if list[0].ID != "claude-prod" || list[1].ID != "openai-default" {
		t.Fatalf("List order unexpected: %s, %s", list[0].ID, list[1].ID)
	}

	if err := registry.Remove("openai-default"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := registry.Get("openai-default"); got != nil {
		t.Fatalf("Get after remove: expected nil")
	}
	if err := registry.Remove("missing"); err == nil {
		t.Fatalf("Remove missing: expected error")
	}
}

func TestProviderRegistryRegisterValidation(t *testing.T) {
	registry := NewProviderRegistry()

	if err := registry.Register(nil); err == nil {
		t.Fatalf("Register nil provider: expected error")
	}

	invalid := &ProviderConfig{
		ID:      "",
		Name:    "",
		BaseURL: "",
	}
	if err := registry.Register(invalid); err == nil {
		t.Fatalf("Register invalid provider: expected error")
	}
}

func TestRegistryGenerationRejectsLatePublication(t *testing.T) {
	r := NewProviderRegistry()
	p := &ProviderConfig{ID: "session", Name: "Session", Type: ProviderOpenAI, BaseURL: "https://api.openai.com/v1"}
	generation := r.Generation()
	if err := r.RegisterGeneration(p, generation); err != nil {
		t.Fatal(err)
	}
	r.Clear()
	if err := r.RegisterGeneration(p, generation); err == nil {
		t.Fatal("old generation accepted")
	}
	if r.Get(p.ID) != nil {
		t.Fatal("cleared provider restored")
	}
	if err := r.RegisterGeneration(p, r.Generation()); err != nil {
		t.Fatal(err)
	}
}
func TestDefaultPublicationPreservesSnapshotsAndRejectsOldSession(t *testing.T) {
	r := NewProviderRegistry()
	old := &ProviderConfig{ID: "first", Name: "First", Type: ProviderOpenAI, BaseURL: "https://example.test/v1", IsDefault: true}
	other := &ProviderConfig{ID: "second", Name: "Second", Type: ProviderOpenAI, BaseURL: "https://example.test/v1"}
	for _, p := range []*ProviderConfig{old, other} {
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	generation := r.Generation()
	if err := r.SetDefaultGeneration(other.ID, generation); err != nil {
		t.Fatal(err)
	}
	if !old.IsDefault || other.IsDefault {
		t.Fatal("mutated snapshots held by readers")
	}
	if r.Get(old.ID).IsDefault || !r.Get(other.ID).IsDefault {
		t.Fatal("default not switched atomically")
	}
	r.Clear()
	if err := r.Register(old); err != nil {
		t.Fatal(err)
	}
	if err := r.SetDefaultGeneration(old.ID, generation); err == nil {
		t.Fatal("accepted previous session")
	}
	if r.Get(old.ID) != old {
		t.Fatal("old session changed current registry")
	}
}
