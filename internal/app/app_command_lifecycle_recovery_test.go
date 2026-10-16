package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func lifecycleRecoveryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lifecycle-recovery.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := commandledger.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := commanddecision.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestAppCommandLifecycleRecoveryAllowsBootWhenDatabaseHasNoRecoverableRows(t *testing.T) {
	lifecycleRecoveryDB(t)
	runtime := &appCommandLifecycleRuntime{}
	if err := runtime.Recover(context.Background(), commandruntime.Generation{Value: "generation"}); err != nil {
		t.Fatalf("preflight vazio bloqueou boot: %v", err)
	}
}

func TestAppCommandLifecycleRecoveryBlocksEveryRecoverableTable(t *testing.T) {
	tests := []struct {
		name   string
		insert func(*gorm.DB) error
	}{
		{name: "invocation", insert: func(db *gorm.DB) error {
			return db.Exec(`INSERT INTO command_invocations
				(invocation_id, schema_version, auth_context_type, auth_context_id, auth_generation, security_generation,
				 registry_version, binding_ids, actor_type, actor_id, arguments_summary, arguments_fingerprint,
				 correlation_id, request_fingerprint_version, request_fingerprint, risk, policy_decision, status, received_at)
				VALUES ('invocation-pending', 1, 'system', 'instance', 'auth', 'security', 'registry', '[]',
				 'system', 'instance', '{}', 'args', 'correlation', 'v1', 'fingerprint', 'low', 'allowed', 'running', CURRENT_TIMESTAMP)`).Error
		}},
		{name: "ledger", insert: func(db *gorm.DB) error {
			return db.Exec(`INSERT INTO command_idempotency_keys
				(id, key, invocation_id, auth_context_type, auth_context_id, request_fingerprint_version,
				 request_fingerprint, status, received_at, expires_at)
				VALUES ('ledger-pending-id', 'ledger-pending-key', 'ledger-pending-invocation', 'system', 'instance',
				 'v1', 'fingerprint', 'queued', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`).Error
		}},
		{name: "decision", insert: func(db *gorm.DB) error {
			return db.Exec(`INSERT INTO command_decision_receipts
				(decision_id, subject_id, user_id, auth_context_id, request_fingerprint, auth_generation,
				 security_generation, expires_at, status, auth_context_type, subject_type, allowed_action_ids)
				VALUES ('decision-pending', 'subject-pending', 'user-pending', 'session-pending', 'fingerprint',
				 'auth', 'security', 4102444800000, 'pending', 'local_session', 'invocation', '["apply","deny"]')`).Error
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := lifecycleRecoveryDB(t)
			if err := tc.insert(db); err != nil {
				t.Fatal(err)
			}
			err := (&appCommandLifecycleRuntime{}).Recover(context.Background(), commandruntime.Generation{Value: "generation"})
			if !errors.Is(err, commandruntime.ErrNotReady) {
				t.Fatalf("pendência %s não bloqueou com ErrNotReady: %v", tc.name, err)
			}
		})
	}
}

func TestAppCommandLifecycleRecoveryDeniesSQLFailureAndHonorsCancellation(t *testing.T) {
	db := lifecycleRecoveryDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := (&appCommandLifecycleRuntime{}).Recover(context.Background(), commandruntime.Generation{Value: "generation"}); !errors.Is(err, commandruntime.ErrNotReady) {
		t.Fatalf("falha SQL não virou ErrNotReady: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&appCommandLifecycleRuntime{}).Recover(ctx, commandruntime.Generation{Value: "generation"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento não foi preservado: %v", err)
	}
}
