package commandautomation

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

func grantTableSQL() string {
	return fmt.Sprintf(`CREATE TABLE command_layer_automation_grants (
		id TEXT NOT NULL PRIMARY KEY CHECK %s,
		user_id TEXT NOT NULL CHECK %s,
		workspace_id TEXT CHECK (workspace_id IS NULL OR %s),
		layer_ref_kind TEXT NOT NULL CHECK (layer_ref_kind IN ('builtin', 'user')),
		layer_ref TEXT NOT NULL CHECK (length(trim(layer_ref)) > 0),
		rule_ref_kind TEXT NOT NULL CHECK (rule_ref_kind IN ('builtin', 'user')),
		rule_ref TEXT NOT NULL CHECK (length(trim(rule_ref)) > 0),
		rule_fingerprint TEXT NOT NULL CHECK (length(trim(rule_fingerprint)) BETWEEN 1 AND 256),
		event_name TEXT NOT NULL CHECK (event_name = 'command-context.job-run-state.v1'),
		producer_types_fingerprint TEXT NOT NULL CHECK (length(trim(producer_types_fingerprint)) BETWEEN 1 AND 256),
		automation_grant_generation INTEGER NOT NULL CHECK (typeof(automation_grant_generation) = 'integer' AND automation_grant_generation >= 1),
		automation_grant_fingerprint TEXT NOT NULL CHECK (length(trim(automation_grant_fingerprint)) BETWEEN 1 AND 256),
		authorization_decision_id TEXT NOT NULL CHECK %s,
		granted_at DATETIME NOT NULL,
		granted_by TEXT NOT NULL CHECK (length(trim(granted_by)) BETWEEN 1 AND 256),
		revoked_at DATETIME,
		revoked_by TEXT CHECK (revoked_by IS NULL OR length(trim(revoked_by)) BETWEEN 1 AND 256),
		revocation_reason TEXT CHECK (revocation_reason IS NULL OR length(trim(revocation_reason)) BETWEEN 1 AND 256)
	)`, uuid7Check("id"), uuid7Check("user_id"), "(length(workspace_id) BETWEEN 1 AND 256 AND trim(workspace_id) = workspace_id AND instr(workspace_id, char(0)) = 0)", uuid7Check("authorization_decision_id"))
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

// Migrate cria somente o armazenamento de grants. O chamador fornece a
// conexão e decide quando migrar; não há registro no migrador central.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if ctx == nil || db == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var object struct{ Type string }
		if err := tx.Raw("SELECT type FROM sqlite_master WHERE name = ?", "command_layer_automation_grants").Scan(&object).Error; err != nil {
			return err
		}
		statement := grantTableSQL()
		if object.Type != "" {
			var existing struct{ Type, SQL string }
			if err := tx.Raw("SELECT type, sql FROM sqlite_master WHERE name = ?", "command_layer_automation_grants").Scan(&existing).Error; err != nil {
				return err
			}
			if existing.Type != "table" || normalizeSQL(existing.SQL) != normalizeSQL(statement) {
				return ErrInvalid
			}
		} else if err := tx.Exec(statement).Error; err != nil {
			return err
		}

		indexes := []struct{ name, sql string }{
			{"ux_command_layer_automation_grants_active_global", `CREATE UNIQUE INDEX ux_command_layer_automation_grants_active_global ON command_layer_automation_grants (user_id, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref) WHERE workspace_id IS NULL AND revoked_at IS NULL`},
			{"ux_command_layer_automation_grants_active_workspace", `CREATE UNIQUE INDEX ux_command_layer_automation_grants_active_workspace ON command_layer_automation_grants (user_id, workspace_id, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref) WHERE workspace_id IS NOT NULL AND revoked_at IS NULL`},
			{"ux_command_layer_automation_grants_generation_global", `CREATE UNIQUE INDEX ux_command_layer_automation_grants_generation_global ON command_layer_automation_grants (user_id, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref, automation_grant_generation) WHERE workspace_id IS NULL`},
			{"ux_command_layer_automation_grants_generation_workspace", `CREATE UNIQUE INDEX ux_command_layer_automation_grants_generation_workspace ON command_layer_automation_grants (user_id, workspace_id, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref, automation_grant_generation) WHERE workspace_id IS NOT NULL`},
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
				if existing.Table != "command_layer_automation_grants" || normalizeSQL(existing.SQL) != normalizeSQL(index.sql) {
					return ErrInvalid
				}
				continue
			}
			if err := tx.Exec(index.sql).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
