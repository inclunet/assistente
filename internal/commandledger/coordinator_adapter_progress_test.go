package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCoordinatorRetentionPreservesConfirmedBatchAndReportsRollback(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, db := testStore(t, &now)
	ctx := context.Background()
	owner := validRequest().Owner
	for i := 0; i < 3; i++ {
		request := validRequest()
		request.Owner = owner
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

	maintenance, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := maintenance.CoordinatorRetention()
	if err != nil {
		t.Fatal(err)
	}
	policy := coordinatorPolicy()
	first, err := adapter.RetainBatch(ctx, policy)
	if err != nil || first.Deleted != 1 || !first.More {
		t.Fatalf("primeiro lote=%+v err=%v", first, err)
	}

	if err := db.Exec(`
CREATE TRIGGER reject_retention_progress
BEFORE DELETE ON command_invocations
BEGIN
  SELECT RAISE(ABORT, 'retention delete failure');
END`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TRIGGER IF EXISTS reject_retention_progress").Error })

	second, err := adapter.RetainBatch(ctx, policy)
	if err == nil || second.Deleted != 0 || !second.More {
		t.Fatalf("rollback não reportado com continuação: lote=%+v err=%v", second, err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("erro inesperadamente convertido em cancelamento: %v", err)
	}

	var remaining int64
	if err := db.Model(&invocationRow{}).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("rollback perdeu auditoria do segundo lote: restantes=%d", remaining)
	}
	if first.Deleted != 1 {
		t.Fatalf("progresso confirmado do primeiro lote foi alterado: %+v", first)
	}
	if err := db.Exec("DROP TRIGGER IF EXISTS reject_retention_progress").Error; err != nil {
		t.Fatal(err)
	}
	resumed, err := adapter.RetainBatch(ctx, policy)
	if err != nil || resumed.Deleted != 1 || resumed.More {
		t.Fatalf("retomada=%+v err=%v", resumed, err)
	}
	if err := db.Model(&invocationRow{}).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("retomada não consumiu o lote restante: restantes=%d", remaining)
	}
}

func TestCoordinatorRetentionCancellationBeforeBatchRequiresContinuation(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, _ := testStore(t, &now)
	owner := validRequest().Owner
	for i := 0; i < 3; i++ {
		request := validRequest()
		request.Owner = owner
		request.InvocationID = uuid.Must(uuid.NewV7()).String()
		request.ReceivedAt = now.Add(-time.Duration(i+1) * time.Hour)
		request.ExpiresAt = now.Add(time.Hour)
		if _, err := store.Reserve(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if ok, err := store.CompareAndSwap(context.Background(), request.Owner, request.InvocationID, Evaluating, Denied); err != nil || !ok {
			t.Fatalf("terminal=%v err=%v", ok, err)
		}
	}
	maintenance, err := store.NewMaintenanceService()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := maintenance.CoordinatorRetention()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := adapter.RetainBatch(ctx, coordinatorPolicy())
	if !errors.Is(err, context.Canceled) || result.Deleted != 0 || !result.More {
		t.Fatalf("cancelamento=%+v err=%v", result, err)
	}
}
