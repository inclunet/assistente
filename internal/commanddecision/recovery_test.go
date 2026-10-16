package commanddecision

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

func recoveryEpoch(t *testing.T) commandsecurity.EpochSnapshot {
	t.Helper()
	return commandsecurity.EpochSnapshot{
		UserID:             testUUIDv7(t),
		SessionID:          testUUIDv7(t),
		AuthGeneration:     "auth-current",
		SecurityGeneration: "security-current",
	}
}

func recoveryRequest(t *testing.T, now time.Time, epoch commandsecurity.EpochSnapshot) Request {
	t.Helper()
	request := defaultRequest(t, now.Add(time.Hour))
	request.UserID = epoch.UserID
	request.SessionID = epoch.SessionID
	request.AuthGeneration = epoch.AuthGeneration
	request.SecurityGeneration = epoch.SecurityGeneration
	return request
}

func seedRecoveryReceipt(t *testing.T, db *gorm.DB, request Request, state string, now time.Time) receiptRow {
	t.Helper()
	row := rowOf(request)
	row.State = state
	if state != Pending {
		respondedAt := now.Add(-time.Second).UnixMilli()
		row.RespondedAt = &respondedAt
	}
	if state == Accepted || state == Consumed {
		action := ApplyAction
		row.AcceptedActionID = &action
	}
	if state == Consumed {
		consumedAt := now.Add(-time.Millisecond).UnixMilli()
		row.ConsumedAt = &consumedAt
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("semear receipt %s: %v", state, err)
	}
	if err := appendEvent(db, row.ID, Pending, now); err != nil {
		t.Fatalf("semear evento pending: %v", err)
	}
	return row
}

