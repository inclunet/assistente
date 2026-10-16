package app

import (
	"testing"

	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandEditorModeCommitRunsRealUITransportForThreeModes(t *testing.T) {
	for _, item := range []struct{ id, mode string }{
		{commandEditorModeMarkdownID, "markdown"},
		{commandEditorModeRichID, "rich"},
		{commandEditorModeViewID, "view"},
	} {
		t.Run(item.mode, func(t *testing.T) {
			a := contextChatFixture(t, workspace.TabTypeEditor)
			r := beginUICommand(t, a, item.id)
			h := takeUICommandFor(t, a, r.Ticket, item.id)
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			if result := getUIResultEventually(t, a, r.Ticket); result.Status != "succeeded" {
				t.Fatalf("resultado=%+v", result)
			}
			active := a.workspaceMgr.Active().ActiveTab()
			if active == nil || active.State["displayMode"] != item.mode {
				t.Fatalf("modo persistido=%v, esperado %q", active, item.mode)
			}
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("duplicate commit aceito")
			}
			if active.State["displayMode"] != item.mode {
				t.Fatal("duplicate commit alterou o modo")
			}
		})
	}
}

func TestCommandEditorModeCommitRejectsNonEditorAndStaleTab(t *testing.T) {
	t.Run("non-editor", func(t *testing.T) {
		a := contextChatFixture(t, workspace.TabTypeChat)
		r := beginUICommand(t, a, commandEditorModeMarkdownID)
		h := takeUICommandFor(t, a, r.Ticket, commandEditorModeMarkdownID)
		if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
			t.Fatal("modo aceito em aba não-editor")
		}
		if active := a.workspaceMgr.Active().ActiveTab(); active == nil || active.State["displayMode"] != nil {
			t.Fatalf("aba não-editor foi alterada: %+v", active)
		}
	})

	t.Run("stale-after-take", func(t *testing.T) {
		a := contextChatFixture(t, workspace.TabTypeEditor)
		r := beginUICommand(t, a, commandEditorModeViewID)
		h := takeUICommandFor(t, a, r.Ticket, commandEditorModeViewID)
		otherID := uuid.NewString()
		if err := a.workspaceMgr.AddTab(workspace.Tab{ID: otherID, Type: workspace.TabTypeEditor}); err != nil {
			t.Fatal(err)
		}
		if err := a.workspaceMgr.SetActiveTab(otherID); err != nil {
			t.Fatal(err)
		}
		if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
			t.Fatal("commit stale aceito")
		}
		if active := a.workspaceMgr.Active().FindTab(otherID); active == nil || active.State["displayMode"] != nil {
			t.Fatalf("aba retargetada: %+v", active)
		}
	})
}
