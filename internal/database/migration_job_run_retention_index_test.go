package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestJobRunRetentionIndexTargetsBatchAndPreservesForeignKeys(t *testing.T) {
	db := newMigratorTestDB(t)
	for _, statement := range []string{
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE job_runs (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, started_at DATETIME)`,
		`CREATE INDEX idx_job_runs_user_started_at ON job_runs(user_id, started_at)`,
		`CREATE TABLE job_events (id TEXT PRIMARY KEY, job_run_id TEXT REFERENCES job_runs(id))`,
		`CREATE INDEX idx_job_events_job_run_id ON job_events(job_run_id)`,
		`INSERT INTO job_runs VALUES ('other','b',NULL), ('protected','a',NULL)`,
		`INSERT INTO job_events VALUES ('event','protected')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{"other"}
	for i := 0; i < 128; i++ {
		id := fmt.Sprintf("run-%03d", i)
		ids = append(ids, id)
		if err := db.Exec(`INSERT INTO job_runs VALUES (?,'a',NULL)`, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := ensureJobRunRetentionIndex(db); err != nil {
			t.Fatal(err)
		}
	}
	ctx := WithUserID(context.Background(), "a")
	query := ScopeByUser(ctx, db, "user_id").Where("id IN ?", ids)
	stmt := query.Session(&gorm.Session{DryRun: true}).Delete(&JobRun{}).Statement
	var plan []struct{ Detail string }
	if err := db.Raw("EXPLAIN QUERY PLAN "+stmt.SQL.String(), stmt.Vars...).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	indexed := false
	for _, step := range plan {
		t.Log(step.Detail)
		if strings.Contains(step.Detail, "idx_job_runs_user_id_id") && strings.Contains(step.Detail, "user_id=? AND id=?") {
			indexed = true
		}
	}
	if !indexed {
		t.Fatal("exclusão deve buscar usuário e IDs do lote, sem varrer o histórico")
	}
	res := query.Delete(&JobRun{})
	if res.Error != nil || res.RowsAffected != 128 {
		t.Fatalf("deleted=%d err=%v", res.RowsAffected, res.Error)
	}
	if err := db.Exec(`DELETE FROM job_runs WHERE id='protected'`).Error; err == nil {
		t.Fatal("FK deixou de proteger o run referenciado")
	}
	var remaining int64
	if err := db.Table("job_runs").Count(&remaining).Error; err != nil || remaining != 2 {
		t.Fatalf("restantes=%d err=%v", remaining, err)
	}
}

func TestJobRunRetentionIndexDefersFailureAndRetries(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec(`CREATE TABLE job_runs (id TEXT PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	var migration37 migration
	for _, m := range schemaMigrations {
		if m.Version == 37 {
			migration37 = m
		}
	}
	if migration37.Run == nil {
		t.Fatal("migração 37 ausente")
	}
	if err := migration37.Run(db); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("falha de criação deve adiar migração: %v", err)
	}
	if err := runMigrationList(db, phasePostAutoMigrate, []migration{migration37}); err != nil {
		t.Fatalf("migração adiada não deve abortar inicialização: %v", err)
	}
	var versions int64
	if err := db.Table("schema_migrations").Where("version = 37").Count(&versions).Error; err != nil || versions != 0 {
		t.Fatalf("migração adiada registrada: %d, %v", versions, err)
	}
	if err := db.Exec(`ALTER TABLE job_runs ADD COLUMN user_id TEXT`).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := runMigrationList(db, phasePostAutoMigrate, []migration{migration37}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Table("schema_migrations").Where("version = 37").Count(&versions).Error; err != nil || versions != 1 {
		t.Fatalf("migração recuperada deve ser registrada uma vez: %d, %v", versions, err)
	}
	var indexes int64
	if err := db.Raw(`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_job_runs_user_id_id'`).Scan(&indexes).Error; err != nil || indexes != 1 {
		t.Fatalf("índice não criado após recuperação: %d, %v", indexes, err)
	}
}
