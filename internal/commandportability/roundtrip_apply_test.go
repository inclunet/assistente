package commandportability

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"gorm.io/gorm"
)

func TestExportApplyPlanImportRoundTripAutenticadoBuiltinWorkspaceNeedsReview(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	workspace := "workspace-roundtrip"
	workspaceLayer := commandconfig.Layer{
		ID: applyImportUUID(t), UserID: f.user, WorkspaceID: &workspace,
		Name: "workspace-roundtrip", Description: "camada workspace exportada",
		Enabled: true, Source: "user", ResolutionPriority: 2,
	}
	if err := f.db.Create(&workspaceLayer).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&commandconfig.Generation{
		ID: applyImportUUID(t), UserID: f.user, WorkspaceID: &workspace, Generation: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	globalBinding := portableBindingSnapshot(applyImportBinding(t, f.layer.ID, applyImportCommand), f.user, nil, f.layer.ID, true)
	workspaceBinding := portableBindingSnapshot(applyImportBinding(t, workspaceLayer.ID, applyImportCommand), f.user, &workspace, workspaceLayer.ID, true)
	globalDelta := roundtripBuiltinBinding(t, f.user, nil)
	workspaceDelta := roundtripBuiltinBinding(t, f.user, &workspace)
	if err := f.db.Create([]commandconfig.Binding{globalBinding, workspaceBinding, globalDelta, workspaceDelta}).Error; err != nil {
		t.Fatal(err)
	}

	f.projection.BuiltinLayers = []commandconfig.BuiltinLayer{roundtripBuiltinLayer()}
	refs := roundtripRefs(t, workspace)
	owner := roundtripOwner(f, workspaceLayer.ID, globalDelta.ID, workspaceDelta.ID)

	globalExport, err := ExportFromStore(context.Background(), f.db, f.user, refs, nil, false)
	if err != nil {
		t.Fatalf("ExportFromStore global: %v", err)
	}
	if len(globalExport) != 2 || !hasPortableLayer(globalExport, f.layer.ID) || !hasBuiltinDelta(globalExport, globalDelta.ID, GlobalScope) {
		t.Fatalf("export global incompleto: %+v", globalExport)
	}
	if got := exportedReviewStatus(globalExport, globalDelta.ID); got != "needs_review" {
		t.Fatalf("needs_review do delta global perdido no export: %q", got)
	}

	t.Run("keep idempotente", func(t *testing.T) {
		beforeCalls := f.presenter.calls
		if _, err := ApplyPlanImport(context.Background(), f.service, "token", nil, globalExport, PlanOptions{Mode: KeepMode}, owner, refs); !errors.Is(err, ErrNoChanges) {
			t.Fatalf("Keep do round-trip retornou %v, esperado ErrNoChanges", err)
		}
		if f.presenter.calls != beforeCalls {
			t.Fatalf("Keep no-op abriu decisão: antes=%d depois=%d", beforeCalls, f.presenter.calls)
		}
	})

	t.Run("replace restaura export autenticado", func(t *testing.T) {
		if err := f.db.Model(&commandconfig.Layer{}).Where("id = ?", f.layer.ID).Update("name", "destino divergente").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := ApplyPlanImport(context.Background(), f.service, "token", nil, globalExport, PlanOptions{Mode: ReplaceMode}, owner, refs); err != nil {
			t.Fatalf("Replace do round-trip: %v", err)
		}
		var restored commandconfig.Layer
		if err := f.db.Where("id = ?", f.layer.ID).First(&restored).Error; err != nil {
			t.Fatal(err)
		}
		if restored.Name != f.layer.Name {
			t.Fatalf("Replace não restaurou camada exportada: %+v", restored)
		}
		var delta commandconfig.Binding
		if err := f.db.Where("id = ?", globalDelta.ID).First(&delta).Error; err != nil {
			t.Fatal(err)
		}
		if delta.ReviewStatus != "needs_review" || delta.ReplacesDefaultFingerprint == nil || *delta.ReplacesDefaultFingerprint != "fp-roundtrip" {
			t.Fatalf("Replace não preservou metadados do delta: %+v", delta)
		}
	})

	t.Run("copy workspace remapeia e preserva delta", func(t *testing.T) {
		globalBeforeCopy, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
		if err != nil {
			t.Fatal(err)
		}
		allExport, err := ExportFromStore(context.Background(), f.db, f.user, refs, nil, true)
		if err != nil {
			t.Fatalf("ExportFromStore workspace: %v", err)
		}
		workspaceExport := onlyPortableWorkspace(allExport, workspace)
		if len(workspaceExport) != 2 || !hasPortableLayer(workspaceExport, workspaceLayer.ID) || !hasBuiltinDelta(workspaceExport, workspaceDelta.ID, WorkspaceScope) {
			t.Fatalf("export workspace incompleto ou com global duplicado: %+v", workspaceExport)
		}
		if got := exportedReviewStatus(workspaceExport, workspaceDelta.ID); got != "needs_review" {
			t.Fatalf("needs_review do delta workspace perdido no export: %q", got)
		}

		if _, err := ApplyPlanImport(context.Background(), f.service, "token", &workspace, workspaceExport, PlanOptions{
			Mode:          CopyMode,
			RenameByLayer: map[string]string{workspaceLayer.ID: "workspace-roundtrip-copy"},
		}, owner, refs); err != nil {
			t.Fatalf("Copy workspace do round-trip: %v", err)
		}
		after, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user, WorkspaceID: &workspace})
		if err != nil {
			t.Fatal(err)
		}
		workspaceLayers := 0
		workspaceDeltas := 0
		workspaceLayerNames := map[string]bool{}
		workspaceLayerIDs := map[string]bool{}
		originalWorkspaceLayerFound := false
		copiedWorkspaceLayerID := ""
		for _, layer := range after.Layers {
			if layer.WorkspaceID != nil && *layer.WorkspaceID == workspace {
				workspaceLayers++
				workspaceLayerNames[layer.Name] = true
				workspaceLayerIDs[layer.ID] = true
				if layer.ID == workspaceLayer.ID {
					originalWorkspaceLayerFound = true
				}
				if layer.Name == "workspace-roundtrip-copy" {
					copiedWorkspaceLayerID = layer.ID
				}
			}
		}
		workspaceUserBindings := 0
		originalWorkspaceBindingFound := false
		copiedWorkspaceBindingID := ""
		for _, binding := range after.Bindings {
			if binding.WorkspaceID == nil || *binding.WorkspaceID != workspace {
				continue
			}
			if binding.LayerRefKind == "user" {
				workspaceUserBindings++
				if !workspaceLayerIDs[binding.LayerRef] {
					t.Errorf("binding user workspace ficou sem camada remapeada: %+v", binding)
				}
				if binding.ID == workspaceBinding.ID {
					originalWorkspaceBindingFound = true
				}
				if binding.LayerRef == copiedWorkspaceLayerID {
					copiedWorkspaceBindingID = binding.ID
				}
			}
			if binding.LayerRefKind == "builtin" {
				workspaceDeltas++
				if binding.ReviewStatus != "needs_review" {
					t.Errorf("delta workspace copiado perdeu needs_review: %+v", binding)
				}
			}
		}
		if workspaceLayers != 2 || workspaceUserBindings != 2 || workspaceDeltas != 2 || !workspaceLayerNames["workspace-roundtrip-copy"] || !originalWorkspaceLayerFound || copiedWorkspaceLayerID == "" || copiedWorkspaceLayerID == workspaceLayer.ID || !originalWorkspaceBindingFound || copiedWorkspaceBindingID == "" || copiedWorkspaceBindingID == workspaceBinding.ID {
			t.Fatalf("Copy não remapeou/preservou camada, binding e delta workspace: layers=%d userBindings=%d deltas=%d snapshot=%+v", workspaceLayers, workspaceUserBindings, workspaceDeltas, after)
		}
		var copiedBinding commandconfig.Binding
		for _, binding := range after.Bindings {
			if binding.ID == copiedWorkspaceBindingID {
				copiedBinding = binding
				break
			}
		}
		if copiedBinding.LayerRef != copiedWorkspaceLayerID {
			t.Fatalf("binding copiado não aponta para a camada nova: binding=%+v copiedLayer=%q", copiedBinding, copiedWorkspaceLayerID)
		}
		copiedWorkspaceDeltaID := ""
		originalWorkspaceDeltaFound := false
		for _, binding := range after.Bindings {
			if binding.WorkspaceID == nil || *binding.WorkspaceID != workspace || binding.LayerRefKind != "builtin" {
				continue
			}
			if binding.ID == workspaceDelta.ID {
				originalWorkspaceDeltaFound = true
			} else {
				copiedWorkspaceDeltaID = binding.ID
			}
		}
		if !originalWorkspaceDeltaFound || copiedWorkspaceDeltaID == "" || copiedWorkspaceDeltaID == workspaceDelta.ID {
			t.Fatalf("Copy não gerou ID novo para delta workspace: original=%q copied=%q snapshot=%+v", workspaceDelta.ID, copiedWorkspaceDeltaID, after)
		}
		globalAfterCopy, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(globalBeforeCopy.Layers, globalAfterCopy.Layers) || !reflect.DeepEqual(globalBeforeCopy.Bindings, globalAfterCopy.Bindings) || !reflect.DeepEqual(globalBeforeCopy.ActivationRules, globalAfterCopy.ActivationRules) || !reflect.DeepEqual(globalBeforeCopy.AutomationGrants, globalAfterCopy.AutomationGrants) || !reflect.DeepEqual(globalBeforeCopy.ActivationClaims, globalAfterCopy.ActivationClaims) {
			t.Fatalf("Copy workspace alterou escopo global: before=%+v after=%+v", globalBeforeCopy, globalAfterCopy)
		}
	})
}

