package commandbindings

import "testing"

func TestLayerPresentationTargetsSurviveConfigurationClones(t *testing.T) {
	config, err := NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]LayerPresentationState{
		"rule": {LayerID: "layer", Scope: "workspace", LayerEnabled: true, AlwaysActive: true},
	}
	projected := config.WithLayerPresentationTargets(targets)
	targets["rule"] = LayerPresentationState{LayerID: "mutated"}
	if state, ok := projected.LayerPresentationTarget("rule"); !ok || state.LayerID != "layer" || state.Scope != "workspace" {
		t.Fatalf("input map aliased projected metadata: %+v %v", state, ok)
	}

	restored, err := projected.WithoutDeltas(nil)
	if err != nil {
		t.Fatal(err)
	}
	if state, ok := restored.LayerPresentationTarget("rule"); !ok || state.LayerID != "layer" || !state.AlwaysActive {
		t.Fatalf("WithoutDeltas dropped layer presentation state: %+v %v", state, ok)
	}
	if state, ok := projected.WithPresentation(NewPresentationSnapshot(nil)).WithValidityDeadline(projected.ValidUntil()).LayerPresentationTarget("rule"); !ok || state.LayerID != "layer" {
		t.Fatalf("shallow configuration clone dropped layer presentation state: %+v %v", state, ok)
	}
}
