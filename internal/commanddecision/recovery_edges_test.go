package commanddecision

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func edgeRecoveryEpoch(request Request, authGeneration, securityGeneration string) commandsecurity.EpochSnapshot {
	return commandsecurity.EpochSnapshot{
		UserID:             request.UserID,
		SessionID:          request.SessionID,
		AuthGeneration:     authGeneration,
		SecurityGeneration: securityGeneration,
	}
}

func recoveryRequestInScope(t *testing.T, base Request, expiresAt time.Time) Request {
	t.Helper()
	request := defaultRequest(t, expiresAt.Add(-time.Minute))
	request.UserID = base.UserID
	request.SessionID = base.SessionID
	request.AuthGeneration = base.AuthGeneration
	request.SecurityGeneration = base.SecurityGeneration
	request.ExpiresAt = expiresAt
	return request
}

func openRecoverySQLiteTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir SQLite temporário: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestReconcileAuditFailureRollsBackWholeBatchIncludingSecondReceipt(t *testing.T) {
	store, db, clock, first := acceptedFixture(t)
	second := recoveryRequestInScope(t, first, first.ExpiresAt.Add(-time.Millisecond))
	if state, err := store.Decide(context.Background(), second); err != nil || state != Accepted {
		t.Fatalf("fixture accepted da segunda receipt: state=%q err=%v", state, err)
	}
	*clock = first.ExpiresAt.Add(time.Millisecond)

	ordered := []Request{first, second}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].DecisionID < ordered[j].DecisionID })
	trigger := fmt.Sprintf("CREATE TRIGGER recovery_edges_abort_second_event BEFORE INSERT ON command_decision_receipt_events WHEN NEW.decision_id = '%s' BEGIN SELECT RAISE(ABORT, 'fixture recovery event failure'); END", ordered[1].DecisionID)
	if err := db.Exec(trigger).Error; err != nil {
		t.Fatalf("criar trigger de falha do segundo evento: %v", err)
	}

	result, err := store.ReconcileSession(context.Background(), edgeRecoveryEpoch(first, first.AuthGeneration, first.SecurityGeneration), 2)
	if err == nil {
		t.Fatal("falha do INSERT de evento foi ignorada")
	}
	if result != (RecoveryResult{}) {
		t.Fatalf("resultado parcial após rollback: %+v", result)
	}
	for _, request := range ordered {
		row := loadReceipt(t, db, request.DecisionID)
		if row.State != Accepted || row.AcceptedActionID == nil || row.ConsumedAt != nil {
			t.Fatalf("receipt alterada apesar do rollback: %+v", row)
		}
		if countEvents(t, db, request.DecisionID) != 2 {
			t.Fatalf("eventos vazaram após rollback para %s", request.DecisionID)
		}
	}
}

func TestReconcileCancellationDuringUpdateRollsBack(t *testing.T) {
	store, db, clock, request := acceptedFixture(t)
	*clock = request.ExpiresAt.Add(time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var updated bool
	const hook = "recovery_edges_cancel_during_update"
	if err := db.Callback().Update().After("gorm:update").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_decision_receipts" && !updated && tx.RowsAffected == 1 {
			updated = true
			cancel()
		}
	}); err != nil {
		t.Fatalf("registrar callback de cancelamento: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove(hook) })

	result, err := store.ReconcileSession(ctx, edgeRecoveryEpoch(request, request.AuthGeneration, request.SecurityGeneration), 1)
	if !updated || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento durante UPDATE não propagou: updated=%t result=%+v err=%v", updated, result, err)
	}
	if result != (RecoveryResult{}) {
		t.Fatalf("resultado parcial após cancelamento: %+v", result)
	}
	row := loadReceipt(t, db, request.DecisionID)
	if row.State != Accepted || row.AcceptedActionID == nil || row.ConsumedAt != nil {
		t.Fatalf("receipt não revertida após cancelamento: %+v", row)
	}
	if countEvents(t, db, request.DecisionID) != 2 {
		t.Fatal("evento de recuperação vazou após cancelamento")
	}
}

