package commandbindings

import "testing"

func TestResolveContextualSuppressionShadowsLowerGlobalAndTies(t *testing.T) {
	const trigger = "keyboard.local:Alt+KeyI"
	defaults := []Default{
		{Candidate: Candidate{ID: "global", Trigger: trigger, CommandID: "navigation.data.import.open", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Application, Enabled: true, LayerActive: true}, Version: "1", Fingerprint: "global-fp"},
		{Candidate: Candidate{ID: "editor", Trigger: trigger, CommandID: "editor.menu.insert.open", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Surface, Condition: Facts{SurfaceType: "editor"}, Enabled: true, LayerActive: true}, Version: "1", Fingerprint: "editor-fp"},
	}
	deltas := []Delta{{
		ID: "editor-suppress", DefaultID: "editor", DefaultVersion: "1", DefaultFingerprint: "editor-fp", Trigger: trigger,
		Effect: Suppress, Condition: Facts{}, Enabled: true, LayerActive: true, ReviewStatus: Active,
		LayerPriority: 1, BindingPriority: 1, LayerRef: "surface",
	}}
	configuration, err := NewConfiguration(defaults, deltas, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := configuration.Resolve(trigger, Facts{SurfaceType: "editor"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != Suppressed {
		t.Fatalf("editor result = %+v, want suppressed", result)
	}

	// Um candidato no mesmo escopo e com a mesma prioridade não pode fazer o
	// tombstone perder por um desempate artificial: o acionador permanece
	// bloqueado até haver uma precedência estritamente maior.
	configuration, err = NewConfiguration(defaults, deltas, []Candidate{{
		ID: "same-surface", Trigger: trigger, CommandID: "editor.other.open", ArgumentsKey: "{}", ExecutionScopeKey: "global",
		Scope: Surface, Condition: Facts{SurfaceType: "editor"}, Enabled: true, LayerActive: true, LayerPriority: 1, BindingPriority: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err = configuration.Resolve(trigger, Facts{SurfaceType: "editor"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != Suppressed {
		t.Fatalf("tie result = %+v, want suppressed", result)
	}
}
