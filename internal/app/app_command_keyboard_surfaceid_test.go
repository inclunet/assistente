package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
)

func TestContextualKeyboardPublishesSurfaceIDPairsAndBarriers(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	registry := a.commandProduct.Load().registry
	const trigger = "keyboard.local:Alt+KeyI"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyI", Modifiers: []string{"Alt"}}
	configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{{Candidate: commandbindings.Candidate{
		ID: "base", Trigger: trigger, CommandID: "navigation.data.import.open", ArgumentsKey: "{}",
		ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true,
	}, Version: "1", Fingerprint: "base-fp"}}, []commandbindings.Delta{{
		ID: "reviewed-tab", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "base-fp",
		Trigger: trigger, Effect: commandbindings.Suppress,
		Condition: commandbindings.Facts{commandbindings.SurfaceType: "editor", commandbindings.SurfaceID: "editor-2"},
		Enabled:   true, LayerActive: true, ReviewStatus: commandbindings.NeedsReview,
	}}, []commandbindings.Candidate{{
		ID: "chat-default", Trigger: trigger, CommandID: "navigation.data.import.open", ArgumentsKey: "{}",
		ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true,
		Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat"},
	}, {
		ID: "editor-tab", Trigger: trigger, CommandID: "editor.menu.insert.open", ArgumentsKey: "{}",
		ExecutionScopeKey: "global", Scope: commandbindings.Surface, Enabled: true, LayerActive: true,
		Condition: commandbindings.Facts{commandbindings.SurfaceType: "editor", commandbindings.SurfaceID: "editor-1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := contextualKeyboardBinding(context.Background(), configuration, registry, trigger, shortcut)
	if !ok || entry.BySurfaceID["editor"]["editor-1"] == nil || entry.BySurfaceID["editor"]["editor-1"].CommandID != "editor.menu.insert.open" {
		t.Fatalf("par específico não publicado: ok=%v binding=%+v entry=%+v", ok, entry.BySurfaceID["editor"]["editor-1"], entry)
	}
	if binding, exists := entry.BySurfaceID["editor"]["editor-2"]; !exists || binding != nil {
		t.Fatal("par literal não resolvido deveria ser barreira explícita")
	}
	if entry.BySurface["chat"] == nil || entry.BySurface["chat"].CommandID != "navigation.data.import.open" {
		t.Fatalf("fallback por tipo foi perdido: %+v", entry)
	}
	clone := cloneContextualKeyboardBindings([]LocalCommandKeyboardContextualBinding{entry})[0]
	clone.BySurfaceID["editor"]["editor-1"].CommandID = "alterado"
	if entry.BySurfaceID["editor"]["editor-1"].CommandID == "alterado" {
		t.Fatal("clone de bySurfaceId compartilha binding")
	}
}