func TestReconcileAfterReopenCancelsOrphanWithoutPresenterOrEffect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-reopen.db")
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := now
	presenter := &testPresenter{fn: func(_ context.Context, request Request) (Response, error) {
		return Response{DecisionID: request.DecisionID, ActionID: ApplyAction}, nil
	}}
	db := openRecoverySQLiteTestDB(t, path)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar banco: %v", err)
	}
	if err := db.AutoMigrate(&decisionEffectRow{}); err != nil {
		t.Fatalf("migrar tabela de efeito: %v", err)
	}
	store, err := New(db, presenter, func() time.Time { return clock })
	if err != nil {
		t.Fatalf("New inicial: %v", err)
	}
	request := defaultRequest(t, now)
	if state, err := store.Decide(context.Background(), request); err != nil || state != Accepted {
		t.Fatalf("criar receipt órfã: state=%q err=%v", state, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter sql.DB inicial: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("fechar banco inicial: %v", err)
	}

	reopened := openRecoverySQLiteTestDB(t, path)
	var reopenedCalls atomic.Int32
	reopenedPresenter := &testPresenter{fn: func(_ context.Context, request Request) (Response, error) {
		reopenedCalls.Add(1)
		return Response{DecisionID: request.DecisionID, ActionID: ApplyAction}, nil
	}}
	reopenedStore, err := New(reopened, reopenedPresenter, func() time.Time { return clock })
	if err != nil {
		t.Fatalf("New após reabertura: %v", err)
	}
	current := edgeRecoveryEpoch(request, "auth-generation-new", "security-generation-new")
	result, err := reopenedStore.ReconcileSession(context.Background(), current, 1)
	if err != nil || result != (RecoveryResult{Closed: 1}) {
		t.Fatalf("reconciliação após reabertura: result=%+v err=%v", result, err)
	}
	if reopenedCalls.Load() != 0 {
		t.Fatal("reconciliação chamou UI")
	}

	row := loadReceipt(t, reopened, request.DecisionID)
	if row.State != Cancelled || row.AcceptedActionID != nil || row.ConsumedAt != nil {
		t.Fatalf("órfã não cancelada corretamente: %+v", row)
	}
	if countEvents(t, reopened, request.DecisionID) != 3 {
		t.Fatal("evento de cancelamento da órfã ausente")
	}
	var originalCalls int
	err = reopenedStore.Consume(context.Background(), request, func(tx *gorm.DB) error {
		originalCalls++
		return tx.Create(&decisionEffectRow{ID: request.DecisionID, Value: "must-not-run-original"}).Error
	})
	if !errors.Is(err, ErrStale) || originalCalls != 0 || countEffects(t, reopened) != 0 {
		t.Fatalf("request original consumiu órfã: err=%v calls=%d effects=%d", err, originalCalls, countEffects(t, reopened))
	}
	var effectCalls int
	expected := request
	expected.AuthGeneration = current.AuthGeneration
	expected.SecurityGeneration = current.SecurityGeneration
	err = reopenedStore.Consume(context.Background(), expected, func(tx *gorm.DB) error {
		effectCalls++
		return tx.Create(&decisionEffectRow{ID: request.DecisionID, Value: "must-not-run"}).Error
	})
	if !errors.Is(err, ErrStale) || effectCalls != 0 || countEffects(t, reopened) != 0 {
		t.Fatalf("órfã consumiu efeito: err=%v calls=%d effects=%d", err, effectCalls, countEffects(t, reopened))
	}
}

