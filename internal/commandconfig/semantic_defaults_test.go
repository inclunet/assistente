package commandconfig

import (
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"testing"
)

func semanticFixture(t *testing.T, change func(*commandcatalog.Definition)) (*commandcatalog.Registry, SemanticDefault) {
	t.Helper()
	d := commandcatalog.Definition{
		ID: "workspace.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		Context: commandcatalog.ContextPolicy{None: true}, AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardLocal, commandcatalog.Palette},
		Risk: commandcatalog.RiskLow, Scopes: []commandcatalog.Scope{commandcatalog.ScopeGlobal},
		Availability:    commandcatalog.Availability{Status: commandcatalog.Available},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"count": {Type: commandcatalog.SchemaInteger}}},
		ResultSchema:    &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Persistence:     commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceSummary},
		HandlerRoute:    "workspace.read", HandlerClassification: "internal",
		Presentation: &commandcatalog.Presentation{Version: "p1", Icon: "file", States: map[string]string{"default": "file"}, Locales: map[string]commandcatalog.LocalizedMetadata{}},
	}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		d.Presentation.Locales[locale] = commandcatalog.LocalizedMetadata{Name: "Read", Description: "Read workspace", Category: "Workspace"}
	}
	if change != nil {
		change(&d)
	}
	r, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: d, Handler: commandcatalog.HandlerContract{Effect: d.Effect, MutatesEffectiveCapability: d.MutatesEffectiveCapability, Route: d.HandlerRoute, Classification: d.HandlerClassification}}})
	if err != nil {
		t.Fatal(err)
	}
	return r, SemanticDefault{
		Candidate: commandbindings.Candidate{ID: "builtin.read", Trigger: "keyboard.local:Control+KeyN", CommandID: d.ID, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true},
		Version:   "d1", TriggerType: commandcatalog.KeyboardLocal, TriggerSpec: []byte(`{"version":1,"code":"KeyN","modifiers":["Control"]}`), Arguments: []byte(`{"count":1}`), AdapterRequirements: []string{"press_edge"},
	}
}

func TestSemanticDefaultPresentationAndPublicationVersionDoNotInvalidate(t *testing.T) {
	r, input := semanticFixture(t, nil)
	base, err := BuildSemanticDefault(r, input)
	if err != nil {
		t.Fatal(err)
	}
	r, input = semanticFixture(t, func(d *commandcatalog.Definition) {
		d.Presentation.Version = "p2"
		d.Presentation.Icon = "another"
		d.Presentation.Locales["en"] = commandcatalog.LocalizedMetadata{Name: "Renamed", Description: "New label", Category: "Other"}
		d.AllowedSources = []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal}
	})
	input.Version = "d2"
	input.Arguments = []byte(`{ "count":1.0 }`)
	input.Candidate.LayerActive = false
	got, err := BuildSemanticDefault(r, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fingerprint != base.Fingerprint {
		t.Fatal("apresentação/versão/estado transitório alterou semântica")
	}
	if got.Candidate.ArgumentsKey != `{"count":1}` {
		t.Fatal("argumentos não canonizados")
	}
}

func TestSemanticDefaultEveryExecutableDimensionInvalidates(t *testing.T) {
	r, input := semanticFixture(t, nil)
	base, err := BuildSemanticDefault(r, input)
	if err != nil {
		t.Fatal(err)
	}
	changes := []func(*SemanticDefault){
		func(d *SemanticDefault) { d.Arguments = []byte(`{"count":2}`) },
		func(d *SemanticDefault) {
			d.TriggerSpec = []byte(`{"version":1,"code":"KeyM","modifiers":["Control"]}`)
		},
		func(d *SemanticDefault) { d.Candidate.ExecutionScopeKey = "different" },
		func(d *SemanticDefault) { d.Candidate.BindingPriority++ },
		func(d *SemanticDefault) { d.Candidate.LayerPriority++ },
		func(d *SemanticDefault) { d.Candidate.Enabled = false },
		func(d *SemanticDefault) { d.Invariant = true },
		func(d *SemanticDefault) { d.AdapterRequirements = []string{"press_edge", "foreground"} },
		func(d *SemanticDefault) {
			d.Candidate.Condition = commandbindings.Facts{commandbindings.AppFocused: true}
		},
	}
	for i, change := range changes {
		_, input = semanticFixture(t, nil)
		change(&input)
		got, err := BuildSemanticDefault(r, input)
		if err != nil || got.Fingerprint == base.Fingerprint {
			t.Fatalf("dimensão %d: %v", i, err)
		}
	}
	r, input = semanticFixture(t, func(d *commandcatalog.Definition) { d.Risk = commandcatalog.RiskHigh })
	got, err := BuildSemanticDefault(r, input)
	if err != nil || got.Fingerprint == base.Fingerprint {
		t.Fatal("risco não altera fingerprint", err)
	}
}

func TestSemanticDefaultRejectsInvalidContracts(t *testing.T) {
	r, input := semanticFixture(t, nil)
	input.Arguments = []byte(`{"extra":1}`)
	if _, err := BuildSemanticDefault(r, input); err == nil {
		t.Fatal("args fora do schema aceitos")
	}
	if _, err := BuildSemanticDefault(nil, input); err == nil {
		t.Fatal("catálogo ausente aceito")
	}
	_, input = semanticFixture(t, nil)
	input.TriggerSpec = []byte(`{"version":2}`)
	if _, err := BuildSemanticDefault(r, input); err == nil {
		t.Fatal("trigger futuro aceito")
	}
}
