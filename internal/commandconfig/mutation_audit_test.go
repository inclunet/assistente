package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commanddecision"
)

func TestGetBindingMutationSucessoPreservaMudancaEVinculos(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	expiresAt := decisionExpiresAt(fixture)
	confirmed := confirmDecisionTest(t, fixture, expiresAt, func(Binding, Binding) (string, error) {
		return "diff", nil
	})
	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); err != nil {
		t.Fatalf("CommitConfirmedBindingEnabled: %v", err)
	}

	got, err := fixture.projection.store.GetBindingMutation(
		context.Background(), fixture.epoch.UserID, fixture.epoch.SessionID, confirmed.request.MutationID,
	)
	if err != nil {
		t.Fatalf("GetBindingMutation: %v", err)
	}
	generation := fixture.projection.snapshot.Generations[0]
	if got.MutationID != confirmed.request.MutationID || got.DecisionID != confirmed.request.DecisionID {
		t.Fatalf("IDs da auditoria divergentes: got mutation=%s decision=%s want mutation=%s decision=%s",
			got.MutationID, got.DecisionID, confirmed.request.MutationID, confirmed.request.DecisionID)
	}
	if got.UserID != fixture.epoch.UserID || got.SessionID != fixture.epoch.SessionID || got.BindingID != fixture.binding.ID {
		t.Fatalf("escopo da auditoria inesperado: got=%#v", got)
	}
	if got.GenerationID != generation.ID || got.BeforeGeneration != generation.Generation || got.AfterGeneration != generation.Generation+1 {
		t.Fatalf("gerações da auditoria inesperadas: got=%#v", got)
	}
	if got.BeforeEnabled != fixture.binding.Enabled || got.AfterEnabled {
		t.Fatalf("before/after enabled inesperados: got before=%v after=%v", got.BeforeEnabled, got.AfterEnabled)
	}
	if got.RequestFingerprint != confirmed.request.Fingerprint || got.AuthGeneration != confirmed.request.AuthGeneration || got.SecurityGeneration != confirmed.request.SecurityGeneration {
		t.Fatalf("vínculos da receipt divergentes na auditoria: got=%#v request=%#v", got, confirmed.request)
	}
	if got.SchemaVersion != 1 || got.Scope != "global" || got.Operation != "binding_enabled" || got.OccurredAt.IsZero() {
		t.Fatalf("metadados da auditoria inesperados: got=%#v", got)
	}
}

func TestGetBindingMutationRecusaUsuarioESessaoEstrangeiros(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) {
		return "diff", nil
	})
	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); err != nil {
		t.Fatalf("CommitConfirmedBindingEnabled: %v", err)
	}

	for _, test := range []struct {
		name      string
		userID    string
		sessionID string
	}{
		{name: "usuario", userID: storeTestUUID7(t), sessionID: fixture.epoch.SessionID},
		{name: "sessao", userID: fixture.epoch.UserID, sessionID: storeTestUUID7(t)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fixture.projection.store.GetBindingMutation(context.Background(), test.userID, test.sessionID, confirmed.request.MutationID)
			if !errors.Is(err, ErrMutationNotFound) {
				t.Fatalf("lookup estrangeiro = %v, esperado ErrMutationNotFound", err)
			}
		})
	}
}

func TestCommitConfirmedBindingEnabledReplayNaoDuplicaAuditoria(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) {
		return "diff", nil
	})
	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); err != nil {
		t.Fatalf("primeiro commit: %v", err)
	}
	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); !errors.Is(err, commanddecision.ErrStale) {
		t.Fatalf("replay = %v, esperado commanddecision.ErrStale", err)
	}
	if got := countRows(t, fixture.projection.db, "command_config_mutations"); got != 1 {
		t.Fatalf("replay criou auditorias adicionais: %d", got)
	}
	if got := countDecisionEvents(t, fixture.projection.db, confirmed.request.DecisionID, commanddecision.Consumed); got != 1 {
		t.Fatalf("replay criou eventos consumed adicionais: %d", got)
	}
}

