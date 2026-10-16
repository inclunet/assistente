package app

import (
	"context"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
)

func TestCommandKeyboardActivationConditionsReachSurfaceProjection(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	registry := a.commandProduct.Load().registry
	const trigger = "keyboard.local:Control+Shift+KeyA"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyA", Modifiers: []string{"Control", "Shift"}}
	candidate := commandbindings.Candidate{ID: "layer-binding", LayerRef: "personal-layer", Trigger: trigger,
		CommandID: "workspace.tab.next", ArgumentsKey: "{}", ExecutionScopeKey: "global",
		Scope: commandbindings.ExplicitLayer, Enabled: true, LayerActive: true,
		LayerConditions: []commandbindings.Facts{{commandbindings.SurfaceType: "chat", commandbindings.AppFocused: true}},
	}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	fields := localKeyboardVariableFacts(configuration.RequiredFacts(trigger))
	if !reflect.DeepEqual(fields, []commandbindings.Field{commandbindings.SurfaceType}) {
		t.Fatalf("layer conditions not projected: %v", fields)
	}
	entry, ok := contextualKeyboardBinding(context.Background(), configuration, registry, trigger, shortcut)
	if !ok || entry.BySurface["chat"] == nil || entry.BySurface["chat"].CommandID != candidate.CommandID || entry.Fallback != nil {
		t.Fatalf("contextual layer escaped chat or disappeared: %+v %t", entry, ok)
	}
	candidate.LayerConditions = []commandbindings.Facts{{commandbindings.SurfaceType: "chat", commandbindings.AppFocused: false}}
	configuration, err = commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok = contextualKeyboardBinding(context.Background(), configuration, registry, trigger, shortcut)
	if !ok || entry.BySurface["chat"] != nil || entry.Fallback != nil {
		t.Fatalf("unfocused layer became active in DOM: %+v %t", entry, ok)
	}
}
