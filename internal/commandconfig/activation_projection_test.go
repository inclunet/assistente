package commandconfig

import (
	"context"
	"reflect"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
)

const activationProjectionSurfaceEditor = `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"editor"}]}`

func TestUnconditionalLayerPathDoesNotRequireFactsFromOtherRules(t *testing.T) {
	for _, unconditionalFirst := range []bool{false, true} {
		var conditions []commandbindings.Facts
		if unconditionalFirst {
			appendUniqueLayerCondition(&conditions, commandbindings.Facts{})
		}
		appendUniqueLayerCondition(&conditions, commandbindings.Facts{commandbindings.Profile: "profile"})
		appendUniqueLayerCondition(&conditions, commandbindings.Facts{})
		appendUniqueLayerCondition(&conditions, commandbindings.Facts{commandbindings.SurfaceType: "chat"})
		if len(conditions) != 1 || len(conditions[0]) != 0 {
			t.Fatalf("unconditional union still requires restricted facts: %+v", conditions)
		}
	}
}

func activationProjectionRule(t *testing.T, user, layer, id string, mode commandactivation.Mode, condition string) commandactivation.Rule {
	t.Helper()
	return commandactivation.Rule{
		ID: id, UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layer,
		RuleRefKind: commandactivation.UserRef, RuleRef: id, Mode: mode, Condition: condition,
		Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReviewStatus: "active",
	}
}

func TestProjectCompleteActivationRulesFormORWithoutChangingBindingIdentity(t *testing.T) {
	user := completeProjectionUUID(t)
	layerID, bindingID := completeProjectionUUID(t), completeProjectionUUID(t)
	layer := Layer{ID: layerID, UserID: user, Name: "Contextual", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 3}
	binding := completeProjectionBinding(t, user, nil, layer, bindingID)
	profileRule := activationProjectionRule(t, user, layerID, completeProjectionUUID(t), commandactivation.ModeCondition,
		`{"version":1,"clauses":[{"field":"profile","op":"eq","value":"developer"}]}`)
	surfaceRule := activationProjectionRule(t, user, layerID, completeProjectionUUID(t), commandactivation.ModeContext, activationProjectionSurfaceEditor)
	manualRule := activationProjectionRule(t, user, layerID, completeProjectionUUID(t), commandactivation.ModeManual, `{}`)
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{binding}, ActivationRules: []commandactivation.Rule{profileRule, surfaceRule, manualRule}}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.BuiltinLayers = nil

	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, facts := range []commandbindings.Facts{{commandbindings.Profile: "developer"}, {commandbindings.SurfaceType: "editor"}} {
		result, err := configuration.Resolve("keyboard.local:KeyA", facts, nil)
		if err != nil || result.Status != commandbindings.Selected || !reflect.DeepEqual(result.BindingIDs, []string{bindingID}) {
			t.Fatalf("regra OR não selecionou o mesmo binding: facts=%v result=%+v err=%v", facts, result, err)
		}
	}
	result, err := configuration.Resolve("keyboard.local:KeyA", commandbindings.Facts{commandbindings.SurfaceType: "chat"}, nil)
	if err != nil || result.Status != commandbindings.NoMatch {
		t.Fatalf("camada contextual vazou fora das regras: result=%+v err=%v", result, err)
	}
	if got := configuration.RequiredFacts("keyboard.local:KeyA"); !reflect.DeepEqual(got, []commandbindings.Field{commandbindings.Profile, commandbindings.SurfaceType}) {
		t.Fatalf("RequiredFacts não publicou gates da camada: %v", got)
	}
	state, ok := configuration.LayerPresentationTarget(manualRule.ID)
	if !ok || state.Scope != "global" || state.LayerID != layerID || !state.Contextual || state.AlwaysActive {
		t.Fatalf("metadados de estado divergiram dos gates projetados: %+v %v", state, ok)
	}
}

func TestProjectCompleteLayerPresentationAlwaysMatchesEffectiveProjection(t *testing.T) {
	user := completeProjectionUUID(t)
	layerID, bindingID := completeProjectionUUID(t), completeProjectionUUID(t)
	layer := Layer{ID: layerID, UserID: user, Name: "Always", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 3}
	binding := completeProjectionBinding(t, user, nil, layer, bindingID)
	manual := activationProjectionRule(t, user, layerID, completeProjectionUUID(t), commandactivation.ModeManual, `{}`)
	always := activationProjectionRule(t, user, layerID, completeProjectionUUID(t), commandactivation.ModeAlways, `{"legacy_condition_payload":true}`)
	// These valid persisted attributes do not gate ModeAlways in
	// projectLayerActivation; presentation metadata must mirror that projection.
	always.Lifecycle = commandactivation.LifecycleSession
	always.Source = "legacy-import"
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{binding}, ActivationRules: []commandactivation.Rule{manual, always}}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.BuiltinLayers = nil
	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := configuration.LayerPresentationTarget(manual.ID)
	if !ok || !state.AlwaysActive || state.Contextual {
		t.Fatalf("ModeAlways efetivo ausente dos metadados: %+v %v", state, ok)
	}
	// ProjectComplete detached its metadata from the loaded config snapshot;
	// changing that source without publishing a new projection cannot mutate it.
	snapshot.Layers[0].Enabled = false
	snapshot.ActivationRules[1].Mode = commandactivation.ModeContext
	snapshot.ActivationRules[1].Condition = activationProjectionSurfaceEditor
	if afterSourceEdit, stillPresent := configuration.LayerPresentationTarget(manual.ID); !stillPresent || afterSourceEdit != state {
		t.Fatalf("source edit changed published metadata without reprojection: before=%+v after=%+v present=%v", state, afterSourceEdit, stillPresent)
	}
	resolved, err := configuration.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || resolved.Status != commandbindings.Selected || !reflect.DeepEqual(resolved.BindingIDs, []string{bindingID}) {
		t.Fatalf("resolução sem facts diverge do estado Always: %+v %v", resolved, err)
	}
}

