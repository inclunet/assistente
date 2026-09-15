package commandportability

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandconfig"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestExportFromStoreFalhaBuiltinDeltaSemUserLayer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&commandconfig.Layer{}, &commandconfig.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE command_layer_activation_rules (user_id TEXT, layer_ref_kind TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	userID := portableStoreUUID(t)
	bindingID := portableStoreUUID(t)
	if err := db.Create(&commandconfig.Binding{ID: bindingID, UserID: userID, LayerRefKind: "builtin", LayerRef: "application.defaults", TriggerType: "keyboard.local", TriggerSpec: `{}`, Arguments: `{}`, Condition: `{}`, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`, ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("fp")}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ExportFromStore(context.Background(), db, userID, portabilityRefs(t), nil, true); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("delta builtin sem user layer não falhou fechado: %v", err)
	}
}

func TestExportFromStoreFalhaBuiltinRuleSemUserLayer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&commandconfig.Layer{}, &commandconfig.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE command_layer_activation_rules (user_id TEXT, layer_ref_kind TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	userID := portableStoreUUID(t)
	if err := db.Exec(`INSERT INTO command_layer_activation_rules (user_id, layer_ref_kind) VALUES (?, ?)`, userID, "builtin").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ExportFromStore(context.Background(), db, userID, portabilityRefs(t), nil, true); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("regra delta builtin sem user layer não falhou fechado: %v", err)
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
