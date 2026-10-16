package commandconfig

import (
	"context"
	"slices"
	"testing"

	"assistente/internal/commandbindings"
)

func TestProjectLocalReadWorkspacePreservesGlobalAndLocalBindings(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	global := projectionTestLayer(t, fixture, true)
	globalBinding := projectionTestBinding(t, fixture, global)
	if err := fixture.db.Model(&global).Update("name", "Atalhos globais").Error; err != nil {
		t.Fatal(err)
	}
	local := projectionTestLayer(t, fixture, true)
	localBinding := projectionTestBinding(t, fixture, local)
	workspace := storeTestUUID7(t)
	if err := fixture.db.Model(&local).Update("workspace_id", workspace).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&localBinding).Updates(map[string]any{
		"workspace_id": workspace,
		"trigger_spec": `{"version":1,"code":"KeyJ","modifiers":["Control"]}`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.scope.WorkspaceID = &workspace
	if err := fixture.store.EnsureScope(context.Background(), fixture.scope); err != nil {
		t.Fatal(err)
	}
	fixture.options.ActiveUserLayerIDs = []string{global.ID, local.ID}
	configuration, err := ProjectLocalRead(context.Background(), projectionTestReload(t, fixture), fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	for trigger, id := range map[string]string{projectionTrigger: globalBinding.ID, "keyboard.local:Control+KeyJ": localBinding.ID} {
		selection, err := configuration.Resolve(trigger, nil, nil)
		if err != nil || selection.Status != commandbindings.Selected || !slices.Contains(selection.BindingIDs, id) {
			t.Fatalf("binding %s perdido na união: %+v err=%v", id, selection, err)
		}
	}
	// O mesmo binding local não pode aparecer ao projetar somente o global.
	fixture.scope.WorkspaceID = nil
	fixture.options.ActiveUserLayerIDs = []string{global.ID}
	configuration, err = ProjectLocalRead(context.Background(), projectionTestReload(t, fixture), fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := configuration.Resolve("keyboard.local:Control+KeyJ", nil, nil)
	if err != nil || slices.Contains(selection.BindingIDs, localBinding.ID) {
		t.Fatalf("binding local vazou para global: %+v err=%v", selection, err)
	}
}
