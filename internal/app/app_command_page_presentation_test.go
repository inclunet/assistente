package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/database"
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
	if len(p.registry.List()) != 151 || len(view.LocalPaletteCommands) != 62 {
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
		if item.id == "command_settings.create.open" {
			if d.Presentation.Version != "page-presentation-v1" || d.Presentation.Locales["pt-BR"].Name != "Novo item de configuração de comandos" {
				t.Fatalf("unexpected command settings presentation: %+v", d.Presentation)
			}
			var before, after int64
			if err := database.DB().Table("command_invocations").Count(&before).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := a.BeginUICommand(item.id); err == nil {
				t.Fatal("local command entered durable handoff")
			}
			if err := database.DB().Table("command_invocations").Count(&after).Error; err != nil || after != before {
				t.Fatalf("local presentation command wrote command ledger: before=%d after=%d err=%v", before, after, err)
			}
			continue
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
		if entry.ByPage["history"] == nil || entry.ByPage["history"].BySurface["history"] == nil || entry.ByPage["history"].BySurface["history"].CommandID != "navigation.workspace.open" {
			t.Fatalf("history must navigate, never create a chat: %+v", entry)
		}
		if entry.ByPage["tasklists"] == nil || entry.ByPage["tasklists"].BySurface["tasklists"] == nil || entry.ByPage["tasklists"].BySurface["tasklists"].CommandID != "tasklists.create.open" || entry.ByPage["profiles"] == nil || entry.ByPage["profiles"].BySurface["profiles"] == nil || entry.ByPage["profiles"].BySurface["profiles"].CommandID != "profiles.create.open" || entry.ByPage["settings"] == nil || entry.ByPage["settings"].BySurface["toolbar"] == nil || entry.ByPage["settings"].BySurface["toolbar"].CommandID != "command_settings.create.open" || entry.Fallback != nil || entry.ByPage["workspace"] == nil || !entry.ByPage["workspace"].FallbackToSequences {
			t.Fatalf("%+v", entry)
		}
		for _, page := range []string{"workspace", "history", "profiles", "tasklists"} {
			pageBinding := entry.ByPage[page]
			if pageBinding == nil {
				continue
			}
			for _, bySurface := range pageBinding.BySurface {
				if bySurface != nil && bySurface.CommandID == "command_settings.create.open" {
					t.Fatalf("settings create leaked into app.page=%q: %+v", page, entry.ByPage[page])
				}
			}
		}
		cloned := cloneContextualKeyboardBindings([]LocalCommandKeyboardContextualBinding{entry})
		if cloned[0].ByPage["workspace"] == nil || !cloned[0].ByPage["workspace"].FallbackToSequences {
			t.Fatal("lost page-scoped prefix fallback on clone")
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
