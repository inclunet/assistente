package commandportability

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"gorm.io/gorm"
)

func TestApplyPlanImportBatchGlobalWorkspaceConfirmaPersisteEKeepNoop(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	ctx := context.Background()
	workspace := applyImportUUID(t)
	if err := f.db.Create(&commandconfig.Generation{ID: applyImportUUID(t), UserID: f.user, WorkspaceID: &workspace, Generation: 1, UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	globalID, workspaceID := applyImportUUID(t), applyImportUUID(t)
	layers := []LayerExport{
		{ID: globalID, Scope: PortableScope{Kind: GlobalScope}, Name: "lote global", Enabled: true, ResolutionPriority: 1},
		{ID: workspaceID, Scope: PortableScope{Kind: WorkspaceScope, WorkspaceID: workspace}, Name: "lote workspace", Enabled: true, ResolutionPriority: 2},
	}
	refs := applyImportRefs(t, f)
	owner := func(context.Context, string, string) (Ownership, error) { return AbsentOwner, nil }
	result, err := ApplyPlanImportBatch(ctx, f.service, "token", layers, PlanOptions{Mode: ReplaceMode}, owner, refs)
	diffs := result.Diffs
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 2 {
		t.Fatalf("diffs do lote: %+v", diffs)
	}
	for _, id := range []string{globalID, workspaceID} {
		var count int64
		if err := f.db.Model(&commandconfig.Layer{}).Where("user_id = ? AND id = ?", f.user, id).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("camada %s não persistida: %d", id, count)
		}
	}
	afterGlobal, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if !containsLayerID(afterGlobal.Layers, globalID) || containsLayerID(afterGlobal.Layers, workspaceID) {
		t.Fatalf("snapshot global contém escopo incorreto: %+v", afterGlobal.Layers)
	}
	afterWorkspace, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user, WorkspaceID: &workspace})
	if err != nil {
		t.Fatal(err)
	}
	if !containsLayerID(afterWorkspace.Layers, globalID) || !containsLayerID(afterWorkspace.Layers, workspaceID) {
		t.Fatalf("snapshot workspace não refletiu a união do lote: %+v", afterWorkspace.Layers)
	}
	decisions := f.presenter.calls
	globalBeforeKeep, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	workspaceBeforeKeep, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user, WorkspaceID: &workspace})
	if err != nil {
		t.Fatal(err)
	}
	if kept, err := ApplyPlanImportBatch(ctx, f.service, "token", layers, PlanOptions{Mode: KeepMode}, owner, refs); !errors.Is(err, ErrNoChanges) || kept.Report == nil || !kept.Report.NoChanges {
		t.Fatalf("Keep do lote não foi no-op com relatório: %+v err=%v", kept, err)
	}
	if f.presenter.calls != decisions {
		t.Fatalf("Keep abriu nova decisão: antes=%d depois=%d", decisions, f.presenter.calls)
	}
	globalAfterKeep, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	workspaceAfterKeep, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user, WorkspaceID: &workspace})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(globalBeforeKeep, globalAfterKeep) || !reflect.DeepEqual(workspaceBeforeKeep, workspaceAfterKeep) {
		t.Fatal("Keep do lote alterou snapshot")
	}
}

func TestApplyPlanImportBatchErroDeCommitNaoDeixaPrimeiroEscopo(t *testing.T) {
	hookErr := errors.New("falha do commit do lote")
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return hookErr })
	workspace := applyImportUUID(t)
	if err := f.db.Create(&commandconfig.Generation{ID: applyImportUUID(t), UserID: f.user, WorkspaceID: &workspace, Generation: 1, UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	layers := []LayerExport{
		{ID: applyImportUUID(t), Scope: PortableScope{Kind: GlobalScope}, Name: "rollback global", Enabled: true},
		{ID: applyImportUUID(t), Scope: PortableScope{Kind: WorkspaceScope, WorkspaceID: workspace}, Name: "rollback workspace", Enabled: true},
	}
	result, err := ApplyPlanImportBatch(context.Background(), f.service, "token", layers, PlanOptions{Mode: ReplaceMode}, applyImportOwner(f), applyImportRefs(t, f))
	if !errors.Is(err, hookErr) {
		t.Fatalf("erro do commit: %v", err)
	}
	if result.Report != nil || len(result.Diffs) != 0 {
		t.Fatal("rollback não pode retornar relatório de importação aplicada")
	}
	for _, layer := range layers {
		var count int64
		if err := f.db.Model(&commandconfig.Layer{}).Where("id = ?", layer.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback deixou camada %s", layer.ID)
		}
	}
}

func TestApplyPlanImportBatchCopyRemapeiaIDsDoLote(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	commandID := applyImportCommand
	binding := applyImportBinding(t, f.layer.ID, commandID)
	input := applyImportLayer(f, "cópia do lote", &binding)
	result, err := ApplyPlanImportBatch(context.Background(), f.service, "token", []LayerExport{input}, PlanOptions{Mode: CopyMode}, applyImportOwner(f), applyImportRefs(t, f))
	diffs := result.Diffs
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 1 {
		t.Fatalf("diff da cópia: %+v", diffs)
	}
	var copiedID string
	for _, layer := range diffs[0].AfterLayers {
		if layer.Name == input.Name {
			copiedID = layer.ID
		}
	}
	if copiedID == input.ID || copiedID == "" {
		t.Fatalf("cópia não remapeou ID: origem=%q destino=%q", input.ID, copiedID)
	}
	if result.Report == nil || result.Report.NoChanges || len(result.Report.Layers) != 1 || result.Report.Layers[0].TargetID != copiedID {
		t.Fatalf("relatório deve conter o ID realmente persistido, não um novo planejamento: %+v", result.Report)
	}
	var copiedBinding commandconfig.Binding
	for _, candidate := range diffs[0].AfterBindings {
		if candidate.LayerRef == copiedID {
			copiedBinding = candidate
		}
	}
	if copiedBinding.ID == binding.ID || copiedBinding.ID == "" || copiedBinding.LayerRef != copiedID {
		t.Fatalf("binding da cópia não foi remapeado: origem=%+v destino=%+v", binding, copiedBinding)
	}
	var count int64
	if err := f.db.Model(&commandconfig.Layer{}).Where("id = ?", copiedID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("cópia não persistida: %d", count)
	}
}

func containsLayerID(layers []commandconfig.Layer, id string) bool {
	for _, layer := range layers {
		if layer.ID == id {
			return true
		}
	}
	return false
}
