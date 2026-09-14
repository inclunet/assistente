package commanddecision

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type testPresenter struct {
	fn    func(context.Context, Request) (Response, error)
	calls atomic.Int32
}

func (p *testPresenter) Present(ctx context.Context, request Request) (Response, error) {
	p.calls.Add(1)
	return p.fn(ctx, request)
}

type decisionEffectRow struct {
	ID    string `gorm:"primaryKey;not null"`
	Value string `gorm:"not null"`
}

func (decisionEffectRow) TableName() string { return "command_decision_test_effects" }

func temporarySQLiteactualMigrate(t *testing.T, presenter Presenter, clock *time.Time) (*Store, *gorm.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command-decision.db")
	db, err := gorm.Open(sqlite.Open("file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir sqlite temporário: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(16)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar schema real: %v", err)
	}
	if err := db.AutoMigrate(&decisionEffectRow{}); err != nil {
		t.Fatalf("migrar tabela de efeito da fixture: %v", err)
	}
	store, err := New(db, presenter, func() time.Time { return *clock })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store, db
}

func testUUIDv7(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar UUIDv7: %v", err)
	}
	return id.String()
}

func defaultRequest(t *testing.T, now time.Time) Request {
	t.Helper()
	return Request{
		DecisionID:         testUUIDv7(t),
		MutationID:         testUUIDv7(t),
		UserID:             testUUIDv7(t),
		SessionID:          testUUIDv7(t),
		Fingerprint:        "request-fingerprint-fixture",
		AuthGeneration:     "auth-generation-fixture",
		SecurityGeneration: "security-generation-fixture",
		ExpiresAt:          now.Add(time.Minute),
		Body:               "rawfixture",
	}
}

func loadReceipt(t *testing.T, db *gorm.DB, decisionID string) receiptRow {
	t.Helper()
	var row receiptRow
	if err := db.Take(&row, "decision_id = ?", decisionID).Error; err != nil {
		t.Fatalf("carregar receipt %s: %v", decisionID, err)
	}
	return row
}

func countEvents(t *testing.T, db *gorm.DB, decisionID string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&auditRow{}).Where("decision_id = ?", decisionID).Count(&count).Error; err != nil {
		t.Fatalf("contar eventos: %v", err)
	}
	return count
}

func countEffects(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&decisionEffectRow{}).Count(&count).Error; err != nil {
		t.Fatalf("contar efeitos: %v", err)
	}
	return count
}

