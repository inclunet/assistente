package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEditorTransferPlanAndCAS(t *testing.T) {
	m := commandSnapshotManager(t)
	ctx := context.Background()
	before, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	count := len(m.Active().Tabs.Items)
	plan, err := m.PrepareEditorTransferForCommand(ctx, before, "", "new-editor")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Active().Tabs.Items) != count {
		t.Fatal("Prepare created tab")
	}
	ws, after, err := m.OpenEditorTransferForCommand(ctx, before, plan, "Title")
	if err != nil {
		t.Fatal(err)
	}
	if ws.Tabs.Active != plan.TabID || after.ActiveTabID != plan.TabID || ws.FindTab(plan.TabID).State["draftId"] != plan.DraftID {
		t.Fatal("wrong target")
	}
	if _, _, err := m.OpenEditorTransferForCommand(ctx, before, plan, "Title"); err == nil {
		t.Fatal("replayed workspace CAS")
	}
	if len(m.Active().Tabs.Items) != count+1 {
		t.Fatal("duplicate tab")
	}
}

func TestEditorTransferStaleAndPersistenceRollback(t *testing.T) {
	for _, scenario := range []string{"aba", "target-change", "persist"} {
		t.Run(scenario, func(t *testing.T) {
			m := commandSnapshotManager(t)
			ctx := context.Background()
			source := m.Active().Tabs.Active
			if err := m.AddTab(Tab{ID: "target", Type: TabTypeEditor, State: map[string]any{"draftId": "draft"}}); err != nil {
				t.Fatal(err)
			}
			if err := m.SetActiveTab(source); err != nil {
				t.Fatal(err)
			}
			expected, err := m.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			plan, err := m.PrepareEditorTransferForCommand(ctx, expected, "target", "")
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "aba":
				if err := m.SetActiveTab("target"); err != nil {
					t.Fatal(err)
				}
				if err := m.SetActiveTab(source); err != nil {
					t.Fatal(err)
				}
			case "target-change":
				if err := m.UpdateTab("target", map[string]any{"state": map[string]any{"draftId": "replacement"}}); err != nil {
					t.Fatal(err)
				}
			case "persist":
				blocker := filepath.Join(t.TempDir(), "blocker")
				if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
				m.activePath = filepath.Join(blocker, "workspace.yaml")
			}
			before, _ := m.CommandSnapshot()
			if _, _, err := m.OpenEditorTransferForCommand(ctx, expected, plan, "Title"); err == nil {
				t.Fatal("unsafe transition")
			}
			after, _ := m.CommandSnapshot()
			if after != before {
				t.Fatal("failed transition changed workspace")
			}
		})
	}
}

func TestEditorTransferRejectsViewTarget(t *testing.T) {
	m := commandSnapshotManager(t)
	if err := m.AddTab(Tab{ID: "view", Type: TabTypeEditor, State: map[string]any{"displayMode": "view"}}); err != nil {
		t.Fatal(err)
	}
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.PrepareEditorTransferForCommand(context.Background(), expected, "view", ""); err == nil {
		t.Fatal("view target accepted")
	}
}
