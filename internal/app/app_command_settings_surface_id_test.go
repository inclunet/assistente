package app

import (
	"testing"

	"assistente/internal/commandbindings"
)

func TestCommandSettingsSpecificTabRulePublishesWithoutUnsupportedDiagnostic(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Aba específica", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "workspace.tab.next", TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyY","modifiers":["Control","Shift"]}`, Effect: "execute", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "chat"}, {Field: "surface.id", Value: "specific-chat"}}}}})
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.ResourceID == rule.ID && diagnostic.Severity == "error" {
			t.Fatalf("operational rule diagnosed unavailable: %+v", diagnostic)
		}
	}
	keyboard, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range keyboard.ContextualBindings {
		if entry.Shortcut.Code != "KeyY" {
			continue
		}
		found = true
		if entry.BySurfaceID["chat"]["specific-chat"] == nil || entry.BySurfaceID["chat"]["specific-chat"].CommandID != "workspace.tab.next" || entry.BySurface["chat"] != nil || entry.Fallback != nil {
			t.Fatalf("specific tab not isolated: %+v", entry)
		}
	}
	if !found {
		t.Fatal("saved rule missing from keyboard projection")
	}
	// A second independent contextual rule composes by union, never replacing
	// the first claim or turning its conjunction into a global wildcard.
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "editor"}}}}})
	p := a.commandProduct.Load()
	configuration, _, _, err := p.host.ResolutionSnapshot(a.ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		surface, id string
		selected    bool
	}{{"chat", "specific-chat", true}, {"chat", "other-chat", false}, {"editor", "other-editor", true}, {"terminal", "specific-chat", false}} {
		resolved, err := configuration.Resolve("keyboard.local:Control+Shift+KeyY", commandbindings.Facts{commandbindings.SurfaceType: tc.surface, commandbindings.SurfaceID: tc.id, commandbindings.AppFocused: true}, nil)
		if err != nil || (resolved.Status == commandbindings.Selected) != tc.selected {
			t.Fatalf("%+v: %+v %v", tc, resolved, err)
		}
	}
}
