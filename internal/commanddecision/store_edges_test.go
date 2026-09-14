package commanddecision

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type edgePresenter func(context.Context, Request) (Response, error)

func (p edgePresenter) Present(ctx context.Context, request Request) (Response, error) {
	return p(ctx, request)
}
func edgeFixture(t *testing.T) (*gorm.DB, Request) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "decision.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	return db, Request{DecisionID: id(), MutationID: id(), UserID: id(), SessionID: id(), Fingerprint: "fixture-fingerprint", AuthGeneration: "auth:1", SecurityGeneration: "security:1", ExpiresAt: time.Now().Add(time.Minute), Body: "fixture diff"}
}
func edgeAccept(_ context.Context, r Request) (Response, error) {
	return Response{DecisionID: r.DecisionID, ActionID: ApplyAction}, nil
}

func TestDecisionRequiresPresenterAndRejectsContradictoryResponse(t *testing.T) {
	db, request := edgeFixture(t)
	if _, err := New(db, nil, time.Now); !errors.Is(err, ErrInvalid) {
		t.Fatal("presenter ausente aceito", err)
	}
	s, err := New(db, edgePresenter(func(_ context.Context, r Request) (Response, error) {
		return Response{DecisionID: r.DecisionID, ActionID: ApplyAction, Cancelled: true}, nil
	}), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.Decide(context.Background(), request)
	if state != Cancelled || !errors.Is(err, ErrInvalid) {
		t.Fatal("resposta contraditória aceita", state, err)
	}
	if err := s.Consume(context.Background(), request, func(*gorm.DB) error { t.Fatal("cancelamento consumido"); return nil }); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}

func TestDecisionConsumptionExpiryAfterEffectRollsBack(t *testing.T) {
	db, request := edgeFixture(t)
	now := time.Now()
	s, err := New(db, edgePresenter(edgeAccept), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if state, err := s.Decide(context.Background(), request); err != nil || state != Accepted {
		t.Fatal(state, err)
	}
	if err := db.Exec("CREATE TABLE decision_fixture_effect (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	err = s.Consume(context.Background(), request, func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO decision_fixture_effect(id) VALUES (?)", request.MutationID).Error; err != nil {
			return err
		}
		now = request.ExpiresAt.Add(time.Second)
		return nil
	})
	if !errors.Is(err, ErrStale) {
		t.Fatal("prazo vencido no efeito não reverteu", err)
	}
	var count int64
	if err := db.Table("decision_fixture_effect").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("efeito não revertido")
	}
	var row receiptRow
	if err := db.Take(&row, "decision_id = ?", request.DecisionID).Error; err != nil {
		t.Fatal(err)
	}
	if row.State != Accepted || row.ConsumedAt != nil {
		t.Fatal("consumo não revertido")
	}
	if err := db.Model(&auditRow{}).Where("decision_id = ?", request.DecisionID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("evento de consumo não revertido", count)
	}
}

func TestDecisionCancelledDuringAcceptanceNeverRecordsAccepted(t *testing.T) {
	db, request := edgeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := New(db, edgePresenter(edgeAccept), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	updated := false
	const hook = "fixture:cancel-receipt"
	if err := db.Callback().Update().After("gorm:update").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_decision_receipts" && !updated && tx.RowsAffected == 1 {
			updated = true
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Update().Remove(hook) })
	state, err := s.Decide(ctx, request)
	if !updated || state != Cancelled || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelamento na gravação aceitou decisão", state, err)
	}
	var row receiptRow
	if err := db.Take(&row, "decision_id = ?", request.DecisionID).Error; err != nil {
		t.Fatal(err)
	}
	if row.State != Cancelled || row.AcceptedActionID != nil {
		t.Fatal("aceitação indevida", row)
	}
	var count int64
	if err := db.Model(&auditRow{}).Where("decision_id = ? AND state = ?", request.DecisionID, Accepted).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("evento afirmativo indevido")
	}
}

func TestDecisionPendingAuditFailureDoesNotPresent(t *testing.T) {
	db, request := edgeFixture(t)
	if err := db.Exec("CREATE TRIGGER reject_decision_event BEFORE INSERT ON command_decision_receipt_events BEGIN SELECT RAISE(ABORT,'fixture'); END").Error; err != nil {
		t.Fatal(err)
	}
	s, err := New(db, edgePresenter(func(context.Context, Request) (Response, error) {
		t.Fatal("presenter antes de pending durável")
		return Response{}, nil
	}), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(context.Background(), request); err == nil {
		t.Fatal("erro de auditoria ignorado")
	}
	var count int64
	if err := db.Model(&receiptRow{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("pending não revertido")
	}
}
