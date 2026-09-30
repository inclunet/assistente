package commanddecision

import (
	"context"
	"database/sql"
	"errors"
	"io"
	stdlog "log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testPresenter struct {
	fn    func(context.Context, Request) (Response, error)
	calls atomic.Int32
}

type busyBeginObserver struct {
	logger.Interface
	busy chan struct{}
	once sync.Once
}

func (l *busyBeginObserver) Trace(ctx context.Context, begin time.Time, query func() (string, int64), err error) {
	sql, rows := query()
	if strings.EqualFold(strings.TrimSpace(sql), "BEGIN IMMEDIATE") && database.IsSQLiteBusyError(err) {
		l.once.Do(func() { close(l.busy) })
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

type decisionRunResult struct {
	state string
	err   error
}

func openContentionStore(t *testing.T, presenter Presenter) (*Store, *gorm.DB, *sql.DB, <-chan struct{}) {
	t.Helper()
	observer := &busyBeginObserver{Interface: logger.New(stdlog.New(io.Discard, "", 0), logger.Config{LogLevel: logger.Info}), busy: make(chan struct{})}
	path := filepath.Join(t.TempDir(), "command-decision-contention.db")
	db, err := gorm.Open(sqlite.Open("file:"+path+"?_pragma=busy_timeout(1)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: observer})
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
		t.Fatalf("migrar receipts: %v", err)
	}
	store, err := New(db, presenter, time.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store, db, sqlDB, observer.busy
}

func holdSQLiteWriter(t *testing.T, sqlDB *sql.DB) (release func()) {
	t.Helper()
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("reservar conexão para lock: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		_ = conn.Close()
		t.Fatalf("adquirir writer lock: %v", err)
	}
	var once sync.Once
	release = func() {
		once.Do(func() {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
			_ = conn.Close()
		})
	}
	t.Cleanup(release)
	return release
}

func startDecisionRun(t *testing.T, store *Store, ctx context.Context, request Request, releaseWriter func()) <-chan decisionRunResult {
	t.Helper()
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan decisionRunResult, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		state, err := store.Decide(workerCtx, request)
		done <- decisionRunResult{state: state, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		releaseWriter()
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			t.Error("goroutine Decide não terminou durante cleanup")
		}
	})
	return done
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

func TestDecideRetriesTransientSQLiteWriterContentionBeforePresentation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	presenter := &testPresenter{fn: func(_ context.Context, req Request) (Response, error) {
		return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
	}}
	store, db, sqlDB, busy := openContentionStore(t, presenter)
	release := holdSQLiteWriter(t, sqlDB)
	request := defaultRequest(t, now)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := startDecisionRun(t, store, ctx, request, release)
	select {
	case <-busy:
	case <-ctx.Done():
		t.Fatal("loggerTrace não observou SQLITE_BUSY em BEGIN IMMEDIATE")
	}
	release()
	select {
	case got := <-done:
		if got.err != nil || got.state != Accepted {
			t.Fatalf("Decide após contenção: state=%q err=%v", got.state, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Decide não terminou após liberar writer lock")
	}
	if presenter.calls.Load() != 1 {
		t.Fatalf("decisão foi apresentada %d vezes, esperado uma", presenter.calls.Load())
	}
	if row := loadReceipt(t, db, request.DecisionID); row.State != Accepted || countEvents(t, db, request.DecisionID) != 2 {
		t.Fatalf("reserva/finalização não persistida após retry: receipt=%+v events=%d", row, countEvents(t, db, request.DecisionID))
	}
}

func TestDecideCancellationWhileWaitingForWriterDoesNotPresentOrInsert(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	presenter := &testPresenter{fn: func(_ context.Context, req Request) (Response, error) {
		return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
	}}
	store, db, sqlDB, busy := openContentionStore(t, presenter)
	release := holdSQLiteWriter(t, sqlDB)
	request := defaultRequest(t, now)
	ctx, cancel := context.WithCancel(context.Background())
	done := startDecisionRun(t, store, ctx, request, release)
	select {
	case <-busy:
	case <-time.After(2 * time.Second):
		t.Fatal("loggerTrace não observou SQLITE_BUSY em BEGIN IMMEDIATE")
	}
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("Decide cancelado retornou state=%q err=%v", got.state, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Decide não encerrou após cancelamento")
	}
	release()
	if presenter.calls.Load() != 0 {
		t.Fatalf("presenter chamado %d vezes após cancelamento pré-reserva", presenter.calls.Load())
	}
	var receipts, events int64
	if err := db.Model(&receiptRow{}).Where("decision_id = ?", request.DecisionID).Count(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&auditRow{}).Where("decision_id = ?", request.DecisionID).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if receipts != 0 || events != 0 {
		t.Fatalf("cancelamento pré-reserva persistiu receipt/eventos: %d/%d", receipts, events)
	}
}

func TestDecideRetriesFinalizationAfterTransientWriterContention(t *testing.T) {
	presenterEntered := make(chan struct{})
	allowResponse := make(chan struct{})
	presenter := &testPresenter{fn: func(ctx context.Context, req Request) (Response, error) {
		close(presenterEntered)
		select {
		case <-allowResponse:
			return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}}
	store, db, sqlDB, busy := openContentionStore(t, presenter)
	request := defaultRequest(t, time.Now().UTC())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var release func()
	done := startDecisionRun(t, store, ctx, request, func() {
		if release != nil {
			release()
		}
	})
	select {
	case <-presenterEntered:
	case <-ctx.Done():
		t.Fatal("receipt não chegou ao presenter")
	}
	release = holdSQLiteWriter(t, sqlDB)
	close(allowResponse)
	select {
	case <-busy:
	case <-ctx.Done():
		t.Fatal("loggerTrace não observou contenção durante finalização")
	}
	release()
	select {
	case got := <-done:
		if got.err != nil || got.state != Accepted {
			t.Fatalf("Decide após retry de finalização: state=%q err=%v", got.state, got.err)
		}
	case <-ctx.Done():
		t.Fatal("finalização não terminou após liberar writer")
	}
	if presenter.calls.Load() != 1 || loadReceipt(t, db, request.DecisionID).State != Accepted {
		t.Fatalf("finalização alterou apresentação/estado: calls=%d receipt=%+v", presenter.calls.Load(), loadReceipt(t, db, request.DecisionID))
	}
}

func TestDecideExpiresWhileWaitingForFinalizationWriter(t *testing.T) {
	presenterEntered := make(chan struct{})
	allowResponse := make(chan struct{})
	presenter := &testPresenter{fn: func(ctx context.Context, req Request) (Response, error) {
		close(presenterEntered)
		select {
		case <-allowResponse:
			return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}}
	store, db, sqlDB, busy := openContentionStore(t, presenter)
	request := defaultRequest(t, time.Now().UTC())
	request.ExpiresAt = time.Now().Add(300 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var release func()
	done := startDecisionRun(t, store, ctx, request, func() {
		if release != nil {
			release()
		}
	})
	select {
	case <-presenterEntered:
	case <-ctx.Done():
		t.Fatal("receipt não chegou ao presenter")
	}
	release = holdSQLiteWriter(t, sqlDB)
	close(allowResponse)
	select {
	case <-busy:
	case <-ctx.Done():
		t.Fatal("loggerTrace não observou contenção durante finalização")
	}
	deadlineWait := time.Until(request.ExpiresAt)
	if deadlineWait > 0 {
		timer := time.NewTimer(deadlineWait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			t.Fatal("teste terminou antes do deadline da decisão")
		}
	}
	release()
	select {
	case got := <-done:
		if got.state != Expired || !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("deadline expirado na finalização: state=%q err=%v", got.state, got.err)
		}
	case <-ctx.Done():
		t.Fatal("finalização expirada não persistiu estado terminal")
	}
	row := loadReceipt(t, db, request.DecisionID)
	if row.State != Expired || row.AcceptedActionID != nil || presenter.calls.Load() != 1 {
		t.Fatalf("deadline permitiu aceite: receipt=%+v presenter_calls=%d", row, presenter.calls.Load())
	}
}

func TestExternalTokenInvocationPersistsExactContextAndConsumes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := now
	presenter := &testPresenter{fn: func(_ context.Context, req Request) (Response, error) {
		return Response{DecisionID: req.DecisionID, ActionID: ApplyAction}, nil
	}}
	store, db := temporarySQLiteactualMigrate(t, presenter, &clock)
	request := defaultRequest(t, now)
	request.AuthContextType = "external_token"
	request.SessionID = auth.ExternalTokenContextID("https://issuer.example", "subject-7", "opaque-signed-token")
	request.SubjectType = "invocation"
	state, err := store.Decide(context.Background(), request)
	if err != nil || state != Accepted {
		t.Fatalf("external invocation: state=%s err=%v", state, err)
	}
	row := loadReceipt(t, db, request.DecisionID)
	if row.AuthContextType != "external_token" || row.SessionID != request.SessionID || row.SubjectType != "invocation" {
		t.Fatalf("ownership externo foi normalizado/perdido: %+v", row)
	}
	for name, altered := range map[string]Request{
		"tipo": func() Request {
			copy := request
			copy.AuthContextType = "local_session"
			copy.SessionID = testUUIDv7(t)
			return copy
		}(),
		"id": func() Request {
			copy := request
			copy.SessionID = auth.ExternalTokenContextID("https://issuer.example", "subject-7", "different-token")
			return copy
		}(),
	} {
		t.Run("consume rejeita "+name, func(t *testing.T) {
			if err := store.Consume(context.Background(), altered, func(*gorm.DB) error {
				t.Fatal("efeito executou com ownership de decisão divergente")
				return nil
			}); !errors.Is(err, ErrStale) {
				t.Fatalf("consume com %s divergente retornou %v", name, err)
			}
			if got := loadReceipt(t, db, request.DecisionID).State; got != Accepted {
				t.Fatalf("receipt original alterada por tentativa divergente: %s", got)
			}
		})
	}
	if err := store.Consume(context.Background(), request, func(tx *gorm.DB) error {
		return tx.Create(&decisionEffectRow{ID: request.MutationID, Value: "done"}).Error
	}); err != nil {
		t.Fatalf("consumir receipt externa: %v", err)
	}
	row = loadReceipt(t, db, request.DecisionID)
	if row.State != Consumed || row.AuthContextType != "external_token" || row.SessionID != request.SessionID || countEffects(t, db) != 1 {
		t.Fatalf("consumo externo não preservou owner/efeito: %+v effects=%d", row, countEffects(t, db))
	}
}

func TestExternalTokenContextIDMustBeCanonicalAndInvocationOnly(t *testing.T) {
	now := time.Now().UTC()
	request := defaultRequest(t, now)
	request.AuthContextType = "external_token"
	request.SubjectType = "invocation"
	valid := auth.ExternalTokenContextID("https://issuer.example", "subject-7", "opaque-signed-token")
	request.SessionID = valid
	if !validRequest(request) {
		t.Fatal("ID canônico emitido pelo autenticador externo foi rejeitado")
	}
	for name, value := range map[string]string{
		"jwt bruto":             "eyJhbGciOiJub25lIn0.eyJzdWIiOiJzdWJqZWN0LTcifQ.",
		"uuid local":            request.DecisionID,
		"tupla não canônica":    `[ "https://issuer.example","subject-7","` + strings.Repeat("a", 64) + `" ]`,
		"fingerprint uppercase": `["https://issuer.example","subject-7","` + strings.Repeat("A", 64) + `"]`,
		"controle escapado":     `["issuer\n","subject-7","` + strings.Repeat("a", 64) + `"]`,
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			candidate.SessionID = value
			if validRequest(candidate) {
				t.Fatalf("auth_context_id não canônico aceito: %q", value)
			}
		})
	}
	request.SubjectType = "config_mutation"
	if validRequest(request) {
		t.Fatal("external_token não pode autorizar config_mutation")
	}
	request.SubjectType = ""
	if validRequest(request) {
		t.Fatal("subject vazio de compatibilidade não pode transformar contexto externo em config_mutation")
	}
}

func TestDecisionReceiptSQLCheckRejectsInvalidExternalSubjectAndType(t *testing.T) {
	now := time.Now().UTC()
	_, db := temporarySQLiteactualMigrate(t, &testPresenter{}, &now)
	request := defaultRequest(t, now)
	row := rowOf(request)
	row.AuthContextType = "external_token"
	row.SessionID = auth.ExternalTokenContextID("issuer", "subject", "token")
	row.SubjectType = "config_mutation"
	if err := db.Create(&row).Error; err == nil {
		t.Fatal("CHECK SQL aceitou config_mutation externo")
	}
	row.SubjectType = "invocation"
	row.AuthContextType = "unrecognized"
	if err := db.Create(&row).Error; err == nil {
		t.Fatal("CHECK SQL aceitou auth_context_type desconhecido")
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
		} else if !errors.Is(err, ErrStale) {
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

func TestConsumeCASRejectsIncompleteOrDifferentAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name, column string
		value        any
	}{
		{"missing response", "responded_at", nil},
		{"missing action", "accepted_action_id", nil},
		{"denied action", "accepted_action_id", DenyAction},
		{"different allowed actions", "allowed_action_ids", `["apply"]`},
		{"already consumed timestamp", "consumed_at", int64(1)},
		{"different subject", "subject_type", "invocation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db, _, request := acceptedFixture(t)
			if err := db.Model(&receiptRow{}).Where("decision_id = ?", request.DecisionID).Update(tc.column, tc.value).Error; err != nil {
				t.Fatal(err)
			}
			calls := 0
			err := store.Consume(context.Background(), request, func(*gorm.DB) error { calls++; return nil })
			if !errors.Is(err, ErrStale) || calls != 0 {
				t.Fatalf("aceitação inconsistente consumida: err=%v calls=%d", err, calls)
			}
			if loadReceipt(t, db, request.DecisionID).State != Accepted || countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
				t.Fatal("tentativa inválida alterou receipt, auditoria ou efeito")
			}
		})
	}
}

func isTransientSQLiteError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") || strings.Contains(message, "database is busy")
}
