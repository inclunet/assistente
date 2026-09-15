package commandjobactivation

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Migrate é chamado somente pela migração central. Nenhuma FK com cascade
// sobre runs/eventos: a retenção do produtor não pode apagar deduplicação.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if ctx == nil || db == nil {
		return ErrUnavailable
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS command_job_activation_leases (
			id TEXT NOT NULL PRIMARY KEY CHECK (%s),
			activation_id TEXT NOT NULL UNIQUE CHECK (%s),
			user_id TEXT NOT NULL CHECK (%s),
			run_id TEXT NOT NULL CHECK (length(trim(run_id)) > 0 AND instr(run_id,char(0))=0),
			runtime_generation TEXT NOT NULL CHECK (length(trim(runtime_generation)) > 0),
			expires_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`, uuid7SQL("id"), uuid7SQL("activation_id"), uuid7SQL("user_id"))).Error; err != nil {
			return err
		}
		for _, sql := range []string{
			"CREATE UNIQUE INDEX IF NOT EXISTS ux_activation_rule_global ON command_layer_activation_rules(user_id,rule_ref_kind,rule_ref) WHERE workspace_id IS NULL",
			"CREATE UNIQUE INDEX IF NOT EXISTS ux_activation_rule_workspace ON command_layer_activation_rules(user_id,workspace_id,rule_ref_kind,rule_ref) WHERE workspace_id IS NOT NULL",
			"CREATE INDEX IF NOT EXISTS ix_command_job_lease_run ON command_job_activation_leases(user_id,run_id,expires_at)",
			"CREATE INDEX IF NOT EXISTS ix_command_job_lease_expiry ON command_job_activation_leases(expires_at)",
			"CREATE UNIQUE INDEX IF NOT EXISTS ux_activation_event_global ON command_activation_idempotency_keys(user_id, rule_ref_kind, rule_ref, source_event_id) WHERE workspace_id IS NULL",
			"CREATE UNIQUE INDEX IF NOT EXISTS ux_activation_event_workspace ON command_activation_idempotency_keys(user_id, workspace_id, rule_ref_kind, rule_ref, source_event_id) WHERE workspace_id IS NOT NULL",
			"CREATE INDEX IF NOT EXISTS ix_activation_cycle_global ON command_layer_activation_state(user_id, rule_ref_kind, rule_ref, source_type, source_correlation_id) WHERE workspace_id IS NULL",
			"CREATE INDEX IF NOT EXISTS ix_activation_cycle_workspace ON command_layer_activation_state(user_id, workspace_id, rule_ref_kind, rule_ref, source_type, source_correlation_id) WHERE workspace_id IS NOT NULL",
		} {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func uuid7SQL(column string) string {
	return fmt.Sprintf("length(%[1]s)=36 AND length(replace(%[1]s,'-',''))=32 AND lower(%[1]s)=%[1]s AND %[1]s NOT GLOB '*[^0-9a-f-]*' AND substr(%[1]s,9,1)='-' AND substr(%[1]s,14,1)='-' AND substr(%[1]s,19,1)='-' AND substr(%[1]s,24,1)='-' AND substr(%[1]s,15,1)='7' AND substr(%[1]s,20,1) IN ('8','9','a','b')", column)
}
