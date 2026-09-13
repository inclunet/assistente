package database

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestHydrationAndTaskListIndexMigrationAndQueryPlans(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&TaskList{}, &Task{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	if err := ensureHydrationAndTaskListIndexes(testDB); err != nil {
		t.Fatal(err)
	}
	if err := ensureHydrationAndTaskListIndexes(testDB); err != nil {
		t.Fatalf("migração deve ser idempotente: %v", err)
	}

	assertQueryPlanUsesIndex(t, testDB,
		`EXPLAIN QUERY PLAN
		 SELECT tool_invocations.id
		   FROM tool_invocations
		  WHERE user_id = 'user-a'
		    AND origin_type = 'chat'
		    AND origin_id IN ('turn-a', 'turn-b')
		    AND tool_call_id <> ''
		    AND (completed_at IS NOT NULL OR status IN ('queued','running','succeeded','failed','cancelled','timed_out'))
		  ORDER BY queued_at ASC, id ASC
		  LIMIT 2000`,
		"idx_tool_invocations_user_origin_queue",
	)
	assertQueryPlanUsesIndex(t, testDB,
		`EXPLAIN QUERY PLAN
		 SELECT *
		   FROM tasks
		  WHERE task_list_id = 'list-a' AND parent_id IS NULL
		  ORDER BY "order" ASC, id ASC
		  LIMIT 101`,
		"idx_tasks_list_parent_order",
	)

	finalMigration := schemaMigrations[len(schemaMigrations)-1]
	if finalMigration.Version != 17 || finalMigration.Name != "hydration_tasklist_query_indexes" {
		t.Fatalf("migração final inesperada: v%d %q", finalMigration.Version, finalMigration.Name)
	}
	if err := runMigrationList(testDB, phasePostAutoMigrate, []migration{finalMigration}); err != nil {
		t.Fatalf("aplicar migração v17: %v", err)
	}
	applied, err := appliedMigrationVersions(testDB)
	if err != nil {
		t.Fatal(err)
	}
	if !applied[17] {
		t.Fatal("migração v17 não foi registrada")
	}
}

func assertQueryPlanUsesIndex(t *testing.T, database *gorm.DB, query, index string) {
	t.Helper()
	var rows []struct {
		Detail string
	}
	if err := database.Raw(query).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	var details []string
	for _, row := range rows {
		details = append(details, row.Detail)
	}
	plan := strings.Join(details, "\n")
	if !strings.Contains(plan, index) {
		t.Fatalf("plano não usa %s:\n%s", index, plan)
	}
	if strings.Contains(plan, "SCAN tool_invocations") || strings.Contains(plan, "SCAN tasks") {
		t.Fatalf("plano contém full scan:\n%s", plan)
	}
}