func acceptedFixture(t *testing.T) (*Store, *gorm.DB, *time.Time, Request) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := now
	presenter := &testPresenter{fn: func(_ context.Context, req Request) (Response, error) {
		return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	request := defaultRequest(t, now)
	state, err := store.Decide(context.Background(), request)
	if err != nil || state != Accepted {
		t.Fatalf("fixture accepted: state=%q err=%v", state, err)
	}
	return store, db, &clock, request
}

func TestMigrateUsesD11ReceiptColumnsAndNeverPersistsBody(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	presenter := &testPresenter{fn: func(_ context.Context, req Request) (Response, error) {
		return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
	}}
	_, db := temporarySQLiteactualMigrate(t, presenter, &now)

	columns, err := db.Migrator().ColumnTypes(&receiptRow{})
	if err != nil {
		t.Fatalf("listar colunas da receipt: %v", err)
	}
	got := make(map[string]bool, len(columns))
	for _, column := range columns {
		got[strings.ToLower(column.Name())] = true
	}
	for _, want := range []string{
		"decision_id", "subject_id", "user_id", "auth_context_id", "request_fingerprint",
		"auth_generation", "security_generation", "expires_at", "status", "auth_context_type",
		"subject_type", "allowed_action_ids", "accepted_action_id", "responded_at", "consumed_at",
	} {
		if !got[want] {
			t.Errorf("coluna D11 ausente: %s (colunas=%v)", want, got)
		}
	}
	if got["body"] {
		t.Error("Body não pode ser persistido")
	}
}

func TestDecidePersistsAcceptedReceiptAndPendingAcceptedEvents(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := now
	var presented Request
	presenter := &testPresenter{fn: func(_ context.Context, req Request) (Response, error) {
		presented = req
		return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	request := defaultRequest(t, now)
	state, err := store.Decide(context.Background(), request)
	if err != nil || state != Accepted {
		t.Fatalf("Decide accepted: state=%q err=%v", state, err)
	}
	if presenter.calls.Load() != 1 || presented != request {
		t.Fatalf("presenter recebeu pedido inesperado: calls=%d request=%+v", presenter.calls.Load(), presented)
	}
	row := loadReceipt(t, db, request.DecisionID)
	if row.State != Accepted || row.ID != request.DecisionID || row.MutationID != request.MutationID ||
		row.UserID != request.UserID || row.SessionID != request.SessionID || row.Fingerprint != request.Fingerprint ||
		row.AuthGeneration != request.AuthGeneration || row.SecurityGeneration != request.SecurityGeneration ||
		row.ExpiresMS != request.ExpiresAt.UnixMilli() {
		t.Fatalf("receipt accepted inesperada: %+v", row)
	}
	if countEvents(t, db, request.DecisionID) != 2 {
		t.Fatal("receipt accepted deveria ter evento pending e accepted")
	}
}

func TestDecideTerminalStatesAndFailuresNeverAccept(t *testing.T) {
	presenterErr := errors.New("presenter indisponível")
	tests := []struct {
		name, action                               string
		presentErr                                 error
		wantState                                  string
		wantErr                                    error
		cancel, expire, panic, mismatch, cancelled bool
	}{
		{name: "denied", action: DenyAction, wantState: Denied},
		{name: "cancelled", wantState: Cancelled, cancelled: true},
		{name: "unknown action", action: "unknown", wantState: Cancelled, wantErr: ErrInvalid},
		{name: "mismatched decision id", action: ApplyAction, mismatch: true, wantState: Cancelled, wantErr: ErrInvalid},
		{name: "presenter error", presentErr: presenterErr, wantState: Cancelled, wantErr: presenterErr},
		{name: "request cancellation", cancel: true, wantState: Cancelled, wantErr: context.Canceled},
		{name: "expiry", expire: true, wantState: Expired},
		{name: "presenter panic", panic: true, wantState: Cancelled, wantErr: ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Millisecond)
			clock := now
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			presenter := &testPresenter{fn: func(pctx context.Context, req Request) (Response, error) {
				if test.panic {
					panic("fixture panic")
				}
				if test.cancel {
					cancel()
					<-pctx.Done()
					return Response{}, pctx.Err()
				}
				if test.expire {
					clock = req.ExpiresAt.Add(time.Millisecond)
				}
				decisionID := req.DecisionID
				if test.mismatch {
					decisionID = "00000000-0000-7000-8000-000000000000"
				}
				return Response{DecisionID: decisionID, ActionID: test.action, Cancelled: test.cancelled}, test.presentErr
			}}
			store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
			request := defaultRequest(t, now)
			state, err := store.Decide(ctx, request)
			if state != test.wantState || !errors.Is(err, test.wantErr) {
				t.Fatalf("resultado: state=%q err=%v; esperado state=%q err=%v", state, err, test.wantState, test.wantErr)
			}
			if loadReceipt(t, db, request.DecisionID).State == Accepted {
				t.Fatal("cancelamento, expiração ou panic produziu accepted")
			}
		})
	}
}

func TestDecideDuplicatePrimaryKeyDoesNotPresentAgain(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := now
	presenter := &testPresenter{fn: func(_ context.Context, req Request) (Response, error) {
		return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	request := defaultRequest(t, now)
	if state, err := store.Decide(context.Background(), request); err != nil || state != Accepted {
		t.Fatalf("primeira decisão: state=%q err=%v", state, err)
	}
	if _, err := store.Decide(context.Background(), request); err == nil {
		t.Fatal("decisão duplicada deveria falhar pela PK")
	}
	if presenter.calls.Load() != 1 || countEvents(t, db, request.DecisionID) != 2 {
		t.Fatalf("duplicata reapresentou ou adicionou evento: calls=%d events=%d", presenter.calls.Load(), countEvents(t, db, request.DecisionID))
	}
}

func TestDecideRejectsEachInvalidRequestFieldWithoutPersistence(t *testing.T) {
	invalid := []struct {
		name   string
		change func(*Request)
	}{
		{name: "decision id", change: func(req *Request) { req.DecisionID = "not-a-uuid" }},
		{name: "mutation id", change: func(req *Request) { req.MutationID = "not-a-uuid" }},
		{name: "user id", change: func(req *Request) { req.UserID = "not-a-uuid" }},
		{name: "session id", change: func(req *Request) { req.SessionID = "not-a-uuid" }},
		{name: "expiry", change: func(req *Request) { req.ExpiresAt = time.Time{} }},
		{name: "fingerprint", change: func(req *Request) { req.Fingerprint = " fingerprint" }},
		{name: "auth generation", change: func(req *Request) { req.AuthGeneration = " auth" }},
		{name: "security generation", change: func(req *Request) { req.SecurityGeneration = " security" }},
		{name: "body", change: func(req *Request) { req.Body = "   " }},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Millisecond)
			clock := now
			presenter := &testPresenter{fn: func(context.Context, Request) (Response, error) { return Response{}, nil }}
			store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
			request := defaultRequest(t, now)
			test.change(&request)
			if _, err := store.Decide(context.Background(), request); !errors.Is(err, ErrInvalid) {
				t.Fatalf("erro=%v, esperado ErrInvalid", err)
			}
			var receipts, events int64
			if err := db.Model(&receiptRow{}).Count(&receipts).Error; err != nil {
				t.Fatalf("contar receipts: %v", err)
			}
			if err := db.Model(&auditRow{}).Count(&events).Error; err != nil {
				t.Fatalf("contar eventos: %v", err)
			}
			if receipts != 0 || events != 0 || presenter.calls.Load() != 0 {
				t.Fatalf("entrada inválida vazou: receipts=%d events=%d calls=%d", receipts, events, presenter.calls.Load())
			}
		})
	}
}

