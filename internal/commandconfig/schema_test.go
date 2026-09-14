package commandconfig

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func schemaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commandconfig.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func schemaUUID7(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestMigrateIsIdempotentAndCreatesExpectedSchema(t *testing.T) {
	db := schemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migração não idempotente: %v", err)
	}

	for _, table := range []string{"command_layers", "command_bindings", "command_config_generations"} {
		var got string
		if err := db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&got).Error; err != nil || got != table {
			t.Fatalf("tabela %s ausente: %v", table, err)
		}
	}

	var foreignKeys int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_foreign_key_list('command_bindings')").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 0 {
		t.Fatal("bindings não devem inventar FK para catálogo/layers builtin")
	}

	assertPartialUnique := func(table, want string) {
		t.Helper()
		var found int
		if err := db.Raw("SELECT COUNT(*) FROM pragma_index_list(?) WHERE name=? AND \"unique\"=1 AND partial=1", table, want).Scan(&found).Error; err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Fatalf("índice parcial único %s.%s ausente", table, want)
		}
	}
	assertPartialUnique("command_layers", "ux_command_layers_user_global_name")
	assertPartialUnique("command_layers", "ux_command_layers_user_workspace_name")
	assertPartialUnique("command_config_generations", "ux_command_config_generations_user_global")
	assertPartialUnique("command_config_generations", "ux_command_config_generations_user_workspace")
}

func TestSchemaConstraints(t *testing.T) {
	db := schemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	userID := schemaUUID7(t)
	validLayer := map[string]any{"id": schemaUUID7(t), "user_id": userID, "name": "global", "description": "", "enabled": true, "source": "user", "resolution_priority": 1, "created_at": now, "updated_at": now}
	if err := db.Table("command_layers").Create(validLayer).Error; err != nil {
		t.Fatal(err)
	}
	localWorkspace := schemaUUID7(t)
	localLayer := map[string]any{"id": schemaUUID7(t), "user_id": userID, "workspace_id": localWorkspace, "name": "global", "description": "", "enabled": true, "source": "user", "resolution_priority": 1, "created_at": now, "updated_at": now}
	if err := db.Table("command_layers").Create(localLayer).Error; err != nil {
		t.Fatalf("workspace UUIDv7 válido foi rejeitado: %v", err)
	}
	invalidWorkspace := uuid.New().String()
	invalidLayer := map[string]any{"id": schemaUUID7(t), "user_id": userID, "workspace_id": invalidWorkspace, "name": "uuid4", "description": "", "enabled": true, "source": "user", "resolution_priority": 1, "created_at": now, "updated_at": now}
	if err := db.Table("command_layers").Create(invalidLayer).Error; err == nil {
		t.Fatal("workspace UUIDv4 deveria ser rejeitado")
	}
	malformedWorkspace := "-" + localWorkspace[1:]
	malformedLayer := map[string]any{"id": schemaUUID7(t), "user_id": userID, "workspace_id": malformedWorkspace, "name": "malformed", "description": "", "enabled": true, "source": "user", "resolution_priority": 1, "created_at": now, "updated_at": now}
	if err := db.Table("command_layers").Create(malformedLayer).Error; err == nil {
		t.Fatal("UUID com hífen hexadecimal extra deveria ser rejeitado")
	}
	duplicate := map[string]any{"id": schemaUUID7(t), "user_id": userID, "name": "global", "description": "", "enabled": true, "source": "user", "resolution_priority": 1, "created_at": now, "updated_at": now}
	if err := db.Table("command_layers").Create(duplicate).Error; err == nil {
		t.Fatal("layers globais duplicadas deveriam falhar")
	}

	baseGeneration := Generation{ID: schemaUUID7(t), UserID: userID, WorkspaceID: nil, Generation: 1, UpdatedAt: now}
	if err := db.Create(&baseGeneration).Error; err != nil {
		t.Fatal(err)
	}
	var scannedGeneration Generation
	if err := db.First(&scannedGeneration, "id = ?", baseGeneration.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !scannedGeneration.UpdatedAt.Equal(baseGeneration.UpdatedAt) {
		t.Fatalf("updated_at não foi escaneado como time.Time: got=%v want=%v", scannedGeneration.UpdatedAt, baseGeneration.UpdatedAt)
	}
	secondGeneration := Generation{ID: schemaUUID7(t), UserID: userID, WorkspaceID: nil, Generation: 2, UpdatedAt: now}
	if err := db.Create(&secondGeneration).Error; err == nil {
		t.Fatal("duas gerações globais deveriam falhar")
	}

	validBinding := map[string]any{
		"id": schemaUUID7(t), "user_id": userID, "layer_ref_kind": "builtin", "layer_ref": "builtin:default",
		"trigger_type": "hotkey", "trigger_spec": `{}`, "command_id": "command:test", "arguments": `{}`, "condition": `{}`, "effect": "execute", "enabled": true,
		"source": "builtin", "resolution_priority": 1, "replaces_default_id": "command:test", "replaces_default_version": "1", "replaces_default_fingerprint": "fp", "review_status": "active", "presentation": `{}`,
	}
	if err := db.Table("command_bindings").Create(validBinding).Error; err != nil {
		t.Fatal(err)
	}

	invalids := []map[string]any{
		{"id": schemaUUID7(t), "user_id": userID, "layer_ref_kind": "builtin", "layer_ref": "builtin:x", "trigger_type": "hotkey", "trigger_spec": `{}`, "arguments": `{}`, "condition": `{}`, "effect": "execute", "enabled": true, "source": "x", "resolution_priority": 1, "review_status": "active", "presentation": `{}`},
		{"id": schemaUUID7(t), "user_id": userID, "layer_ref_kind": "user", "layer_ref": "x", "trigger_type": "hotkey", "trigger_spec": `[]`, "arguments": `{}`, "condition": `{}`, "effect": "execute", "enabled": true, "source": "x", "resolution_priority": 1, "review_status": "active", "presentation": `{}`},
		{"id": schemaUUID7(t), "user_id": userID, "layer_ref_kind": "user", "layer_ref": "x", "trigger_type": "hotkey", "trigger_spec": `{}`, "arguments": `{"x":1}`, "condition": `{}`, "effect": "suppress", "enabled": true, "source": "x", "resolution_priority": 1, "review_status": "active", "presentation": `{}`},
	}
	for i, row := range invalids {
		if err := db.Table("command_bindings").Create(row).Error; err == nil {
			t.Fatalf("binding inválido %d foi aceito", i)
		}
	}
}

func TestMigrateRollsBackOnFailure(t *testing.T) {
	db := schemaTestDB(t)
	if err := db.Exec("CREATE TABLE conflict_holder (value TEXT); CREATE INDEX ux_command_layers_user_global_name ON conflict_holder (value)").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err == nil {
		t.Fatal("migração deveria falhar com índice conflitante")
	}
	for _, table := range []string{"command_layers", "command_bindings", "command_config_generations"} {
		var count int
		if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s deveria ter sido revertida junto com a migração", table)
		}
	}
}

func TestMigrateRejectsSameTableWrongIndex(t *testing.T) {
	db := schemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP INDEX ux_command_layers_user_global_name").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX ux_command_layers_user_global_name ON command_layers (name)").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err == nil {
		t.Fatal("migração deveria rejeitar definição errada do índice na mesma tabela")
	}
}

func TestMigrateHonorsContext(t *testing.T) {
	db := schemaTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Migrate(ctx, db); !errors.Is(err, context.Canceled) {
		t.Fatalf("erro de contexto = %v", err)
	}
}
