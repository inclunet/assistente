package commandledger

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSchemaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ledger.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestMigrateCreatesExpectedSchema(t *testing.T) {
	db := openSchemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migração não reexecutável: %v", err)
	}

	for _, table := range []string{"command_idempotency_keys", "command_invocations"} {
		var got string
		if err := db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&got).Error; err != nil || got != table {
			t.Fatalf("tabela %s ausente: %v", table, err)
		}
	}

	assertNotNull := func(table, column string, want bool) {
		t.Helper()
		var notNull int
		if err := db.Raw("SELECT \"notnull\" FROM pragma_table_info(?) WHERE name=?", table, column).Scan(&notNull).Error; err != nil {
			t.Fatal(err)
		}
		if (notNull != 0) != want {
			t.Fatalf("%s.%s notnull=%d, want %v", table, column, notNull, want)
		}
	}
	assertNotNull("command_idempotency_keys", "user_id", false)
	assertNotNull("command_idempotency_keys", "source_occurred_at", false)
	assertNotNull("command_idempotency_keys", "result_summary", false)
	assertNotNull("command_idempotency_keys", "result_ref", false)
	assertNotNull("command_idempotency_keys", "received_at", true)
	assertNotNull("command_invocations", "session_id", false)
	assertNotNull("command_invocations", "global_config_generation", false)
	assertNotNull("command_invocations", "context_captured_at_by_provider", false)
	assertNotNull("command_invocations", "auth_context_type", true)

	var pkCount int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('command_idempotency_keys') WHERE pk=1 AND name='id'").Scan(&pkCount).Error; err != nil || pkCount != 1 {
		t.Fatalf("ledger.id não é PK: %v", err)
	}
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('command_invocations') WHERE pk=1 AND name='invocation_id'").Scan(&pkCount).Error; err != nil || pkCount != 1 {
		t.Fatalf("invocation.invocation_id não é PK: %v", err)
	}

	var fkCount int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_foreign_key_list('command_idempotency_keys')").Scan(&fkCount).Error; err != nil || fkCount != 0 {
		t.Fatalf("ledger não deve ter FK para auditoria: %v", err)
	}

	var indexes []struct {
		Name    string
		Unique  int
		Partial int
	}
	if err := db.Raw("SELECT name, \"unique\", partial FROM pragma_index_list('command_idempotency_keys')").Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, index := range indexes {
		seen[index.Name] = index.Unique == 1 && index.Partial == 1
	}
	if !seen["ux_command_idempotency_keys_source_event_id"] {
		t.Fatal("índice parcial único de source_event_id ausente")
	}
}

func TestMigrateUniqueKeysAndPartialSourceEvent(t *testing.T) {
	db := openSchemaTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ledger := ledgerRow{ID: "id-1", Key: "key-1", InvocationID: "inv-1", AuthContextType: "local", AuthContextID: "ctx", RequestFingerprintVersion: "v1", RequestFingerprint: "fp", Status: Evaluating, ReceivedAt: now, ExpiresAt: now}
	if err := db.Create(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := ledger
	duplicate.ID = "id-2"
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("key/invocation_id deveriam ser únicos")
	}

	second := ledgerRow{ID: "id-3", Key: "key-3", InvocationID: "inv-3", AuthContextType: "local", AuthContextID: "ctx", SourceEventID: stringPtr("event-1"), RequestFingerprintVersion: "v1", RequestFingerprint: "fp", Status: Evaluating, ReceivedAt: now, ExpiresAt: now}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	third := second
	third.ID = "id-4"
	third.Key = "key-4"
	third.InvocationID = "inv-4"
	if err := db.Create(&third).Error; err == nil {
		t.Fatal("source_event_id não nulo deveria ser único")
	}
}

func stringPtr(s string) *string { return &s }
