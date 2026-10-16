package commandportability

import (
	"reflect"
	"testing"

	"assistente/internal/commandconfig"
	"github.com/google/uuid"
)

func importKeepTestID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestBuildImportedSnapshotKeepPreservaCamadaFilhoEDeltaBuiltinExistentes(t *testing.T) {
	userID := importKeepTestID(t)
	layerID := importKeepTestID(t)
	userBindingID := importKeepTestID(t)
	builtinBindingID := importKeepTestID(t)
	scope := commandconfig.Scope{UserID: userID}
	beforeLayer := commandconfig.Layer{ID: layerID, UserID: userID, Name: "destino", Source: "user", Enabled: true}
	beforeUserBinding := commandconfig.Binding{ID: userBindingID, UserID: userID, LayerRefKind: "user", LayerRef: layerID, Source: "user"}
	beforeBuiltinBinding := commandconfig.Binding{ID: builtinBindingID, UserID: userID, LayerRefKind: "builtin", LayerRef: "application.defaults", Source: "user"}
	before := commandconfig.Snapshot{
		Scope: scope, Layers: []commandconfig.Layer{beforeLayer},
		Bindings: []commandconfig.Binding{beforeUserBinding, beforeBuiltinBinding},
	}
	plan := Plan{Version: ExportVersion, Layers: []PlannedLayer{
		{
			Layer:    LayerExport{ID: layerID, Scope: PortableScope{Kind: GlobalScope}, Name: "nome importado", Enabled: true},
			TargetID: layerID, TargetScope: PortableScope{Kind: GlobalScope}, Action: KeepMode, Enabled: true,
		},
		{
			Layer: LayerExport{
				Scope: PortableScope{Kind: GlobalScope}, DeltaOnly: true,
				BuiltinDeltas: []BindingExport{{ID: builtinBindingID, LayerRefKind: "builtin", LayerRef: "application.defaults", Arguments: "{\"changed\":true}"}},
			},
			TargetScope: PortableScope{Kind: GlobalScope}, Action: KeepMode, Enabled: true,
		},
	}}

	built, err := buildImportedSnapshot(plan, before, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(built.Snapshot.Layers, []commandconfig.Layer{beforeLayer}) ||
		!reflect.DeepEqual(built.Snapshot.Bindings, []commandconfig.Binding{beforeUserBinding, beforeBuiltinBinding}) {
		t.Fatalf("Keep substituiu conteúdo existente: %+v", built.Snapshot)
	}
	if len(built.TouchedLayerIDs) != 0 || len(built.TouchedBindingIDs) != 0 {
		t.Fatalf("Keep marcou conteúdo existente como tocado: %+v", built)
	}
}

func TestBuildImportedSnapshotReplaceSubstituiECopyPreservaDestino(t *testing.T) {
	userID := importKeepTestID(t)
	layerID := importKeepTestID(t)
	copyID := importKeepTestID(t)
	bindingID := importKeepTestID(t)
	scope := commandconfig.Scope{UserID: userID}
	beforeLayer := commandconfig.Layer{ID: layerID, UserID: userID, Name: "destino", Source: "user", Enabled: true}
	beforeBinding := commandconfig.Binding{ID: bindingID, UserID: userID, LayerRefKind: "user", LayerRef: layerID, Source: "user"}
	before := commandconfig.Snapshot{Scope: scope, Layers: []commandconfig.Layer{beforeLayer}, Bindings: []commandconfig.Binding{beforeBinding}}

	replaced, err := buildImportedSnapshot(Plan{Version: ExportVersion, Layers: []PlannedLayer{{
		Layer:    LayerExport{ID: layerID, Scope: PortableScope{Kind: GlobalScope}, Name: "substituta", Enabled: true},
		TargetID: layerID, TargetScope: PortableScope{Kind: GlobalScope}, Action: ReplaceMode, Enabled: true,
	}}}, before, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(replaced.Snapshot.Layers) != 1 || replaced.Snapshot.Layers[0].Name != "substituta" || len(replaced.Snapshot.Bindings) != 0 {
		t.Fatalf("Replace não substituiu camada/filhos: %+v", replaced.Snapshot)
	}
	if len(replaced.TouchedLayerIDs) != 1 || len(replaced.TouchedBindingIDs) != 1 || replaced.TouchedBindingIDs[0] != bindingID {
		t.Fatalf("Replace não registrou filho removido: %+v", replaced)
	}

	copied, err := buildImportedSnapshot(Plan{Version: ExportVersion, Layers: []PlannedLayer{{
		Layer:    LayerExport{ID: layerID, Scope: PortableScope{Kind: GlobalScope}, Name: "cópia", Enabled: true},
		TargetID: copyID, TargetScope: PortableScope{Kind: GlobalScope}, Action: CopyMode, Enabled: true,
	}}}, before, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied.Snapshot.Layers) != 2 || copied.Snapshot.Layers[1].ID != copyID || copied.Snapshot.Layers[1].Name != "cópia" {
		t.Fatalf("Copy não criou camada independente: %+v", copied.Snapshot.Layers)
	}
	if len(copied.Snapshot.Bindings) != 1 || copied.Snapshot.Bindings[0].ID != bindingID {
		t.Fatalf("Copy removeu filho do destino: %+v", copied.Snapshot.Bindings)
	}
}
