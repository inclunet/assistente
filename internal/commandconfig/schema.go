package commandconfig

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const uuid7CanonicalSQL = "(length(%s) = 36 AND length(replace(%s, '-', '')) = 32 AND lower(%s) = %s AND %s NOT GLOB '*[^0-9a-f-]*' AND substr(%s, 9, 1) = '-' AND substr(%s, 14, 1) = '-' AND substr(%s, 19, 1) = '-' AND substr(%s, 24, 1) = '-' AND substr(%s, 15, 1) = '7' AND substr(%s, 20, 1) IN ('8', '9', 'a', 'b'))"

func uuid7Check(column string) string {
	return fmt.Sprintf(uuid7CanonicalSQL, column, column, column, column, column, column, column, column, column, column, column)
}

// Migrate cria somente as tabelas de configuração de comandos. A migração não
// registra modelos no App nem acessa o banco global; o chamador fornece a
// conexão explicitamente.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("commandconfig: banco nil")
	}
	if ctx == nil {
		return fmt.Errorf("commandconfig: contexto nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"command_layers", "command_bindings", "command_config_generations"} {
			var objectType string
			if err := tx.Raw("SELECT type FROM sqlite_master WHERE name=?", table).Scan(&objectType).Error; err != nil {
				return err
			}
			if objectType != "" && objectType != "table" {
				return fmt.Errorf("commandconfig: objeto %s não é uma tabela", table)
			}
		}
		statements := []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_layers (
				id TEXT NOT NULL PRIMARY KEY CHECK %s,
				user_id TEXT NOT NULL CHECK %s,
				workspace_id TEXT CHECK (workspace_id IS NULL OR %s),
				name TEXT NOT NULL CHECK (length(trim(name)) > 0),
				description TEXT NOT NULL,
				enabled BOOLEAN NOT NULL CHECK (enabled IN (0, 1)),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				resolution_priority INTEGER NOT NULL,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL
			)`, uuid7Check("id"), uuid7Check("user_id"), uuid7Check("workspace_id")),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_bindings (
				id TEXT NOT NULL PRIMARY KEY CHECK %s,
				user_id TEXT NOT NULL CHECK %s,
				workspace_id TEXT CHECK (workspace_id IS NULL OR %s),
				layer_ref_kind TEXT NOT NULL CHECK (layer_ref_kind IN ('builtin', 'user')),
				layer_ref TEXT NOT NULL CHECK (length(trim(layer_ref)) > 0),
				trigger_type TEXT NOT NULL CHECK (length(trim(trigger_type)) > 0),
				trigger_spec TEXT NOT NULL CHECK (json_valid(trigger_spec) AND json_type(trigger_spec) = 'object'),
				command_id TEXT CHECK (command_id IS NULL OR length(trim(command_id)) > 0),
				arguments TEXT NOT NULL CHECK (json_valid(arguments) AND json_type(arguments) = 'object'),
				condition TEXT NOT NULL CHECK (json_valid(condition) AND json_type(condition) = 'object'),
				effect TEXT NOT NULL CHECK (effect IN ('execute', 'suppress')),
				enabled BOOLEAN NOT NULL CHECK (enabled IN (0, 1)),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				resolution_priority INTEGER NOT NULL,
				replaces_default_id TEXT,
				replaces_default_version TEXT,
				replaces_default_fingerprint TEXT,
				review_status TEXT NOT NULL CHECK (review_status IN ('active', 'needs_review')),
				presentation TEXT NOT NULL CHECK (json_valid(presentation) AND json_type(presentation) = 'object'),
				CHECK (
					(replaces_default_id IS NULL AND replaces_default_version IS NULL AND replaces_default_fingerprint IS NULL)
					OR (replaces_default_id IS NOT NULL AND replaces_default_version IS NOT NULL AND replaces_default_fingerprint IS NOT NULL AND length(trim(replaces_default_id)) > 0 AND length(trim(replaces_default_version)) > 0 AND length(trim(replaces_default_fingerprint)) > 0)
				),
				CHECK (layer_ref_kind <> 'builtin' OR (replaces_default_id IS NOT NULL AND replaces_default_version IS NOT NULL AND replaces_default_fingerprint IS NOT NULL AND length(trim(replaces_default_id)) > 0 AND length(trim(replaces_default_version)) > 0 AND length(trim(replaces_default_fingerprint)) > 0)),
				CHECK (effect = 'execute' AND command_id IS NOT NULL AND length(trim(command_id)) > 0 OR effect = 'suppress' AND command_id IS NULL),
				CHECK (effect <> 'suppress' OR arguments = '{}'),
				CHECK (effect <> 'suppress' OR (replaces_default_id IS NOT NULL AND length(trim(replaces_default_id)) > 0))
			)`, uuid7Check("id"), uuid7Check("user_id"), uuid7Check("workspace_id")),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_config_generations (
				id TEXT NOT NULL PRIMARY KEY CHECK %s,
				user_id TEXT NOT NULL CHECK %s,
				workspace_id TEXT CHECK (workspace_id IS NULL OR %s),
				generation INTEGER NOT NULL CHECK (typeof(generation) = 'integer' AND generation >= 1),
				updated_at DATETIME NOT NULL
			)`, uuid7Check("id"), uuid7Check("user_id"), uuid7Check("workspace_id")),
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		indexes := []struct {
			name, table, statement string
		}{
			{"ux_command_layers_user_global_name", "command_layers", `CREATE UNIQUE INDEX ux_command_layers_user_global_name ON command_layers (user_id, name) WHERE workspace_id IS NULL`},
			{"ux_command_layers_user_workspace_name", "command_layers", `CREATE UNIQUE INDEX ux_command_layers_user_workspace_name ON command_layers (user_id, workspace_id, name) WHERE workspace_id IS NOT NULL`},
			{"ux_command_config_generations_user_global", "command_config_generations", `CREATE UNIQUE INDEX ux_command_config_generations_user_global ON command_config_generations (user_id) WHERE workspace_id IS NULL`},
			{"ux_command_config_generations_user_workspace", "command_config_generations", `CREATE UNIQUE INDEX ux_command_config_generations_user_workspace ON command_config_generations (user_id, workspace_id) WHERE workspace_id IS NOT NULL`},
		}
		for _, index := range indexes {
			var existing struct {
				Table string `gorm:"column:tbl_name"`
				SQL   string `gorm:"column:sql"`
			}
			if err := tx.Raw("SELECT tbl_name, sql FROM sqlite_master WHERE type='index' AND name=?", index.name).Scan(&existing).Error; err != nil {
				return err
			}
			if existing.Table != "" {
				if existing.Table != index.table {
					return fmt.Errorf("commandconfig: índice %s pertence a %s", index.name, existing.Table)
				}
				if normalizeSQL(existing.SQL) != normalizeSQL(index.statement) {
					return fmt.Errorf("commandconfig: definição incompatível do índice %s", index.name)
				}
				continue
			}
			if err := tx.Exec(index.statement).Error; err != nil {
				return err
			}
		}
		return migrateMutationAudit(tx)
	})
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}
