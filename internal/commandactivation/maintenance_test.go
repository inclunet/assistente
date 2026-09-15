package commandactivation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func maintenanceClaim(t *testing.T, db *gorm.DB, owner string, state State, updated, replayDeadline time.Time) string {
	t.Helper()
	id := activationID(t)
	rule := "rule"
	layer := "layer"
	args := []any{id, layer, rule, owner, "local_session", "session", "auth", "security", "job", id, state, updated, updated, replayDeadline}
	if err := db.Exec(`INSERT INTO command_layer_activation_state
 (activation_id, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref, user_id,
  auth_context_type, auth_context_id, auth_generation, security_generation,
  source_type, source_event_id, state, activated_at, updated_at, source_replay_deadline)
 VALUES (?, 'user', ?, 'user', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func maintenanceLedger(t *testing.T, db *gorm.DB, owner string, expires, replayDeadline time.Time, sourceEventID ...string) string {
	t.Helper()
	id := activationID(t)
	key := fmt.Sprintf("maintenance-%s", id)
	eventID := uuid.NewString()
	if len(sourceEventID) > 0 {
		eventID = sourceEventID[0]
	}
	if err := db.Exec(`INSERT INTO command_activation_idempotency_keys
 (id, key, user_id, rule_ref_kind, rule_ref, source_type, source_instance_id,
  source_event_id, sequence, event_fingerprint, source_replay_policy_generation,
  source_replay_deadline, terminal_state, created_at, expires_at)
 VALUES (?, ?, ?, 'user', 'rule', 'job', 'instance', ?, 1, 'fingerprint', 'policy', ?, 'deactivate', ?, ?)`,
		id, key, owner, eventID, replayDeadline, expires, expires).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func maintenanceService(t *testing.T) (*gorm.DB, *Store, *MaintenanceService, time.Time) {
	t.Helper()
	db := activationDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	maintenance, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	return db, store, maintenance, now
}

func TestMaintenanceRetainAgeCapAndPreservesLiveRows(t *testing.T) {
	db, _, maintenance, now := maintenanceService(t)
	userA := activationID(t)
	userB := activationID(t)
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-time.Hour)
	deadline := now.Add(-time.Minute)
	maintenanceClaim(t, db, userA, StateDeactivated, old, deadline)
	maintenanceClaim(t, db, userA, StateDeactivated, recent, deadline)
	activeID := maintenanceClaim(t, db, userA, StateActive, old, deadline)
	maintenanceClaim(t, db, userB, StateDeactivated, old, deadline)
	liveReplayID := maintenanceClaim(t, db, userA, StateDeactivated, old, now.Add(time.Hour))
	expiredLedgerID := maintenanceLedger(t, db, userA, old, deadline)
	liveLedgerID := maintenanceLedger(t, db, userA, old, now.Add(time.Hour))

	result, err := maintenance.Retain(context.Background(), RetentionPolicy{Now: now, MaxAge: 24 * time.Hour, PerUserKeep: 1, BatchSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 4 || result.More {
		t.Fatalf("resultado inesperado: %+v", result)
	}
	var count int64
	if err := db.Model(&Claim{}).Where("activation_id = ?", activeID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("claim ativa removida: count=%d err=%v", count, err)
	}
	if err := db.Model(&Claim{}).Where("activation_id = ?", liveReplayID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("claim terminal antiga não respeitou cap/idade: count=%d err=%v", count, err)
	}
	if err := db.Table("command_activation_idempotency_keys").Where("id = ?", expiredLedgerID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("ledger expirado não removido: count=%d err=%v", count, err)
	}
	if err := db.Table("command_activation_idempotency_keys").Where("id = ?", liveLedgerID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("ledger replay-válido removido: count=%d err=%v", count, err)
	}
}

func TestMaintenanceRetainIsBoundedAcrossAuditAndLedger(t *testing.T) {
	db, _, maintenance, now := maintenanceService(t)
	user := activationID(t)
	for i := 0; i < 3; i++ {
		maintenanceClaim(t, db, user, StateExpired, now.Add(-48*time.Hour-time.Duration(i)*time.Minute), now.Add(-time.Hour))
	}
	for i := 0; i < 3; i++ {
		maintenanceLedger(t, db, user, now.Add(-48*time.Hour-time.Duration(i)*time.Minute), now.Add(-time.Hour))
	}

	result, err := maintenance.Retain(context.Background(), RetentionPolicy{Now: now, MaxAge: time.Hour, PerUserKeep: 0 + 1, BatchSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 2 || !result.More {
		t.Fatalf("retenção excedeu lote ou perdeu More: %+v", result)
	}
	var remaining int64
	if err := db.Table("command_layer_activation_state").Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("cap de auditoria inesperado após lote: %d", remaining)
	}
}

func TestMaintenanceRetainProbesLedgerAfterFullClaimBatch(t *testing.T) {
	db, _, maintenance, now := maintenanceService(t)
	user := activationID(t)
	maintenanceClaim(t, db, user, StateExpired, now.Add(-48*time.Hour), now.Add(-time.Hour))
	ledgerID := maintenanceLedger(t, db, user, now.Add(-48*time.Hour), now.Add(-time.Hour))

	result, err := maintenance.Retain(context.Background(), RetentionPolicy{Now: now, MaxAge: time.Hour, PerUserKeep: 1, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 || !result.More {
		t.Fatalf("ledger pendente não apareceu em More: %+v", result)
	}
	result, err = maintenance.Retain(context.Background(), RetentionPolicy{Now: now, MaxAge: time.Hour, PerUserKeep: 1, BatchSize: 1})
	if err != nil || result.Deleted != 1 || result.More {
		t.Fatalf("segunda passagem do ledger: %+v %v", result, err)
	}
	var count int64
	if err := db.Table("command_activation_idempotency_keys").Where("id = ?", ledgerID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("ledger não removido após deadline: count=%d err=%v", count, err)
	}
}

func TestMaintenanceRetainPreservesLedgerForActiveClaim(t *testing.T) {
	db, _, maintenance, now := maintenanceService(t)
	user := activationID(t)
	activeID := maintenanceClaim(t, db, user, StateActive, now.Add(-48*time.Hour), now.Add(-time.Hour))
	ledgerID := maintenanceLedger(t, db, user, now.Add(-48*time.Hour), now.Add(-time.Hour), activeID)

	result, err := maintenance.Retain(context.Background(), RetentionPolicy{Now: now, MaxAge: time.Hour, PerUserKeep: 1, BatchSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.More {
		t.Fatalf("ledger de claim ativa foi tratado como lixo: %+v", result)
	}
	var count int64
	if err := db.Table("command_activation_idempotency_keys").Where("id = ?", ledgerID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("ledger associado à claim ativa removido: count=%d err=%v", count, err)
	}
}

func TestCoordinatorRetentionUsesInjectedHostClock(t *testing.T) {
	db := activationDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	maintenance, err := store.NewMaintenanceServiceAt(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := maintenance.CoordinatorRetention()
	if err != nil {
		t.Fatal(err)
	}
	maintenanceClaim(t, db, activationID(t), StateExpired, now.Add(-48*time.Hour), now.Add(-time.Hour))
	result, err := adapter.RetainBatch(context.Background(), commandmaintenance.Policy{
		InvocationRetention:   24 * time.Hour,
		InvocationsPerUser:    1,
		InvocationsSystemKeep: 1,
		ActivationRetention:   24 * time.Hour,
		ActivationsPerUser:    1,
		LeaseDuration:         time.Minute,
		BatchSize:             1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 || result.More {
		t.Fatalf("adapter não traduziu relógio/política: %+v", result)
	}
}

func TestMaintenanceRetainRollbackKeepsEarlierDeletes(t *testing.T) {
	db, _, maintenance, now := maintenanceService(t)
	user := activationID(t)
	claimID := maintenanceClaim(t, db, user, StateDeactivated, now.Add(-48*time.Hour), now.Add(-time.Hour))
	maintenanceLedger(t, db, user, now.Add(-48*time.Hour), now.Add(-time.Hour))
	if err := db.Exec(`CREATE TRIGGER maintenance_retention_failure
BEFORE DELETE ON command_activation_idempotency_keys
BEGIN SELECT RAISE(ABORT, 'retention test failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TRIGGER maintenance_retention_failure").Error })

	_, err := maintenance.Retain(context.Background(), RetentionPolicy{Now: now, MaxAge: time.Hour, PerUserKeep: 1, BatchSize: 128})
	if err == nil {
		t.Fatal("retenção deveria falhar")
	}
	var count int64
	if err := db.Model(&Claim{}).Where("activation_id = ?", claimID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("rollback não preservou claim: count=%d err=%v", count, err)
	}
}

func TestMaintenanceServiceIsBoundToItsStore(t *testing.T) {
	db, store, maintenance, now := maintenanceService(t)
	otherDB := activationDB(t)
	other, err := NewStore(otherDB)
	if err != nil {
		t.Fatal(err)
	}
	forged := &MaintenanceService{store: other, seal: maintenance.seal}
	_, err = forged.Retain(context.Background(), RetentionPolicy{Now: now, MaxAge: time.Hour, PerUserKeep: 1})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("serviço forjado aceito: %v", err)
	}
	if store == nil || db == nil {
		t.Fatal("fixture inválio")
	}
}
