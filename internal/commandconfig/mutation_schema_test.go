package commandconfig

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mutationSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"mutation_id":         schemaUUID7(t),
		"schema_version":      1,
		"user_id":             schemaUUID7(t),
		"session_id":          schemaUUID7(t),
		"scope":               "global",
		"operation":           "binding_enabled",
		"binding_id":          schemaUUID7(t),
		"decision_id":         schemaUUID7(t),
		"request_fingerprint": "v1:fingerprint",
		"auth_generation":     "auth-1",
		"security_generation": "security-1",
		"generation_id":       schemaUUID7(t),
		"before_generation":   int64(41),
		"after_generation":    int64(42),
		"before_enabled":      true,
		"after_enabled":       false,
		"occurred_at":         time.Now().UTC(),
	}
}

func TestMutationAuditSchemaConstraints(t *testing.T) {
	db := schemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	base := mutationSchemaFixture(t)
	if err := db.Table("command_config_mutations").Create(base).Error; err != nil {
		t.Fatalf("fixture válida rejeitada: %v", err)
	}

	cases := map[string]func(map[string]any){
		"nil PK": func(row map[string]any) {
			row["mutation_id"] = nil
		},
		"UUID errado mutation": func(row map[string]any) {
			row["mutation_id"] = uuid.NewString()
		},
		"UUID errado user": func(row map[string]any) {
			row["user_id"] = uuid.NewString()
		},
		"UUID errado session": func(row map[string]any) {
			row["session_id"] = uuid.NewString()
		},
		"UUID errado binding": func(row map[string]any) {
			row["binding_id"] = uuid.NewString()
		},
		"UUID errado decision": func(row map[string]any) {
			row["decision_id"] = uuid.NewString()
		},
		"UUID errado generation": func(row map[string]any) {
			row["generation_id"] = uuid.NewString()
		},
		"mutation PK duplicada": func(row map[string]any) {
			row["mutation_id"] = base["mutation_id"]
		},
		"decision única duplicada": func(row map[string]any) {
			row["decision_id"] = base["decision_id"]
		},
		"noop booleano": func(row map[string]any) {
			row["after_enabled"] = row["before_enabled"]
		},
		"after generation errada": func(row map[string]any) {
			row["after_generation"] = int64(43)
		},
		"overflow de generation": func(row map[string]any) {
			row["before_generation"] = int64(1<<63 - 1)
		},
		"schema version errada": func(row map[string]any) {
			row["schema_version"] = 2
		},
		"scope errado": func(row map[string]any) {
			row["scope"] = "workspace"
		},
		"operation errada": func(row map[string]any) {
			row["operation"] = "binding_created"
		},
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			row := mutationSchemaFixture(t)
			change(row)
			if err := db.Table("command_config_mutations").Create(row).Error; err == nil {
				t.Fatal("constraint ausente")
			}
		})
	}
}

func TestMutationAuditMigrateIsIdempotentWithData(t *testing.T) {
	db := schemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	row := mutationSchemaFixture(t)
	if err := db.Table("command_config_mutations").Create(row).Error; err != nil {
		t.Fatalf("inserção da auditoria válida: %v", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migração idempotente com dados falhou: %v", err)
	}

	var count int
	if err := db.Raw("SELECT COUNT(*) FROM command_config_mutations WHERE mutation_id = ?", row["mutation_id"]).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("auditoria existente não foi preservada: count=%d", count)
	}
}

func TestMutationAuditMigrateRejectsViewOrIncompatibleTablePreservingPreviousData(t *testing.T) {
	tests := map[string]string{
		"view":                "CREATE VIEW command_config_mutations AS SELECT 1 AS mutation_id",
		"tabela incompatível": "CREATE TABLE command_config_mutations (mutation_id TEXT PRIMARY KEY, marker TEXT NOT NULL)",
	}

	for name, objectSQL := range tests {
		t.Run(name, func(t *testing.T) {
			db := schemaTestDB(t)
			if err := db.Exec("CREATE TABLE previous_data (value TEXT NOT NULL)").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("INSERT INTO previous_data (value) VALUES (?)", "preservar").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec(objectSQL).Error; err != nil {
				t.Fatal(err)
			}

			if err := Migrate(context.Background(), db); err == nil {
				t.Fatal("migração deveria rejeitar o objeto incompatível")
			}

			var value string
			if err := db.Raw("SELECT value FROM previous_data").Scan(&value).Error; err != nil {
				t.Fatal(err)
			}
			if value != "preservar" {
				t.Fatalf("dados anteriores alterados: %q", value)
			}

			var objectType string
			if err := db.Raw("SELECT type FROM sqlite_master WHERE name = ?", "command_config_mutations").Scan(&objectType).Error; err != nil {
				t.Fatal(err)
			}
			wantType := "view"
			if name == "tabela incompatível" {
				wantType = "table"
			}
			if objectType != wantType {
				t.Fatalf("objeto pré-existente alterado: got=%q want=%q", objectType, wantType)
			}
		})
	}
}

func TestMutationAuditMigrateRollsBackConfigurationCreationOnIncompatibleAudit(t *testing.T) {
	db := schemaTestDB(t)
	if err := db.Exec("CREATE TABLE command_config_mutations (mutation_id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}

	if err := Migrate(context.Background(), db); err == nil {
		t.Fatal("migração deveria falhar com auditoria incompatível")
	}

	for _, table := range []string{"command_layers", "command_bindings", "command_config_generations"} {
		var count int
		if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s foi criada apesar do rollback", table)
		}
	}
	var indexCount int
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name LIKE 'ux_command_%'").Scan(&indexCount).Error; err != nil {
		t.Fatal(err)
	}
	if indexCount != 0 {
		t.Fatalf("índices da migração sobreviveram ao rollback: %d", indexCount)
	}
}
