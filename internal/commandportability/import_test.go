package commandportability

import (
	"reflect"
	"testing"

	"assistente/internal/commandconfig"
	"github.com/google/uuid"
)

func importPlanTestID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestBuildImportedSnapshotKeepPreservaCamadaEFilhosExistentes(t *testing.T) {
	userID := importPlanTestID(t)
	layerID := importPlanTestID(t)
	bindingID := importPlanTestID(t)
	scope := commandconfig.Scope{UserID: userID}
	beforeLayer := commandconfig.Layer{ID: layerID, UserID: userID, Name: "destino", Source: "user", Enabled: true}
	beforeBinding := commandconfig.Binding{ID: bindingID, UserID: userID, LayerRefKind: "user", LayerRef: layerID, Source: "user"}
	before := commandconfig.Snapshot{Scope: scope, Layers: []commandconfig.Layer{beforeLayer}, Bindings: []commandconfig.Binding{beforeBinding}}
	plan := Plan{Version: ExportVersion, Layers: []PlannedLayer{{
		Layer: LayerExport{ID: layerID, Scope: PortableScope{Kind: GlobalScope}, Name: "arquivo diferente", Enabled: true},
		TargetID: layerID, TargetScope: PortableScope{Kind: GlobalScope}, Action: KeepMode, Enabled: true,
	}}}

	built, err := buildImportedSnapshot(plan, before, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(built.Snapshot.Layers, []commandconfig.Layer{beforeLayer}) || !reflect.DeepEqual(built.Snapshot.Bindings, []commandconfig.Binding{beforeBinding}) {
		t.Fatalf("Keep substituiu estado existente: layers=%+v bindings=%+v", built.Snapshot.Layers, built.Snapshot.Bindings)
	}
	if len(built.TouchedLayerIDs) != 0 || len(built.TouchedBindingIDs) != 0 {
		t.Fatalf("Keep marcou IDs existentes como tocados: %+v", built)
	}
}

func TestBuildImportedSnapshotReplaceECopyMantemSemanticaExplicita(t *testing.T) {
	userID := importPlanTestID(t)
	layerID := importPlanTestID(t)
	newLayerID := importPlanTestID(t)
	bindingID := importPlanTestID(t)
	scope := commandconfig.Scope{UserID: userID}
	before := commandconfig.Snapshot{
		Scope: scope,
		Layers: []commandconfig.Layer{{ID: layerID, UserID: userID, Name: "destino", Source: "user", Enabled: true}},
		Bindings: []commandconfig.Binding{{ID: bindingID, UserID: userID, LayerRefKind: "user", LayerRef: layerID, Source: "user"}},
	}
	replace := Plan{Version: ExportVersion, Layers: []PlannedLayer{{
		Layer: LayerExport{ID: layerID, Scope: PortableScope{Kind: GlobalScope}, Name: "novo", Enabled: true},
		TargetID: layerID, TargetScope: PortableScope{Kind: GlobalScope}, Action: ReplaceMode, Enabled: true,
	}}}
	replaced, err := buildImportedSnapshot(replace, before, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(replaced.Snapshot.Layers) != 1 || replaced.Snapshot.Layers[0].Name != "novo" || len(replaced.Snapshot.Bindings) != 0 {
		t.Fatalf("Replace não substituiu camada/filhos: %+v", replaced.Snapshot)
	}
	if len(replaced.TouchedLayerIDs) != 1 || len(replaced.TouchedBindingIDs) != 1 || replaced.TouchedBindingIDs[0] != bindingID {
		t.Fatalf("Replace não registrou filhos removidos: %+v", replaced)
	}

	copyPlan := Plan{Version: ExportVersion, Layers: []PlannedLayer{{
		Layer: LayerExport{ID: layerID, Scope: PortableScope{Kind: GlobalScope}, Name: "cópia", Enabled: true},
		TargetID: newLayerID, TargetScope: PortableScope{Kind: GlobalScope}, Action: CopyMode, Enabled: true,
	}}}
	copied, err := buildImportedSnapshot(copyPlan, before, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied.Snapshot.Layers) != 2 || copied.Snapshot.Layers[1].ID != newLayerID || copied.Snapshot.Layers[1].Name != "cópia" {
		t.Fatalf("Copy não criou camada nova sem remover destino: %+v", copied.Snapshot.Layers)
	}
	if len(copied.Snapshot.Bindings) != 1 || copied.Snapshot.Bindings[0].ID != bindingID {
		t.Fatalf("Copy removeu filho do destino: %+v", copied.Snapshot.Bindings)
	}
}