func roundtripBuiltinLayer() commandconfig.BuiltinLayer {
	return commandconfig.BuiltinLayer{
		ID: "application.defaults", Active: true, ResolutionPriority: 1,
		Defaults: []commandbindings.Default{{
			Candidate: commandbindings.Candidate{
				ID: "builtin.tab.new", Trigger: "keyboard.local:KeyA", CommandID: applyImportCommand,
				ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global,
				LayerPriority: 1, Enabled: true, LayerActive: true,
			},
			Version: "1", Fingerprint: "fp-roundtrip",
		}},
	}
}

func roundtripBuiltinBinding(t *testing.T, user string, workspace *string) commandconfig.Binding {
	t.Helper()
	return commandconfig.Binding{
		ID: applyImportUUID(t), UserID: user, WorkspaceID: cloneWorkspaceForTest(workspace),
		LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local",
		TriggerSpec: applyImportTrigger, Arguments: "{}", Condition: applyImportCondition,
		Effect: "suppress", Enabled: true, Source: "user", ResolutionPriority: 3,
		ReviewStatus: "needs_review", Presentation: `{"version":1}`,
		ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"),
		ReplacesDefaultFingerprint: stringPtr("fp-roundtrip"),
	}
}

func roundtripRefs(t *testing.T, workspace string) ReferencePort {
	t.Helper()
	refs := portabilityRefsWithBuiltin(t)
	refs.Workspace = func(_ context.Context, id string) (string, error) {
		if id != workspace {
			return "", ErrWorkspaceResolution
		}
		return id, nil
	}
	refs.Trigger = func(_ context.Context, triggerType, raw string) (string, error) {
		if triggerType != "keyboard.local" || raw != applyImportTrigger {
			return "", ErrInvalid
		}
		return "keyboard.local:KeyA", nil
	}
	return refs
}

