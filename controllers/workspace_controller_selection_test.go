package controllers

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/workspace"
)

func TestWorkspaceControllerSetActiveWorkspaceTabForWorkspaceEmitsReturnedSnapshot(t *testing.T) {
	manager, _, other := newWorkspaceControllerTestFixture(t)
	active := manager.Active()
	if active == nil || len(active.Tabs.Items) == 0 {
		t.Fatal("fixture sem aba ativa")
	}
	if err := manager.AddTab(workspace.Tab{ID: "controller-selection-editor", Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetActiveTab(active.Tabs.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	targetID := "controller-selection-editor"
	var emitted *workspace.Workspace
	emitter := &captureWorkspaceControllerEmitter{capture: func(name string, data any) {
		if name == "workspace:tab_activated" {
			emitted, _ = data.(*workspace.Workspace)
		}
	}}
	controller := NewWorkspaceController(WorkspaceControllerConfig{WorkspaceMgr: manager, Emitter: emitter})

	got, err := controller.SetActiveWorkspaceTabForWorkspace(context.Background(), active.ID, targetID)
	if err != nil || got == nil || emitted != got || got.Tabs.Active != targetID || got.SnapshotEpoch == "" || got.SnapshotSequence == "" {
		t.Fatalf("seleção/evento divergentes: got=%#v emitted=%#v err=%v", got, emitted, err)
	}

	before := manager.Active()
	emitted = nil
	if got, err := controller.SetActiveWorkspaceTabForWorkspace(context.Background(), other.ID, targetID); !errors.Is(err, workspace.ErrWorkspaceSelectionWorkspaceStale) || got != nil || emitted != nil {
		t.Fatalf("workspace incorreto publicou ou retornou: got=%#v emitted=%#v err=%v", got, emitted, err)
	}
	if current := manager.Active(); current.ID != before.ID || current.Tabs.Active != before.Tabs.Active {
		t.Fatalf("recusa do controller alterou estado: before=%#v current=%#v", before.Tabs, current.Tabs)
	}
}
