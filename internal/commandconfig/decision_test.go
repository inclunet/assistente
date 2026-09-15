package commandconfig

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

type decisionTestPresenter struct {
	action    string
	cancelled bool
	calls     int
	last      commanddecision.Request
}

func (p *decisionTestPresenter) Present(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
	p.calls++
	p.last = request
	return commanddecision.Response{
		DecisionID: request.DecisionID,
		ActionID:   p.action,
		Cancelled:  p.cancelled,
	}, nil
}

type decisionTestFixture struct {
	projection projectionTestFixture
	binding    Binding
	presenter  *decisionTestPresenter
	clock      *time.Time
	receipts   *commanddecision.Store
	epoch      commandsecurity.EpochSnapshot
}

type decisionReceiptProbe struct {
	DecisionID     string         `gorm:"column:decision_id"`
	Status         string         `gorm:"column:status"`
	AcceptedAction sql.NullString `gorm:"column:accepted_action_id"`
	ConsumedAt     sql.NullInt64  `gorm:"column:consumed_at"`
}

func decisionTestFixtureFor(t *testing.T, action string, cancelled bool) decisionTestFixture {
	t.Helper()
	projection, binding := writeTestBaseline(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	clock := now
	presenter := &decisionTestPresenter{action: action, cancelled: cancelled}
	if err := commanddecision.Migrate(context.Background(), projection.db); err != nil {
		t.Fatalf("commanddecision.Migrate: %v", err)
	}
	receipts, err := commanddecision.New(projection.db, presenter, func() time.Time { return clock })
	if err != nil {
		t.Fatalf("commanddecision.New: %v", err)
	}
	return decisionTestFixture{
		projection: projection,
		binding:    binding,
		presenter:  presenter,
		clock:      &clock,
		receipts:   receipts,
		epoch: commandsecurity.EpochSnapshot{
			UserID:             projection.scope.UserID,
			SessionID:          storeTestUUID7(t),
			AuthGeneration:     "auth-generation-fixture",
			SecurityGeneration: "security-generation-fixture",
		},
	}
}

func decisionExpiresAt(fixture decisionTestFixture) time.Time {
	return (*fixture.clock).Add(time.Minute)
}

func confirmDecisionTest(t *testing.T, fixture decisionTestFixture, expiresAt time.Time, render func(Binding, Binding) (string, error)) *ConfirmedBindingEnabledChange {
	t.Helper()
	change, err := fixture.projection.store.PrepareBindingEnabled(
		context.Background(), fixture.projection.scope, fixture.binding.ID, false, fixture.projection.options,
	)
	if err != nil {
		t.Fatalf("PrepareBindingEnabled: %v", err)
	}
	confirmed, err := fixture.projection.store.ConfirmBindingEnabled(
		context.Background(), change, fixture.epoch, fixture.receipts, "v1",
		func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x42}, 32), nil },
		expiresAt, render,
	)
	if err != nil {
		t.Fatalf("ConfirmBindingEnabled: %v", err)
	}
	return confirmed
}

func loadDecisionReceiptProbe(t *testing.T, db interface{ Raw(string, ...any) *gorm.DB }, decisionID string) decisionReceiptProbe {
	t.Helper()
	var row decisionReceiptProbe
	result := db.Raw("SELECT decision_id, status, accepted_action_id, consumed_at FROM command_decision_receipts WHERE decision_id = ?", decisionID).Scan(&row)
	if result.Error != nil {
		t.Fatalf("consultar receipt %s: %v", decisionID, result.Error)
	}
	if result.RowsAffected != 1 {
		t.Fatalf("receipt %s não encontrada: rows=%d", decisionID, result.RowsAffected)
	}
	return row
}

func countDecisionEvents(t *testing.T, db interface{ Raw(string, ...any) *gorm.DB }, decisionID, state string) int64 {
	t.Helper()
	var count int64
	result := db.Raw("SELECT COUNT(*) FROM command_decision_receipt_events WHERE decision_id = ? AND state = ?", decisionID, state).Scan(&count)
	if result.Error != nil {
		t.Fatalf("consultar evento %s/%s: %v", decisionID, state, result.Error)
	}
	return count
}

func assertDecisionConfig(t *testing.T, fixture decisionTestFixture, expectedBinding Binding, generation int64) {
	t.Helper()
	if got := writeTestBindingRow(t, fixture.projection.db, fixture.binding.ID); !reflect.DeepEqual(got, expectedBinding) {
		t.Fatalf("binding inesperado: got=%#v want=%#v", got, expectedBinding)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.projection.db, fixture.projection.snapshot.Generations[0].ID), fixture.projection.snapshot.Generations[0].ID, generation)
}