func TestConsumeAcceptedOnceOnly(t *testing.T) {
	store, db, _, request := acceptedFixture(t)
	var calls atomic.Int32
	apply := func(tx *gorm.DB) error {
		calls.Add(1)
		return tx.Create(&decisionEffectRow{ID: request.DecisionID, Value: "applied"}).Error
	}
	if err := store.Consume(context.Background(), request, apply); err != nil {
		t.Fatalf("primeiro Consume: %v", err)
	}
	if err := store.Consume(context.Background(), request, apply); !errors.Is(err, ErrStale) {
		t.Fatalf("replay Consume=%v, esperado ErrStale", err)
	}
	if calls.Load() != 1 || loadReceipt(t, db, request.DecisionID).State != Consumed || countEvents(t, db, request.DecisionID) != 3 || countEffects(t, db) != 1 {
		t.Fatalf("consumo não foi único/atômico: calls=%d row=%+v events=%d effects=%d", calls.Load(), loadReceipt(t, db, request.DecisionID), countEvents(t, db, request.DecisionID), countEffects(t, db))
	}
}

func TestConsumeRejectsEveryMismatchedExpectedFieldWithoutEffect(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Request, *testing.T)
	}{
		{name: "decision id", change: func(req *Request, t *testing.T) { req.DecisionID = testUUIDv7(t) }},
		{name: "mutation id", change: func(req *Request, t *testing.T) { req.MutationID = testUUIDv7(t) }},
		{name: "user id", change: func(req *Request, t *testing.T) { req.UserID = testUUIDv7(t) }},
		{name: "session id", change: func(req *Request, t *testing.T) { req.SessionID = testUUIDv7(t) }},
		{name: "fingerprint", change: func(req *Request, _ *testing.T) { req.Fingerprint = "other-fingerprint" }},
		{name: "auth generation", change: func(req *Request, _ *testing.T) { req.AuthGeneration = "auth-generation-other" }},
		{name: "security generation", change: func(req *Request, _ *testing.T) { req.SecurityGeneration = "security-generation-other" }},
		{name: "expiry", change: func(req *Request, _ *testing.T) { req.ExpiresAt = req.ExpiresAt.Add(time.Second) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, db, _, request := acceptedFixture(t)
			expected := request
			test.change(&expected, t)
			var calls atomic.Int32
			err := store.Consume(context.Background(), expected, func(*gorm.DB) error { calls.Add(1); return nil })
			if !errors.Is(err, ErrStale) || calls.Load() != 0 {
				t.Fatalf("mismatch: err=%v calls=%d", err, calls.Load())
			}
			if loadReceipt(t, db, request.DecisionID).State != Accepted || countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
				t.Fatal("mismatch alterou receipt, auditoria ou efeito")
			}
		})
	}
}

