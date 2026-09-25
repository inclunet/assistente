package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
	"github.com/google/uuid"
)

func coordinatorPolicy() commandmaintenance.Policy {
	return commandmaintenance.Policy{
		InvocationRetention: 24 * time.Hour, InvocationsPerUser: 1,
		InvocationsSystemKeep: 1, ActivationRetention: time.Hour,
		ActivationsPerUser: 1, LeaseDuration: time.Minute, BatchSize: 1,
	}
}

func TestCoordinatorRetentionUsesCurrentPolicyAndPreservesContinuation(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	ctx := context.Background()
	request := validRequest()
	request.ExpiresAt = now.Add(time.Hour)
	for i := 0; i < 4; i++ {
		request.InvocationID = uuid.Must(uuid.NewV7()).String()
		request.ReceivedAt = now.Add(-time.Duration(i+1) * time.Hour)
		if _, err := store.Reserve(ctx, request); err != nil {
			t.Fatal(err)
		}
		if ok, err := store.CompareAndSwap(ctx, request.Owner, request.InvocationID, Evaluating, Denied); err != nil || !ok {
			t.Fatalf("terminal=%v err=%v", ok, err)
		}
	}
	m, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := m.CoordinatorRetention()
	if err != nil {
		t.Fatal(err)
	}
	policy := coordinatorPolicy()
	first, err := adapter.RetainBatch(ctx, policy)
	if err != nil || first.Deleted != 1 || !first.More {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	policy.InvocationsPerUser = 4
	second, err := adapter.RetainBatch(ctx, policy)
	if err != nil || second.Deleted != 0 || second.More {
		t.Fatalf("changed policy=%+v err=%v", second, err)
	}
	policy.InvocationsPerUser = 1
	for i := 0; i < 2; i++ {
		result, err := adapter.RetainBatch(ctx, policy)
		if err != nil || result.Deleted != 1 || result.More != (i == 0) {
			t.Fatalf("batch %d=%+v err=%v", i, result, err)
		}
	}
	var ledgers int64
	if err := db.Model(&ledgerRow{}).Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if ledgers != 4 {
		t.Fatalf("cap apagou barreiras de replay: %d", ledgers)
	}
}

func TestCoordinatorRetentionRejectsInvalidPolicyAndCancellation(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, _ := testStore(t, &now)
	m, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	a, err := m.CoordinatorRetention()
	if err != nil {
		t.Fatal(err)
	}
	p := coordinatorPolicy()
	p.InvocationsSystemKeep = 0
	if _, err := a.RetainBatch(context.Background(), p); !errors.Is(err, commandmaintenance.ErrInvalid) {
		t.Fatalf("policy=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.RetainBatch(ctx, coordinatorPolicy()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	var absent *CoordinatorRetention
	if _, err := absent.RetainBatch(context.Background(), coordinatorPolicy()); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil adapter=%v", err)
	}
	now = time.Time{}
	if _, err := a.RetainBatch(context.Background(), coordinatorPolicy()); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("clock=%v", err)
	}
}

func TestCoordinatorRetentionCapsEveryUserAndSystemIndependently(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	ctx := context.Background()
	users := []string{uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()}
	for _, user := range users {
		for i := 0; i < 3; i++ {
			request := validRequest()
			request.Owner.UserID = user
			request.InvocationID = uuid.Must(uuid.NewV7()).String()
			request.ReceivedAt = now.Add(-time.Duration(i+1) * time.Hour)
			request.ExpiresAt = now.Add(time.Hour)
			if _, err := store.Reserve(ctx, request); err != nil {
				t.Fatal(err)
			}
			if ok, err := store.CompareAndSwap(ctx, request.Owner, request.InvocationID, Evaluating, Denied); err != nil || !ok {
				t.Fatalf("terminal=%v err=%v", ok, err)
			}
		}
	}
	for i := 0; i < 4; i++ {
		id := uuid.Must(uuid.NewV7()).String()
		status := Denied
		if i == 3 {
			status = Running
		}
		if err := db.Create(&ledgerRow{ID: uuid.Must(uuid.NewV7()).String(), Key: "invocation:" + id, InvocationID: id,
			AuthContextType: "system", AuthContextID: "instance", RequestFingerprintVersion: "v1", RequestFingerprint: "fp", Status: status,
			ReceivedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&invocationRow{InvocationID: id, SchemaVersion: 1, AuthContextType: "system", AuthContextID: "instance", AuthGeneration: "auth", SecurityGeneration: "sec",
			RegistryVersion: "r1", BindingIDs: "[]", ActorType: "system", ActorID: "instance", ArgumentsSummary: "{}", ArgumentsFingerprint: "fp", CorrelationID: "corr",
			RequestFingerprintVersion: "v1", RequestFingerprint: "fp", Risk: "low", PolicyDecision: "allowed", Status: status, ReceivedAt: now.Add(-time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	m, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	a, err := m.CoordinatorRetention()
	if err != nil {
		t.Fatal(err)
	}
	p := coordinatorPolicy()
	p.BatchSize, p.InvocationsSystemKeep = 128, 2
	result, err := a.RetainBatch(ctx, p)
	if err != nil || result.Deleted != 5 || result.More {
		t.Fatalf("retention=%+v err=%v", result, err)
	}
	for _, user := range users {
		var count int64
		if err := db.Model(&invocationRow{}).Where("user_id = ?", user).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("user cap=%d", count)
		}
	}
	var system, active, ledgers int64
	if err := db.Model(&invocationRow{}).Where("user_id IS NULL").Count(&system).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&invocationRow{}).Where("user_id IS NULL AND status = ?", Running).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ledgerRow{}).Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if system != 3 || active != 1 || ledgers != 10 {
		t.Fatalf("system=%d active=%d ledgers=%d", system, active, ledgers)
	}
}