func TestReconcileConcurrentCallsHaveOneRecoveryEvent(t *testing.T) {
	store, db, clock, request := acceptedFixture(t)
	*clock = request.ExpiresAt.Add(time.Millisecond)
	current := edgeRecoveryEpoch(request, request.AuthGeneration, request.SecurityGeneration)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var arrived atomic.Int32
	bothSelected := make(chan struct{})
	release := make(chan struct{})
	const hook = "recovery_edges_reconcile_query_barrier"
	if err := db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table != "command_decision_receipts" {
			return
		}
		if arrived.Add(1) == 2 {
			close(bothSelected)
		}
		select {
		case <-release:
		case <-ctx.Done():
			_ = tx.AddError(ctx.Err())
		}
	}); err != nil {
		t.Fatalf("registrar barreira de concorrência: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(hook) })

	type outcome struct {
		result RecoveryResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.ReconcileSession(ctx, current, 1)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	select {
	case <-bothSelected:
	case <-ctx.Done():
		t.Error("as duas reconciliações não chegaram à seleção da mesma receipt")
	}
	close(release)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("goroutines de reconciliação não encerraram após cancelamento")
	}
	close(outcomes)

	successes := 0
	for outcome := range outcomes {
		if outcome.err == nil {
			if outcome.result != (RecoveryResult{Closed: 1}) {
				t.Errorf("sucesso concorrente inesperado: %+v", outcome.result)
			}
			successes++
			continue
		}
		if !errors.Is(outcome.err, ErrStale) && !isTransientSQLiteError(outcome.err) {
			t.Errorf("erro não transitório mascarado na perda concorrente: %v", outcome.err)
		}
	}
	if successes != 1 {
		t.Fatalf("reconciliação concorrente teve %d sucessos, esperado 1", successes)
	}
	if row := loadReceipt(t, db, request.DecisionID); row.State != Expired {
		t.Fatalf("estado final concorrente inesperado: %s", row.State)
	}
	if countEvents(t, db, request.DecisionID) != 3 {
		t.Fatal("corrida duplicou o evento de recuperação")
	}
}

func TestReconcileDeadlineAndBatchPreserveCurrentReceipts(t *testing.T) {
	store, db, clock, first := acceptedFixture(t)
	second := recoveryRequestInScope(t, first, first.ExpiresAt.Add(-time.Millisecond))
	third := recoveryRequestInScope(t, first, first.ExpiresAt.Add(9*time.Minute))
	current := recoveryRequestInScope(t, first, first.ExpiresAt.Add(9*time.Minute))
	for _, request := range []Request{second, third, current} {
		if state, err := store.Decide(context.Background(), request); err != nil || state != Accepted {
			t.Fatalf("criar fixture de lote %s: state=%q err=%v", request.DecisionID, state, err)
		}
	}
	if err := db.Model(&receiptRow{}).Where("decision_id = ?", third.DecisionID).
		Update("auth_generation", "auth-generation-old").Error; err != nil {
		t.Fatalf("marcar receipt de epoch antigo: %v", err)
	}
	*clock = first.ExpiresAt.Add(time.Minute)

	epoch := edgeRecoveryEpoch(first, first.AuthGeneration, first.SecurityGeneration)
	firstResult, err := store.ReconcileSession(context.Background(), epoch, 2)
	if err != nil || firstResult != (RecoveryResult{Closed: 2, More: true}) {
		t.Fatalf("primeiro lote de recuperação: result=%+v err=%v", firstResult, err)
	}
	if row := loadReceipt(t, db, current.DecisionID); row.State != Accepted {
		t.Fatal("receipt atual foi tocada pelo primeiro lote")
	}
	if countEvents(t, db, current.DecisionID) != 2 {
		t.Fatal("evento foi adicionado à receipt atual no primeiro lote")
	}

	secondResult, err := store.ReconcileSession(context.Background(), epoch, 2)
	if err != nil || secondResult != (RecoveryResult{Closed: 1}) {
		t.Fatalf("segundo lote de recuperação: result=%+v err=%v", secondResult, err)
	}
	for _, request := range []Request{first, second} {
		if row := loadReceipt(t, db, request.DecisionID); row.State != Expired {
			t.Fatalf("prazo não produziu expired para %s: %s", request.DecisionID, row.State)
		}
		if countEvents(t, db, request.DecisionID) != 3 {
			t.Fatalf("evento de prazo ausente para %s", request.DecisionID)
		}
	}
	if row := loadReceipt(t, db, third.DecisionID); row.State != Cancelled {
		t.Fatalf("epoch antigo não produziu cancelled: %s", row.State)
	}
	if countEvents(t, db, third.DecisionID) != 3 {
		t.Fatal("evento de epoch antigo ausente")
	}
	if row := loadReceipt(t, db, current.DecisionID); row.State != Accepted || countEvents(t, db, current.DecisionID) != 2 {
		t.Fatal("receipt atual foi tocada após o segundo lote")
	}
}
