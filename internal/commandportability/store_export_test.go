package commandportability

import (
	"context"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestExportFromStoreExportaBuiltinDeltaSemUserLayer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&commandconfig.Layer{}, &commandconfig.Binding{}, &commandconfig.Generation{}); err != nil {
		t.Fatal(err)
	}
	userID := portableStoreUUID(t)
	bindingID := portableStoreUUID(t)
	if err := db.Create(&commandconfig.Binding{ID: bindingID, UserID: userID, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp")}).Error; err != nil {
		t.Fatal(err)
	}
	exported, err := ExportFromStore(context.Background(), db, userID, portabilityRefsWithBuiltin(t), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || !exported[0].DeltaOnly || len(exported[0].BuiltinDeltas) != 1 || exported[0].BuiltinDeltas[0].ID != bindingID {
		t.Fatalf("delta builtin não foi exportado no contêiner: %+v", exported)
	}
}

func TestExportFromStoreExportaBuiltinRuleSemUserLayer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&commandconfig.Layer{}, &commandconfig.Binding{}, &commandactivation.Rule{}); err != nil {
		t.Fatal(err)
	}
	userID := portableStoreUUID(t)
	ruleID := portableStoreUUID(t)
	if err := db.Create(&commandactivation.Rule{ID: ruleID, UserID: userID, LayerRefKind: commandactivation.BuiltinRef, LayerRef: "application.defaults", RuleRefKind: commandactivation.BuiltinRef, RuleRef: "builtin.rule", Mode: commandactivation.ModeAlways, Condition: `{}`, Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReplacesDefaultID: stringPtr("builtin.rule"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp"), ReviewStatus: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	exported, err := ExportFromStore(context.Background(), db, userID, portabilityRefsWithBuiltin(t), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || !exported[0].DeltaOnly || len(exported[0].BuiltinRuleDeltas) != 1 || exported[0].BuiltinRuleDeltas[0].ID != ruleID {
		t.Fatalf("regra builtin não foi exportada no contêiner: %+v", exported)
	}
}

func TestExportFromStoreNaoDuplicaDeltaGlobalAoExportarWorkspace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&commandconfig.Layer{}, &commandconfig.Binding{}, &commandconfig.Generation{}); err != nil {
		t.Fatal(err)
	}
	userID := portableStoreUUID(t)
	workspace := "workspace-destino"
	globalLayerID, workspaceLayerID := portableStoreUUID(t), portableStoreUUID(t)
	if err := db.Create([]commandconfig.Layer{
		{ID: globalLayerID, UserID: userID, Name: "Global", Source: "user", Enabled: true},
		{ID: workspaceLayerID, UserID: userID, WorkspaceID: &workspace, Name: "Workspace", Source: "user", Enabled: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create([]commandconfig.Generation{
		{ID: portableStoreUUID(t), UserID: userID, Generation: 1},
		{ID: portableStoreUUID(t), UserID: userID, WorkspaceID: &workspace, Generation: 1},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create([]commandconfig.Binding{
		{ID: portableStoreUUID(t), UserID: userID, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("global")},
		{ID: portableStoreUUID(t), UserID: userID, WorkspaceID: &workspace, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("workspace")},
	}).Error; err != nil {
		t.Fatal(err)
	}
	exported, err := ExportFromStore(context.Background(), db, userID, portabilityRefsWithBuiltin(t), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var globalContainers, workspaceContainers, globalDeltas, workspaceDeltas int
	for _, layer := range exported {
		if !layer.DeltaOnly {
			continue
		}
		if layer.Scope.Kind == GlobalScope {
			globalContainers++
			globalDeltas += len(layer.BuiltinDeltas)
		} else if layer.Scope.WorkspaceID == workspace {
			workspaceContainers++
			workspaceDeltas += len(layer.BuiltinDeltas)
		}
	}
	if globalContainers != 1 || workspaceContainers != 1 || globalDeltas != 1 || workspaceDeltas != 1 {
		t.Fatalf("deltas global/workspace duplicados ou perdidos: %+v", exported)
	}
}

func TestExportFromStoreSelecaoUserNaoIncluiDeltaBuiltinNaoSolicitado(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&commandconfig.Layer{}, &commandconfig.Binding{}, &commandconfig.Generation{}); err != nil {
		t.Fatal(err)
	}
	userID := portableStoreUUID(t)
	layerID := portableStoreUUID(t)
	if err := db.Create(&commandconfig.Layer{ID: layerID, UserID: userID, Name: "Selecionada", Source: "user", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&commandconfig.Generation{ID: portableStoreUUID(t), UserID: userID, Generation: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&commandconfig.Binding{ID: portableStoreUUID(t), UserID: userID, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp")}).Error; err != nil {
		t.Fatal(err)
	}
	exported, err := ExportFromStore(context.Background(), db, userID, portabilityRefsWithBuiltin(t), []string{layerID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].DeltaOnly || exported[0].ID != layerID {
		t.Fatalf("seleção de camada incluiu delta builtin não solicitado: %+v", exported)
	}
}

func portableStoreUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}
