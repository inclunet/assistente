package commandbootstrap

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"github.com/google/uuid"
)

func TestKnownV25UpgradePreservesMutationAuditAndAddsRuntimeBarriers(t *testing.T) {
	ctx := context.Background()
	current := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "new.db"))
	if err := Migrate(ctx, current); err != nil {
		t.Fatal(err)
	}
	rows, err := objects(current)
	if err != nil {
		t.Fatal(err)
	}
	legacy := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "v25.db"))
	for _, o := range rows {
		if o.TblName == "command_job_activation_leases" || o.TblName == "command_closed_generations" || strings.HasPrefix(o.Name, "ux_activation_event_") || strings.HasPrefix(o.Name, "ix_activation_cycle_") || strings.HasPrefix(o.Name, "ux_activation_rule_") {
			continue
		}
		if err := legacy.Exec(legacyRulesObject(o).SQL).Error; err != nil {
			t.Fatalf("%s: %v", o.Name, err)
		}
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, session, mutation, decision, generation := id(), id(), id(), id(), id()
	now := time.Now().UTC()
	old := map[string]any{"mutation_id": mutation, "schema_version": 2, "user_id": user, "session_id": session, "scope": "global", "operation": "layer_create", "decision_id": decision, "request_fingerprint": "preserve", "auth_generation": "1", "security_generation": "1", "generation_id": generation, "before_generation": 1, "after_generation": 2, "before_document": `{"version":2,"before":"preserved"}`, "after_document": `{"version":2,"after":"preserved"}`, "occurred_at": now}
	if err := legacy.Table("command_config_mutations").Create(old).Error; err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at DATETIME NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	for v, n := range map[int]string{20: "command_storage_initial", 21: "command_envelope_ownership", 22: "command_config_complete", 23: "command_activation_durable"} {
		if err := legacy.Exec("INSERT INTO schema_migrations VALUES(?,?,?)", v, n, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := Migrate(ctx, legacy); err != nil {
			t.Fatal(err)
		}
	}
	var got struct{ BeforeDocument, AfterDocument, RequestFingerprint string }
	if err := legacy.Table("command_config_mutations").Where("mutation_id = ?", mutation).Take(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.BeforeDocument != old["before_document"] || got.AfterDocument != old["after_document"] || got.RequestFingerprint != "preserve" {
		t.Fatal("audit lost on upgrade")
	}
	if !legacy.Migrator().HasTable("command_job_activation_leases") {
		t.Fatal("missing lease schema")
	}
	if err := commandconfig.Migrate(ctx, legacy); err != nil {
		t.Fatal("rule operations schema did not upgrade", err)
	}
}