func TestConfirmBindingEnabledNaoEscreveConfiguracao(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	beforeBinding := writeTestBindingRow(t, fixture.projection.db, fixture.binding.ID)
	beforeGeneration := writeTestGenerationRow(t, fixture.projection.db, fixture.projection.snapshot.Generations[0].ID)
	counts := map[string]int64{}
	for _, table := range []string{"command_layers", "command_bindings", "command_config_generations"} {
		counts[table] = countRows(t, fixture.projection.db, table)
	}

	confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(before, after Binding) (string, error) {
		if before.Enabled != beforeBinding.Enabled || after.Enabled {
			t.Fatalf("diff apresentado inesperado: before=%#v after=%#v", before, after)
		}
		return "diff de teste", nil
	})
	if confirmed == nil || confirmed.request.Body != "" {
		t.Fatalf("proposta confirmada reteve body ou é nil: %#v", confirmed)
	}
	if fixture.presenter.calls != 1 || fixture.presenter.last.Body != "diff de teste" {
		t.Fatalf("presenter não recebeu o diff: calls=%d request=%+v", fixture.presenter.calls, fixture.presenter.last)
	}
	if got := writeTestBindingRow(t, fixture.projection.db, fixture.binding.ID); !reflect.DeepEqual(got, beforeBinding) {
		t.Fatalf("confirm alterou binding: got=%#v want=%#v", got, beforeBinding)
	}
	if got := writeTestGenerationRow(t, fixture.projection.db, fixture.projection.snapshot.Generations[0].ID); !reflect.DeepEqual(got, beforeGeneration) {
		t.Fatalf("confirm alterou geração: got=%#v want=%#v", got, beforeGeneration)
	}
	for table, want := range counts {
		if got := countRows(t, fixture.projection.db, table); got != want {
			t.Fatalf("confirm alterou quantidade em %s: antes=%d depois=%d", table, want, got)
		}
	}
	receipt := loadDecisionReceiptProbe(t, fixture.projection.db, confirmed.request.DecisionID)
	if receipt.Status != commanddecision.Accepted || receipt.AcceptedAction.String != commanddecision.ApplyAction || receipt.ConsumedAt.Valid {
		t.Fatalf("receipt após confirmação inesperada: %+v", receipt)
	}
}

func TestCommitConfirmedBindingEnabledAtomicoConsomeReceipt(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil })

	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); err != nil {
		t.Fatalf("CommitConfirmedBindingEnabled: %v", err)
	}
	wantBinding := fixture.binding
	wantBinding.Enabled = false
	assertDecisionConfig(t, fixture, wantBinding, fixture.projection.snapshot.Generations[0].Generation+1)
	receipt := loadDecisionReceiptProbe(t, fixture.projection.db, confirmed.request.DecisionID)
	if receipt.Status != commanddecision.Consumed || !receipt.ConsumedAt.Valid {
		t.Fatalf("receipt consumida inesperada: %+v", receipt)
	}
	if countDecisionEvents(t, fixture.projection.db, confirmed.request.DecisionID, commanddecision.Consumed) != 1 {
		t.Fatal("evento consumed ausente")
	}
}

func TestCommitConfirmedBindingEnabledReplayNaoMudaNada(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil })
	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); err != nil {
		t.Fatal(err)
	}
	committedBinding := writeTestBindingRow(t, fixture.projection.db, fixture.binding.ID)
	committedGeneration := writeTestGenerationRow(t, fixture.projection.db, fixture.projection.snapshot.Generations[0].ID)

	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); !errors.Is(err, commanddecision.ErrStale) {
		t.Fatalf("replay = %v, esperado commanddecision.ErrStale", err)
	}
	if got := writeTestBindingRow(t, fixture.projection.db, fixture.binding.ID); !reflect.DeepEqual(got, committedBinding) {
		t.Fatalf("replay alterou binding: got=%#v want=%#v", got, committedBinding)
	}
	if got := writeTestGenerationRow(t, fixture.projection.db, fixture.projection.snapshot.Generations[0].ID); !reflect.DeepEqual(got, committedGeneration) {
		t.Fatalf("replay alterou geração: got=%#v want=%#v", got, committedGeneration)
	}
	if countDecisionEvents(t, fixture.projection.db, confirmed.request.DecisionID, commanddecision.Consumed) != 1 {
		t.Fatal("replay adicionou evento consumed")
	}
}

