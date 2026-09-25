package commanddecision

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

func acceptedInvocationFixture(t *testing.T) (*Store, *gorm.DB, Request) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := now
	presenter := &testPresenter{fn: func(_ context.Context, request Request) (Response, error) {
		return Response{DecisionID: request.DecisionID, ActionID: ApplyAction}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	request := defaultRequest(t, now)
	request.SubjectType = "invocation"
	// MutationID é subject_id e representa uma invocação distinta da receipt.
	request.MutationID = testUUIDv7(t)
	state, err := store.Decide(context.Background(), request)
	if err != nil || state != Accepted {
		t.Fatalf("receipt invocation não aceita: state=%q err=%v", state, err)
	}
	row := loadReceipt(t, db, request.DecisionID)
	if row.SubjectType != "invocation" || row.MutationID != request.MutationID || row.MutationID == request.DecisionID || row.State != Accepted {
		t.Fatalf("receipt invocation incorreta: %+v", row)
	}
	return store, db, request
}

func TestInvocationReceiptAcceptedConsumesOnlySameSubjectAndOwnerBindings(t *testing.T) {
	store, db, request := acceptedInvocationFixture(t)
	var calls atomic.Int32
	if err := store.Consume(context.Background(), request, func(tx *gorm.DB) error {
		calls.Add(1)
		return tx.Create(&decisionEffectRow{ID: request.DecisionID, Value: "invocation executed"}).Error
	}); err != nil {
		t.Fatalf("consumo invocation: %v", err)
	}
	if calls.Load() != 1 || loadReceipt(t, db, request.DecisionID).State != Consumed || countEvents(t, db, request.DecisionID) != 3 || countEffects(t, db) != 1 {
		t.Fatalf("consumo invocation inconsistente: calls=%d receipt=%+v events=%d effects=%d", calls.Load(), loadReceipt(t, db, request.DecisionID), countEvents(t, db, request.DecisionID), countEffects(t, db))
	}
}

func TestConfigReceiptCannotExecuteInvocationAndInvocationCannotExecuteConfig(t *testing.T) {
	configStore, configDB, _, configRequest := acceptedFixture(t)
	configAsInvocation := configRequest
	configAsInvocation.SubjectType = "invocation"
	var configCalls atomic.Int32
	if err := configStore.Consume(context.Background(), configAsInvocation, func(*gorm.DB) error { configCalls.Add(1); return nil }); !errors.Is(err, ErrStale) {
		t.Fatalf("receipt config executou como invocation: %v", err)
	}
	if configCalls.Load() != 0 || loadReceipt(t, configDB, configRequest.DecisionID).State != Accepted || countEvents(t, configDB, configRequest.DecisionID) != 2 {
		t.Fatal("tentativa cross-subject alterou receipt config")
	}

	invocationStore, invocationDB, invocationRequest := acceptedInvocationFixture(t)
	invocationAsConfig := invocationRequest
	invocationAsConfig.SubjectType = "config_mutation"
	var invocationCalls atomic.Int32
	if err := invocationStore.Consume(context.Background(), invocationAsConfig, func(*gorm.DB) error { invocationCalls.Add(1); return nil }); !errors.Is(err, ErrStale) {
		t.Fatalf("receipt invocation executou como config_mutation: %v", err)
	}
	if invocationCalls.Load() != 0 || loadReceipt(t, invocationDB, invocationRequest.DecisionID).State != Accepted || countEvents(t, invocationDB, invocationRequest.DecisionID) != 2 {
		t.Fatal("tentativa cross-subject alterou receipt invocation")
	}
}

func TestInvocationConsumeRejectsMismatchedIDOwnerFingerprintAuthSecurity(t *testing.T) {
	changes := []struct {
		name   string
		change func(*Request, *testing.T)
	}{
		{name: "subject id", change: func(request *Request, t *testing.T) { request.MutationID = testUUIDv7(t) }},
		{name: "owner user", change: func(request *Request, t *testing.T) { request.UserID = testUUIDv7(t) }},
		{name: "owner auth context", change: func(request *Request, t *testing.T) { request.SessionID = testUUIDv7(t) }},
		{name: "fingerprint", change: func(request *Request, _ *testing.T) { request.Fingerprint = "different-fingerprint" }},
		{name: "auth generation", change: func(request *Request, _ *testing.T) { request.AuthGeneration = "different-auth-generation" }},
		{name: "security generation", change: func(request *Request, _ *testing.T) { request.SecurityGeneration = "different-security-generation" }},
	}
	for _, test := range changes {
		t.Run(test.name, func(t *testing.T) {
			store, db, request := acceptedInvocationFixture(t)
			expected := request
			test.change(&expected, t)
			var calls atomic.Int32
			err := store.Consume(context.Background(), expected, func(*gorm.DB) error { calls.Add(1); return nil })
			if !errors.Is(err, ErrStale) || calls.Load() != 0 {
				t.Fatalf("mismatch aceito: err=%v calls=%d", err, calls.Load())
			}
			if loadReceipt(t, db, request.DecisionID).State != Accepted || countEvents(t, db, request.DecisionID) != 2 {
				t.Fatal("mismatch alterou receipt invocation")
			}
		})
	}
}

func TestInvocationConsumeCallbackFailureRollsBackReceiptEventAndEffect(t *testing.T) {
	store, db, request := acceptedInvocationFixture(t)
	wantErr := errors.New("invocation callback failed")
	var calls atomic.Int32
	err := store.Consume(context.Background(), request, func(tx *gorm.DB) error {
		calls.Add(1)
		if err := tx.Create(&decisionEffectRow{ID: request.DecisionID, Value: "must rollback"}).Error; err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) || calls.Load() != 1 {
		t.Fatalf("erro/callback inesperado: err=%v calls=%d", err, calls.Load())
	}
	if loadReceipt(t, db, request.DecisionID).State != Accepted || countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
		t.Fatal("falha do callback não reverteu receipt invocation, evento e efeito")
	}
}

func TestReconcileInvocationCancelsOnlyMatchingSessionStaleEpoch(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	clock := now
	presenter := &testPresenter{fn: func(context.Context, Request) (Response, error) {
		t.Fatal("ReconcileSession não deve chamar Presenter")
		return Response{}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	current := commandsecurity.EpochSnapshot{
		UserID:             testUUIDv7(t),
		SessionID:          testUUIDv7(t),
		AuthGeneration:     "auth-current",
		SecurityGeneration: "security-current",
	}

	stale := recoveryRequest(t, now, current)
	stale.SubjectType = "invocation"
	stale.AuthGeneration = "auth-old"
	staleRow := seedRecoveryReceipt(t, db, stale, Accepted, now)

	currentInvocation := recoveryRequest(t, now, current)
	currentInvocation.SubjectType = "invocation"
	currentRow := seedRecoveryReceipt(t, db, currentInvocation, Accepted, now)

	otherOwner := current
	otherOwner.UserID = testUUIDv7(t)
	otherOwner.AuthGeneration = "auth-old"
	otherOwnerInvocation := recoveryRequest(t, now, otherOwner)
	otherOwnerInvocation.SubjectType = "invocation"
	otherOwnerRow := seedRecoveryReceipt(t, db, otherOwnerInvocation, Accepted, now)

	otherSession := current
	otherSession.SessionID = testUUIDv7(t)
	otherSession.AuthGeneration = "auth-old"
	otherSessionInvocation := recoveryRequest(t, now, otherSession)
	otherSessionInvocation.SubjectType = "invocation"
	otherSessionRow := seedRecoveryReceipt(t, db, otherSessionInvocation, Accepted, now)

	result, err := store.ReconcileSession(context.Background(), current, 10)
	if err != nil {
		t.Fatalf("ReconcileSession invocation: %v", err)
	}
	if result != (RecoveryResult{Closed: 1}) {
		t.Fatalf("resultado=%+v, esperado apenas a invocation stale da sessão corrente", result)
	}
	if presenter.calls.Load() != 0 {
		t.Fatalf("Presenter chamado %d vezes", presenter.calls.Load())
	}

	gotStale := loadReceipt(t, db, stale.DecisionID)
	if gotStale.State != Cancelled || gotStale.AcceptedActionID != nil || countEvents(t, db, stale.DecisionID) != 2 {
		t.Fatalf("invocation stale não recuperada corretamente: before=%+v after=%+v", staleRow, gotStale)
	}
	for _, want := range []receiptRow{currentRow, otherOwnerRow, otherSessionRow} {
		if got := loadReceipt(t, db, want.ID); got.State != Accepted || got.AcceptedActionID == nil || countEvents(t, db, want.ID) != 1 {
			t.Fatalf("invocation fora do escopo foi alterada: want=%+v got=%+v", want, got)
		}
	}
}
