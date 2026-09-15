package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func sealedProof(scope GenerationScope, marker string) ClosedGenerationProof {
	return ClosedGenerationProof{scope: scope, marker: marker}
}

func TestClosedGenerationProofRequiresInternalMarkerAndRecoversSystemAtomically(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	ctx := context.Background()
	invocationID := uuid.Must(uuid.NewV7()).String()
	contextID := "instance-epoch-1"
	if err := db.Create(&ledgerRow{ID: uuid.Must(uuid.NewV7()).String(), Key: "invocation:" + invocationID, InvocationID: invocationID, AuthContextType: "system", AuthContextID: contextID, RequestFingerprintVersion: "v1", RequestFingerprint: "fp-system", Status: Running, ReceivedAt: now, ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&invocationRow{InvocationID: invocationID, SchemaVersion: 1, AuthContextType: "system", AuthContextID: contextID, AuthGeneration: "system-auth", SecurityGeneration: "system-sec", RegistryVersion: "r1", BindingIDs: "[]", ActorType: "system", ActorID: "instance", ArgumentsSummary: "{}", ArgumentsFingerprint: "args", CorrelationID: "corr-system", RequestFingerprintVersion: "v1", RequestFingerprint: "fp-system", Risk: "low", PolicyDecision: "allowed", Status: Running, ReceivedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	marker := uuid.Must(uuid.NewV7()).String()
	if err := db.Create(&closedGenerationRow{ID: marker, AuthContextType: "system", AuthContextID: contextID, SecurityGeneration: "system-sec", ClosedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	proof := sealedProof(GenerationScope{AuthContextType: "system", AuthContextID: contextID, SecurityGeneration: "system-sec"}, marker)
	batch, err := store.RecoverClosedGenerationWithProof(ctx, proof, 10)
	if err != nil || batch.Processed != 1 || batch.More {
		t.Fatalf("recovery=%+v err=%v", batch, err)
	}
	var inv invocationRow
	if err := db.Where("invocation_id = ?", invocationID).First(&inv).Error; err != nil {
		t.Fatal(err)
	}
	if inv.Status != OutcomeUnknown {
		t.Fatalf("invocation system=%s", inv.Status)
	}
	var ledger ledgerRow
	if err := db.Where("invocation_id = ?", invocationID).First(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.Status != OutcomeUnknown {
		t.Fatalf("ledger system=%s", ledger.Status)
	}
	if again, err := store.RecoverClosedGenerationWithProof(ctx, proof, 10); err != nil || again.Processed != 0 {
		t.Fatalf("replay=%+v err=%v", again, err)
	}
}

func TestRecoveryProofRejectsUnsealedOrWrongScopeAndRollsBackPair(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	request := validRequest()
	request.ExpiresAt = now.Add(time.Hour)
	if _, err := store.Reserve(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	userID := request.Owner.UserID
	scope := GenerationScope{UserID: &userID, AuthContextType: "local_session", AuthContextID: request.Owner.AuthContextID, SecurityGeneration: request.SecurityGeneration}
	if _, err := store.RecoverClosedGenerationWithProof(context.Background(), ClosedGenerationProof{}, 1); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("proof vazia=%v", err)
	}
	marker := uuid.Must(uuid.NewV7()).String()
	if err := db.Create(&closedGenerationRow{ID: marker, UserID: &userID, AuthContextType: scope.AuthContextType, AuthContextID: scope.AuthContextID, SecurityGeneration: scope.SecurityGeneration, ClosedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	proof := sealedProof(scope, marker)
	wrong := scope
	wrong.SecurityGeneration = "other"
	if _, err := store.RecoverClosedGenerationWithProof(context.Background(), sealedProof(wrong, marker), 1); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("scope adulterado=%v", err)
	}
	if err := db.Exec("CREATE TRIGGER reject_maintenance BEFORE UPDATE ON command_invocations BEGIN SELECT RAISE(ABORT, 'maintenance failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	batch, err := store.RecoverClosedGenerationWithProof(context.Background(), proof, 1)
	if err == nil || batch.Processed != 0 {
		t.Fatalf("rollback=%+v err=%v", batch, err)
	}
	record, err := store.Get(context.Background(), request.Owner, request.InvocationID)
	if err != nil || record.Status != Evaluating {
		t.Fatalf("pair avançou em rollback: %+v err=%v", record, err)
	}
}

func TestRetainOnlyTerminalAuditAndKeepsLedgerUntilExpiry(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	ctx := context.Background()
	request := validRequest()
	request.ExpiresAt = now.Add(time.Hour)
	for i := 0; i < 3; i++ {
		request.InvocationID = uuid.Must(uuid.NewV7()).String()
		request.ReceivedAt = now.Add(-time.Duration(3-i) * time.Hour)
		if _, err := store.Reserve(ctx, request); err != nil {
			t.Fatal(err)
		}
		if ok, err := store.CompareAndSwap(ctx, request.Owner, request.InvocationID, Evaluating, Denied); err != nil || !ok {
			t.Fatalf("terminal=%v err=%v", ok, err)
		}
	}
	active := validRequest()
	active.InvocationID = uuid.Must(uuid.NewV7()).String()
	active.ReceivedAt = now.Add(-10 * time.Hour)
	active.ExpiresAt = now.Add(time.Hour)
	if _, err := store.Reserve(ctx, active); err != nil {
		t.Fatal(err)
	}
	maintenance, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	result, err := maintenance.Retain(ctx, InvocationRetentionPolicy{Now: now, MaxAge: 24 * time.Hour, PerUserKeep: 1, SystemKeep: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.InvocationsDeleted != 2 {
		t.Fatalf("auditorias removidas=%d", result.InvocationsDeleted)
	}
	var auditCount, ledgerCount int64
	db.Model(&invocationRow{}).Count(&auditCount)
	db.Model(&ledgerRow{}).Count(&ledgerCount)
	if auditCount != 2 || ledgerCount != 4 {
		t.Fatalf("auditorias=%d ledgers=%d", auditCount, ledgerCount)
	}
	if result.LedgersDeleted != 0 {
		t.Fatalf("ledger recente removido: %d", result.LedgersDeleted)
	}
}

func TestRetainRejectsZeroCapsAndHonorsBoundedBatch(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	maintenance, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest()
	request.ExpiresAt = now.Add(time.Hour)
	for i := 0; i < 4; i++ {
		request.InvocationID = uuid.Must(uuid.NewV7()).String()
		request.ReceivedAt = now.Add(-time.Duration(i+1) * time.Hour)
		if _, err := store.Reserve(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if ok, err := store.CompareAndSwap(context.Background(), request.Owner, request.InvocationID, Evaluating, Denied); err != nil || !ok {
			t.Fatalf("terminal=%v err=%v", ok, err)
		}
	}
	base := InvocationRetentionPolicy{Now: now, MaxAge: 24 * time.Hour, PerUserKeep: 1, SystemKeep: 1}
	for _, policy := range []InvocationRetentionPolicy{
		base,
		{Now: now, MaxAge: base.MaxAge, PerUserKeep: 1, SystemKeep: 0},
		{Now: now, MaxAge: base.MaxAge, PerUserKeep: 0, SystemKeep: 1},
	} {
		if policy == base {
			continue
		}
		if _, err := maintenance.Retain(context.Background(), policy); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("limite inválido aceito: %+v err=%v", policy, err)
		}
	}
	result, err := maintenance.Retain(context.Background(), InvocationRetentionPolicy{Now: now, MaxAge: 24 * time.Hour, PerUserKeep: 1, SystemKeep: 1, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.InvocationsDeleted != 1 || !result.More {
		t.Fatalf("lote não bounded: %+v", result)
	}
	var count int64
	if err := db.Model(&invocationRow{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("auditoria fora do lote: %d", count)
	}
}

func TestRetainRequiresPersistedReplayDeadlineForEventLedger(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	maintenance, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	old := now.Add(-time.Hour)
	eventID1, eventID2, eventID3 := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	for _, row := range []ledgerRow{
		{ID: uuid.Must(uuid.NewV7()).String(), Key: "event-no-deadline", InvocationID: uuid.Must(uuid.NewV7()).String(), AuthContextType: "local_session", AuthContextID: uuid.Must(uuid.NewV7()).String(), RequestFingerprintVersion: "v1", RequestFingerprint: "fp", SourceEventID: &eventID1, Status: Denied, ReceivedAt: old, ExpiresAt: old},
		{ID: uuid.Must(uuid.NewV7()).String(), Key: "event-live-deadline", InvocationID: uuid.Must(uuid.NewV7()).String(), AuthContextType: "local_session", AuthContextID: uuid.Must(uuid.NewV7()).String(), RequestFingerprintVersion: "v1", RequestFingerprint: "fp", SourceEventID: &eventID2, SourceReplayDeadline: maintenanceTimePtr(now.Add(time.Hour)), Status: Denied, ReceivedAt: old, ExpiresAt: old},
		{ID: uuid.Must(uuid.NewV7()).String(), Key: "event-expired-deadline", InvocationID: uuid.Must(uuid.NewV7()).String(), AuthContextType: "local_session", AuthContextID: uuid.Must(uuid.NewV7()).String(), RequestFingerprintVersion: "v1", RequestFingerprint: "fp", SourceEventID: &eventID3, SourceReplayDeadline: maintenanceTimePtr(old), Status: Denied, ReceivedAt: old, ExpiresAt: old},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	result, err := maintenance.Retain(context.Background(), InvocationRetentionPolicy{Now: now, MaxAge: 24 * time.Hour, PerUserKeep: 1, SystemKeep: 1, BatchSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	if result.LedgersDeleted != 1 {
		t.Fatalf("deadline não protegeu evento: %+v", result)
	}
	var remaining int64
	if err := db.Model(&ledgerRow{}).Where("source_event_id IS NOT NULL").Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("eventos restantes=%d", remaining)
	}
}

func TestMaintenanceServiceSealIsBoundToItsStore(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, _ := testStore(t, &now)
	other, _ := testStore(t, &now)
	maintenance, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	forgedAcrossStore := &MaintenanceService{store: other, seal: maintenance.seal}
	_, err = forgedAcrossStore.Retain(context.Background(), InvocationRetentionPolicy{
		Now: now, MaxAge: time.Hour, PerUserKeep: 1, SystemKeep: 1,
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("serviço aceitou selo de outro Store: %v", err)
	}
}

func maintenanceTimePtr(value time.Time) *time.Time { return &value }
