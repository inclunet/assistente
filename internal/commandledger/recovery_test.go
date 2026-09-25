package commandledger

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"testing"
)

func TestRecoverClosedGenerationIsScopedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	req := validRequest()
	now := req.ReceivedAt
	s, db := testStore(t, &now)
	ids := []string{}
	for _, status := range []Status{Evaluating, Queued, Running} {
		r := req
		r.InvocationID = uuid.Must(uuid.NewV7()).String()
		ids = append(ids, r.InvocationID)
		if _, err := s.Reserve(ctx, r); err != nil {
			t.Fatal(err)
		}
		if status != Evaluating {
			if ok, err := s.CompareAndSwap(ctx, r.Owner, r.InvocationID, Evaluating, Queued); !ok || err != nil {
				t.Fatal(err)
			}
		}
		if status == Running {
			if ok, err := s.CompareAndSwap(ctx, r.Owner, r.InvocationID, Queued, Running); !ok || err != nil {
				t.Fatal(err)
			}
		}
	}
	active := req
	active.InvocationID = uuid.Must(uuid.NewV7()).String()
	active.SecurityGeneration = "active"
	other := req
	other.InvocationID = uuid.Must(uuid.NewV7()).String()
	other.Owner.AuthContextID = uuid.Must(uuid.NewV7()).String()
	for _, r := range []LocalReadRequest{active, other} {
		if _, err := s.Reserve(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.RecoverClosedGeneration(ctx, req.Owner, req.SecurityGeneration)
	if err != nil || n != 3 {
		t.Fatalf("recuperados %d: %v", n, err)
	}
	for _, id := range ids {
		record, err := s.Get(ctx, req.Owner, id)
		if err != nil || record.Status != OutcomeUnknown {
			t.Fatalf("%+v %v", record, err)
		}
		var audit invocationRow
		if err := db.Where("invocation_id = ?", id).First(&audit).Error; err != nil {
			t.Fatal(err)
		}
		if audit.Status != OutcomeUnknown || audit.CompletedAt == nil || audit.ErrorCode == nil || audit.ResultSummary == nil {
			t.Fatal("terminal incompleto")
		}
	}
	for _, r := range []LocalReadRequest{active, other} {
		record, err := s.Get(ctx, r.Owner, r.InvocationID)
		if err != nil || record.Status != Evaluating {
			t.Fatal("escopo ultrapassado")
		}
	}
	n, err = s.RecoverClosedGeneration(ctx, req.Owner, req.SecurityGeneration)
	if err != nil || n != 0 {
		t.Fatalf("repetição: %d %v", n, err)
	}
	replay := req
	replay.InvocationID = ids[0]
	result, err := s.Reserve(ctx, replay)
	if err != nil || result.Created || result.Record.Status != OutcomeUnknown {
		t.Fatalf("reexecutável: %+v %v", result, err)
	}
}

func TestRecoveryRollsBackOnAuditFailure(t *testing.T) {
	ctx := context.Background()
	req := validRequest()
	now := req.ReceivedAt
	s, db := testStore(t, &now)
	if _, err := s.Reserve(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_recovery BEFORE UPDATE ON command_invocations BEGIN SELECT RAISE(ABORT, 'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	n, err := s.RecoverClosedGeneration(ctx, req.Owner, req.SecurityGeneration)
	if err == nil || n != 0 {
		t.Fatalf("rollback %d %v", n, err)
	}
	record, err := s.Get(ctx, req.Owner, req.InvocationID)
	if err != nil || record.Status != Evaluating {
		t.Fatal("ledger avançou sem auditoria")
	}
}

func TestRecoveryRejectsInconsistentPairAndCancelledContext(t *testing.T) {
	req := validRequest()
	now := req.ReceivedAt
	s, db := testStore(t, &now)
	ctx := context.Background()
	if _, err := s.Reserve(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ledgerRow{}).Where("invocation_id = ?", req.InvocationID).Update("request_fingerprint", "different").Error; err != nil {
		t.Fatal(err)
	}
	if n, err := s.RecoverClosedGeneration(ctx, req.Owner, req.SecurityGeneration); n != 0 || !errors.Is(err, ErrInconsistent) {
		t.Fatalf("%d %v", n, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.RecoverClosedGeneration(cancelled, req.Owner, req.SecurityGeneration); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.RecoverClosedGeneration(ctx, req.Owner, ""); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}
