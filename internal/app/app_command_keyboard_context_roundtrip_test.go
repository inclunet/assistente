package app

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCommandKeyboardPageArgumentCloneRoundTrip(t *testing.T) {
	arguments := map[string]any{"workspace_id": "workspace-a", "target_mode": "position", "position": json.Number("17")}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyG", Modifiers: []string{"Control", "Alt"}}
	binding := &LocalCommandKeyboardBinding{Shortcut: shortcut, CommandID: commandWorkspaceTabGoToID, Handler: "local_ui", Arguments: arguments}
	original := LocalCommandKeyboardMap{
		ContextualBindings: []LocalCommandKeyboardContextualBinding{{
			Shortcut: shortcut,
			ByPage: map[string]*LocalCommandKeyboardContextualBinding{"workspace": {
				Shortcut: shortcut,
				ByProfile: map[string]*LocalCommandKeyboardContextualBinding{"focused": {
					Shortcut: shortcut,
					BySurface: map[string]*LocalCommandKeyboardBinding{"chat": binding},
				}},
			}},
		}},
		LocalPaletteConditions: []LocalCommandPaletteCondition{{CommandID: commandWorkspaceTabGoToID,
			ByPage: map[string]LocalCommandPaletteCondition{"workspace": {CommandID: commandWorkspaceTabGoToID,
				ByProfile: map[string]LocalCommandPaletteCondition{"focused": {CommandID: commandWorkspaceTabGoToID,
					BySurface: map[string]bool{"chat": true}, BySurfaceArguments: map[string]map[string]any{"chat": arguments},
				}},
			}},
		}},
	}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	cloned := cloneLocalCommandKeyboardMap(original)
	clonedRaw, err := json.Marshal(cloned)
	if err != nil {
		t.Fatalf("clone não serializa argumentos por página/perfil: %v", err)
	}
	var decoded LocalCommandKeyboardMap
	if err := json.Unmarshal(clonedRaw, &decoded); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"workspace_id": "workspace-a", "target_mode": "position", "position": float64(17)}
	if !reflect.DeepEqual(decoded.ContextualBindings[0].ByPage["workspace"].ByProfile["focused"].BySurface["chat"].Arguments, want) ||
		!reflect.DeepEqual(decoded.LocalPaletteConditions[0].ByPage["workspace"].ByProfile["focused"].BySurfaceArguments["chat"], want) {
		t.Fatal("round-trip perdeu o destino aninhado por página/perfil")
	}
	cloned.ContextualBindings[0].ByPage["workspace"].ByProfile["focused"].BySurface["chat"].Arguments["position"] = 99
	cloned.LocalPaletteConditions[0].ByPage["workspace"].ByProfile["focused"].BySurfaceArguments["chat"]["workspace_id"] = "other"
	after, err := json.Marshal(original)
	if err != nil || string(after) != string(before) {
		t.Fatal("clone por página/perfil compartilha argumentos com o mapa original")
	}
}

func TestCommandKeyboardContextualSequenceCloneRoundTrip(t *testing.T) {
	shortcut := LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{
		{Code: "KeyY", Modifiers: []string{"Control"}}, {Code: "KeyL", Modifiers: []string{}},
	}}
	binding := LocalCommandKeyboardBinding{Shortcut: shortcut, CommandID: commandWorkspaceTabGoToID, Handler: "local_ui", Arguments: map[string]any{"workspace_id": "workspace-a", "target_mode": "position", "position": json.Number("17"), "nested": map[string]any{"values": []any{"one", map[string]any{"flag": true}}}}}
	original := LocalCommandKeyboardMap{Bindings: []LocalCommandKeyboardBinding{binding}, LocalPaletteArguments: map[string]map[string]any{commandWorkspaceTabGoToID: {"workspace_id": "workspace-a", "target_mode": "specific", "tab_id": "tab-a"}}, ContextualBindings: []LocalCommandKeyboardContextualBinding{{
		Shortcut: shortcut, BySurface: map[string]*LocalCommandKeyboardBinding{"chat": &binding}, Fallback: &binding,
	}}, LocalPaletteConditions: []LocalCommandPaletteCondition{{CommandID: commandWorkspaceTabGoToID, FallbackArguments: map[string]any{"nested": map[string]any{"items": []any{map[string]any{"id": "a"}}}}, BySurfaceArguments: map[string]map[string]any{"chat": {"tab_id": "tab-chat"}}, BySurfaceIDArguments: map[string]map[string]map[string]any{"chat": {"surface-1": {"tab_id": "tab-id"}}}, ByProfile: map[string]LocalCommandPaletteCondition{"profile-a": {CommandID: commandWorkspaceTabGoToID, FallbackArguments: map[string]any{"tab_id": "profile-tab"}}}}}}
	clone := cloneLocalCommandKeyboardMap(original)
	raw, err := json.Marshal(clone)
	if err != nil {
		t.Fatalf("mapa clonado não serializa sequência: %v", err)
	}
	var decoded LocalCommandKeyboardMap
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Bindings[0].Shortcut.Version != 2 || decoded.ContextualBindings[0].BySurface["chat"].Shortcut.Version != 2 {
		t.Fatal("sequência alterada no round-trip")
	}
	clone.ContextualBindings[0].Shortcut.Steps[0].Modifiers[0] = "Alt"
	clone.ContextualBindings[0].BySurface["chat"].Shortcut.Steps[0].Code = "KeyZ"
	clone.ContextualBindings[0].BySurface["chat"].Arguments["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["flag"] = false
	clone.ContextualBindings[0].Fallback.Shortcut.Steps[1].Code = "KeyP"
	clone.ContextualBindings[0].Fallback.Arguments["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["flag"] = false
	clone.Bindings[0].Arguments["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["flag"] = false
	clone.LocalPaletteArguments[commandWorkspaceTabGoToID]["tab_id"] = "mutated"
	clone.LocalPaletteConditions[0].FallbackArguments["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["id"] = "changed"
	clone.LocalPaletteConditions[0].BySurfaceIDArguments["chat"]["surface-1"]["tab_id"] = "changed"
	clone.LocalPaletteConditions[0].ByProfile["profile-a"].FallbackArguments["tab_id"] = "changed"
	if original.ContextualBindings[0].Shortcut.Steps[0].Modifiers[0] != "Control" ||
		original.ContextualBindings[0].BySurface["chat"].Shortcut.Steps[0].Code != "KeyY" ||
		original.ContextualBindings[0].BySurface["chat"].Arguments["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["flag"] != true ||
		original.ContextualBindings[0].Fallback.Shortcut.Steps[1].Code != "KeyL" ||
		original.ContextualBindings[0].Fallback.Arguments["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["flag"] != true ||
		original.Bindings[0].Arguments["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["flag"] != true ||
		original.LocalPaletteArguments[commandWorkspaceTabGoToID]["tab_id"] != "tab-a" ||
		original.LocalPaletteConditions[0].FallbackArguments["nested"].(map[string]any)["items"].([]any)[0].(map[string]any)["id"] != "a" ||
		original.LocalPaletteConditions[0].BySurfaceIDArguments["chat"]["surface-1"]["tab_id"] != "tab-id" ||
		original.LocalPaletteConditions[0].ByProfile["profile-a"].FallbackArguments["tab_id"] != "profile-tab" {
		t.Fatal("clone compartilha passos com mapa original")
	}
}
