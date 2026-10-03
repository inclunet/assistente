package llm

import (
	"errors"
	"testing"
)

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

func TestProviderRegistryCompatibilityRevisionNeverRegresses(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &ProviderConfig{
		ID:                    "provider-a",
		Name:                  "Provider A",
		Type:                  ProviderOpenAI,
		BaseURL:               "https://api.example.test/v1",
		CompatibilityRevision: 3,
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}

	if err := registry.UpdateCompatibilityRevisions(map[string]int{provider.ID: 2}); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(provider.ID).CompatibilityRevision; got != 3 {
		t.Fatalf("revisão regrediu para %d; esperada 3", got)
	}
	if err := registry.UpdateCompatibilityRevisions(map[string]int{provider.ID: 4}); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(provider.ID).CompatibilityRevision; got != 4 {
		t.Fatalf("revisão atualizada=%d, esperada 4", got)
	}
}

func TestProviderRegistryRejectsConfigurationChangeWithoutNewRevision(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &ProviderConfig{
		ID: "provider", Name: "Provider", Type: ProviderCustom,
		APIFormat: APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		Model: "model-a", CompatibilityRevision: 1, ConfigRevision: 7,
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	changedWithoutRevision := *provider
	changedWithoutRevision.Model = "model-b"
	if err := registry.Register(&changedWithoutRevision); !errors.Is(err, ErrStaleProviderSnapshot) {
		t.Fatalf("mudança de configuração com revisão repetida foi aceita: %v", err)
	}
	if got := registry.Get(provider.ID); got == nil || got.Model != "model-a" {
		t.Fatalf("snapshot sem revisão nova substituiu o modelo: %+v", got)
	}
}

func TestProviderRegistryDelayedRegistrationPreservesRevisionAndStaleState(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &ProviderConfig{
		ID: "provider", Name: "Provider", Type: ProviderCustom,
		APIFormat: APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test", CompatibilityRevision: 1,
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateCompatibilityRevisions(map[string]int{provider.ID: 2}); err != nil {
		t.Fatal(err)
	}
	authoritative := *provider
	authoritative.BaseURL = "https://new.example.test/v1"
	authoritative.CompatibilityRevision = 3
	authoritative.ConfigRevision = 1
	if err := registry.Register(&authoritative); err != nil {
		t.Fatal(err)
	}
	staleSnapshot := *provider
	staleSnapshot.CompatibilityRevision = 2
	if err := registry.Register(&staleSnapshot); !errors.Is(err, ErrStaleProviderSnapshot) {
		t.Fatalf("snapshot atrasado aceito: %v", err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CompatibilityRevision != 3 || got.BaseURL != authoritative.BaseURL {
		t.Fatalf("snapshot atrasado substituiu campos autoritativos: %+v", got)
	}

	generation := registry.BeginCredentialPatternRevisionSync(provider.CredentialPattern)
	newer := authoritative
	newer.CompatibilityRevision = 4
	if err := registry.Register(&newer); err != nil {
		t.Fatal(err)
	}
	if registry.Get(provider.ID) != nil || len(registry.List()) != 0 {
		t.Fatal("registro normal reabriu um snapshot marcado stale")
	}
	if err := registry.PublishCredentialPatternRevisions(provider.CredentialPattern, generation, map[string]int{provider.ID: 4}); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CompatibilityRevision != 4 {
		t.Fatalf("leitura autoritativa não recuperou o snapshot: %+v", got)
	}
}

func TestProviderRegistryKeepsRevisionWatermarkBeforeFirstRegistration(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &ProviderConfig{
		ID: "provider", Name: "Provider", Type: ProviderCustom,
		APIFormat: APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test", CompatibilityRevision: 1,
	}
	if err := registry.UpdateCompatibilityRevisions(map[string]int{provider.ID: 2}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(provider); !errors.Is(err, ErrStaleProviderSnapshot) {
		t.Fatalf("snapshot anterior ao watermark aceito: %v", err)
	}
	if got := registry.Get(provider.ID); got != nil {
		t.Fatalf("snapshot desatualizado apareceu no registry: %+v", got)
	}
	authoritative := *provider
	authoritative.CompatibilityRevision = 2
	if err := registry.Register(&authoritative); err != nil {
		t.Fatalf("registrar snapshot autoritativo: %v", err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CompatibilityRevision != 2 {
		t.Fatalf("primeiro registro autoritativo ignorou watermark: %+v", got)
	}
}

func TestProviderRegistryRemoveClearsOrphanedStalePattern(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &ProviderConfig{
		ID: "provider", Name: "Provider", Type: ProviderCustom,
		APIFormat: APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test", CompatibilityRevision: 1,
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	generation := registry.BeginCredentialPatternRevisionSync(provider.CredentialPattern)
	if err := registry.PublishCredentialPatternRevisions(provider.CredentialPattern, generation, map[string]int{}); err != nil {
		t.Fatal(err)
	}
	if registry.Get(provider.ID) != nil {
		t.Fatal("provider sem revisão autoritativa ficou disponível")
	}
	if err := registry.Remove(provider.ID); err != nil {
		t.Fatal(err)
	}
	newProvider := *provider
	newProvider.ID = "new-provider"
	if err := registry.Register(&newProvider); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(newProvider.ID); got == nil {
		t.Fatal("pattern stale órfão ocultou um provider novo")
	}
}

func TestProviderRegistryRemoveInvalidatesLateRefreshBeforeProviderIDReuse(t *testing.T) {
	registry := NewProviderRegistry()
	removed := &ProviderConfig{
		ID: "reused", Name: "Removed", Type: ProviderCustom,
		APIFormat: APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test", CompatibilityRevision: 1,
	}
	remaining := *removed
	remaining.ID = "remaining"
	remaining.Name = "Remaining"
	for _, provider := range []*ProviderConfig{removed, &remaining} {
		if err := registry.Register(provider); err != nil {
			t.Fatal(err)
		}
	}
	oldGeneration := registry.BeginCredentialPatternRevisionSync(removed.CredentialPattern)
	if err := registry.Remove(removed.ID); err != nil {
		t.Fatal(err)
	}
	if err := registry.PublishCredentialPatternRevisions(removed.CredentialPattern, oldGeneration, map[string]int{
		removed.ID: 2, remaining.ID: 2,
	}); err != nil {
		t.Fatal(err)
	}

	recreated := *removed
	recreated.Name = "Recreated"
	if err := registry.Register(&recreated); err != nil {
		t.Fatalf("ID reutilizado rejeitado por watermark de refresh antigo: %v", err)
	}
	newGeneration := registry.BeginCredentialPatternRevisionSync(removed.CredentialPattern)
	if err := registry.PublishCredentialPatternRevisions(removed.CredentialPattern, newGeneration, map[string]int{
		recreated.ID: 1, remaining.ID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(recreated.ID); got == nil || got.Name != recreated.Name || got.CompatibilityRevision != 1 {
		t.Fatalf("provider recriado não foi publicado na revisão própria: %+v", got)
	}
	if got := registry.Get(remaining.ID); got == nil || got.CompatibilityRevision != 1 {
		t.Fatalf("provider restante não voltou após o refresh atual: %+v", got)
	}
}

func TestProviderRegistryFailureBeforeRegistrationKeepsPatternStale(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &ProviderConfig{
		ID: "provider", Name: "Provider", Type: ProviderCustom,
		APIFormat: APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test", CompatibilityRevision: 1,
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	registry.Clear()
	generation := registry.BeginCredentialPatternRevisionSync(provider.CredentialPattern)
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if registry.Get(provider.ID) != nil {
		t.Fatal("registro novo ficou disponível antes de recuperar a revisão stale")
	}
	if err := registry.PublishCredentialPatternRevisions(provider.CredentialPattern, generation, map[string]int{provider.ID: 2}); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CompatibilityRevision != 2 {
		t.Fatalf("refresh autoritativo não publicou o provider na revisão atual: %+v", got)
	}
}

func TestProviderRegistryProviderMovedOffStalePatternBecomesAvailable(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &ProviderConfig{
		ID: "provider", Name: "Provider", Type: ProviderCustom,
		APIFormat: APIFormatOpenAI, BaseURL: "https://old.example.test/v1",
		CredentialPattern: "old.example.test", CompatibilityRevision: 1,
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	oldPatternGeneration := registry.BeginCredentialPatternRevisionSync(provider.CredentialPattern)

	moved := *provider
	moved.BaseURL = "https://new.example.test/v1"
	moved.CredentialPattern = "new.example.test"
	moved.CompatibilityRevision = 2
	moved.ConfigRevision = 1
	if err := registry.Register(&moved); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CredentialPattern != moved.CredentialPattern {
		t.Fatalf("provider migrado continuou stale pelo pattern antigo: %+v", got)
	}
	if err := registry.PublishCredentialPatternRevisions(provider.CredentialPattern, oldPatternGeneration, map[string]int{}); err != nil {
		t.Fatal(err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CredentialPattern != moved.CredentialPattern {
		t.Fatalf("conclusão do refresh antigo ocultou o provider migrado: %+v", got)
	}
}
