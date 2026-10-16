package commandledger

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func testStore(t *testing.T, now *time.Time) (*Store, *gorm.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("schema: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	store, err := New(db, func() time.Time { return *now })
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store, db
}

func validRequest() LocalReadRequest {
	invocationID, _ := uuid.NewV7()
	authContextID, _ := uuid.NewV7()
	return LocalReadRequest{
		InvocationID: invocationID.String(), Owner: Owner{UserID: authContextID.String(), AuthContextID: authContextID.String()},
		AuthGeneration: "a1", SecurityGeneration: "s1", RegistryVersion: "r1",
		GlobalConfigGeneration: "g1", ActiveLayersGeneration: "l1", CommandID: "workspace.read",
		SourceType: "palette", ArgumentsFingerprint: "args", RequestFingerprintVersion: "v1",
		RequestFingerprint: "request", CorrelationID: "corr", ReceivedAt: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 9, 14, 10, 5, 0, 0, time.UTC),
	}
}

func TestReserveIsIdempotentAndDoesNotRenew(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, db := testStore(t, &now)
	req := validRequest()
	first, err := s.Reserve(context.Background(), req)
	if err != nil || !first.Created {
		t.Fatalf("primeira reserva: %+v, %v", first, err)
	}
	retry, err := s.Reserve(context.Background(), req)
	if err != nil || retry.Created || retry.Record.ExpiresAt != req.ExpiresAt {
		t.Fatalf("reentrega: %+v, %v", retry, err)
	}
	var ledgers, audits int64
	db.Model(&ledgerRow{}).Count(&ledgers)
	db.Model(&invocationRow{}).Count(&audits)
	if ledgers != 1 || audits != 1 {
		t.Fatalf("linhas inesperadas: ledger=%d audit=%d", ledgers, audits)
	}
}

func TestReserveRejectsConflictAndExpiryWithoutLeaking(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, db := testStore(t, &now)
	req := validRequest()
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	conflict := req
	conflict.RequestFingerprint = "other"
	if _, err := s.Reserve(context.Background(), conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflito = %v", err)
	}
	expired := validRequest()
	expired.ExpiresAt = now.Add(-time.Second)
	if _, err := s.Reserve(context.Background(), expired); !errors.Is(err, ErrExpired) {
		t.Fatalf("expirado = %v", err)
	}
	var n int64
	db.Model(&invocationRow{}).Where("invocation_id = ?", expired.InvocationID).Count(&n)
	if n != 0 {
		t.Fatal("reserva expirada deixou auditoria")
	}
}

func TestGetScopesOwnerAndCASUpdatesBothRows(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, _ := testStore(t, &now)
	req := validRequest()
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	otherID, _ := uuid.NewV7()
	if _, err := s.Get(context.Background(), Owner{UserID: otherID.String(), AuthContextID: req.Owner.AuthContextID}, req.InvocationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("vazamento de escopo: %v", err)
	}
	for _, step := range []struct{ from, to Status }{{Evaluating, Queued}, {Queued, Running}, {Running, Succeeded}} {
		ok, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, step.from, step.to)
		if err != nil || !ok {
			t.Fatalf("cas %s->%s: %v, %v", step.from, step.to, ok, err)
		}
	}
	ok, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, Running, Succeeded)
	if err != nil || ok {
		t.Fatalf("cas duplicado: %v, %v", ok, err)
	}
	got, err := s.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || got.Status != Succeeded {
		t.Fatalf("get final: %+v, %v", got, err)
	}
	var ledger ledgerRow
	var audit invocationRow
	if err := s.db.Where("invocation_id = ?", req.InvocationID).First(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Where("invocation_id = ?", req.InvocationID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.ResultSummary == nil || *ledger.ResultSummary != "{}" || audit.ResultSummary == nil || *audit.ResultSummary != "{}" {
		t.Fatalf("resumo terminal ausente: ledger=%v audit=%v", ledger.ResultSummary, audit.ResultSummary)
	}
}

func TestReserveConcorrenteAdmiteUmaReserva(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, db := testStore(t, &now)
	req := validRequest()
	results := make(chan Reservation, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := s.Reserve(context.Background(), req)
			results <- result
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	created := 0
	for result := range results {
		if result.Created {
			created++
		}
	}
	for err := range errs {
		if err != nil {
			var coded interface{ Code() int }
			if !errors.As(err, &coded) || (coded.Code()&255 != 5 && coded.Code()&255 != 6) {
				t.Fatal(err)
			}
		}
	}
	if created != 1 {
		t.Fatalf("reservas novas concorrentes: %d", created)
	}
	var count int64
	if err := db.Model(&ledgerRow{}).Where("invocation_id = ?", req.InvocationID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("mais de um ledger para uma invocação: %d", count)
	}
	if err := db.Model(&invocationRow{}).Where("invocation_id = ?", req.InvocationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("auditorias: %d %v", count, err)
	}
}
