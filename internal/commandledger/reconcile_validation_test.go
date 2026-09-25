package commandledger

import (
	"context"
	"errors"
	"testing"
)

func TestReconcileCancellationAfterVerificationPreservesUnknown(t *testing.T) {
	req := validRequest()
	now := req.ReceivedAt
	s, _ := testStore(t, &now)
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecoverClosedGeneration(context.Background(), req.Owner, req.SecurityGeneration); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed, err := s.Reconcile(ctx, req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) { cancel(); return Succeeded, nil })
	if changed || !errors.Is(err, context.Canceled) {
		t.Fatalf("%v %v", changed, err)
	}
	record, err := s.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || record.Status != OutcomeUnknown {
		t.Fatal("cancelamento não preservou incerteza")
	}
}

func TestReconcileRejectsMissingDependencies(t *testing.T) {
	var s *Store
	if ok, err := s.Reconcile(context.Background(), Owner{}, "", nil); ok || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("%v %v", ok, err)
	}
	req := validRequest()
	now := req.ReceivedAt
	s, _ = testStore(t, &now)
	if ok, err := s.Reconcile(nil, req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) { //nolint:staticcheck // Testa deliberadamente a recusa de contexto nil.
		t.Fatal("verificador inesperado")
		return Succeeded, nil
	}); ok || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("%v %v", ok, err)
	}
}

func TestReconcileAuditIdentityMismatchRollsBack(t *testing.T) {
	req := validRequest()
	now := req.ReceivedAt
	s, db := testStore(t, &now)
	ctx := context.Background()
	if _, err := s.Reserve(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecoverClosedGeneration(ctx, req.Owner, req.SecurityGeneration); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&invocationRow{}).Where("invocation_id = ?", req.InvocationID).Update("request_fingerprint", "mismatch").Error; err != nil {
		t.Fatal(err)
	}
	changed, err := s.Reconcile(ctx, req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) { return Succeeded, nil })
	if changed || !errors.Is(err, ErrInconsistent) {
		t.Fatalf("%v %v", changed, err)
	}
	record, err := s.Get(ctx, req.Owner, req.InvocationID)
	if err != nil || record.Status != OutcomeUnknown {
		t.Fatal("ledger avançou com identidade divergente")
	}
}
