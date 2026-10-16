package commandcatalog

import (
	"errors"
	"reflect"
	"testing"
)

func readinessRegistration() Registration {
	return Registration{
		Definition: Definition{
			ID:             "workspace.tab.read",
			Effect:         Read,
			Decision:       NoDecision,
			AllowedSources: []Source{UI},
			Context: ContextPolicy{Facts: []ContextFact{
				{Provider: "workspace", Fact: "active_tab", Mode: ExactVersion},
			}},
			Presentation: presentation(),
		},
		Handler: HandlerContract{Effect: Read},
	}
}

func readyRegistry(t *testing.T, registration Registration) *Registry {
	t.Helper()
	r, err := New([]Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCheckReadinessAceitaLeituraComProviderEDesprendeDefinicao(t *testing.T) {
	r := readyRegistry(t, readinessRegistration())

	got, err := r.CheckReadiness("workspace.tab.read", UI)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := r.Lookup("workspace.tab.read")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("definição inesperada: got %#v want %#v", got, want)
	}

	got.AllowedSources[0] = System
	got.Context.Facts[0].Provider = "alterado"
	got.Presentation.Locales["pt-BR"].Aliases[0] = "alterado"
	again, err := r.CheckReadiness("workspace.tab.read", UI)
	if err != nil {
		t.Fatal(err)
	}
	if again.AllowsSource(System) || again.Context.Facts[0].Provider != "workspace" || again.Presentation.Locales["pt-BR"].Aliases[0] == "alterado" {
		t.Fatalf("retorno não é detached: %#v", again)
	}
}

func TestCheckReadinessRecusaIDDesconhecidoEAlias(t *testing.T) {
	r := readyRegistry(t, readinessRegistration())
	for _, id := range []string{"workspace.tab.missing", "Nova aba", "aba nova", "Workspace.Tab.Read"} {
		t.Run(id, func(t *testing.T) {
			_, err := r.CheckReadiness(id, UI)
			if !errors.Is(err, ErrNotReady) {
				t.Fatalf("erro não preservou ErrNotReady: %v", err)
			}
		})
	}
}

func TestCheckReadinessRecusaOrigemProibidaOuDesconhecida(t *testing.T) {
	r := readyRegistry(t, readinessRegistration())
	for _, source := range []Source{CLI, System, Source("desktop.unknown")} {
		t.Run(string(source), func(t *testing.T) {
			_, err := r.CheckReadiness("workspace.tab.read", source)
			if !errors.Is(err, ErrNotReady) {
				t.Fatalf("erro não preservou ErrNotReady: %v", err)
			}
		})
	}
}

func TestCheckReadinessRecusaMetadataAusente(t *testing.T) {
	r := readinessRegistration()
	r.Definition.Presentation = nil
	registry := readyRegistry(t, r)
	if _, err := registry.CheckReadiness(r.Definition.ID, UI); !errors.Is(err, ErrNotReady) {
		t.Fatalf("aceitou apresentação ausente: %v", err)
	}
}

func TestCheckReadinessRecusaWriteDestructiveInteractiveEMutabilidade(t *testing.T) {
	tests := map[string]func(*Registration){
		"write": func(r *Registration) {
			r.Definition.Effect = Write
			r.Handler.Effect = Write
		},
		"destructive": func(r *Registration) {
			r.Definition.Effect = Destructive
			r.Definition.Decision = Interactive
			r.Handler.Effect = Destructive
		},
		"interactive":  func(r *Registration) { r.Definition.Decision = Interactive },
		"alvo mutável": func(r *Registration) { r.Definition.HasMutableTarget = true },
		"capability mutável": func(r *Registration) {
			r.Definition.MutatesEffectiveCapability = true
			r.Handler.MutatesEffectiveCapability = true
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			r := readinessRegistration()
			change(&r)
			registry := readyRegistry(t, r)
			if _, err := registry.CheckReadiness(r.Definition.ID, UI); !errors.Is(err, ErrNotReady) {
				t.Fatalf("aceitou definição fora do gate: %v", err)
			}
		})
	}
}

func TestCheckReadinessRegistryNulo(t *testing.T) {
	var registry *Registry
	if _, err := registry.CheckReadiness("workspace.tab.read", UI); !errors.Is(err, ErrNotReady) {
		t.Fatalf("erro inesperado para registry nulo: %v", err)
	}
}