func TestConsumeExpiredAfterAcceptanceFailsWithoutEffect(t *testing.T) {
	store, db, clock, request := acceptedFixture(t)
	*clock = request.ExpiresAt.Add(time.Millisecond)
	var calls atomic.Int32
	err := store.Consume(context.Background(), request, func(*gorm.DB) error { calls.Add(1); return nil })
	if !errors.Is(err, ErrStale) || calls.Load() != 0 {
		t.Fatalf("consume expirado: err=%v calls=%d", err, calls.Load())
	}
	if loadReceipt(t, db, request.DecisionID).State != Accepted || countEvents(t, db, request.DecisionID) != 2 {
		t.Fatal("receipt expirada após accepted não deveria ser consumida")
	}
}

func TestConsumeCallbackWriteAndFailureRollsBackReceiptEventAndEffect(t *testing.T) {
	store, db, _, request := acceptedFixture(t)
	wantErr := errors.New("falha do efeito")
	err := store.Consume(context.Background(), request, func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO command_decision_test_effects (id, value) VALUES (?, ?)", request.DecisionID, "written").Error; err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("erro do callback=%v", err)
	}
	if loadReceipt(t, db, request.DecisionID).State != Accepted || countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
		t.Fatal("falha do callback não reverteu receipt, evento e efeito")
	}
}

func TestConsumeAuditInsertionFailureRollsBackReceiptAndEffect(t *testing.T) {
	store, db, _, request := acceptedFixture(t)
	if err := db.Exec(`CREATE TRIGGER reject_command_decision_consume_audit
BEFORE INSERT ON command_decision_receipt_events
WHEN NEW.state = 'consumed'
BEGIN SELECT RAISE(ABORT, 'audit fixture failure'); END`).Error; err != nil {
		t.Fatalf("criar trigger de auditoria: %v", err)
	}
	var calls atomic.Int32
	err := store.Consume(context.Background(), request, func(tx *gorm.DB) error {
		calls.Add(1)
		return tx.Create(&decisionEffectRow{ID: request.DecisionID, Value: "must rollback"}).Error
	})
	if err == nil || calls.Load() != 0 {
		t.Fatalf("falha de auditoria não retornou/chegou ao callback: err=%v calls=%d", err, calls.Load())
	}
	if loadReceipt(t, db, request.DecisionID).State != Accepted || countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
		t.Fatal("falha de auditoria não reverteu receipt e efeito")
	}
}

func TestConsumeConcurrentExactlyOneCallback(t *testing.T) {
	store, db, _, request := acceptedFixture(t)
	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	var calls atomic.Int32
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := store.Consume(context.Background(), request, func(tx *gorm.DB) error {
				calls.Add(1)
				return tx.Create(&decisionEffectRow{ID: request.DecisionID, Value: "one callback"}).Error
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrStale) && !isTransientSQLiteError(err) {
			t.Errorf("erro inesperado no consumo concorrente: %v", err)
		}
	}
	if successes != 1 || calls.Load() != 1 {
		t.Fatalf("consumo concorrente: successes=%d callback=%d", successes, calls.Load())
	}
	if loadReceipt(t, db, request.DecisionID).State != Consumed || countEvents(t, db, request.DecisionID) != 3 || countEffects(t, db) != 1 {
		t.Fatal("estado concorrente final inconsistente")
	}
}

func isTransientSQLiteError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") || strings.Contains(message, "database is busy")
}
