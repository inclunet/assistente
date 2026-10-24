package database

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestToolParentRetentionIndexUpgradeAndCleanupPlan(t *testing.T) {
	db := newMigratorTestDB(t)
	// Subconjunto do schema publicado, incluindo a FK e os índices concorrentes.
	for _, sql := range []string{
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE tool_invocations (id TEXT PRIMARY KEY, user_id TEXT NOT NULL,
		 origin_type TEXT, origin_id TEXT, parent_invocation_id TEXT REFERENCES tool_invocations(id), updated_at DATETIME)`,
		`CREATE INDEX idx_tool_invocations_parent_invocation_id ON tool_invocations(parent_invocation_id)`,
		`CREATE INDEX idx_tool_invocations_user_origin ON tool_invocations(user_id,origin_type,origin_id)`,
		`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id) VALUES ('parent','a','job_run','old')`,
		`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id,parent_invocation_id)
		 VALUES ('child','a','chat','surviving','parent'), ('other','b','job_run','old',NULL)`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < 2000; i++ {
			if err := tx.Exec(`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id)
			 VALUES (?,'a','job_run','unrelated')`, fmt.Sprintf("unrelated-%d", i)).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var migration36 migration
	for _, m := range schemaMigrations {
		if m.Version == 36 {
			migration36 = m
		}
	}
	if migration36.Run == nil {
		t.Fatal("migração 36 ausente")
	}
	for i := 0; i < 2; i++ {
		if err := runMigrationList(db, phasePostAutoMigrate, []migration{migration36}); err != nil {
			t.Fatal(err)
		}
		// Simula também reexecução após crash antes do registro da migração.
		if err := migration36.Run(db); err != nil {
			t.Fatal(err)
		}
	}
	var versions int64
	if err := db.Table("schema_migrations").Where("version = 36").Count(&versions).Error; err != nil || versions != 1 {
		t.Fatalf("registro da migração: %d, %v", versions, err)
	}
	var capturedSQL string
	var capturedArgs []any
	if err := db.Callback().Update().After("gorm:update").Register("capture_retention_update", func(tx *gorm.DB) {
		capturedSQL = tx.Statement.SQL.String()
		capturedArgs = append([]any(nil), tx.Statement.Vars...)
	}); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "a")
	if err := db.Transaction(func(tx *gorm.DB) error {
		target := ScopeByUser(ctx, tx.Model(&ToolInvocation{}), "user_id").Where("origin_type = ? AND origin_id = ?", "job_run", "old")
		n, err := DeleteToolInvocationsWithChildrenDetachedTx(ctx, tx, target)
		if err == nil && n != 1 {
			return fmt.Errorf("deleted=%d", n)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if capturedSQL == "" {
		t.Fatal("UPDATE produtivo não observado")
	}
	var plan []struct{ Detail string }
	if err := db.Raw("EXPLAIN QUERY PLAN "+capturedSQL, capturedArgs...).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	indexed := false
	for _, step := range plan {
		t.Log(step.Detail)
		if strings.Contains(step.Detail, "idx_tool_invocations_parent_user") && strings.Contains(step.Detail, "parent_invocation_id=? AND user_id=?") {
			indexed = true
		}
	}
	if !indexed {
		t.Fatal("limpeza não busca filhos por pai e usuário")
	}
	var child ToolInvocation
	if err := db.First(&child, "id = ?", "child").Error; err != nil || child.ParentInvocationID != nil {
		t.Fatalf("filha sobrevivente: %+v, %v", child, err)
	}
	var remaining int64
	if err := db.Table("tool_invocations").Count(&remaining).Error; err != nil || remaining != 2002 {
		t.Fatalf("remaining=%d err=%v", remaining, err)
	}
	var violations []map[string]any
	if err := db.Raw("PRAGMA foreign_key_check").Scan(&violations).Error; err != nil || len(violations) != 0 {
		t.Fatalf("FKs: %v, %v", violations, err)
	}
}
