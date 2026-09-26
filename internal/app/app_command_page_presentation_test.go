package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

func TestCommandPagePresentationSequenceFallbackRequiresNoMatch(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	registry := a.commandProduct.Load().registry
	const trigger = "keyboard.local:Control+KeyN"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyN", Modifiers: []string{"Control"}}
	page := commandbindings.Candidate{ID: "page", Trigger: trigger, CommandID: "profiles.create.open",
		ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Surface,
		Condition: commandbindings.Facts{commandbindings.SurfaceType: "profiles"}, Enabled: true, LayerActive: true}
	base := commandbindings.Default{Candidate: commandbindings.Candidate{ID: "global", Trigger: trigger,
		CommandID: "navigation.help.open", ArgumentsKey: "{}", ExecutionScopeKey: "global",
		Scope: commandbindings.Application, Enabled: true, LayerActive: true}, Version: "1", Fingerprint: "fp"}
	for _, reason := range []string{"no-match", "suppressed", "review", "conflict", "durable"} {
		t.Run(reason, func(t *testing.T) {
			custom := []commandbindings.Candidate{page}
			var defaults []commandbindings.Default
			var deltas []commandbindings.Delta
			if reason == "suppressed" || reason == "review" {
				defaults = append(defaults, base)
				status := commandbindings.Active
				if reason == "review" {
					status = commandbindings.NeedsReview
				}
				deltas = append(deltas, commandbindings.Delta{ID: "suppress", DefaultID: base.Candidate.ID,
					DefaultVersion: base.Version, DefaultFingerprint: base.Fingerprint, Trigger: trigger,
					Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: status})
			}
			if reason == "conflict" || reason == "durable" {
				candidate := base.Candidate
				candidate.CommandID = "workspace.tab.chat.create"
				custom = append(custom, candidate)
				if reason == "conflict" {
					candidate.ID, candidate.CommandID = "other", "navigation.help.open"
					custom = append(custom, candidate)
				}
			}
			configuration, err := commandbindings.NewConfiguration(defaults, deltas, custom)
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := contextualKeyboardBinding(context.Background(), configuration, registry, trigger, shortcut)
			if !ok || entry.FallbackToSequences != (reason == "no-match") {
				t.Fatalf("unexpected sequence fallback for %s: %+v", reason, entry)
			}
		})
	}
}

func TestCommandPagePresentationClosedLocalCatalog(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.registry.List()) != 150 || len(view.LocalPaletteCommands) != 61 {
		t.Fatal("catalog counts")
	}
	for _, item := range commandProductPagePresentation {
		d, ok := p.registry.Lookup(item.id)
		if !ok || !isLocalUICommand(item.id) || !localKeyboardCommandAllowed(item.id) || commandDeckLedgerCommand(d) || d.Persistence.Audit != commandcatalog.PersistenceNever || d.HasMutableTarget {
			t.Fatalf("unsafe: %+v", d)
		}
		for _, source := range []commandcatalog.Source{commandcatalog.UI, commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
			if !d.AllowsSource(source) {
				t.Fatalf("missing %s: %s", source, item.id)
			}
		}
		for _, source := range []commandcatalog.Source{commandcatalog.KeyboardGlobal, commandcatalog.CLI, commandcatalog.System, commandcatalog.Chat} {
			if d.AllowsSource(source) {
				t.Fatalf("expanded source %s", source)
			}
		}
		for _, locale := range []string{"pt-BR", "en", "es"} {
			if d.Presentation.Locales[locale].Name == "" {
				t.Fatal(locale)
			}
		}
		if !containsString(view.LocalPaletteCommands, item.id) {
			t.Fatal(item.id)
		}
		if _, err := a.BeginUICommand(item.id); err == nil {
			t.Fatal("local command entered durable handoff")
		}
	}
	for _, id := range []string{"terminal.interrupt", "terminal.session.close", "profiles.delete", "tasklists.delete"} {
		if isLocalUICommand(id) {
			t.Fatal("domain effect classified local:", id)
		}
	}
}

func TestCommandPagePresentationCtrlNProjection(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range view.ContextualBindings {
		if entry.Shortcut.Code != "KeyN" {
			continue
		}
		found = true
		if entry.BySurface["history"] == nil || entry.BySurface["history"].CommandID != "navigation.workspace.open" {
			t.Fatalf("history must navigate, never create a chat: %+v", entry)
		}
		if entry.BySurface["tasklists"] == nil || entry.BySurface["tasklists"].CommandID != "tasklists.create.open" || entry.BySurface["profiles"] == nil || entry.BySurface["profiles"].CommandID != "profiles.create.open" || entry.Fallback != nil || !entry.FallbackToSequences {
			t.Fatalf("%+v", entry)
		}
		cloned := cloneContextualKeyboardBindings([]LocalCommandKeyboardContextualBinding{entry})
		if !cloned[0].FallbackToSequences {
			t.Fatal("lost prefix fallback on clone")
		}
	}
	if !found {
		t.Fatal("missing Ctrl+N contextual projection")
	}
	sequences := 0
	for _, item := range view.Bindings {
		if item.Shortcut.Version == 2 && item.Shortcut.Steps[0].Code == "KeyN" {
			sequences++
		}
	}
	if sequences != 4 {
		t.Fatalf("workspace sequences=%d", sequences)
	}
}
