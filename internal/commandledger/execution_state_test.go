package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCompareAndSwapEvaluatingUpdatesPolicyDecision(t *testing.T) {
	for _, test := range []struct {
		name       string
		to         Status
		wantPolicy string
	}{
		{name: "allowed", to: Queued, wantPolicy: "allowed"},
		{name: "denied", to: Denied, wantPolicy: "denied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
			s, db := testStore(t, &now)
			req := validRequest()
			if _, err := s.Reserve(context.Background(), req); err != nil {
				t.Fatal(err)
			}

			changed, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, Evaluating, test.to)
			if err != nil || !changed {
				t.Fatalf("CAS = %v, %v", changed, err)
			}

			var audit invocationRow
			if err := db.Where("invocation_id = ?", req.InvocationID).First(&audit).Error; err != nil {
				t.Fatal(err)
			}
			if audit.Status != test.to || audit.PolicyDecision != test.wantPolicy {
				t.Fatalf("auditoria = status %s, policy %q", audit.Status, audit.PolicyDecision)
			}
		})
	}
}

func TestCompareAndSwapAuditMismatchRollsBackLedgerAndPolicy(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, db := testStore(t, &now)
	req := validRequest()
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&invocationRow{}).Where("invocation_id = ?", req.InvocationID).Updates(map[string]any{
		"status":          Failed,
		"policy_decision": "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}

	changed, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, Evaluating, Queued)
	if changed || !errors.Is(err, ErrInconsistent) {
		t.Fatalf("CAS parcial: %v %v", changed, err)
	}
	var ledger ledgerRow
	var audit invocationRow
	if err := db.Where("invocation_id = ?", req.InvocationID).First(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("invocation_id = ?", req.InvocationID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.Status != Evaluating || audit.Status != Failed || audit.PolicyDecision != "pending" {
		t.Fatalf("estado após rollback: ledger=%s audit=%s policy=%q", ledger.Status, audit.Status, audit.PolicyDecision)
	}
}

func TestCompareAndSwapAllowsQueuedToFailed(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, _ := testStore(t, &now)
	req := validRequest()
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, Evaluating, Queued); !changed || err != nil {
		t.Fatalf("evaluating->queued: %v, %v", changed, err)
	}
	if changed, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, Queued, Failed); !changed || err != nil {
		t.Fatalf("queued->failed: %v, %v", changed, err)
	}
}

func TestCompareAndSwapKeepsRunningToFailed(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, _ := testStore(t, &now)
	req := validRequest()
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ from, to Status }{{Evaluating, Queued}, {Queued, Running}, {Running, Failed}} {
		changed, err := s.CompareAndSwap(context.Background(), req.Owner, req.InvocationID, step.from, step.to)
		if !changed || err != nil {
			t.Fatalf("%s->%s: %v, %v", step.from, step.to, changed, err)
		}
	}
}
