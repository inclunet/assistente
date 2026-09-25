package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
)

func TestCommandKeyboardSpecificTabNoMatchPermitsIndependentSequence(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	const trigger = "keyboard.local:Control+Shift+KeyZ"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{ID: "specific", Trigger: trigger, CommandID: "workspace.tab.next", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Surface, Enabled: true, LayerActive: true, Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-a"}}})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := contextualKeyboardBinding(context.Background(), configuration, a.commandProduct.Load().registry, trigger, shortcut)
	if !ok || !entry.SequenceFallbacks["chat"][""] || entry.SequenceFallbacks["chat"]["chat-a"] || entry.BySurface["chat"] != nil || entry.BySurfaceID["chat"]["chat-a"] == nil {
		t.Fatalf("NoMatch or selection lost: %+v", entry)
	}
	copy := cloneContextualKeyboardBindings([]LocalCommandKeyboardContextualBinding{entry})[0]
	copy.SequenceFallbacks["chat"][""] = false
	if !entry.SequenceFallbacks["chat"][""] {
		t.Fatal("sequence fallback aliases source map")
	}
}

func TestCommandKeyboardSpecificTabReviewNeverPermitsSequenceFallback(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	const trigger = "keyboard.local:Control+Shift+KeyZ"
	base := commandbindings.Default{Version: "1", Fingerprint: "fp", Candidate: commandbindings.Candidate{ID: "base", Trigger: trigger, CommandID: "workspace.tab.next", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true}}
	for _, status := range []commandbindings.ReviewStatus{commandbindings.Active, commandbindings.NeedsReview} {
		configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{{ID: "suppress", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: trigger, Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: status, Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-a"}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := contextualKeyboardBinding(context.Background(), configuration, a.commandProduct.Load().registry, trigger, LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}})
		if !ok || entry.BySurfaceID["chat"]["chat-a"] != nil || entry.SequenceFallbacks["chat"]["chat-a"] {
			t.Fatalf("%s bypassed via sequence: %+v", status, entry)
		}
	}
}
