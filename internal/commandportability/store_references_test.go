package commandportability

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func referenceStoreFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"command_layers", "command_bindings", "command_layer_activation_rules"} {
		if err := db.Exec("CREATE TABLE " + table + " (id TEXT, user_id TEXT NOT NULL, workspace_id TEXT, name TEXT)").Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestStoreOwnershipScopesAllowlistedReferences(t *testing.T) {
	db := referenceStoreFixture(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	rows := []struct {
		table, id, user, workspace string
	}{
		{"command_layers", "layer-a", "user-a", "ws-a"},
		{"command_bindings", "binding-a", "user-a", "ws-a"},
		{"command_layer_activation_rules", "rule-a", "user-a", "ws-a"},
		{"command_layers", "layer-foreign", "user-b", "ws-a"},
		{"command_layers", "layer-other-workspace", "user-a", "ws-b"},
		{"command_bindings", "duplicate-id", "user-a", "ws-a"},
		{"command_layers", "duplicate-id", "user-a", "ws-a"},
	}
	for _, row := range rows {
		if err := db.Table(row.table).Create(map[string]any{"id": row.id, "user_id": row.user, "workspace_id": row.workspace}).Error; err != nil {
			t.Fatal(err)
		}
	}
	port := NewStoreOwnership(db)
	for _, tc := range []struct {
		name, id, workspace string
		want                Ownership
	}{
		{"layer", "layer-a", "ws-a", CurrentUserOwner},
		{"binding", "binding-a", "ws-a", CurrentUserOwner},
		{"rule", "rule-a", "ws-a", CurrentUserOwner},
		{"foreign", "layer-foreign", "ws-a", ForeignUserOwner},
		{"same owner other workspace", "layer-other-workspace", "ws-a", AmbiguousOwnership},
		{"same exact ID in two tables", "duplicate-id", "ws-a", AmbiguousOwnership},
		{"missing", "missing", "ws-a", AbsentOwner},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := port(ctx, tc.workspace, tc.id)
			if err != nil || got != tc.want {
				t.Fatalf("ownership=%q err=%v, want %q", got, err, tc.want)
			}
		})
	}
	if _, err := port(ctx, "ws-a", ""); !errors.Is(err, ErrReferenceStoreUnavailable) {
		t.Fatalf("ID vazio não falhou fechado: %v", err)
	}
	if _, err := port(context.Background(), "ws-a", "layer-a"); err == nil {
		t.Fatal("owner ausente foi aceito")
	}
}

func TestStoreLayerNameScopesOwnerAndWorkspace(t *testing.T) {
	db := referenceStoreFixture(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	rows := []map[string]any{
		{"id": "layer-a", "user_id": "user-a", "workspace_id": "ws-a", "name": "shared"},
		{"id": "layer-b", "user_id": "user-b", "workspace_id": "ws-a", "name": "foreign"},
		{"id": "layer-c", "user_id": "user-a", "workspace_id": "ws-b", "name": "other-scope"},
	}
	for _, row := range rows {
		if err := db.Table("command_layers").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	port := NewStoreLayerName(db)
	checks := []struct {
		name, workspace string
		want            Ownership
	}{
		{"shared", "ws-a", CurrentUserOwner},
		{"foreign", "ws-a", AbsentOwner},
		{"other-scope", "ws-a", AbsentOwner},
		{"missing", "ws-a", AbsentOwner},
	}
	for _, tc := range checks {
		got, err := port(ctx, tc.workspace, tc.name)
		if err != nil || got != tc.want {
			t.Fatalf("name %q ownership=%q err=%v, want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := port(ctx, "ws-a", " "); !errors.Is(err, ErrReferenceStoreUnavailable) {
		t.Fatalf("nome inválido não falhou fechado: %v", err)
	}
}
