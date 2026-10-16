package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func prepareUnknown(t *testing.T, s *Store, req LocalReadRequest) {
	t.Helper()
	if _, err := s.Reserve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RecoverClosedGeneration(context.Background(), req.Owner, req.SecurityGeneration); err != nil || n != 1 {
		t.Fatalf("preparar outcome_unknown: %d, %v", n, err)
	}
}

func TestReconcilePropagatesVerifierErrorsWithoutChangingUnknown(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 1, 0, 0, time.UTC)
	s, _ := testStore(t, &now)
	req := validRequest()
	prepareUnknown(t, s, req)

	want := errors.New("fonte indisponível")
	ok, err := s.Reconcile(context.Background(), req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) {
		return Status("ignored"), want
	})
	if ok || !errors.Is(err, want) {
		t.Fatalf("erro do verificador: %v, %v", ok, err)
	}
	got, err := s.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || got.Status != OutcomeUnknown {
		t.Fatalf("incerteza alterada após erro: %+v, %v", got, err)
	}
}

func TestReconcileSuccessAndFailureUpdateLedgerAndAudit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome Status
		errCode *string
	}{
		{name: "sucesso", outcome: Succeeded},
		{name: "falha", outcome: Failed, errCode: func() *string { v := "command_failed"; return &v }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 9, 14, 10, 2, 3, 0, time.UTC)
			s, db := testStore(t, &now)
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			req := validRequest()
			prepareUnknown(t, s, req)
			callbackCalled := false

			ok, err := s.Reconcile(context.Background(), req.Owner, req.InvocationID, func(ctx context.Context, record Record) (Status, error) {
				callbackCalled = true
				if record.Status != OutcomeUnknown || record.Owner != req.Owner {
					t.Fatalf("registro entregue ao verificador: %+v", record)
				}
				var row ledgerRow
				callbackCtx, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				if err := db.WithContext(callbackCtx).Where("invocation_id = ?", req.InvocationID).First(&row).Error; err != nil {
					t.Fatalf("callback não conseguiu ler fora da transação: %v", err)
				}
				return tc.outcome, nil
			})
			if err != nil || !ok || !callbackCalled {
				t.Fatalf("reconciliação: %v, %v, callback=%v", ok, err, callbackCalled)
			}

			var ledger ledgerRow
			var audit invocationRow
			if err := db.Where("invocation_id = ?", req.InvocationID).First(&ledger).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Where("invocation_id = ?", req.InvocationID).First(&audit).Error; err != nil {
				t.Fatal(err)
			}
			if ledger.Status != tc.outcome || audit.Status != tc.outcome || ledger.ResultSummary == nil || *ledger.ResultSummary != "{}" || audit.ResultSummary == nil || *audit.ResultSummary != "{}" {
				t.Fatalf("estado terminal: ledger=%+v audit=%+v", ledger, audit)
			}
			if audit.CompletedAt == nil || !audit.CompletedAt.Equal(now) {
				t.Fatalf("completed_at=%v, esperado %v", audit.CompletedAt, now)
			}
			if tc.errCode == nil && audit.ErrorCode != nil {
				t.Fatalf("sucesso manteve error_code=%v", *audit.ErrorCode)
			}
			if tc.errCode != nil && (audit.ErrorCode == nil || *audit.ErrorCode != *tc.errCode) {
				t.Fatalf("error_code=%v, esperado %v", audit.ErrorCode, *tc.errCode)
			}
		})
	}
}

