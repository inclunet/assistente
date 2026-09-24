package commanddecision

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandmaintenance"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

func receiptCore(t *testing.T) (*commandsecurity.EpochService, commandsecurity.EpochSnapshot) {
	t.Helper()
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := core.CaptureAuthenticated(context.Background(), func(context.Context) (string, string, error) { return testUUIDv7(t), testUUIDv7(t), nil })
	if err != nil {
		t.Fatal(err)
	}
	return core, epoch
}

func TestCoordinatorRecoveryAllUsersBoundedAndOtherCoreUntouched(t *testing.T) {
	now := time.Now().UTC()
	store, db := temporarySQLiteactualMigrate(t, &testPresenter{}, &now)
	core, epoch := receiptCore(t)
	_, live := receiptCore(t)
	foreign := recoveryRequest(t, now, live)
	foreign.ExpiresAt = now.Add(-time.Hour)
	untouched := seedRecoveryReceipt(t, db, foreign, Accepted, now)
	terminal := seedRecoveryReceipt(t, db, recoveryRequest(t, now, epoch), Consumed, now)
	var ids []string
	for i := 0; i < 129; i++ {
		if i == 64 {
			if err := core.InvalidateSecurity(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		userEpoch, err := core.CaptureAuthenticated(context.Background(), func(context.Context) (string, string, error) { return testUUIDv7(t), testUUIDv7(t), nil })
		if err != nil {
			t.Fatal(err)
		}
		r := recoveryRequest(t, now, userEpoch)
		if i%2 == 0 {
			r.SubjectType = "invocation"
			r.ExpiresAt = now.Add(-time.Second)
		}
		seedRecoveryReceipt(t, db, r, Accepted, now)
		ids = append(ids, r.DecisionID)
	}
	proof, err := core.CloseAndDrain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewCoordinatorRecovery(store, proof)
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.Recover(context.Background(), 128)
	if err != nil || first.Processed != 127 || !first.More {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := a.Recover(context.Background(), 128)
	if err != nil || second.Processed != 2 || second.More || a.after != "" {
		t.Fatalf("second=%+v err=%v cursor=%q", second, err, a.after)
	}
	for i, id := range ids {
		want := Cancelled
		if i%2 == 0 {
			want = Expired
		}
		got := loadReceipt(t, db, id)
		if got.State != want || got.AcceptedActionID != nil || countEvents(t, db, id) != 2 {
			t.Fatalf("receipt=%+v", got)
		}
	}
	for _, want := range []receiptRow{untouched, terminal} {
		if got := loadReceipt(t, db, want.ID); !reflect.DeepEqual(got, want) {
			t.Fatalf("alterou outra autoridade/terminal: %+v", got)
		}
	}
	// Um ciclo completo precisa revisitar IDs anteriores, sem inferir restart.
	r := recoveryRequest(t, now, epoch)
	seedRecoveryReceipt(t, db, r, Pending, now)
	third, err := a.Recover(context.Background(), 128)
	if err != nil || third.Processed != 1 || third.More {
		t.Fatalf("third=%+v err=%v", third, err)
	}
}

func TestCoordinatorRecoveryCancellationDuringSecondTransaction(t *testing.T) {
	now := time.Now().UTC()
	store, db := temporarySQLiteactualMigrate(t, &testPresenter{}, &now)
	core, epoch := receiptCore(t)
	first := seedRecoveryReceipt(t, db, recoveryRequest(t, now, epoch), Pending, now)
	second := seedRecoveryReceipt(t, db, recoveryRequest(t, now, epoch), Pending, now)
	proof, err := core.CloseAndDrain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewCoordinatorRecovery(store, proof)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const callback = "test:cancel-second-recovery-event"
	if err := db.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
		if event, ok := tx.Statement.Dest.(*auditRow); ok && event.DecisionID == second.ID {
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Create().Remove(callback); err != nil {
			t.Error(err)
		}
	})
	result, err := a.Recover(ctx, 128)
	if !errors.Is(err, context.Canceled) || result.Processed != 1 || !result.More || a.after != first.ID {
		t.Fatalf("result=%+v err=%v cursor=%q", result, err, a.after)
	}
	if loadReceipt(t, db, first.ID).State != Cancelled || !reflect.DeepEqual(loadReceipt(t, db, second.ID), second) || countEvents(t, db, second.ID) != 1 {
		t.Fatal("cancelamento perdeu commit anterior ou manteve TX parcial")
	}
}

func TestCoordinatorRecoveryIncludesDrainedExternalTokenReceipts(t *testing.T) {
	now := time.Now().UTC()
	store, db := temporarySQLiteactualMigrate(t, &testPresenter{}, &now)
	core, epoch := receiptCore(t)
	request := recoveryRequest(t, now, epoch)
	request.AuthContextType = "external_token"
	request.SessionID = auth.ExternalTokenContextID("https://issuer.example", "subject-7", "opaque-signed-token")
	request.SubjectType = "invocation"
	row := seedRecoveryReceipt(t, db, request, Accepted, now)
	// Recovery per-session continua explicitamente local e não toca receipt externa.
	localResult, err := store.ReconcileSession(context.Background(), epoch, MaxRecoveryBatch)
	if err != nil || localResult.Closed != 0 || loadReceipt(t, db, row.ID).State != Accepted {
		t.Fatalf("ReconcileSession afetou contexto externo: result=%+v err=%v receipt=%+v", localResult, err, loadReceipt(t, db, row.ID))
	}
	proof, err := core.CloseAndDrain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := NewCoordinatorRecovery(store, proof)
	if err != nil {
		t.Fatal(err)
	}
	result, err := recovery.Recover(context.Background(), 1)
	if err != nil || result.Processed != 1 || result.More {
		t.Fatalf("recovery drained não fechou external_token: result=%+v err=%v", result, err)
	}
	closed := loadReceipt(t, db, row.ID)
	if closed.State != Cancelled || closed.AuthContextType != "external_token" || closed.SessionID != request.SessionID || countEvents(t, db, row.ID) != 2 {
		t.Fatalf("receipt externa não foi encerrada com ownership exato: %+v", closed)
	}
}

func TestCoordinatorRecoveryRollbackPreservesCommittedProgressAndCursor(t *testing.T) {
	now := time.Now().UTC()
	store, db := temporarySQLiteactualMigrate(t, &testPresenter{}, &now)
	core, epoch := receiptCore(t)
	first := seedRecoveryReceipt(t, db, recoveryRequest(t, now, epoch), Pending, now)
	second := seedRecoveryReceipt(t, db, recoveryRequest(t, now, epoch), Accepted, now)
	proof, err := core.CloseAndDrain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewCoordinatorRecovery(store, proof)
	if err != nil {
		t.Fatal(err)
	}
	// Falha real do evento no segundo TX deve reverter também o CAS da receipt.
	if err := db.Exec("CREATE TRIGGER fail_receipt_recovery BEFORE INSERT ON command_decision_receipt_events WHEN NEW.decision_id = '" + second.ID + "' BEGIN SELECT RAISE(ABORT, 'fixture'); END").Error; err != nil {
		t.Fatal(err)
	}
	result, err := a.Recover(context.Background(), 128)
	if err == nil || result.Processed != 1 || !result.More || a.after != first.ID {
		t.Fatalf("result=%+v err=%v cursor=%s", result, err, a.after)
	}
	if got := loadReceipt(t, db, second.ID); !reflect.DeepEqual(got, second) || countEvents(t, db, second.ID) != 1 {
		t.Fatalf("rollback=%+v", got)
	}
	if err := db.Exec("DROP TRIGGER fail_receipt_recovery").Error; err != nil {
		t.Fatal(err)
	}
	result, err = a.Recover(context.Background(), 128)
	if err != nil || result.Processed != 1 || result.More {
		t.Fatalf("retry=%+v err=%v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = a.Recover(ctx, 1)
	if !errors.Is(err, context.Canceled) || result.Processed != 0 {
		t.Fatalf("cancel=%+v %v", result, err)
	}
	a.mu.Lock()
	_, err = a.Recover(context.Background(), 1)
	a.mu.Unlock()
	if !errors.Is(err, commandmaintenance.ErrAlreadyRunning) {
		t.Fatal(err)
	}
	for _, limit := range []int{0, 129} {
		if _, err := a.Recover(context.Background(), limit); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, err := NewCoordinatorRecovery(store, commandsecurity.DrainedGenerations{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Time{} }
	if result, err := a.Recover(context.Background(), 1); !errors.Is(err, ErrInvalid) || result != (commandmaintenance.BatchResult{}) {
		t.Fatalf("clock=%+v %v", result, err)
	}
}