func TestConfirmBindingEnabledDenyECancelNaoCriamConfirmed(t *testing.T) {
	for _, test := range []struct {
		name      string
		action    string
		cancelled bool
		state     string
	}{
		{name: "deny", action: commanddecision.DenyAction, state: commanddecision.Denied},
		{name: "cancel", cancelled: true, state: commanddecision.Cancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := decisionTestFixtureFor(t, test.action, test.cancelled)
			confirmed, err := func() (*ConfirmedBindingEnabledChange, error) {
				change, prepareErr := fixture.projection.store.PrepareBindingEnabled(context.Background(), fixture.projection.scope, fixture.binding.ID, false, fixture.projection.options)
				if prepareErr != nil {
					return nil, prepareErr
				}
				return fixture.projection.store.ConfirmBindingEnabled(context.Background(), change, fixture.epoch, fixture.receipts, "v1", func(context.Context, string) ([]byte, error) {
					return bytes.Repeat([]byte{0x42}, 32), nil
				}, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil })
			}()
			if confirmed != nil || !errors.Is(err, commanddecision.ErrStale) {
				t.Fatalf("Confirm %s = (%#v, %v), esperado (nil, ErrStale)", test.name, confirmed, err)
			}
			assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation)
			if fixture.presenter.calls != 1 {
				t.Fatalf("presenter calls = %d, want 1", fixture.presenter.calls)
			}
			receipt := loadDecisionReceiptProbe(t, fixture.projection.db, fixture.presenter.last.DecisionID)
			if receipt.Status != test.state || receipt.ConsumedAt.Valid {
				t.Fatalf("receipt terminal inesperada: %+v", receipt)
			}
			if countDecisionEvents(t, fixture.projection.db, fixture.presenter.last.DecisionID, commanddecision.Consumed) != 0 {
				t.Fatal("deny/cancel registrou consumed")
			}
		})
	}
}

func TestCommitConfirmedBindingEnabledErroSQLDepoisDeConsumirFazRollbackTotal(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil })
	if err := fixture.projection.db.Exec("CREATE TRIGGER decision_test_reject_binding_update BEFORE UPDATE ON command_bindings BEGIN SELECT RAISE(ABORT, 'fixture'); END").Error; err != nil {
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
}

func TestCommitConfirmedBindingEnabledRejeitaGeracaoStaleEBindingForaDoProtocolo(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, decisionTestFixture)
	}{
		{name: "geração stale", mutate: func(t *testing.T, fixture decisionTestFixture) {
			if err := fixture.projection.db.Exec("UPDATE command_config_generations SET generation = generation + 1 WHERE id = ?", fixture.projection.snapshot.Generations[0].ID).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "binding out-of-band", mutate: func(t *testing.T, fixture decisionTestFixture) {
			if err := fixture.projection.db.Exec("UPDATE command_bindings SET resolution_priority = resolution_priority + 1 WHERE id = ?", fixture.binding.ID).Error; err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
			confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil })
			test.mutate(t, fixture)
			if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); !errors.Is(err, ErrStale) {
				t.Fatalf("commit = %v, esperado ErrStale", err)
			}
			receipt := loadDecisionReceiptProbe(t, fixture.projection.db, confirmed.request.DecisionID)
			if receipt.Status != commanddecision.Accepted || receipt.ConsumedAt.Valid {
				t.Fatalf("receipt stale foi consumida: %+v", receipt)
			}
			if countDecisionEvents(t, fixture.projection.db, confirmed.request.DecisionID, commanddecision.Consumed) != 0 {
				t.Fatal("stale registrou evento consumed")
			}
			if test.name == "geração stale" {
				assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation+1)
			} else {
				want := fixture.binding
				want.ResolutionPriority++
				assertDecisionConfig(t, fixture, want, fixture.projection.snapshot.Generations[0].Generation)
			}
		})
	}
}

func TestCommitConfirmedBindingEnabledRejeitaCadaEpochDivergente(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*commandsecurity.EpochSnapshot, *decisionTestFixture, *testing.T)
	}{
		{name: "user", mutate: func(epoch *commandsecurity.EpochSnapshot, fixture *decisionTestFixture, t *testing.T) {
			epoch.UserID = storeTestUUID7(t)
			_ = fixture
		}},
		{name: "session", mutate: func(epoch *commandsecurity.EpochSnapshot, fixture *decisionTestFixture, t *testing.T) {
			epoch.SessionID = storeTestUUID7(t)
			_ = fixture
		}},
		{name: "auth generation", mutate: func(epoch *commandsecurity.EpochSnapshot, fixture *decisionTestFixture, t *testing.T) {
			epoch.AuthGeneration = "auth-generation-other"
			_ = fixture
			_ = t
		}},
		{name: "security generation", mutate: func(epoch *commandsecurity.EpochSnapshot, fixture *decisionTestFixture, t *testing.T) {
			epoch.SecurityGeneration = "security-generation-other"
			_ = fixture
			_ = t
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
			confirmed := confirmDecisionTest(t, fixture, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil })
			epoch := fixture.epoch
			test.mutate(&epoch, &fixture, t)
			if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, epoch); !errors.Is(err, ErrInvalid) {
				t.Fatalf("epoch mismatch = %v, esperado ErrInvalid", err)
			}
			assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation)
			receipt := loadDecisionReceiptProbe(t, fixture.projection.db, confirmed.request.DecisionID)
			if receipt.Status != commanddecision.Accepted || receipt.ConsumedAt.Valid {
				t.Fatalf("receipt após epoch mismatch: %+v", receipt)
			}
		})
	}
}