func TestReconcileNeverCallsVerifierForOtherOwnerMissingOrTerminal(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 3, 0, 0, time.UTC)
	s, _ := testStore(t, &now)
	req := validRequest()
	prepareUnknown(t, s, req)
	called := 0
	verify := func(context.Context, Record) (Status, error) {
		called++
		return Succeeded, nil
	}

	other := validRequest().Owner
	if ok, err := s.Reconcile(context.Background(), other, req.InvocationID, verify); ok || !errors.Is(err, ErrNotFound) {
		t.Fatalf("outro owner: %v, %v", ok, err)
	}
	missing := validRequest()
	if ok, err := s.Reconcile(context.Background(), req.Owner, missing.InvocationID, verify); ok || !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v, %v", ok, err)
	}
	if called != 0 {
		t.Fatalf("verificador chamado antes de encontrar registro escopado: %d", called)
	}

	terminalReq := validRequest()
	if _, err := s.Reserve(context.Background(), terminalReq); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ from, to Status }{{Evaluating, Queued}, {Queued, Running}, {Running, Succeeded}} {
		if ok, err := s.CompareAndSwap(context.Background(), terminalReq.Owner, terminalReq.InvocationID, step.from, step.to); err != nil || !ok {
			t.Fatalf("terminalizar: %v, %v", ok, err)
		}
	}
	if ok, err := s.Reconcile(context.Background(), terminalReq.Owner, terminalReq.InvocationID, verify); ok || err != nil {
		t.Fatalf("terminal existente: %v, %v", ok, err)
	}
	if called != 0 {
		t.Fatal("verificador chamado para estado terminal")
	}
}

func TestReconcileInvalidOutcomeDoesNotChangeUnknown(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 4, 0, 0, time.UTC)
	s, _ := testStore(t, &now)
	req := validRequest()
	prepareUnknown(t, s, req)
	if ok, err := s.Reconcile(context.Background(), req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) {
		return Running, nil
	}); ok || !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("outcome inválido: %v, %v", ok, err)
	}
	got, err := s.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || got.Status != OutcomeUnknown {
		t.Fatalf("outcome_unknown alterado: %+v, %v", got, err)
	}
}

func TestReconcileRaceDoesNotOverwriteTerminalWinner(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 4, 0, 0, time.UTC)
	s, db := testStore(t, &now)
	req := validRequest()
	prepareUnknown(t, s, req)
	now = time.Date(2026, 9, 14, 10, 5, 0, 0, time.UTC)
	winnerTime := now.Add(time.Minute)

	ok, err := s.Reconcile(context.Background(), req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) {
		return Succeeded, db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&ledgerRow{}).Where("invocation_id = ? AND user_id = ? AND status = ?", req.InvocationID, req.Owner.UserID, OutcomeUnknown).Updates(map[string]any{"status": Succeeded, "result_summary": "{}"}).Error; err != nil {
				return err
			}
			return tx.Model(&invocationRow{}).Where("invocation_id = ? AND user_id = ? AND status = ?", req.InvocationID, req.Owner.UserID, OutcomeUnknown).Updates(map[string]any{"status": Succeeded, "result_summary": "{}", "completed_at": winnerTime, "error_code": nil}).Error
		})
	})
	if err != nil || ok {
		t.Fatalf("CAS deveria perder a corrida: %v, %v", ok, err)
	}
	got, err := s.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || got.Status != Succeeded {
		t.Fatalf("vencedor sobrescrito: %+v, %v", got, err)
	}
	var audit invocationRow
	if err := db.Where("invocation_id = ?", req.InvocationID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Status != Succeeded || audit.CompletedAt == nil || !audit.CompletedAt.Equal(winnerTime) {
		t.Fatalf("auditoria do vencedor sobrescrita: %+v", audit)
	}
}

func TestReconcileRollsBackLedgerWhenAuditTriggerFails(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 4, 0, 0, time.UTC)
	s, db := testStore(t, &now)
	req := validRequest()
	prepareUnknown(t, s, req)
	now = time.Date(2026, 9, 14, 10, 6, 0, 0, time.UTC)
	if err := db.Exec(`CREATE TRIGGER reject_reconcile BEFORE UPDATE ON command_invocations BEGIN SELECT RAISE(ABORT, 'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}

	ok, err := s.Reconcile(context.Background(), req.Owner, req.InvocationID, func(context.Context, Record) (Status, error) {
		return Succeeded, nil
	})
	if ok || err == nil {
		t.Fatalf("falha de auditoria deveria reverter: %v, %v", ok, err)
	}
	got, err := s.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || got.Status != OutcomeUnknown {
		t.Fatalf("ledger avançou sem auditoria: %+v, %v", got, err)
	}
	var audit invocationRow
	if err := db.Where("invocation_id = ?", req.InvocationID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Status != OutcomeUnknown {
		t.Fatalf("auditoria não reverteu: %s", audit.Status)
	}
}
