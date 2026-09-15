package commandactivation

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const uuid7Check = "(length(%s) = 36 AND length(replace(%s, '-', '')) = 32 AND lower(%s) = %s AND %s NOT GLOB '*[^0-9a-f-]*' AND substr(%s, 9, 1) = '-' AND substr(%s, 14, 1) = '-' AND substr(%s, 19, 1) = '-' AND substr(%s, 24, 1) = '-' AND substr(%s, 15, 1) = '7' AND substr(%s, 20, 1) IN ('8', '9', 'a', 'b'))"

func uuid7Expr(column string) string {
	return fmt.Sprintf(uuid7Check, column, column, column, column, column, column, column, column, column, column, column)
}

// Migrate cria somente o armazenamento de ativação. A conexão é explícita e
// a migração não registra modelos no App nem toca commandconfig/database.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if ctx == nil || db == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"command_layer_activation_rules", "command_layer_activation_state", "command_activation_idempotency_keys", "command_layer_activation_generations"} {
			if err := ensureTable(tx, table); err != nil {
				return err
			}
		}
		statements := []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_layer_activation_rules (
				id TEXT NOT NULL PRIMARY KEY CHECK %s,
				user_id TEXT NOT NULL CHECK %s,
				workspace_id TEXT CHECK (workspace_id IS NULL OR (length(trim(workspace_id)) BETWEEN 1 AND 256 AND trim(workspace_id) = workspace_id AND instr(workspace_id, char(0)) = 0)),
				layer_ref_kind TEXT NOT NULL CHECK (layer_ref_kind IN ('builtin', 'user')),
				layer_ref TEXT NOT NULL CHECK (length(trim(layer_ref)) > 0),
				rule_ref_kind TEXT NOT NULL CHECK (rule_ref_kind IN ('builtin', 'user')),
				rule_ref TEXT NOT NULL CHECK (length(trim(rule_ref)) > 0),
				mode TEXT NOT NULL CHECK (mode IN ('always', 'context', 'condition', 'manual', 'toggle', 'event')),
				condition TEXT NOT NULL CHECK (json_valid(condition) AND json_type(condition) = 'object'),
				lifecycle TEXT NOT NULL CHECK (lifecycle IN ('persistent', 'session', 'temporary')),
				event_name TEXT,
				allowed_internal_producer_types TEXT,
				authorization_decision_id TEXT,
				automation_grant_id TEXT,
				automation_grant_generation INTEGER CHECK (automation_grant_generation IS NULL OR automation_grant_generation >= 0),
				automation_grant_fingerprint TEXT,
				enabled BOOLEAN NOT NULL CHECK (enabled IN (0, 1)),
				source TEXT NOT NULL CHECK (length(trim(source)) > 0),
				replaces_default_id TEXT,
				replaces_default_version TEXT,
				replaces_default_fingerprint TEXT,
				review_status TEXT NOT NULL CHECK (review_status IN ('active', 'needs_review')),
				CHECK ((replaces_default_id IS NULL AND replaces_default_version IS NULL AND replaces_default_fingerprint IS NULL) OR (replaces_default_id IS NOT NULL AND replaces_default_version IS NOT NULL AND replaces_default_fingerprint IS NOT NULL AND length(trim(replaces_default_id)) > 0 AND length(trim(replaces_default_version)) > 0 AND length(trim(replaces_default_fingerprint)) > 0))
			)`, uuid7Expr("id"), uuid7Expr("user_id")),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_layer_activation_state (
				activation_id TEXT NOT NULL PRIMARY KEY CHECK %s,
				layer_ref_kind TEXT NOT NULL CHECK (layer_ref_kind IN ('builtin', 'user')),
				layer_ref TEXT NOT NULL CHECK (length(trim(layer_ref)) > 0),
				rule_ref_kind TEXT NOT NULL CHECK (rule_ref_kind IN ('builtin', 'user')),
				rule_ref TEXT NOT NULL CHECK (length(trim(rule_ref)) > 0),
				user_id TEXT NOT NULL CHECK %s,
				workspace_id TEXT CHECK (workspace_id IS NULL OR (length(trim(workspace_id)) BETWEEN 1 AND 256 AND trim(workspace_id) = workspace_id AND instr(workspace_id, char(0)) = 0)),
				auth_context_type TEXT NOT NULL,
				auth_context_id TEXT NOT NULL,
				auth_generation TEXT NOT NULL,
				security_generation TEXT NOT NULL,
				source_type TEXT NOT NULL,
				source_instance_id TEXT,
				source_event_id TEXT,
				source_correlation_id TEXT,
				sequence INTEGER,
				source_job_database_id TEXT,
				source_job_slug TEXT,
				event_fingerprint TEXT,
				source_replay_policy_generation TEXT,
				source_replay_deadline DATETIME,
				state TEXT NOT NULL CHECK (state IN ('active', 'inactive', 'deactivated', 'expired', 'stale')),
				terminal_reason TEXT,
				provenance TEXT,
				manual_stack_key TEXT,
				activated_at DATETIME NOT NULL,
				expires_at DATETIME,
				updated_at DATETIME NOT NULL
			)`, uuid7Expr("activation_id"), uuid7Expr("user_id")),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_activation_idempotency_keys (
				id TEXT NOT NULL PRIMARY KEY CHECK %s,
				key TEXT NOT NULL UNIQUE,
				user_id TEXT NOT NULL CHECK %s,
				workspace_id TEXT CHECK (workspace_id IS NULL OR (length(trim(workspace_id)) BETWEEN 1 AND 256 AND trim(workspace_id) = workspace_id AND instr(workspace_id, char(0)) = 0)),
				rule_ref_kind TEXT NOT NULL CHECK (rule_ref_kind IN ('builtin', 'user')),
				rule_ref TEXT NOT NULL CHECK (length(trim(rule_ref)) > 0),
				source_type TEXT NOT NULL,
				source_instance_id TEXT NOT NULL,
				source_event_id TEXT NOT NULL,
				source_correlation_id TEXT,
				sequence INTEGER NOT NULL,
				event_fingerprint TEXT NOT NULL,
				source_job_database_id TEXT,
				source_job_slug TEXT,
				source_replay_policy_generation TEXT NOT NULL,
				source_replay_deadline DATETIME NOT NULL,
				terminal_state TEXT NOT NULL,
				created_at DATETIME NOT NULL,
				 expires_at DATETIME NOT NULL
			)`, uuid7Expr("id"), uuid7Expr("user_id")),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_layer_activation_generations (
				id TEXT NOT NULL PRIMARY KEY CHECK %s,
				user_id TEXT NOT NULL CHECK %s,
				workspace_id TEXT CHECK (workspace_id IS NULL OR (length(trim(workspace_id)) BETWEEN 1 AND 256 AND trim(workspace_id) = workspace_id AND instr(workspace_id, char(0)) = 0)),
				generation INTEGER NOT NULL CHECK (typeof(generation) = 'integer' AND generation >= 1),
				updated_at DATETIME NOT NULL
			)`, uuid7Expr("id"), uuid7Expr("user_id")),
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		indexes := []string{
			"CREATE INDEX IF NOT EXISTS ix_activation_rules_scope ON command_layer_activation_rules (user_id, workspace_id, layer_ref_kind, layer_ref, enabled)",
			"CREATE INDEX IF NOT EXISTS ix_activation_state_scope ON command_layer_activation_state (user_id, workspace_id, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref, state)",
			"CREATE INDEX IF NOT EXISTS ix_activation_state_manual_stack ON command_layer_activation_state (user_id, workspace_id, manual_stack_key, activated_at, activation_id)",
			"CREATE UNIQUE INDEX IF NOT EXISTS ux_activation_generations_user_global ON command_layer_activation_generations (user_id) WHERE workspace_id IS NULL",
			"CREATE UNIQUE INDEX IF NOT EXISTS ux_activation_generations_user_workspace ON command_layer_activation_generations (user_id, workspace_id) WHERE workspace_id IS NOT NULL",
		}
		for _, statement := range indexes {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ensureTable(tx *gorm.DB, name string) error {
	var objectType string
	if err := tx.Raw("SELECT type FROM sqlite_master WHERE name = ?", name).Scan(&objectType).Error; err != nil {
		return err
	}
	if objectType != "" && objectType != "table" {
		return fmt.Errorf("commandactivation: objeto %s não é uma tabela", name)
	}
	return nil
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}
