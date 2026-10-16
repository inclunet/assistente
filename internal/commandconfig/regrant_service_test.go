package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

type regrantBlockingPresenter struct {
	started chan struct{}
	once    sync.Once
}

func (p *regrantBlockingPresenter) Present(ctx context.Context, _ commanddecision.Request) (commanddecision.Response, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return commanddecision.Response{}, ctx.Err()
}

func newRegrantService(t *testing.T, f activationHookFixture, receipts *commanddecision.Store, beforeCommit func(context.Context, Scope) error, version func(context.Context) (string, error), keys commandautomation.FingerprintKeyProvider) (activationHookFixture, *CompleteMutationService, commandactivation.Rule) {
	t.Helper()
	if err := f.db.Where("id = ?", f.base.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	eventName, producers := commandautomation.JobRunStateEvent, `["jobs.runtime"]`
	rule := commandactivation.Rule{ID: storeTestUUID7(t), UserID: f.base.projection.scope.UserID, LayerRefKind: commandactivation.UserRef,
		LayerRef: f.layer.ID, RuleRefKind: commandactivation.UserRef, Mode: commandactivation.ModeEvent, Condition: `{}`,
		Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers,
		Enabled: false, Source: "user", ReviewStatus: "active"}
	rule.RuleRef = rule.ID
	if err := f.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	principal := auth.LocalSessionPrincipal{UserID: f.base.epoch.UserID, SessionID: f.base.epoch.SessionID}
	service, err := NewCompleteMutationService(MutationServiceConfig{
		Store: f.config, Automation: f.grants, Sessions: mutationSessionFixture{principal}, Epochs: epochs,
		Receipts: receipts, Keys: keys, KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize: func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil },
		Version:   version, Render: func(MutationDiff) (string, error) { return "regrant event", nil },
		OnMutationTx: f.hook(t), BeforeCommit: beforeCommit,
	}, func(context.Context, Scope) (CompleteProjection, error) {
		return options, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return f, service, rule
}

func regrantKeys(_ context.Context, _ string) ([]byte, error) {
	return bytes.Repeat([]byte{0x31}, 32), nil
}

func TestRegrantEventRuleUsaFingerprintReceiptEHookNoMesmoCommit(t *testing.T) {
	f := newActivationHookFixture(t)
	// Reuse the isolated database/records from the fixture but construct a
	// fresh service below so the presenter/epoch remain explicit.
	if err := f.db.Where("id = ?", f.base.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	eventName, producers := commandautomation.JobRunStateEvent, `["jobs.runtime"]`
	rule := commandactivation.Rule{ID: storeTestUUID7(t), UserID: f.base.projection.scope.UserID, LayerRefKind: commandactivation.UserRef,
		LayerRef: f.layer.ID, RuleRefKind: commandactivation.UserRef, Mode: commandactivation.ModeEvent, Condition: `{}`,
		Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers,
		Enabled: false, Source: "user", ReviewStatus: "active"}
	rule.RuleRef = rule.ID
	if err := f.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	principal := auth.LocalSessionPrincipal{UserID: f.base.epoch.UserID, SessionID: f.base.epoch.SessionID}
	keyCalls := []string{}
	receipts := f.base.receipts
	service, err := NewCompleteMutationService(MutationServiceConfig{
		Store: f.config, Automation: f.grants, Sessions: mutationSessionFixture{principal}, Epochs: epochs,
		Receipts: receipts, Keys: func(ctx context.Context, name string) ([]byte, error) {
			keyCalls = append(keyCalls, name)
			return regrantKeys(ctx, name)
		}, KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize: func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil },
		Version:   func(context.Context) (string, error) { return "catalog-v1", nil }, Render: func(MutationDiff) (string, error) { return "regrant event", nil },
		OnMutationTx: f.hook(t),
	}, func(context.Context, Scope) (CompleteProjection, error) { return options, nil })
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.RegrantEventRule(context.Background(), "token", nil, rule.ID)
	if err != nil {
		t.Fatalf("RegrantEventRule: %v", err)
	}
	if !diff.RequiresDecision || diff.DecisionFingerprint == "" || diff.Operation != RuleEnable {
		t.Fatalf("diff não reteve vínculo de decisão: %+v", diff)
	}
	var stored commandactivation.Rule
	if err := f.db.Where("id = ?", rule.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Enabled || stored.AutomationGrantID == nil {
		t.Fatalf("regra não habilitada com grant: %+v", stored)
	}
	active, err := f.grants.LoadActive(context.Background(), commandautomation.Owner{UserID: rule.UserID}, commandautomation.NaturalKey{
		Owner: commandautomation.Owner{UserID: rule.UserID}, LayerRef: commandautomation.RuleRef{Kind: string(rule.LayerRefKind), Ref: rule.LayerRef}, RuleRef: commandautomation.RuleRef{Kind: string(rule.RuleRefKind), Ref: rule.RuleRef},
	})
	if err != nil || active.ID != *stored.AutomationGrantID {
		t.Fatalf("grant ativo divergente: grant=%+v err=%v rule=%+v", active, err, stored)
	}
	if active.AutomationGrantFingerprint != diff.DecisionFingerprint {
		t.Fatalf("decisão não usa fingerprint exato: grant=%q diff=%q", active.AutomationGrantFingerprint, diff.DecisionFingerprint)
	}
	if countRows(t, f.db, "command_config_mutations") != 1 {
		t.Fatal("auditoria comum ausente ou duplicada")
	}
	if loadDecisionReceiptProbe(t, f.db, active.AuthorizationDecisionID).Status != commanddecision.Consumed {
		t.Fatal("receipt do regrant não consumida")
	}
	for _, name := range keyCalls {
		if name != "command-request-hmac:v1" {
			t.Fatalf("chave fora de KeyVersion: %q", name)
		}
	}
}

func TestRegrantEventRuleCancelamentoDaDecisaoNaoEscreve(t *testing.T) {
	f := newActivationHookFixture(t)
	presenter := &regrantBlockingPresenter{started: make(chan struct{})}
	receipts, err := commanddecision.New(f.db, presenter, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	var service *CompleteMutationService
	var rule commandactivation.Rule
	f, service, rule = newRegrantService(t, f, receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	result := make(chan error, 1)
	go func() {
		_, err := service.RegrantEventRule(context.Background(), "token", nil, rule.ID)
		result <- err
	}()
	select {
	case <-presenter.started:
	case <-time.After(2 * time.Second):
		t.Fatal("presenter não recebeu decisão")
	}
	if err := service.service.config.Epochs.InvalidateSession(context.Background(), rule.UserID, f.base.epoch.SessionID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelamento permitiu regrant")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("regrant não terminou após invalidação")
	}
	var stored commandactivation.Rule
	if err := f.db.Where("id = ?", rule.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Enabled {
		t.Fatal("regra habilitada após decisão cancelada")
	}
	var grants int64
	if err := f.db.Table("command_layer_automation_grants").Where("user_id = ?", rule.UserID).Count(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("grant sobreviveu ao cancelamento: %d", grants)
	}
}

func TestRegrantEventRuleDivergenciaDeVersaoNaoEscreveEChaveUsaKeyVersion(t *testing.T) {
	f := newActivationHookFixture(t)
	versionCalls := 0
	keyCalls := []string{}
	_, service, rule := newRegrantService(t, f, f.base.receipts, nil, func(context.Context) (string, error) {
		versionCalls++
		if versionCalls == 1 {
			return "catalog-v1", nil
		}
		return "catalog-v2", nil
	}, func(ctx context.Context, name string) ([]byte, error) {
		keyCalls = append(keyCalls, name)
		return regrantKeys(ctx, name)
	})
	if _, err := service.RegrantEventRule(context.Background(), "token", nil, rule.ID); !errors.Is(err, ErrStale) {
		t.Fatalf("versão divergente aceita: %v", err)
	}
	if len(keyCalls) == 0 {
		t.Fatal("decisão não solicitou chave")
	}
	for _, name := range keyCalls {
		if name != "command-request-hmac:v1" {
			t.Fatalf("chave solicitada com versão do catálogo: %q", name)
		}
	}
	var enabled bool
	if err := f.db.Table("command_layer_activation_rules").Where("id = ?", rule.ID).Pluck("enabled", &enabled).Error; err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("regra habilitada após stale")
	}
}

func TestRegrantEventRuleBeforeCommitAbortaSemEfeito(t *testing.T) {
	f := newActivationHookFixture(t)
	want := errors.New("host ainda ativo")
	_, service, rule := newRegrantService(t, f, f.base.receipts, func(context.Context, Scope) error { return want }, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	if _, err := service.RegrantEventRule(context.Background(), "token", nil, rule.ID); !errors.Is(err, want) {
		t.Fatalf("BeforeCommit ignorado: %v", err)
	}
	var enabled bool
	if err := f.db.Table("command_layer_activation_rules").Where("id = ?", rule.ID).Pluck("enabled", &enabled).Error; err != nil {
		t.Fatal(err)
	}
	if enabled || countRows(t, f.db, "command_layer_automation_grants") != 0 || countRows(t, f.db, "command_config_mutations") != 0 {
		t.Fatal("efeito parcial após falha BeforeCommit")
	}
}

func TestRegrantEventRuleHookFailureRollbackConfigGrantReceipt(t *testing.T) {
	f := newActivationHookFixture(t)
	_, service, rule := newRegrantService(t, f, f.base.receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	want := errors.New("hook falhou")
	service.service.config.OnMutationTx = func(context.Context, *gorm.DB, MutationDiff) error { return want }
	if _, err := service.RegrantEventRule(context.Background(), "token", nil, rule.ID); !errors.Is(err, want) {
		t.Fatalf("hook não abortou regrant: %v", err)
	}
	var enabled bool
	if err := f.db.Table("command_layer_activation_rules").Where("id = ?", rule.ID).Pluck("enabled", &enabled).Error; err != nil {
		t.Fatal(err)
	}
	if enabled || countRows(t, f.db, "command_layer_automation_grants") != 0 || countRows(t, f.db, "command_config_mutations") != 0 {
		t.Fatal("rollback do regrant deixou efeito parcial")
	}
	var consumed int64
	if err := f.db.Table("command_decision_receipts").Where("status = ?", commanddecision.Consumed).Count(&consumed).Error; err != nil {
		t.Fatal(err)
	}
	if consumed != 0 {
		t.Fatal("rollback deixou receipt consumida")
	}
}

func TestMutationPreviewsDoNotWriteOrConferAuthority(t *testing.T) {
	f := newActivationHookFixture(t)
	_, service, rule := newRegrantService(t, f, f.base.receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	ctx := context.Background()
	scope := Scope{UserID: rule.UserID}
	before, err := f.config.Load(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(ctx, "token", nil, MutationIntent{Operation: LayerCreate, Layer: &Layer{Name: "somente-preview", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresDecision || preview.DecisionFingerprint != "" || len(preview.AfterLayers) != len(before.Layers)+1 {
		t.Fatalf("preview comum incorreto: %+v", preview)
	}
	regrant, err := service.PreviewRegrantEventRule(ctx, "token", nil, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !regrant.RequiresDecision || regrant.DecisionFingerprint != "" || len(regrant.AfterActivationRules) != 1 || !regrant.AfterActivationRules[0].Enabled {
		t.Fatalf("preview estrutural incorreto: %+v", regrant)
	}
	after, err := f.config.Load(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !sameAggregateSnapshot(before, after) || !reflect.DeepEqual(before.Generations, after.Generations) || countRows(t, f.db, "command_decision_receipts") != 0 || countRows(t, f.db, "command_config_mutations") != 0 {
		t.Fatal("preview escreveu ou alterou geração")
	}
}

func TestRegrantRejectsRuleChangedDuringDecision(t *testing.T) {
	f := newActivationHookFixture(t)
	var ruleID string
	receipts, err := commanddecision.New(f.db, mutationPresenterFunc(func(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
		// Simula alteração fora do serviço, sem poder contar apenas com epoch.
		if err := f.db.Model(&commandactivation.Rule{}).Where("id = ?", ruleID).Update("lifecycle", commandactivation.LifecycleSession).Error; err != nil {
			return commanddecision.Response{}, err
		}
		return commanddecision.Response{DecisionID: request.DecisionID, ActionID: commanddecision.ApplyAction}, nil
	}), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, service, rule := newRegrantService(t, f, receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	ruleID = rule.ID
	if _, err := service.RegrantEventRule(context.Background(), "token", nil, ruleID); !errors.Is(err, commandautomation.ErrStale) {
		t.Fatalf("semântica diferente da confirmada: %v", err)
	}
	var stored commandactivation.Rule
	if err := f.db.Where("id = ?", ruleID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Enabled || countRows(t, f.db, "command_layer_automation_grants") != 0 || countRows(t, f.db, "command_config_mutations") != 0 {
		t.Fatal("decisão antiga habilitou semântica nova")
	}
}

func TestRegrantPreviewRequiresExactScopeAndOperation(t *testing.T) {
	f := newActivationHookFixture(t)
	_, service, rule := newRegrantService(t, f, f.base.receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	ctx := context.Background()
	workspace := "preview-workspace"
	if err := f.config.EnsureScope(ctx, Scope{UserID: rule.UserID, WorkspaceID: &workspace}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewRegrantEventRule(ctx, "token", &workspace, rule.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("regra global apresentada como local: %v", err)
	}
	want := errors.New("habilitação negada")
	service.service.config.Authorize = func(_ context.Context, _ auth.LocalSessionPrincipal, _ Scope, operation Operation) error {
		if operation == RuleEnable {
			return want
		}
		return nil
	}
	if _, err := service.PreviewRegrantEventRule(ctx, "token", nil, rule.ID); !errors.Is(err, want) {
		t.Fatalf("preview não autorizou RuleEnable: %v", err)
	}
}
