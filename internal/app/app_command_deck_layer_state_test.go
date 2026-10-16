package app

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
)

func TestCommandDeckPersistentStateTracksPublishedLayerAcrossOrigins(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	_, ruleID := settingsActivationSecurityLayerAndRule(t, a, decisions)
	p := a.commandProduct.Load()
	state := func(commandID string) string {
		t.Helper()
		_, activeIDs, versions, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
		if err != nil {
			t.Fatal(err)
		}
		arguments, err := json.Marshal(commandLayerActionArguments{Scope: "global", RuleID: ruleID})
		if err != nil {
			t.Fatal(err)
		}
		return p.commandDeckPersistentState(context.Background(), commandbindings.Result{CommandID: commandID, ArgumentsKey: string(arguments)}, activeIDs, versions)
	}
	if got := state(commandLayerActivateID); got != "off" {
		t.Fatalf("initial state=%q, want off", got)
	}
	if result, err := a.ApplyCommandLayerAction("global", ruleID, "pin", 0); err != nil || !result.Published {
		t.Fatalf("palette activation result=%+v err=%v", result, err)
	}
	if got := state(commandLayerActivateID); got != "on" {
		t.Fatalf("after activate state=%q, want on", got)
	}

	// A claim from another trusted origin is reflected through the published
	// active layer set; presentation does not depend on a Deck invocation.
	external := commandactivation.Origin{Type: "streamdeck.key", SessionID: p.principal.SessionID, DeviceID: "fixture-deck"}
	result, err := a.applyCommandLayerActionWithOrigin(a.commandBridgeContext(), p.principal, "global", ruleID, "pin", 0, external, nil, nil)
	if err != nil || !result.Published {
		t.Fatalf("external activation result=%+v err=%v", result, err)
	}
	if result, err := a.ApplyCommandLayerAction("global", ruleID, "deactivate", 0); err != nil || !result.Published {
		t.Fatalf("palette deactivation result=%+v err=%v", result, err)
	}
	if got := state(commandLayerToggleID); got != "on" {
		t.Fatalf("layer should remain on for external origin, got %q", got)
	}
	result, err = a.applyCommandLayerActionWithOrigin(a.commandBridgeContext(), p.principal, "global", ruleID, "deactivate", 0, external, nil, nil)
	if err != nil || !result.Published {
		t.Fatalf("external deactivation result=%+v err=%v", result, err)
	}
	if got := state(commandLayerToggleID); got != "off" {
		t.Fatalf("after all origins deactivate state=%q, want off", got)
	}
	if result, err := a.ApplyCommandLayerAction("global", ruleID, "toggle", 0); err != nil || !result.Published {
		t.Fatalf("toggle on result=%+v err=%v", result, err)
	}
	if got := state(commandLayerToggleID); got != "on" {
		t.Fatalf("after toggle on state=%q, want on", got)
	}
	if result, err := a.ApplyCommandLayerAction("global", ruleID, "toggle", 0); err != nil || !result.Published {
		t.Fatalf("toggle off result=%+v err=%v", result, err)
	}
	if got := state(commandLayerToggleID); got != "off" {
		t.Fatalf("after toggle off state=%q, want off", got)
	}
}

func TestCommandDeckPersistentStateUnknownForUnprovenTargets(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	_, ruleID := settingsActivationSecurityLayerAndRule(t, a, decisions)
	p := a.commandProduct.Load()
	_, activeIDs, versions, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	makeResult := func(id string) commandbindings.Result {
		arguments, marshalErr := json.Marshal(commandLayerActionArguments{Scope: "global", RuleID: id})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return commandbindings.Result{CommandID: commandLayerActivateID, ArgumentsKey: string(arguments)}
	}
	if got := p.commandDeckPersistentState(context.Background(), makeResult("foreign-or-removed-rule"), activeIDs, versions); got != "" {
		t.Fatalf("foreign/removed rule state=%q, want unknown", got)
	}
	wrongScope := makeResult(ruleID)
	wrongScope.ArgumentsKey = `{"scope":"workspace","rule_id":"` + ruleID + `","duration_seconds":0}`
	if got := p.commandDeckPersistentState(context.Background(), wrongScope, activeIDs, versions); got != "" {
		t.Fatalf("scope mismatch state=%q, want unknown", got)
	}
	invalid := makeResult(ruleID)
	invalid.ArgumentsKey = `{"scope":"global","rule_id":"` + ruleID + `","forged":true}`
	if got := p.commandDeckPersistentState(context.Background(), invalid, activeIDs, versions); got != "" {
		t.Fatalf("invalid arguments state=%q, want unknown", got)
	}
	stale := versions
	stale.ActiveLayers = "stale-version"
	if got := p.commandDeckPersistentState(context.Background(), makeResult(ruleID), activeIDs, stale); got != "" {
		t.Fatalf("stale versions state=%q, want unknown", got)
	}
}

func TestCommandDeckEffectiveLayerStateAccountsForAlwaysAndContextualRules(t *testing.T) {
	base := commandbindings.LayerPresentationState{LayerID: "layer", LayerEnabled: true}
	for _, test := range []struct {
		name   string
		target commandbindings.LayerPresentationState
		active []string
		want   string
	}{
		{name: "manual inactive", target: base, want: "off"},
		{name: "manual active", target: base, active: []string{"layer"}, want: "on"},
		{name: "disabled layer", target: commandbindings.LayerPresentationState{LayerID: "layer", AlwaysActive: true}, want: "off"},
		{name: "always sibling", target: commandbindings.LayerPresentationState{LayerID: "layer", LayerEnabled: true, AlwaysActive: true}, want: "on"},
		{name: "context sibling unresolved", target: commandbindings.LayerPresentationState{LayerID: "layer", LayerEnabled: true, Contextual: true}, want: ""},
		{name: "manual claim proves contextual layer", target: commandbindings.LayerPresentationState{LayerID: "layer", LayerEnabled: true, Contextual: true}, active: []string{"layer"}, want: "on"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := commandDeckEffectiveLayerState(test.target, test.active); got != test.want {
				t.Fatalf("state=%q, want %q", got, test.want)
			}
		})
	}
}
