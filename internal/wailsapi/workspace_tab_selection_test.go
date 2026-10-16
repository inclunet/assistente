package wailsapi

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"assistente/controllers"
	"assistente/internal/workspace"
)

type workspaceSelectionTestEmitter struct{}

func (workspaceSelectionTestEmitter) Emit(string, any) {}

func TestWorkspaceSetActiveWorkspaceTabForWorkspaceUsesWithUserAndReturnsStampedSnapshot(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "home"))
	if err := manager.Initialize(filepath.Join(t.TempDir(), "active")); err != nil {
		t.Fatal(err)
	}
	initial := manager.Active()
	if initial == nil || len(initial.Tabs.Items) == 0 {
		t.Fatal("fixture sem aba")
	}
	if err := manager.AddTab(workspace.Tab{ID: "wails-selection-editor", Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	controller := controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{
		WorkspaceMgr: manager,
		Emitter:      workspaceSelectionTestEmitter{},
	})
	api := NewWorkspace()
	AttachWorkspace(api, stubSession{ctx: context.Background()}, controller)

	got, err := api.SetActiveWorkspaceTabForWorkspace(initial.ID, "wails-selection-editor")
	if err != nil || got == nil || got.ID != initial.ID || got.Tabs.Active != "wails-selection-editor" || got.SnapshotEpoch == "" || got.SnapshotSequence == "" {
		t.Fatalf("retorno Wails inválido: got=%#v err=%v", got, err)
	}
}

func TestWorkspaceSetActiveWorkspaceTabForWorkspaceAuthAndWiringFailClosed(t *testing.T) {
	api := NewWorkspace()
	if _, err := api.SetActiveWorkspaceTabForWorkspace("workspace", "tab"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("API não wired: %v", err)
	}
	AttachWorkspace(api, stubSession{err: errors.New("unauthenticated")}, controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{}))
	if _, err := api.SetActiveWorkspaceTabForWorkspace("workspace", "tab"); err == nil || err.Error() != "unauthenticated" {
		t.Fatalf("API sem autenticação: %v", err)
	}
}
