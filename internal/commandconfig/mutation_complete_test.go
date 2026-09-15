package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

func completeTestValidator(ctx context.Context, s Snapshot) error    { return validateSnapshot(s) }
func completeNoopHook(context.Context, *gorm.DB, MutationDiff) error { return nil }

func confirmCompleteTest(t *testing.T, f decisionTestFixture, p *PreparedMutation) *ConfirmedMutation {
	t.Helper()
	c, e := f.projection.store.ConfirmMutation(context.Background(), p, f.epoch, f.receipts, "v1", func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{42}, 32), nil }, time.Now().Add(time.Minute), func(d MutationDiff) (string, error) { d.AfterBindings = nil; return "diff exato", nil })
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestCompleteMutationCRUDReceiptAuditAndReplay(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	s := f.projection.store
	ctx := context.Background()
	p, err := s.PrepareMutation(ctx, f.projection.scope, MutationIntent{Operation: BindingDisable, ID: f.binding.ID}, completeTestValidator)
	if err != nil {
		t.Fatal(err)
	}
	d := p.Diff()
	d.AfterBindings[0].UserID = "forged"
	c := confirmCompleteTest(t, f, p)
	if err := s.CommitConfirmedMutation(ctx, c, f.epoch, completeNoopHook); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitConfirmedMutation(ctx, c, f.epoch, completeNoopHook); err == nil {
		t.Fatal("receipt reutilizada")
	}
	if got := writeTestBindingRow(t, f.projection.db, f.binding.ID); got.Enabled || got.UserID != f.epoch.UserID {
		t.Fatal("mutação fora do diff")
	}
	var row struct {
		SchemaVersion                 int
		BeforeDocument, AfterDocument string
	}
	if e := f.projection.db.Table("command_config_mutations").Where("mutation_id = ?", p.diff.MutationID).Take(&row).Error; e != nil {
		t.Fatal(e)
	}
	if row.SchemaVersion != 2 || row.BeforeDocument == row.AfterDocument {
		t.Fatal("auditoria não preservou diff")
	}
	if got := loadDecisionReceiptProbe(t, f.projection.db, c.request.DecisionID); got.Status != commanddecision.Consumed {
		t.Fatal("receipt não consumida")
	}
}

func TestCompleteMutationHookFailureRollsBackAll(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	s := f.projection.store
	ctx := context.Background()
	p, e := s.PrepareMutation(ctx, f.projection.scope, MutationIntent{Operation: BindingDisable, ID: f.binding.ID}, completeTestValidator)
	if e != nil {
		t.Fatal(e)
	}
	c := confirmCompleteTest(t, f, p)
	if e := s.CommitConfirmedMutation(ctx, c, f.epoch, func(context.Context, *gorm.DB, MutationDiff) error { return errors.New("grant rollback") }); e == nil {
		t.Fatal("falha ignorada")
	}
	assertDecisionConfig(t, f, f.binding, 1)
	if row := loadDecisionReceiptProbe(t, f.projection.db, c.request.DecisionID); row.Status != commanddecision.Accepted {
		t.Fatal("receipt consumida apesar rollback")
	}
	if countRows(t, f.projection.db, "command_config_mutations") != 0 {
		t.Fatal("auditoria de sucesso após rollback")
	}
}

func TestCompleteMutationCASAndForeignScope(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	ctx := context.Background()
	s := f.projection.store
	p, e := s.PrepareMutation(ctx, f.projection.scope, MutationIntent{Operation: BindingDisable, ID: f.binding.ID}, completeTestValidator)
	if e != nil {
		t.Fatal(e)
	}
	c := confirmCompleteTest(t, f, p)
	if e := f.projection.db.Model(&Generation{}).Where("user_id = ?", f.epoch.UserID).Update("generation", 2).Error; e != nil {
		t.Fatal(e)
	}
	if e := s.CommitConfirmedMutation(ctx, c, f.epoch, completeNoopHook); !errors.Is(e, ErrStale) {
		t.Fatalf("CAS: %v", e)
	}
	ws := "global" // não colide com a chave interna de scope global
	local := Scope{UserID: f.epoch.UserID, WorkspaceID: &ws}
	if e := s.EnsureScope(ctx, local); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Load(ctx, local); e != nil {
		t.Fatal(e)
	}
	if _, e := s.PrepareMutation(ctx, local, MutationIntent{Operation: BindingDelete, ID: f.binding.ID}, completeTestValidator); e == nil {
		t.Fatal("workspace apagou binding global")
	}
}

