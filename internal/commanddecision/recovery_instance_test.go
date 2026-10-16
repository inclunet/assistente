package commanddecision

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
)

func TestReconcileSessionsCobreMultiplosUsuariosEmLoteBounded(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	clock := now
	store, db := temporarySQLiteactualMigrate(t, &testPresenter{}, &clock)
	first := recoveryEpoch(t)
	second := recoveryEpoch(t)
	firstOld := first
	firstOld.AuthGeneration = "auth-old"
	secondOld := second
	secondOld.SecurityGeneration = "security-old"
	seedRecoveryReceipt(t, db, recoveryRequest(t, now, firstOld), Pending, now)
	seedRecoveryReceipt(t, db, recoveryRequest(t, now, secondOld), Accepted, now)
	result, err := store.ReconcileSessions(context.Background(), []commandsecurity.EpochSnapshot{first, second}, 2)
	if err != nil || result.Closed != 2 || result.More {
		t.Fatalf("resultado=%+v err=%v", result, err)
	}
}

func TestReconcileSessionsRespeitaCancelamentoAntesDoLote(t *testing.T) {
	clock := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	store, _ := temporarySQLiteactualMigrate(t, &testPresenter{}, &clock)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	epoch := recoveryEpoch(t)
	_, err := store.ReconcileSessions(ctx, []commandsecurity.EpochSnapshot{epoch}, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro=%v", err)
	}
}