func TestCommitConfirmedBindingEnabledRejeitaReceiptDeOutroBanco(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	otherDB := storeTestDB(t)
	otherPresenter := &decisionTestPresenter{action: commanddecision.ApplyAction}
	otherClock := *fixture.clock
	if err := commanddecision.Migrate(context.Background(), otherDB); err != nil {
		t.Fatalf("Migrate outro banco: %v", err)
	}
	otherReceipts, err := commanddecision.New(otherDB, otherPresenter, func() time.Time { return otherClock })
	if err != nil {
		t.Fatalf("New outro banco: %v", err)
	}

	change, err := fixture.projection.store.PrepareBindingEnabled(context.Background(), fixture.projection.scope, fixture.binding.ID, false, fixture.projection.options)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := fixture.projection.store.ConfirmBindingEnabled(context.Background(), change, fixture.epoch, otherReceipts, "v1", func(context.Context, string) ([]byte, error) {
		return bytes.Repeat([]byte{0x42}, 32), nil
	}, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "diff", nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.projection.store.CommitConfirmedBindingEnabled(context.Background(), confirmed, fixture.epoch); !errors.Is(err, commanddecision.ErrInvalid) {
		t.Fatalf("receipt estrangeira = %v, esperado commanddecision.ErrInvalid", err)
	}
	assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation)
	receipt := loadDecisionReceiptProbe(t, otherDB, confirmed.request.DecisionID)
	if receipt.Status != commanddecision.Accepted || receipt.ConsumedAt.Valid {
		t.Fatalf("receipt de outro banco foi consumida: %+v", receipt)
	}
}

func TestCommitConfirmedBindingEnabledPrazoExpiraComClockFakeSemSleep(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	expiresAt := decisionExpiresAt(fixture)
	confirmed := confirmDecisionTest(t, fixture, expiresAt, func(Binding, Binding) (string, error) { return "diff", nil })
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
		t.Fatal("prazo expirado registrou evento consumed")
	}
}

func TestConfirmBindingEnabledAssinaDiffERenderNaoMutaProposta(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	change, err := fixture.projection.store.PrepareBindingEnabled(context.Background(), fixture.projection.scope, fixture.binding.ID, false, fixture.projection.options)
	if err != nil {
		t.Fatal(err)
	}
	wantBefore, wantAfter := change.Diff()
	first, err := fixture.projection.store.ConfirmBindingEnabled(context.Background(), change, fixture.epoch, fixture.receipts, "v1", func(_ context.Context, _ string) ([]byte, error) {
		return bytes.Repeat([]byte{0x42}, 32), nil
	}, decisionExpiresAt(fixture), func(before, after Binding) (string, error) {
		if before.CommandID == nil || after.CommandID == nil {
			t.Fatal("fixture precisa de ponteiros de CommandID")
		}
		*before.CommandID = "render-mutated-before"
		*after.CommandID = "render-mutated-after"
		return "rendered", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	gotBefore, gotAfter := change.Diff()
	if !reflect.DeepEqual(gotBefore, wantBefore) || !reflect.DeepEqual(gotAfter, wantAfter) {
		t.Fatalf("render alterou a proposta: before=%#v/%#v after=%#v/%#v", gotBefore, wantBefore, gotAfter, wantAfter)
	}
	if first.request.Fingerprint == "" {
		t.Fatal("Confirm não produziu assinatura do diff")
	}

	variant := *change
	variant.after = cloneBinding(change.after)
	variant.after.Arguments = `{"variant":true}`
	second, err := fixture.projection.store.ConfirmBindingEnabled(context.Background(), &variant, fixture.epoch, fixture.receipts, "v1", func(context.Context, string) ([]byte, error) {
		return bytes.Repeat([]byte{0x42}, 32), nil
	}, decisionExpiresAt(fixture), func(Binding, Binding) (string, error) { return "rendered", nil })
	if err != nil {
		t.Fatal(err)
	}
	if second.request.Fingerprint == first.request.Fingerprint {
		t.Fatal("assinatura não vinculou a mudança do diff")
	}
	assertDecisionConfig(t, fixture, fixture.binding, fixture.projection.snapshot.Generations[0].Generation)
}
