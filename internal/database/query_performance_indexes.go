package database

import (
	"fmt"

	"gorm.io/gorm"
)

// ensureHydrationAndTaskListIndexes mantém os índices dos hot paths medidos na
// issue #739. A migração versionada chama esta função para bancos existentes;
// CREATE INDEX IF NOT EXISTS também torna a execução segura após crash.
func ensureHydrationAndTaskListIndexes(database *gorm.DB) error {
	if database == nil {
		return nil
	}
	statements := []struct {
		table string
		sql   string
	}{
		{
			table: "tool_invocations",
			sql:   `CREATE INDEX IF NOT EXISTS idx_tool_invocations_user_origin_queue ON tool_invocations (user_id, origin_type, origin_id, queued_at, id)`,
		},
		{
			table: "tasks",
			sql:   `CREATE INDEX IF NOT EXISTS idx_tasks_list_parent_order ON tasks (task_list_id, parent_id, "order", id)`,
		},
	}
	for _, statement := range statements {
		if !database.Migrator().HasTable(statement.table) {
			continue
		}
		if err := database.Exec(statement.sql).Error; err != nil {
			return fmt.Errorf("criar índice de performance de %s: %w", statement.table, err)
		}
	}
	return nil
}
