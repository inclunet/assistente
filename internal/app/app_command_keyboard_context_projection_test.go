package app

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/commandbindings"
)

func TestCommandKeyboardContextProjectionAltIAndClone(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	first, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range first.Bindings {
		if binding.Shortcut.Code == "KeyI" && len(binding.Shortcut.Modifiers) == 1 && binding.Shortcut.Modifiers[0] == "Alt" {
			t.Fatal("Alt+I não pode permanecer no mapa incondicional")
		}
	}
	if len(first.ContextualBindings) != 2 {
		t.Fatalf("tabelas contextuais = %+v", first.ContextualBindings)
	}
	var entry LocalCommandKeyboardContextualBinding
	found := false
	for _, candidate := range first.ContextualBindings {
		if candidate.Shortcut.Code == "KeyI" && len(candidate.Shortcut.Modifiers) == 1 && candidate.Shortcut.Modifiers[0] == "Alt" {
			entry, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatalf("tabela contextual Alt+I ausente: %+v", first.ContextualBindings)
	}
	if entry.Shortcut.Code != "KeyI" || entry.BySurface["editor"] == nil ||
		entry.BySurface["editor"].CommandID != "editor.menu.insert.open" ||
		entry.Fallback == nil || entry.Fallback.CommandID != "navigation.data.import.open" {
		t.Fatalf("resolução contextual inesperada: %+v", entry)
	}
	entry.BySurface["editor"].CommandID = "alterado"
	entry.BySurface["editor"].Shortcut.Modifiers[0] = "Control"
	entry.Fallback.CommandID = "alterado"
	entry.Shortcut.Modifiers[0] = "Shift"
	second, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	var got LocalCommandKeyboardContextualBinding
	for _, candidate := range second.ContextualBindings {
		if candidate.Shortcut.Code == "KeyI" && len(candidate.Shortcut.Modifiers) == 1 && candidate.Shortcut.Modifiers[0] == "Alt" {
			got = candidate
			break
		}
	}
	if got.BySurface["editor"].CommandID != "editor.menu.insert.open" ||
		got.BySurface["editor"].Shortcut.Modifiers[0] != "Alt" ||
		got.Shortcut.Modifiers[0] != "Alt" || got.Fallback.CommandID != "navigation.data.import.open" {
		t.Fatal("mapa devolvido compartilha memória com o snapshot do host")
	}
}

func TestCommandKeyboardContextProjectionSerializesExplicitBarriers(t *testing.T) {
	entry := LocalCommandKeyboardContextualBinding{
		Shortcut:  LocalCommandShortcut{Version: 1, Code: "KeyI", Modifiers: []string{"Alt"}},
		BySurface: map[string]*LocalCommandKeyboardBinding{"editor": nil},
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if fallback, exists := decoded["fallback"]; !exists || fallback != nil {
		t.Fatalf("fallback deve ser null explícito: %s", raw)
	}
	bySurface := decoded["bySurface"].(map[string]any)
	if editor, exists := bySurface["editor"]; !exists || editor != nil {
		t.Fatalf("barreira do editor perdida: %s", raw)
	}
}

func TestCommandKeyboardContextProjectionPreservesUnavailableBranches(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	registry := a.commandProduct.Load().registry
	const trigger = "keyboard.local:Alt+KeyI"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyI", Modifiers: []string{"Alt"}}
	base := commandbindings.Default{Candidate: commandbindings.Candidate{
		ID: "import", Trigger: trigger, CommandID: "navigation.data.import.open",
		ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Application,
		Enabled: true, LayerActive: true,
	}, Version: "1", Fingerprint: "import-fp"}
	for _, reason := range []string{"suppressed", "review", "durable", "conflict"} {
		t.Run(reason, func(t *testing.T) {
			var deltas []commandbindings.Delta
			var custom []commandbindings.Candidate
			if reason == "suppressed" || reason == "review" {
				status := commandbindings.Active
				if reason == "review" {
					status = commandbindings.NeedsReview
				}
				deltas = []commandbindings.Delta{{ID: "delta", DefaultID: base.Candidate.ID,
					DefaultVersion: base.Version, DefaultFingerprint: base.Fingerprint,
					Trigger: trigger, Effect: commandbindings.Suppress,
					Condition: commandbindings.Facts{commandbindings.SurfaceType: "editor"},
					Enabled:   true, LayerActive: true, ReviewStatus: status}}
			} else {
				candidate := base.Candidate
				candidate.ID, candidate.Scope = "context", commandbindings.Surface
				candidate.Condition = commandbindings.Facts{commandbindings.SurfaceType: "editor"}
				candidate.CommandID = "workspace.tab.chat.create"
				custom = append(custom, candidate)
				if reason == "conflict" {
					candidate.ID, candidate.CommandID = "context2", "editor.menu.insert.open"
					custom = append(custom, candidate)
				}
			}
			configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, deltas, custom)
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := contextualKeyboardBinding(context.Background(), configuration, registry, trigger, shortcut)
			if !ok {
				t.Fatal("uma superfície indisponível descartou a tabela inteira")
			}
			if branch, exists := entry.BySurface["editor"]; reason == "durable" {
				if !exists || branch == nil || branch.CommandID != "workspace.tab.chat.create" || branch.Handler != "contextual" {
					t.Fatalf("ação durável contextual não foi publicada: %+v", entry)
				}
			} else if !exists || branch != nil {
				t.Fatalf("editor deveria ser barreira explícita: %+v", entry)
			}
			if entry.Fallback == nil || entry.Fallback.CommandID != base.Candidate.CommandID {
				t.Fatal("outras superfícies perderam a resolução válida")
			}
		})
	}
}

func TestCommandKeyboardAppPageProfileProjectionKeepsCanonicalSurfaces(t *testing.T) {
	_, _ = settingsSecurityFixture(t)
	registry := paletteConditionTestRegistry(t)
	trigger := "keyboard.local:Control+KeyK"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control"}}
	for _, test := range []struct {
		name, page, surface string
	}{
		{name: "route profile surface", page: "profiles", surface: "profiles"},
		{name: "route toolbar surface", page: "settings", surface: "toolbar"},
		{name: "workspace tab surface", page: "workspace", surface: "tasklist"},
	} {
		t.Run(test.name, func(t *testing.T) {
			configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
				ID: "page-profile", Trigger: trigger, CommandID: "help.shortcuts.show", ArgumentsKey: "{}",
				ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true,
				Condition: commandbindings.Facts{
					commandbindings.AppPage: test.page, commandbindings.Profile: "dev", commandbindings.SurfaceType: test.surface,
				},
			}})
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := contextualKeyboardBinding(context.Background(), configuration, registry, trigger, shortcut)
			if !ok {
				t.Fatal("contextual keyboard binding omitted")
			}
			page, ok := entry.ByPage[test.page]
			if !ok || page == nil {
				t.Fatalf("page branch missing: %+v", entry)
			}
			profile, ok := page.ByProfile["dev"]
			if !ok || profile == nil || profile.BySurface[test.surface] == nil || profile.BySurface[test.surface].CommandID != "help.shortcuts.show" {
				t.Fatalf("page/profile/canonical surface branch missing: %+v", entry)
			}
			for _, other := range []string{"tasklist", "tasklists", "profiles", "toolbar"} {
				if other != test.surface && profile.BySurface[other] != nil {
					t.Fatalf("binding leaked to noncanonical surface %q: %+v", other, profile)
				}
			}
		})
	}
}