func TestProjectCompleteLayerPresentationMirrorsContextWithEmptyCanonicalClauses(t *testing.T) {
	user := completeProjectionUUID(t)
	layerID, bindingID := completeProjectionUUID(t), completeProjectionUUID(t)
	layer := Layer{ID: layerID, UserID: user, Name: "Empty context", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 3}
	binding := completeProjectionBinding(t, user, nil, layer, bindingID)
	manual := activationProjectionRule(t, user, layerID, completeProjectionUUID(t), commandactivation.ModeManual, `{}`)
	contextRule := activationProjectionRule(t, user, layerID, completeProjectionUUID(t), commandactivation.ModeContext, `{"version":1,"clauses":[]}`)
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{binding}, ActivationRules: []commandactivation.Rule{manual, contextRule}}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.BuiltinLayers = nil
	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := configuration.LayerPresentationTarget(manual.ID)
	if !ok || !state.AlwaysActive || state.Contextual {
		t.Fatalf("condição Context vazia divergiu do activation projection: %+v %v", state, ok)
	}
	resolved, err := configuration.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || resolved.Status != commandbindings.Selected || !reflect.DeepEqual(resolved.BindingIDs, []string{bindingID}) {
		t.Fatalf("resolução não trata clauses vazias como caminho incondicional: %+v %v", resolved, err)
	}
}

func TestProjectLocalReadIncluiGateSurfaceTypeNoMapaLocal(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	layer := projectionTestLayer(t, fixture, true)
	binding := projectionTestBinding(t, fixture, layer)
	rule := activationProjectionRule(t, fixture.scope.UserID, layer.ID, storeTestUUID7(t), commandactivation.ModeContext, activationProjectionSurfaceEditor)
	fixture.snapshot.Layers = []Layer{layer}
	fixture.snapshot.Bindings = []Binding{binding}
	fixture.snapshot.ActivationRules = []commandactivation.Rule{rule}

	configuration, err := ProjectLocalRead(context.Background(), fixture.snapshot, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := configuration.Resolve(projectionTrigger, commandbindings.Facts{commandbindings.SurfaceType: "editor"}, nil)
	if err != nil || selected.Status != commandbindings.Selected || selected.BindingIDs[0] != binding.ID {
		t.Fatalf("gate surface.type não selecionou binding local: result=%+v err=%v", selected, err)
	}
	fallback, err := configuration.Resolve(projectionTrigger, commandbindings.Facts{commandbindings.SurfaceType: "chat"}, nil)
	if err != nil || fallback.Status != commandbindings.Selected || fallback.BindingIDs[0] != "builtin.key-k" {
		t.Fatalf("fallback builtin não foi preservado fora do gate: result=%+v err=%v", fallback, err)
	}
	if got := configuration.SurfaceValues(projectionTrigger); !reflect.DeepEqual(got, []string{"editor"}) {
		t.Fatalf("SurfaceValues não publicou regra da camada: %v", got)
	}
}

func TestProjectCompleteManualClaimAndExpiredClaim(t *testing.T) {
	user := completeProjectionUUID(t)
	layerID, bindingID, ruleID := completeProjectionUUID(t), completeProjectionUUID(t), completeProjectionUUID(t)
	layer := Layer{ID: layerID, UserID: user, Name: "Manual", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 3}
	binding := completeProjectionBinding(t, user, nil, layer, bindingID)
	rule := activationProjectionRule(t, user, layerID, ruleID, commandactivation.ModeManual, `{}`)
	now := completeProjectionTime()
	stack := "stack"
	claim := commandactivation.Claim{
		ActivationID: completeProjectionUUID(t), LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, UserID: user,
		AuthContextType: "session", AuthContextID: "ctx", AuthGeneration: "auth-1", SecurityGeneration: "sec-1",
		SourceType: "manual", ManualStackKey: &stack, State: commandactivation.StateInactive,
		ActivatedAt: now, UpdatedAt: now,
	}
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{binding}, ActivationRules: []commandactivation.Rule{rule}, ActivationClaims: []commandactivation.Claim{claim}}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.BuiltinLayers = nil
	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := configuration.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || result.Status != commandbindings.NoMatch {
		t.Fatalf("claim bruta não deveria autorizar a camada: result=%+v err=%v", result, err)
	}
	options.ActiveUserLayerIDs = []string{layerID}
	configuration, err = ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	result, err = configuration.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || result.Status != commandbindings.Selected || result.BindingIDs[0] != bindingID {
		t.Fatalf("assertedActive não ativou a camada: result=%+v err=%v", result, err)
	}
}

func completeProjectionTime() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }
