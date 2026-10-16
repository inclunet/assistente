package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddecision"
)

func TestRulePipelineEventCreateIsDisabledAndDirectEnableNeedsGrant(t *testing.T) {
	f := newActivationHookFixture(t)
	eventName, producers := "command-context.job-run-state.v1", `["jobs.runtime"]`
	prepared, err := f.config.PrepareMutation(context.Background(), f.base.projection.scope, MutationIntent{
		Operation: RuleCreate,
		Rule: &commandactivation.Rule{
			LayerRefKind: commandactivation.UserRef, LayerRef: f.layer.ID,
			RuleRefKind: commandactivation.UserRef, Mode: commandactivation.ModeEvent,
			Condition: "{}", Lifecycle: commandactivation.LifecyclePersistent,
			EventName: &eventName, AllowedInternalProducerTypes: &producers,
			ReviewStatus: "active",
		},
	}, completeTestValidator)
	if err != nil {
		t.Fatalf("RuleCreate: %v", err)
	}
	var created commandactivation.Rule
	for _, rule := range prepared.Diff().AfterActivationRules {
		if rule.LayerRef == f.layer.ID {
			created = rule
		}
	}
	if created.ID == "" || created.Enabled || created.AutomationGrantID != nil {
		t.Fatalf("regra criada não está fechada: %+v", created)
	}
	confirmed, err := f.config.ConfirmMutation(context.Background(), prepared, f.base.epoch, f.base.receipts, "v1", func(context.Context, string) ([]byte, error) {
		return bytes.Repeat([]byte{0x42}, 32), nil
	}, time.Now().Add(time.Minute), func(MutationDiff) (string, error) { return "rule create", nil })
	if err != nil {
		t.Fatalf("ConfirmMutation: %v", err)
	}
	if err := f.config.CommitConfirmedMutation(context.Background(), confirmed, f.base.epoch, f.hook(t)); err != nil {
		t.Fatalf("CommitConfirmedMutation: %v", err)
	}
	loaded, err := f.act.Store().ResolveRule(context.Background(), f.owner, commandactivation.Ref{Kind: commandactivation.UserRef, ID: created.ID})
	if err != nil || loaded.Enabled {
		t.Fatalf("regra persistida = %+v, erro=%v", loaded, err)
	}
	if _, err := f.config.PrepareMutation(context.Background(), f.base.projection.scope, MutationIntent{Operation: RuleEnable, ID: created.ID}, completeTestValidator); !errors.Is(err, commandactivation.ErrGrantUnavailable) {
		t.Fatalf("RuleEnable sem grant = %v", err)
	}
}

func TestConfigRestoreAggregadoPersisteReceiptGrantRevogadoEClaimTerminal(t *testing.T) {
	f := newActivationHookFixture(t)
	ruleID := storeTestUUID7(t)
	grantID := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID}, f.layer.ID, ruleID)
	claimID := insertActivationClaim(t, f.db, f.owner, f.layer.ID, ruleID)
	var decisionID string
	if err := f.db.Table("command_layer_automation_grants").Where("id = ?", grantID).Pluck("authorization_decision_id", &decisionID).Error; err != nil {
		t.Fatal(err)
	}
	// A regra ativa representa o estado produzido pelo protocolo de grant;
	// somente o caminho commandautomation cria esse estado em runtime. O
	// fixture insere a linha para testar o restore composto, não para abrir um
	// writer alternativo na produção.
	eventName, producers := commandautomation.JobRunStateEvent, `["jobs.runtime"]`
	generation := int64(1)
	grantFingerprint := "grant-fingerprint"
	rule := commandactivation.Rule{ID: ruleID, UserID: f.owner.UserID, LayerRefKind: commandactivation.UserRef, LayerRef: f.layer.ID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent, Condition: "{}", Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers, AuthorizationDecisionID: &decisionID, AutomationGrantID: &grantID, AutomationGrantGeneration: &generation, AutomationGrantFingerprint: &grantFingerprint, Enabled: true, Source: "user", ReviewStatus: "active"}
	if err := f.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	prepared, err := f.config.PrepareMutation(context.Background(), f.base.projection.scope, MutationIntent{Operation: ConfigRestore}, completeTestValidator)
	if err != nil {
		t.Fatalf("Prepare ConfigRestore: %v", err)
	}
	confirmed, err := f.config.ConfirmMutation(context.Background(), prepared, f.base.epoch, f.base.receipts, "v1", func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x42}, 32), nil }, time.Now().Add(time.Minute), func(MutationDiff) (string, error) { return "restore", nil })
	if err != nil {
		t.Fatalf("Confirm ConfigRestore: %v", err)
	}
	if err := f.config.CommitConfirmedMutation(context.Background(), confirmed, f.base.epoch, f.hook(t)); err != nil {
		t.Fatalf("Commit ConfigRestore: %v", err)
	}
	if got := loadDecisionReceiptProbe(t, f.db, confirmed.request.DecisionID); got.Status != commanddecision.Consumed {
		t.Fatalf("receipt = %q", got.Status)
	}
	var count int64
	if err := f.db.Model(&commandactivation.Rule{}).Where("id = ?", ruleID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("regra após restore count=%d err=%v", count, err)
	}
	if !grantRevokedAt(t, f.db, grantID).Valid {
		t.Fatal("grant não revogado")
	}
	claim, err := f.act.Store().GetClaim(context.Background(), f.owner, claimID)
	if err != nil || claim.State != commandactivation.StateInactive || claim.TerminalReason == nil || *claim.TerminalReason != "layer_deleted" {
		t.Fatalf("claim após restore = %+v, erro=%v", claim, err)
	}
	if !claim.UpdatedAt.Equal(prepared.Diff().RevocationAt) {
		t.Fatalf("timestamp da claim após restore divergiu do diff: stored=%s prepared=%s", claim.UpdatedAt, prepared.Diff().RevocationAt)
	}
	var audit struct{ BeforeDocument, AfterDocument string }
	if err := f.db.Table("command_config_mutations").Where("mutation_id = ?", prepared.Diff().MutationID).Take(&audit).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ActivationRules", "AutomationGrants", "ActivationClaims"} {
		if !strings.Contains(audit.BeforeDocument, name) || !strings.Contains(audit.AfterDocument, name) {
			t.Fatalf("documento agregado sem %s: before=%s after=%s", name, audit.BeforeDocument, audit.AfterDocument)
		}
	}
}

func TestRuleUpdateComGrantAntigoRevogaNoCommit(t *testing.T) {
	f := newActivationHookFixture(t)
	ruleID := storeTestUUID7(t)
	grantID := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID}, f.layer.ID, ruleID)
	claimID := insertActivationClaim(t, f.db, f.owner, f.layer.ID, ruleID)
	var decisionID string
	if err := f.db.Table("command_layer_automation_grants").Where("id = ?", grantID).Pluck("authorization_decision_id", &decisionID).Error; err != nil {
		t.Fatal(err)
	}
	eventName, producers := commandautomation.JobRunStateEvent, `["jobs.runtime"]`
	generation := int64(1)
	grantFingerprint := "grant-fingerprint"
	rule := commandactivation.Rule{ID: ruleID, UserID: f.owner.UserID, LayerRefKind: commandactivation.UserRef, LayerRef: f.layer.ID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent, Condition: "{}", Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers, AuthorizationDecisionID: &decisionID, AutomationGrantID: &grantID, AutomationGrantGeneration: &generation, AutomationGrantFingerprint: &grantFingerprint, Enabled: true, Source: "user", ReviewStatus: "active"}
	if err := f.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	candidate := rule
	candidate.Condition = `{"changed":true}`
	prepared, err := f.config.PrepareMutation(context.Background(), f.base.projection.scope, MutationIntent{Operation: RuleUpdate, ID: ruleID, Rule: &candidate}, completeTestValidator)
	if err != nil {
		t.Fatalf("Prepare RuleUpdate granted: %v", err)
	}
	updated := prepared.Diff().AfterActivationRules[0]
	if updated.Enabled || updated.AutomationGrantID != nil || updated.AuthorizationDecisionID != nil {
		t.Fatalf("update não invalidou grant: %+v", updated)
	}
	confirmed, err := f.config.ConfirmMutation(context.Background(), prepared, f.base.epoch, f.base.receipts, "v1", func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x42}, 32), nil }, time.Now().Add(time.Minute), func(MutationDiff) (string, error) { return "rule update", nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := f.config.CommitConfirmedMutation(context.Background(), confirmed, f.base.epoch, f.hook(t)); err != nil {
		t.Fatalf("Commit RuleUpdate granted: %v", err)
	}
	if !grantRevokedAt(t, f.db, grantID).Valid {
		t.Fatal("grant não revogado após edição")
	}
	preparedRevocationAt := prepared.Diff().RevocationAt
	var storedRevocationAt time.Time
	if err := f.db.Raw("SELECT revoked_at FROM command_layer_automation_grants WHERE id = ?", grantID).Scan(&storedRevocationAt).Error; err != nil {
		t.Fatal(err)
	}
	if !storedRevocationAt.Equal(preparedRevocationAt) {
		t.Fatalf("timestamp do grant divergiu do diff: stored=%s prepared=%s", storedRevocationAt, preparedRevocationAt)
	}
	loaded, err := f.act.Store().ResolveRule(context.Background(), f.owner, commandactivation.Ref{Kind: commandactivation.UserRef, ID: ruleID})
	if err != nil || loaded.Enabled || loaded.AutomationGrantID != nil {
		t.Fatalf("regra após edição = %+v, erro=%v", loaded, err)
	}
	claim, err := f.act.Store().GetClaim(context.Background(), f.owner, claimID)
	if err != nil || claim.State != commandactivation.StateStale {
		t.Fatalf("claim após edição = %+v, erro=%v", claim, err)
	}
	if !claim.UpdatedAt.Equal(preparedRevocationAt) {
		t.Fatalf("timestamp da claim divergiu do diff: stored=%s prepared=%s", claim.UpdatedAt, preparedRevocationAt)
	}
	activationGeneration, err := f.act.Store().SnapshotGeneration(context.Background(), f.owner)
	if err != nil || activationGeneration.Generation != 2 {
		t.Fatalf("geração após invalidação de regra = %+v, erro=%v", activationGeneration, err)
	}
}
