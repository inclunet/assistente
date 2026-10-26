package database

import (
	"errors"
	"strings"
	"testing"
)

func TestOrphanScanIndexMigrationAndKeysetPlan(t *testing.T) {
	db := newMigratorTestDB(t)
	var m migration
	for _, candidate := range schemaMigrations {
		if candidate.Version == 38 {
			m = candidate
		}
	}
	if m.Run == nil || m.Phase != phasePostAutoMigrate {
		t.Fatal("migration 38 must run after AutoMigrate")
	}
	if err := m.Run(db); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("missing table must defer: %v", err)
	}
	if err := db.Exec(`CREATE TABLE tool_invocations(id TEXT PRIMARY KEY,user_id TEXT,origin_type TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.Run(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`SELECT id FROM tool_invocations WHERE user_id='a' AND origin_type='chat' AND id>'cursor' ORDER BY id LIMIT 129`,
		`SELECT id FROM tool_invocations WHERE user_id='a' AND origin_type='chat' AND id IN ('one','two')`,
	} {
		var plan []struct{ Detail string }
		if err := db.Raw("EXPLAIN QUERY PLAN " + query).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		indexed := false
		for _, step := range plan {
			if strings.Contains(step.Detail, "idx_tool_invocations_orphan_scan") && strings.Contains(step.Detail, "user_id=? AND origin_type=? AND id") {
				indexed = true
			}
			if strings.Contains(step.Detail, "TEMP B-TREE") {
				t.Fatal("page requires sorting full history:", step.Detail)
			}
		}
		if !indexed {
			t.Fatalf("bounded index search missing: %+v", plan)
		}
	}
}
