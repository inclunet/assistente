package commandbootstrap

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"github.com/google/uuid"
)

func TestV26UpgradePreservesAuditAndAddsImportOperation(t *testing.T) {
	ctx := context.Background()
	reference := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "current.db"))
	if err := Migrate(ctx, reference); err != nil {
		t.Fatal(err)
	}
	rows, err := objects(reference)
	if err != nil {
		t.Fatal(err)
	}
	db := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "v26.db"))
	for _, row := range rows {
		if err := db.Exec(legacyImportObject(row).SQL).Error; err != nil {
			t.Fatalf("%s: %v", row.Name, err)
		}
	}
	if err := db.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at DATETIME NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for version, name := range map[int]string{21: "command_storage_initial", 22: "command_envelope_ownership", 23: "command_config_complete", 24: "command_activation_durable", 27: "command_job_activation_consumer"} {
		if err := db.Exec("INSERT INTO schema_migrations VALUES(?,?,?)", version, name, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	mutation := id()
	row := map[string]any{"mutation_id": mutation, "schema_version": 2, "user_id": id(), "session_id": id(), "scope": "global", "operation": "rule_create", "decision_id": id(), "request_fingerprint": "preserved", "auth_generation": "1", "security_generation": "1", "generation_id": id(), "before_generation": 1, "after_generation": 2, "before_document": `{"version":2,"old":true}`, "after_document": `{"version":2,"new":true}`, "occurred_at": now}
	if err := db.Table("command_config_mutations").Create(row).Error; err != nil {
		t.Fatal(err)
	}
	if err := commandconfig.Migrate(ctx, db); err == nil {
		t.Fatal("v26 adotada sem migração explícita")
	}
	for range 2 {
		if err := Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	var saved struct{ BeforeDocument, AfterDocument, RequestFingerprint string }
	if err := db.Table("command_config_mutations").Where("mutation_id = ?", mutation).Take(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.BeforeDocument != row["before_document"] || saved.AfterDocument != row["after_document"] || saved.RequestFingerprint != "preserved" {
		t.Fatalf("auditoria alterada: %+v", saved)
	}
	var stamps int64
	if err := db.Table("schema_migrations").Where("version = 28 AND name = ?", "command_config_import_audit").Count(&stamps).Error; err != nil || stamps != 1 {
		t.Fatalf("carimbo=%d err=%v", stamps, err)
	}
	row["mutation_id"], row["decision_id"], row["operation"] = id(), id(), "config_import"
	delete(row, "@id") // GORM pode preencher o rowid auxiliar no map após Create.
	if err := db.Table("command_config_mutations").Create(row).Error; err != nil {
		t.Fatalf("novo verbo recusado: %v", err)
	}
}