func TestCommitConfirmedBindingEnabledErroInsercaoAuditoriaFazRollbackTotal(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) {
		return "diff", nil
	})
	if err := fixture.projection.db.Exec("CREATE TRIGGER mutation_audit_test_reject_insert BEFORE INSERT ON command_config_mutations BEGIN SELECT RAISE(ABORT, 'fixture'); END").Error; err != nil {
		t.Fatalf("criar trigger: %v", err)
	}

	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); err == nil {
		t.Fatal("trigger não provocou erro")
	}
	assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation)
	receipt := loadDecisionReceiptProbe(t, fixture.projection.db, confirmed.request.DecisionID)
	if receipt.Status != commanddecision.Accepted || receipt.ConsumedAt.Valid {
		t.Fatalf("rollback não preservou receipt accepted: %+v", receipt)
	}
	if countDecisionEvents(t, fixture.projection.db, confirmed.request.DecisionID, commanddecision.Consumed) != 0 {
		t.Fatal("evento consumed sobreviveu ao rollback")
	}
	if got := countRows(t, fixture.projection.db, "command_config_mutations"); got != 0 {
		t.Fatalf("auditoria sobreviveu ao rollback: %d", got)
	}
}

func TestConfirmBindingEnabledCanceladaNaoCriaAuditoria(t *testing.T) {
	fixture := decisionTestFixtureFor(t, "", true)
	change, err := fixture.projection.store.PrepareBindingEnabled(
		context.Background(), fixture.projection.scope, fixture.binding.ID, false, fixture.projection.options,
	)
	if err != nil {
		t.Fatalf("PrepareBindingEnabled: %v", err)
	}
	confirmed, err := fixture.projection.store.ConfirmBindingEnabled(
		context.Background(), change, fixture.epoch, fixture.receipts, "v1",
		func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x42}, 32), nil },
		decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil },
	)
	if confirmed != nil || !errors.Is(err, commanddecision.ErrStale) {
		t.Fatalf("confirmação cancelada = (%#v, %v), esperado (nil, ErrStale)", confirmed, err)
	}
	receipt := loadDecisionReceiptProbe(t, fixture.projection.db, fixture.presenter.last.DecisionID)
	if receipt.Status != commanddecision.Cancelled || receipt.ConsumedAt.Valid {
		t.Fatalf("receipt cancelada inesperada: %+v", receipt)
	}
	assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation)
	if got := countRows(t, fixture.projection.db, "command_config_mutations"); got != 0 {
		t.Fatalf("cancelamento criou auditoria: %d", got)
	}
}

func TestCommitConfirmedBindingEnabledExpiradaNaoCriaAuditoria(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	expiresAt := decisionExpiresAt(fixture)
	confirmed := confirmDecisionTest(t, fixture, expiresAt, func(Binding, Binding) (string, error) {
		return "diff", nil
	})
	*fixture.clock = expiresAt.Add(time.Millisecond)

	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); !errors.Is(err, commanddecision.ErrStale) {
		t.Fatalf("commit expirado = %v, esperado commanddecision.ErrStale", err)
	}
	assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation)
	receipt := loadDecisionReceiptProbe(t, fixture.projection.db, confirmed.request.DecisionID)
	if receipt.Status != commanddecision.Accepted || receipt.ConsumedAt.Valid {
		t.Fatalf("receipt expirada foi consumida: %+v", receipt)
	}
	if countDecisionEvents(t, fixture.projection.db, confirmed.request.DecisionID, commanddecision.Consumed) != 0 {
		t.Fatal("expiração registrou evento consumed")
	}
	if got := countRows(t, fixture.projection.db, "command_config_mutations"); got != 0 {
		t.Fatalf("expiração criou auditoria: %d", got)
	}
}
