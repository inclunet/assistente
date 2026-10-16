package database

import "testing"

func TestCommandJobQueuedAtBackfillPreservesLegacyFacts(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("CREATE TABLE job_runs (id TEXT PRIMARY KEY, started_at DATETIME NOT NULL, trigger_data TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO job_runs VALUES ('opaque-run', '2026-09-01 12:00:00', '{}')").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateCommandJobQueuedAt(db); err != nil {
			t.Fatal(err)
		}
	}
	var row struct {
		ID, TriggerData string
		Same            bool
	}
	if err := db.Raw("SELECT id, trigger_data, queued_at = started_at AS same FROM job_runs").Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ID != "opaque-run" || row.TriggerData != "{}" || !row.Same {
		t.Fatalf("backfill alterou identidade/proveniência: %+v", row)
	}
}

func TestCommandJobQueuedAtBackfillRejectsUnknownTimeAtomically(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("CREATE TABLE job_runs (id TEXT PRIMARY KEY, started_at DATETIME)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO job_runs VALUES ('legacy', NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateCommandJobQueuedAt(db); err == nil {
		t.Fatal("timestamp inventado")
	}
	if db.Migrator().HasColumn("job_runs", "queued_at") {
		t.Fatal("DDL parcial após rollback")
	}
}
