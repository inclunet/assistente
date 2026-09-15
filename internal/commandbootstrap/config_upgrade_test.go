package commandbootstrap

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"github.com/google/uuid"
)

func TestKnownV21ConfigUpgradePreservesRowsAndAcceptsRealWorkspace(t *testing.T) {
	ctx := context.Background()
	current := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "current.db"))
	if e := Migrate(ctx, current); e != nil {
		t.Fatal(e)
	}
	rows, e := objects(current)
	if e != nil {
		t.Fatal(e)
	}
	legacy := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "legacy.db"))
	for _, o := range rows {
		if !preActivationObject(o) {
			continue
		}
		old := legacyConfigObject(o)
		if e := legacy.Exec(old.SQL).Error; e != nil {
			t.Fatalf("%s: %v", o.Name, e)
		}
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, layerID, genID := id(), id(), id()
	now := time.Now().UTC()
	if e := legacy.Create(&commandconfig.Layer{ID: layerID, UserID: user, Name: "preservar", Source: "user", CreatedAt: now, UpdatedAt: now}).Error; e != nil {
		t.Fatal(e)
	}
	if e := legacy.Create(&commandconfig.Generation{ID: genID, UserID: user, Generation: 7, UpdatedAt: now}).Error; e != nil {
		t.Fatal(e)
	}
	if e := legacy.Exec("CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at DATETIME NOT NULL)").Error; e != nil {
		t.Fatal(e)
	}
	for v, n := range map[int]string{20: "command_storage_initial", 21: "command_envelope_ownership"} {
		if e := legacy.Exec("INSERT INTO schema_migrations VALUES (?,?,?)", v, n, now).Error; e != nil {
			t.Fatal(e)
		}
	}
	if e := Migrate(ctx, legacy); e != nil {
		t.Fatal(e)
	}
	store, e := commandconfig.New(legacy)
	if e != nil {
		t.Fatal(e)
	}
	snapshot, e := store.Load(ctx, commandconfig.Scope{UserID: user})
	if e != nil {
		t.Fatal(e)
	}
	if len(snapshot.Layers) != 1 || snapshot.Layers[0].ID != layerID || snapshot.Generations[0].ID != genID || snapshot.Generations[0].Generation != 7 {
		t.Fatal("upgrade alterou dados")
	}
	ws := "ws-actual-opaque"
	if e := store.EnsureScope(ctx, commandconfig.Scope{UserID: user, WorkspaceID: &ws}); e != nil {
		t.Fatal(e)
	}
	if e := Migrate(ctx, legacy); e != nil {
		t.Fatalf("reabertura: %v", e)
	}
	var count int64
	if e := legacy.Table("schema_migrations").Where("version = 22 AND name = ?", "command_config_complete").Count(&count).Error; e != nil || count != 1 {
		t.Fatalf("carimbo v22: %d %v", count, e)
	}
}

// Fixtures v20/v21 não podem conter tabelas introduzidas só na v23.
func preActivationObject(o schemaObject) bool {
	switch o.TblName {
	case "command_layer_activation_rules", "command_layer_activation_state", "command_activation_idempotency_keys", "command_layer_activation_generations", "command_layer_automation_grants", "command_job_activation_outbox", "command_event_replay_policy_epochs":
		return false
	}
	return true
}