func TestCompleteMutationCreatesBackendIDsAndRestoresExactScope(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	ctx := context.Background()
	s := f.projection.store
	ws := "ws-real"
	local := Scope{UserID: f.epoch.UserID, WorkspaceID: &ws}
	if e := s.EnsureScope(ctx, local); e != nil {
		t.Fatal(e)
	}
	p, e := s.PrepareMutation(ctx, local, MutationIntent{Operation: LayerCreate, Layer: &Layer{Name: "nova", Enabled: true}}, completeTestValidator)
	if e != nil {
		t.Fatal(e)
	}
	c := confirmCompleteTest(t, f, p)
	if e := s.CommitConfirmedMutation(ctx, c, f.epoch, completeNoopHook); e != nil {
		t.Fatal(e)
	}
	d := p.Diff()
	var created Layer
	for _, l := range d.AfterLayers {
		if l.Name == "nova" {
			created = l
		}
	}
	if !validID(created.ID) || created.WorkspaceID == nil || *created.WorkspaceID != ws {
		t.Fatal("ID/escopo não derivados")
	}
	p, e = s.PrepareMutation(ctx, local, MutationIntent{Operation: ConfigRestore}, completeTestValidator)
	if e != nil {
		t.Fatal(e)
	}
	c = confirmCompleteTest(t, f, p)
	if e := s.CommitConfirmedMutation(ctx, c, f.epoch, completeNoopHook); e != nil {
		t.Fatal(e)
	}
	if got := writeTestBindingRow(t, f.projection.db, f.binding.ID); got.ID != f.binding.ID {
		t.Fatal("restore local atingiu global")
	}
}

type mutationSessionFixture struct{ principal auth.LocalSessionPrincipal }

func (f mutationSessionFixture) AuthenticateLocalAccess(context.Context, string) (auth.LocalSessionPrincipal, error) {
	return f.principal, nil
}

type mutationPresenterFunc func(context.Context, commanddecision.Request) (commanddecision.Response, error)

func (f mutationPresenterFunc) Present(ctx context.Context, r commanddecision.Request) (commanddecision.Response, error) {
	return f(ctx, r)
}

func TestMutationServiceRevalidatesAfterDecisionWithoutHoldingGate(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	ctx := context.Background()
	gate := &commandsecurity.DispatchGate{}
	epochs, e := commandsecurity.NewEpochService(gate)
	if e != nil {
		t.Fatal(e)
	}
	version := "v1"
	receipts, e := commanddecision.New(f.projection.db, mutationPresenterFunc(func(ctx context.Context, r commanddecision.Request) (commanddecision.Response, error) {
		// Tomar o gate aqui prova que a espera de decisão não o reteve.
		if e := gate.WithMutation(ctx, func() error { version = "v2"; return nil }); e != nil {
			return commanddecision.Response{}, e
		}
		return commanddecision.Response{DecisionID: r.DecisionID, ActionID: commanddecision.ApplyAction}, nil
	}), time.Now)
	if e != nil {
		t.Fatal(e)
	}
	s, e := NewMutationService(MutationServiceConfig{Store: f.projection.store, Sessions: mutationSessionFixture{auth.LocalSessionPrincipal{UserID: f.epoch.UserID, SessionID: f.epoch.SessionID}}, Epochs: epochs, Receipts: receipts, Keys: func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{1}, 32), nil }, KeyVersion: "v1", DecisionTTL: time.Minute, Authorize: func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil }, Validate: completeTestValidator, Version: func(context.Context) (string, error) { return version, nil }, Render: func(MutationDiff) (string, error) { return "diff", nil }, OnMutationTx: completeNoopHook})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := s.Apply(ctx, "token", nil, MutationIntent{Operation: BindingDisable, ID: f.binding.ID}); !errors.Is(e, ErrStale) {
		t.Fatalf("catálogo mudou mas commit não recusou: %v", e)
	}
	assertDecisionConfig(t, f, f.binding, 1)
}