func TestReconcileSessionReconcilesOnlyEligibleRows(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	clock := now
	presenter := &testPresenter{fn: func(context.Context, Request) (Response, error) {
		t.Fatal("ReconcileSession não deve chamar Presenter")
		return Response{}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	epoch := recoveryEpoch(t)

	authOldEpoch := epoch
	authOldEpoch.AuthGeneration = "auth-old"
	oldPending := recoveryRequest(t, now, authOldEpoch)
	oldPendingRow := seedRecoveryReceipt(t, db, oldPending, Pending, now)
	securityOldEpoch := epoch
	securityOldEpoch.SecurityGeneration = "security-old"
	oldAccepted := recoveryRequest(t, now, securityOldEpoch)
	oldAcceptedRow := seedRecoveryReceipt(t, db, oldAccepted, Accepted, now)
	expired := recoveryRequest(t, now, epoch)
	expired.ExpiresAt = now.Add(-time.Second)
	expiredRow := seedRecoveryReceipt(t, db, expired, Pending, now)
	expiredAccepted := recoveryRequest(t, now, epoch)
	expiredAccepted.ExpiresAt = now.Add(-time.Second)
	expiredAcceptedRow := seedRecoveryReceipt(t, db, expiredAccepted, Accepted, now)
	currentPending := recoveryRequest(t, now, epoch)
	currentPendingRow := seedRecoveryReceipt(t, db, currentPending, Pending, now)
	currentAccepted := recoveryRequest(t, now, epoch)
	currentAcceptedRow := seedRecoveryReceipt(t, db, currentAccepted, Accepted, now)

	terminalRows := make(map[string]receiptRow)
	for _, state := range []string{Consumed, Denied, Cancelled, Expired} {
		request := recoveryRequest(t, now, epoch)
		terminalRows[request.DecisionID] = seedRecoveryReceipt(t, db, request, state, now)
	}

	otherUser := epoch
	otherUser.UserID = testUUIDv7(t)
	otherUser.AuthGeneration = authOldEpoch.AuthGeneration
	otherUser.SecurityGeneration = authOldEpoch.SecurityGeneration
	otherUserRow := seedRecoveryReceipt(t, db, recoveryRequest(t, now, otherUser), Pending, now)

	otherSession := epoch
	otherSession.SessionID = testUUIDv7(t)
	otherSession.AuthGeneration = authOldEpoch.AuthGeneration
	otherSession.SecurityGeneration = authOldEpoch.SecurityGeneration
	otherSessionRow := seedRecoveryReceipt(t, db, recoveryRequest(t, now, otherSession), Accepted, now)

	result, err := store.ReconcileSession(context.Background(), epoch, 128)
	if err != nil {
		t.Fatalf("ReconcileSession: %v", err)
	}
	if result != (RecoveryResult{Closed: 4}) {
		t.Fatalf("resultado=%+v, esperado quatro encerramentos sem More", result)
	}
	if presenter.calls.Load() != 0 {
		t.Fatalf("Presenter chamado %d vezes", presenter.calls.Load())
	}

	if row := loadReceipt(t, db, oldPending.DecisionID); row.State != Cancelled || row.AcceptedActionID != nil || row.RespondedAt == nil || oldPendingRow.RespondedAt != nil || *row.RespondedAt != now.UnixMilli() {
		t.Fatalf("pending com auth antiga não cancelado com timestamp novo: before=%+v after=%+v", oldPendingRow, row)
	}
	if row := loadReceipt(t, db, oldAccepted.DecisionID); row.State != Cancelled || row.AcceptedActionID != nil || row.RespondedAt == nil || oldAcceptedRow.RespondedAt == nil || *row.RespondedAt != *oldAcceptedRow.RespondedAt {
		t.Fatalf("accepted com security antiga não preservou timestamp: before=%+v after=%+v", oldAcceptedRow, row)
	}
	if row := loadReceipt(t, db, expired.DecisionID); row.State != Expired || row.AcceptedActionID != nil || row.RespondedAt == nil || expiredRow.RespondedAt != nil || *row.RespondedAt != now.UnixMilli() {
		t.Fatalf("pending expirada não virou Expired com timestamp novo: before=%+v after=%+v", expiredRow, row)
	}
	if row := loadReceipt(t, db, expiredAccepted.DecisionID); row.State != Expired || row.AcceptedActionID != nil || row.RespondedAt == nil || expiredAcceptedRow.RespondedAt == nil || *row.RespondedAt != *expiredAcceptedRow.RespondedAt {
		t.Fatalf("accepted expirado não virou Expired: before=%+v after=%+v", expiredAcceptedRow, row)
	}

	for _, want := range []receiptRow{currentPendingRow, currentAcceptedRow} {
		if got := loadReceipt(t, db, want.ID); !reflect.DeepEqual(got, want) {
			t.Fatalf("receipt da geração atual foi alterada:\n got=%+v\nwant=%+v", got, want)
		}
	}
	for _, want := range terminalRows {
		if got := loadReceipt(t, db, want.ID); !reflect.DeepEqual(got, want) {
			t.Fatalf("receipt terminal foi alterada:\n got=%+v\nwant=%+v", got, want)
		}
	}
	for _, want := range []receiptRow{otherUserRow, otherSessionRow} {
		if got := loadReceipt(t, db, want.ID); !reflect.DeepEqual(got, want) {
			t.Fatalf("receipt fora do escopo foi alterada:\n got=%+v\nwant=%+v", got, want)
		}
	}
	if countEvents(t, db, oldPending.DecisionID) != 2 || countEvents(t, db, oldAccepted.DecisionID) != 2 || countEvents(t, db, expired.DecisionID) != 2 || countEvents(t, db, expiredAccepted.DecisionID) != 2 {
		t.Fatal("recuperação não registrou exatamente um evento por receipt encerrada")
	}
	if countEvents(t, db, currentPending.DecisionID) != 1 || countEvents(t, db, currentAccepted.DecisionID) != 1 {
		t.Fatal("receipt preservada recebeu evento indevido")
	}
}

func TestReconcileSessionHonorsBatchLimitMoreAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	clock := now
	presenter := &testPresenter{fn: func(context.Context, Request) (Response, error) {
		t.Fatal("ReconcileSession não deve chamar Presenter")
		return Response{}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	epoch := recoveryEpoch(t)
	oldEpoch := epoch
	oldEpoch.AuthGeneration = "auth-old"
	oldEpoch.SecurityGeneration = "security-old"

	const eligible = MaxRecoveryBatch + 1
	ids := make([]string, 0, eligible)
	for i := 0; i < eligible; i++ {
		request := recoveryRequest(t, now, oldEpoch)
		seedRecoveryReceipt(t, db, request, Pending, now)
		ids = append(ids, request.DecisionID)
	}

	first, err := store.ReconcileSession(context.Background(), epoch, MaxRecoveryBatch)
	if err != nil {
		t.Fatalf("primeira reconciliação: %v", err)
	}
	if first != (RecoveryResult{Closed: MaxRecoveryBatch, More: true}) {
		t.Fatalf("primeiro lote=%+v, esperado limite e More", first)
	}

	var cancelled, pending int64
	if err := db.Model(&receiptRow{}).Where("status = ?", Cancelled).Count(&cancelled).Error; err != nil {
		t.Fatalf("contar canceladas: %v", err)
	}
	if err := db.Model(&receiptRow{}).Where("status = ?", Pending).Count(&pending).Error; err != nil {
		t.Fatalf("contar pending: %v", err)
	}
	if cancelled != MaxRecoveryBatch || pending != 1 {
		t.Fatalf("lote excedeu limite: cancelled=%d pending=%d", cancelled, pending)
	}

	second, err := store.ReconcileSession(context.Background(), epoch, MaxRecoveryBatch)
	if err != nil {
		t.Fatalf("segunda reconciliação: %v", err)
	}
	if second != (RecoveryResult{Closed: 1}) {
		t.Fatalf("segundo lote=%+v, esperado um encerramento sem More", second)
	}
	third, err := store.ReconcileSession(context.Background(), epoch, MaxRecoveryBatch)
	if err != nil {
		t.Fatalf("repetição idempotente: %v", err)
	}
	if third != (RecoveryResult{}) {
		t.Fatalf("repetição alterou o resultado: %+v", third)
	}
	if presenter.calls.Load() != 0 {
		t.Fatalf("Presenter chamado %d vezes", presenter.calls.Load())
	}
	for _, id := range ids {
		if countEvents(t, db, id) != 2 {
			t.Fatalf("receipt %s recebeu evento duplicado", id)
		}
	}
}

func TestReconcileSessionRejectsInvalidInputsWithZeroResult(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	clock := now
	presenter := &testPresenter{fn: func(context.Context, Request) (Response, error) {
		t.Fatal("ReconcileSession não deve chamar Presenter")
		return Response{}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	epoch := recoveryEpoch(t)

	tests := []struct {
		name    string
		store   *Store
		ctx     context.Context
		current commandsecurity.EpochSnapshot
		limit   int
	}{
		{name: "nil store", store: nil, ctx: context.Background(), current: epoch, limit: 1},
		{name: "nil database", store: &Store{now: func() time.Time { return now }}, ctx: context.Background(), current: epoch, limit: 1},
		{name: "nil clock", store: &Store{db: db}, ctx: context.Background(), current: epoch, limit: 1},
		{name: "nil context", store: store, ctx: nil, current: epoch, limit: 1},
		{name: "zero limit", store: store, ctx: context.Background(), current: epoch, limit: 0},
		{name: "limit acima do máximo", store: store, ctx: context.Background(), current: epoch, limit: MaxRecoveryBatch + 1},
		{name: "user inválido", store: store, ctx: context.Background(), current: commandsecurity.EpochSnapshot{UserID: "not-a-uuid", SessionID: epoch.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}, limit: 1},
		{name: "session inválida", store: store, ctx: context.Background(), current: commandsecurity.EpochSnapshot{UserID: epoch.UserID, SessionID: "not-a-uuid", AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}, limit: 1},
		{name: "auth vazio", store: store, ctx: context.Background(), current: commandsecurity.EpochSnapshot{UserID: epoch.UserID, SessionID: epoch.SessionID, SecurityGeneration: epoch.SecurityGeneration}, limit: 1},
		{name: "auth com espaços", store: store, ctx: context.Background(), current: commandsecurity.EpochSnapshot{UserID: epoch.UserID, SessionID: epoch.SessionID, AuthGeneration: " auth-current", SecurityGeneration: epoch.SecurityGeneration}, limit: 1},
		{name: "auth longo", store: store, ctx: context.Background(), current: commandsecurity.EpochSnapshot{UserID: epoch.UserID, SessionID: epoch.SessionID, AuthGeneration: strings.Repeat("a", 257), SecurityGeneration: epoch.SecurityGeneration}, limit: 1},
		{name: "security vazio", store: store, ctx: context.Background(), current: commandsecurity.EpochSnapshot{UserID: epoch.UserID, SessionID: epoch.SessionID, AuthGeneration: epoch.AuthGeneration}, limit: 1},
		{name: "security inválido", store: store, ctx: context.Background(), current: commandsecurity.EpochSnapshot{UserID: epoch.UserID, SessionID: epoch.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: " security-current"}, limit: 1},
		{name: "clock zero", store: &Store{db: db, now: func() time.Time { return time.Time{} }}, ctx: context.Background(), current: epoch, limit: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.store.ReconcileSession(test.ctx, test.current, test.limit)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("erro=%v, esperado ErrInvalid", err)
			}
			if result != (RecoveryResult{}) {
				t.Fatalf("resultado inválido não zerado: %+v", result)
			}
		})
	}
	if presenter.calls.Load() != 0 {
		t.Fatalf("Presenter chamado %d vezes", presenter.calls.Load())
	}
}
