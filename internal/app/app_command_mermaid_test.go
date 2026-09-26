package app

import (
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/workspace"
)

func TestCommandMermaidRemoveDialogTimeout(t *testing.T) {
	a := readyCommandProduct(t)
	_, handlers, err := a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if handlers[commandEditorMermaidRemoveID].ExecutionTimeout != 5*time.Minute || commandUIRunTimeout(commandEditorMermaidRemoveID) != 5*time.Minute || commandUIResultTTL(commandEditorMermaidRemoveID) != 6*time.Minute {
		t.Fatal("remove dialog does not retain execution/result")
	}
	if handlers[commandEditorMermaidApplyID].ExecutionTimeout != 0 || commandUIRunTimeout(commandEditorMermaidApplyID) != 30*time.Second {
		t.Fatal("apply timeout broadened")
	}
	if commandUITakeTimeout(commandEditorMermaidRemoveID) != 35*time.Second {
		t.Fatal("Take wait unnecessarily broadened")
	}
	before := time.Now()
	r := beginUICommand(t, a, commandEditorMermaidRemoveID)
	_, run, err := a.commandUIRun(r.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if run.expiresAt.Before(before.Add(5 * time.Minute)) {
		t.Fatal("result lifetime too short")
	}
	if err := a.CancelUICommand(r.Ticket); err != nil {
		t.Fatal(err)
	}
}

func TestCommandMermaidScopedKeyboard(t *testing.T) {
	for _, shortcut := range []LocalCommandShortcut{
		{Version: 1, Code: "KeyS", Modifiers: []string{"Control"}},
		{Version: 1, Code: "KeyS", Modifiers: []string{"Meta"}},
		{Version: 1, Code: "Enter", Modifiers: []string{"Control"}},
	} {
		t.Run(shortcut.Modifiers[0]+shortcut.Code, func(t *testing.T) {
			a, _, _ := clearCommandFixture(t, workspace.TabTypeEditor)
			view, err := a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			r, err := a.BeginEditorMermaidUIKey(view.Generation, shortcut, false)
			if err != nil || r == nil {
				t.Fatalf("begin: %+v %v", r, err)
			}
			if _, err := a.BeginEditorMermaidUIKey(view.Generation, shortcut, true); err == nil {
				t.Fatal("repeat admitted")
			}
			if duplicate, err := a.BeginEditorMermaidUIKey(view.Generation, shortcut, false); err != nil || duplicate != nil {
				t.Fatalf("duplicate: %+v %v", duplicate, err)
			}
			if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"title": "Editor"}); err != nil {
				t.Fatal(err)
			}
			h := takeUICommandFor(t, a, r.Ticket, commandEditorMermaidApplyID)
			if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
				t.Fatal(err)
			}
			if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
				t.Fatalf("%+v", got)
			}
			var source string
			if err := database.DB().Raw("SELECT source_type FROM command_invocations WHERE invocation_id = ?", r.InvocationID).Scan(&source).Error; err != nil {
				t.Fatal(err)
			}
			if source != "keyboard.local" {
				t.Fatalf("source=%q", source)
			}
			if _, err := a.DispatchLocalCommandKey(view.Generation, shortcut, "up", false); err != nil {
				t.Fatal(err)
			}
			next, err := a.BeginEditorMermaidUIKey(view.Generation, shortcut, false)
			if err != nil || next == nil {
				t.Fatalf("keyup did not release: %+v %v", next, err)
			}
			if err := a.CancelUICommand(next.Ticket); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCommandMermaidScopeClosedAndNormalSavePreserved(t *testing.T) {
	a := readyCommandProduct(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []LocalCommandShortcut{
		{Version: 1, Code: "KeyB", Modifiers: []string{"Control"}},
		{Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyN", Modifiers: []string{"Control"}}, {Code: "KeyE", Modifiers: []string{}}}},
	} {
		if _, err := a.BeginEditorMermaidUIKey(view.Generation, key, false); err == nil {
			t.Fatal("scope fallback/sequence admitted")
		}
	}
	key := LocalCommandShortcut{Version: 1, Code: "KeyS", Modifiers: []string{"Control"}}
	for _, modalOnly := range []LocalCommandShortcut{{Version: 1, Code: "KeyS", Modifiers: []string{"Meta"}}, {Version: 1, Code: "Enter", Modifiers: []string{"Control"}}} {
		if r, err := a.BeginLocalCommandUIKey(view.Generation, modalOnly, false); err == nil || r != nil {
			t.Fatalf("modal invariant leaked globally: %+v %v", r, err)
		}
	}
	r, err := a.BeginLocalCommandUIKey(view.Generation, key, false)
	if err != nil || r == nil {
		t.Fatalf("normal save: %+v %v", r, err)
	}
	takeUICommandFor(t, a, r.Ticket, commandEditorFileSaveID)
	if err := a.CancelUICommand(r.Ticket); err != nil {
		t.Fatal(err)
	}
	a.ResetLocalCommandKeyboard(view.Generation)
	if _, err := a.BeginEditorMermaidUIKey(view.Generation, key, false); err == nil {
		t.Fatal("old generation admitted")
	}
	if !containsString(view.LocalPaletteCommands, "editor.mermaid.open") || len(view.LocalPaletteCommands) != 62 {
		t.Fatal("local presentation missing")
	}
	if _, err := a.BeginUICommand("editor.mermaid.open"); err == nil {
		t.Fatal("local open audited")
	}
}

func TestCommandMermaidRequiresEditorAndRejectsChangedTarget(t *testing.T) {
	key := LocalCommandShortcut{Version: 1, Code: "KeyS", Modifiers: []string{"Control"}}
	t.Run("chat", func(t *testing.T) {
		a, _, _ := clearCommandFixture(t, workspace.TabTypeChat)
		view, err := a.GetLocalCommandKeyboardMap()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.BeginEditorMermaidUIKey(view.Generation, key, false); err == nil {
			t.Fatal("chat accepted")
		}
	})
	t.Run("changed-target", func(t *testing.T) {
		a, _, _ := clearCommandFixture(t, workspace.TabTypeEditor)
		view, err := a.GetLocalCommandKeyboardMap()
		if err != nil {
			t.Fatal(err)
		}
		r, err := a.BeginEditorMermaidUIKey(view.Generation, key, false)
		if err != nil || r == nil {
			t.Fatalf("%+v %v", r, err)
		}
		if err := a.workspaceMgr.AddTab(workspace.Tab{ID: "another-editor", Type: workspace.TabTypeEditor}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.TakeUICommand(r.Ticket); err == nil {
			t.Fatal("changed target admitted")
		}
		_ = a.CancelUICommand(r.Ticket)
	})
}
