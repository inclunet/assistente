package commandledger

import (
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"testing"
)

func TestReplayOwnershipAndSource(t *testing.T) {
	req := validRequest()
	now := req.ReceivedAt
	s, _ := testStore(t, &now)
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	other := uuid.Must(uuid.NewV7()).String()
	for _, mutate := range []func(*LocalReadRequest){
		func(r *LocalReadRequest) { r.Owner.UserID = other },
		func(r *LocalReadRequest) { r.Owner.AuthContextID = other },
		func(r *LocalReadRequest) { r.SourceType = "cli" },
		func(r *LocalReadRequest) { r.RequestFingerprintVersion = "v2" },
	} {
		retry := req
		mutate(&retry)
		result, err := s.Reserve(context.Background(), retry)
		if !errors.Is(err, ErrConflict) || result.Record.InvocationID != "" {
			t.Fatalf("reentrega indevida: %+v %v", result, err)
		}
	}
	for _, owner := range []Owner{{UserID: other, AuthContextID: req.Owner.AuthContextID}, {UserID: req.Owner.UserID, AuthContextID: other}} {
		if _, err := s.Get(context.Background(), owner, req.InvocationID); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		if changed, err := s.CompareAndSwap(context.Background(), owner, req.InvocationID, Evaluating, Queued); changed || err != nil {
			t.Fatalf("CAS outro escopo: %v %v", changed, err)
		}
	}
	retry := req
	retry.ExpiresAt = req.ExpiresAt.AddDate(0, 0, 1)
	now = req.ExpiresAt
	if result, err := s.Reserve(context.Background(), retry); !errors.Is(err, ErrExpired) || result.Created {
		t.Fatalf("expiração renovada: %+v %v", result, err)
	}
}

func TestAuditFailureRollsBackReservation(t *testing.T) {
	req := validRequest()
	now := req.ReceivedAt
	s, db := testStore(t, &now)
	if err := db.Exec(`CREATE TRIGGER reject_audit BEFORE INSERT ON command_invocations BEGIN SELECT RAISE(ABORT, 'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	result, err := s.Reserve(context.Background(), req)
	if err == nil || result.Created || result.Record.InvocationID != "" {
		t.Fatalf("resultado parcial: %+v %v", result, err)
	}
	var count int64
	if err := db.Model(&ledgerRow{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("ledger sobreviveu rollback: %d %v", count, err)
	}
}

func TestAuditMismatchRollsBackCAS(t *testing.T) {
	req := validRequest()
	now := req.ReceivedAt
	s, db := testStore(t, &now)
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&invocationRow{}).Where("invocation_id = ?", req.InvocationID).Update("status", Failed).Error; err != nil {
		t.Fatal(err)
	}
	changed, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, Evaluating, Queued)
	if changed || !errors.Is(err, ErrInconsistent) {
		t.Fatalf("CAS parcial: %v %v", changed, err)
	}
	record, err := s.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || record.Status != Evaluating {
		t.Fatalf("ledger avançou: %+v %v", record, err)
	}
}

func TestReopenPreservesReservation(t *testing.T) {
	req := validRequest()
	now := req.ReceivedAt
	s, db := testStore(t, &now)
	first, err := s.Reserve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var databases []struct {
		Name string
		File string
	}
	if err := db.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		t.Fatal(err)
	}
	path := ""
	for _, d := range databases {
		if d.Name == "main" {
			path = d.File
		}
	}
	if path == "" {
		t.Fatal("teste exige banco em arquivo")
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	resumed, err := New(reopened, s.now)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := resumed.Reserve(context.Background(), req)
	if err != nil || retry.Created || retry.Record.ID != first.Record.ID {
		t.Fatalf("reserva perdida após reabrir: %+v %v", retry, err)
	}
}