func roundtripOwner(f applyImportFixture, workspaceLayerID, globalDeltaID, workspaceDeltaID string) OwnershipPort {
	return func(_ context.Context, _ string, id string) (Ownership, error) {
		switch id {
		case f.layer.ID, workspaceLayerID, globalDeltaID, workspaceDeltaID:
			return CurrentUserOwner, nil
		default:
			return AbsentOwner, nil
		}
	}
}

func hasPortableLayer(layers []LayerExport, id string) bool {
	for _, layer := range layers {
		if !layer.DeltaOnly && layer.ID == id {
			return true
		}
	}
	return false
}

func hasBuiltinDelta(layers []LayerExport, id string, scope ScopeKind) bool {
	for _, layer := range layers {
		if !layer.DeltaOnly || layer.Scope.Kind != scope {
			continue
		}
		for _, binding := range layer.BuiltinDeltas {
			if binding.ID == id {
				return true
			}
		}
	}
	return false
}

func exportedReviewStatus(layers []LayerExport, id string) string {
	for _, layer := range layers {
		for _, binding := range layer.BuiltinDeltas {
			if binding.ID == id {
				return binding.ReviewStatus
			}
		}
	}
	return ""
}

func onlyPortableWorkspace(layers []LayerExport, workspace string) []LayerExport {
	result := make([]LayerExport, 0, 2)
	for _, layer := range layers {
		if layer.Scope.Kind == WorkspaceScope && layer.Scope.WorkspaceID == workspace {
			result = append(result, layer)
		}
	}
	return result
}

func cloneWorkspaceForTest(workspace *string) *string {
	if workspace == nil {
		return nil
	}
	value := *workspace
	return &value
}
