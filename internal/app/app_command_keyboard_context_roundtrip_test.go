package app

import (
	"encoding/json"
	"testing"
)

func TestCommandKeyboardContextualSequenceCloneRoundTrip(t *testing.T) {
	shortcut := LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{
		{Code: "KeyY", Modifiers: []string{"Control"}}, {Code: "KeyL", Modifiers: []string{}},
	}}
	binding := LocalCommandKeyboardBinding{Shortcut: shortcut, CommandID: commandProductWorkspaceListID, Handler: "backend"}
	original := LocalCommandKeyboardMap{Bindings: []LocalCommandKeyboardBinding{binding}, ContextualBindings: []LocalCommandKeyboardContextualBinding{{
		Shortcut: shortcut, BySurface: map[string]*LocalCommandKeyboardBinding{"chat": &binding}, Fallback: &binding,
	}}}
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
	clone.ContextualBindings[0].Fallback.Shortcut.Steps[1].Code = "KeyP"
	if original.ContextualBindings[0].Shortcut.Steps[0].Modifiers[0] != "Control" ||
		original.ContextualBindings[0].BySurface["chat"].Shortcut.Steps[0].Code != "KeyY" ||
		original.ContextualBindings[0].Fallback.Shortcut.Steps[1].Code != "KeyL" {
		t.Fatal("clone compartilha passos com mapa original")
	}
}
