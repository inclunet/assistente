package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commanddecision"
	"gorm.io/gorm"
)

func TestRegrantCompleteCyclePreservesRevokedHistory(t *testing.T) {
	f := newActivationHookFixture(t)
	_, service, rule := newRegrantService(t, f, f.base.receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	ctx := context.Background()
	first, err := service.RegrantEventRule(ctx, "token", nil, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.AfterAutomationGrants) != 1 {
		t.Fatalf("first grants: %+v", first.AfterAutomationGrants)
	}
	g1 := first.AfterAutomationGrants[0]
	if g1.AutomationGrantGeneration != 1 || g1.RevokedAt != nil {
		t.Fatalf("first grant: %+v", g1)
	}
	preview, err := service.Preview(ctx, "token", nil, MutationIntent{Operation: RuleDisable, ID: rule.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.AfterAutomationGrants) != 1 || preview.AfterAutomationGrants[0].RevokedAt == nil {
		t.Fatal("preview omitted revocation")
	}
	if grantRevokedAt(t, f.db, g1.ID).Valid {
		t.Fatal("preview revoked persisted grant")
	}
	if _, err := service.Apply(ctx, "token", nil, MutationIntent{Operation: RuleDisable, ID: rule.ID}); err != nil {
		t.Fatal(err)
	}
	if !grantRevokedAt(t, f.db, g1.ID).Valid {
		t.Fatal("disable did not revoke grant")
	}
	var disabled commandactivation.Rule
	if err := f.db.Where("id = ?", rule.ID).Take(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled || hasGrantMetadata(disabled) {
		t.Fatalf("disable retained authority: %+v", disabled)
	}
	if _, err := service.Apply(ctx, "token", nil, MutationIntent{Operation: RuleEnable, ID: rule.ID}); !errors.Is(err, commandactivation.ErrGrantUnavailable) {
		t.Fatalf("direct enable: %v", err)
	}
	second, err := service.RegrantEventRule(ctx, "token", nil, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.AfterAutomationGrants) != 2 {
		t.Fatalf("history lost: %+v", second.AfterAutomationGrants)
	}
	var active int
	for _, grant := range second.AfterAutomationGrants {
		if grant.ID == g1.ID {
			if grant.RevokedAt == nil || grant.AutomationGrantGeneration != 1 {
				t.Fatal("old grant resurrected")
			}
			continue
		}
		active++
		if grant.RevokedAt != nil || grant.AutomationGrantGeneration != 2 || grant.AuthorizationDecisionID == g1.AuthorizationDecisionID || grant.AutomationGrantFingerprint == g1.AutomationGrantFingerprint {
			t.Fatalf("second grant: %+v", grant)
		}
		if loadDecisionReceiptProbe(t, f.db, grant.AuthorizationDecisionID).Status != commanddecision.Consumed {
			t.Fatal("second receipt not consumed")
		}
	}
	if active != 1 || countRows(t, f.db, "command_config_mutations") != 3 || countRows(t, f.db, "command_decision_receipts") != 3 {
		t.Fatal("cycle did not commit exactly three decisions/mutations")
	}
	var enabled commandactivation.Rule
	if err := f.db.Where("id = ?", rule.ID).Take(&enabled).Error; err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled || enabled.AutomationGrantGeneration == nil || *enabled.AutomationGrantGeneration != 2 || enabled.AutomationGrantID == nil || *enabled.AutomationGrantID == g1.ID || enabled.AutomationGrantFingerprint == nil || *enabled.AutomationGrantFingerprint != second.DecisionFingerprint {
		t.Fatalf("rule does not reference generation 2: %+v", enabled)
	}
}

func TestRegrantDisableRollbackAfterRealHook(t *testing.T) {
	f := newActivationHookFixture(t)
	_, service, rule := newRegrantService(t, f, f.base.receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	ctx := context.Background()
	if _, err := service.RegrantEventRule(ctx, "token", nil, rule.ID); err != nil {
		t.Fatal(err)
	}
	before, err := f.config.Load(ctx, Scope{UserID: rule.UserID})
	if err != nil {
		t.Fatal(err)
	}
	realHook := service.service.config.OnMutationTx
	want := errors.New("failure after real hook")
	called := false
	service.service.config.OnMutationTx = func(ctx context.Context, tx *gorm.DB, diff MutationDiff) error {
		if err := realHook(ctx, tx, diff); err != nil {
			return err
		}
		called = true
		return want
	}
	if _, err := service.Apply(ctx, "token", nil, MutationIntent{Operation: RuleDisable, ID: rule.ID}); !errors.Is(err, want) {
		t.Fatalf("disable: %v", err)
	}
	after, err := f.config.Load(ctx, Scope{UserID: rule.UserID})
	if err != nil {
		t.Fatal(err)
	}
	if !called || !sameAggregateSnapshot(before, after) || !reflect.DeepEqual(before.Generations, after.Generations) || countRows(t, f.db, "command_config_mutations") != 1 {
		t.Fatal("real hook rollback left partial effects")
	}
	var consumed int64
	if err := f.db.Table("command_decision_receipts").Where("status = ?", commanddecision.Consumed).Count(&consumed).Error; err != nil {
		t.Fatal(err)
	}
	if consumed != 1 {
		t.Fatal("failed disable consumed receipt")
	}
}

func TestRegrantForeignWorkspaceAndCopyRestoreDoNotConferAuthority(t *testing.T) {
	f := newActivationHookFixture(t)
	_, service, rule := newRegrantService(t, f, f.base.receipts, nil, func(context.Context) (string, error) { return "catalog-v1", nil }, regrantKeys)
	ctx := context.Background()
	workspace := "other-workspace"
	if err := f.config.EnsureScope(ctx, Scope{UserID: rule.UserID, WorkspaceID: &workspace}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RegrantEventRule(ctx, "token", &workspace, rule.ID); err == nil {
		t.Fatal("foreign workspace granted global rule")
	}
	if countRows(t, f.db, "command_decision_receipts") != 0 {
		t.Fatal("foreign workspace opened decision")
	}
	if _, err := service.RegrantEventRule(ctx, "token", nil, rule.ID); err != nil {
		t.Fatal(err)
	}
	var original commandactivation.Rule
	if err := f.db.Where("id = ?", rule.ID).Take(&original).Error; err != nil {
		t.Fatal(err)
	}
	copy := cloneActivationRule(original)
	copy.ID, copy.RuleRef = "", ""
	if _, err := service.Apply(ctx, "token", nil, MutationIntent{Operation: RuleCreate, Rule: &copy}); err == nil {
		t.Fatal("copy transported grant")
	}
	clearRuleGrant(&copy)
	diff, err := service.Apply(ctx, "token", nil, MutationIntent{Operation: RuleCreate, Rule: &copy})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range diff.AfterActivationRules {
		if candidate.ID != rule.ID && (candidate.Enabled || hasGrantMetadata(candidate)) {
			t.Fatal("copy acquired authority")
		}
	}
	if len(diff.AfterActivationRules) != 2 || countRows(t, f.db, "command_layer_automation_grants") != 1 {
		t.Fatal("copy created grant or missing rule")
	}
	if _, err := service.Apply(ctx, "token", nil, MutationIntent{Operation: ConfigRestore}); err != nil {
		t.Fatal(err)
	}
	if countRows(t, f.db, "command_layer_activation_rules") != 0 || countRows(t, f.db, "command_layer_automation_grants") != 1 || !grantRevokedAt(t, f.db, *original.AutomationGrantID).Valid {
		t.Fatal("restore retained authority or lost history")
	}
}
