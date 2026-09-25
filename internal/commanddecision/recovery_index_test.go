package commanddecision

import (
	"context"
	"testing"
)

func TestRecoveryIndexMigrationIsIdempotentAndRejectsWrongDefinition(t *testing.T) {
	_, db, _, request := acceptedFixture(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var indexes int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ix_command_decision_recovery_session'").Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	if indexes != 1 {
		t.Fatal("índice ausente", indexes)
	}
	// Somente SQLite temporário: simula schema anterior incompatível.
	if err := db.Exec("DROP INDEX ix_command_decision_recovery_session").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX ix_command_decision_recovery_session ON command_decision_receipts (status)").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err == nil {
		t.Fatal("índice incompatível aceito")
	}
	if row := loadReceipt(t, db, request.DecisionID); row.State != Accepted {
		t.Fatal("migração alterou receipt", row.State)
	}
}
