package commandportability

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"gorm.io/gorm"
)

func TestFromSnapshotNaoSerializaGrantsClaimsHistoricoOuDefaultsPuros(t *testing.T) {
	userID := portabilityUUID(t)
	layerID, bindingID, builtinBindingID, ruleID := portabilityUUID(t), portabilityUUID(t), portabilityUUID(t), portabilityUUID(t)
	command := "workspace.tab.new"
	decisionID, grantID := portabilityUUID(t), portabilityUUID(t)
	now := time.Now().UTC()
	workspace := "workspace-origem"

	snapshot := commandconfig.Snapshot{
		Scope:  commandconfig.Scope{UserID: userID, WorkspaceID: &workspace},
		Layers: []commandconfig.Layer{{ID: layerID, UserID: userID, WorkspaceID: &workspace, Name: "Camada", Description: "teste", Enabled: true, Source: "user", ResolutionPriority: 1}},
		Bindings: []commandconfig.Binding{{
			ID: bindingID, UserID: userID, WorkspaceID: &workspace, LayerRefKind: "user", LayerRef: layerID,
			TriggerType: "keyboard.local", TriggerSpec: `{}`, CommandID: &command, Arguments: `{}`, Condition: `{}`,
			Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`,
		}, {
			ID: builtinBindingID, UserID: userID, WorkspaceID: &workspace, LayerRefKind: "builtin", LayerRef: "application.defaults",
			TriggerType: "keyboard.local", TriggerSpec: `{}`, CommandID: &command, Arguments: `{}`, Condition: `{}`,
			Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{}`,
			ReplacesDefaultID: stringPtr("builtin.tab.new"), ReplacesDefaultVersion: stringPtr("1"), ReplacesDefaultFingerprint: stringPtr("default-fingerprint"),
		}},
		ActivationRules: []commandactivation.Rule{{
			ID: ruleID, UserID: userID, WorkspaceID: &workspace, LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
			RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeAlways, Condition: `{}`,
			Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReviewStatus: "active",
		}},
		Generations: []commandconfig.Generation{{ID: portabilityUUID(t), UserID: userID, WorkspaceID: &workspace, Generation: 17, UpdatedAt: now}},
		AutomationGrants: []commandautomation.Grant{{
			ID: grantID, Owner: commandautomation.Owner{UserID: userID, WorkspaceID: &workspace},
			LayerRef: commandautomation.RuleRef{Kind: "user", Ref: layerID}, RuleRef: commandautomation.RuleRef{Kind: "user", Ref: ruleID},
			RuleFingerprint: "rule-fingerprint", EventName: commandautomation.JobRunStateEvent,
			ProducerTypesFingerprint: "producer-fingerprint", AutomationGrantGeneration: 17,
			AutomationGrantFingerprint: "grant-fingerprint", AuthorizationDecisionID: decisionID,
			GrantedAt: now, GrantedBy: userID,
		}},
		ActivationClaims: []commandactivation.Claim{{
			ActivationID: portabilityUUID(t), LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
			RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, UserID: userID, WorkspaceID: &workspace,
			AuthContextType: "local_session", AuthContextID: portabilityUUID(t), AuthGeneration: "auth-generation",
			SecurityGeneration: "security-generation", SourceType: "event", State: commandactivation.StateActive,
			ActivatedAt: now, UpdatedAt: now,
		}},
	}

	exported, err := FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"automationGrants", "activationClaims", "generations", "history", "defaults", "builtinLayers", "claims", "grants"}
	for _, key := range forbidden {
		if strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("campo não portável %q serializado: %s", key, raw)
		}
	}
	if !strings.Contains(string(raw), `"activationRules"`) || !strings.Contains(string(raw), `"bindings"`) {
		t.Fatalf("conteúdo portável esperado ausente: %s", raw)
	}
}

func TestApplyPlanImportPreservaGrantEClaimDeRegraIntocada(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	layerB := commandconfig.Layer{ID: applyImportUUID(t), UserID: f.user, Name: "intocada", Description: "preservada", Enabled: true, Source: "user", ResolutionPriority: 1}
	if err := f.db.Create(&layerB).Error; err != nil {
		t.Fatal(err)
	}
	ruleID, grantID := applyImportUUID(t), applyImportUUID(t)
	eventName := commandautomation.JobRunStateEvent
	producers := `["jobs.runtime"]`
	decisionID := applyImportUUID(t)
	generation := int64(1)
	now := time.Now().UTC()
	ruleB := commandactivation.Rule{
		ID: ruleID, UserID: f.user, LayerRefKind: commandactivation.UserRef, LayerRef: layerB.ID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent,
		Condition: applyImportCondition, Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName,
		AllowedInternalProducerTypes: &producers, AuthorizationDecisionID: &decisionID, AutomationGrantID: &grantID,
		AutomationGrantGeneration: &generation, AutomationGrantFingerprint: stringPtr("grant-fingerprint"),
		Enabled: true, Source: "user", ReviewStatus: "active",
	}
	grantB := commandautomation.Grant{
		ID: grantID, Owner: commandautomation.Owner{UserID: f.user},
		LayerRef: commandautomation.RuleRef{Kind: "user", Ref: layerB.ID}, RuleRef: commandautomation.RuleRef{Kind: "user", Ref: ruleID},
		RuleFingerprint: "rule-fingerprint", EventName: eventName, ProducerTypesFingerprint: "producer-fingerprint",
		AutomationGrantGeneration: generation, AutomationGrantFingerprint: "grant-fingerprint", AuthorizationDecisionID: decisionID,
		GrantedAt: now, GrantedBy: f.user,
	}
	claimB := commandactivation.Claim{
		ActivationID: applyImportUUID(t), LayerRefKind: commandactivation.UserRef, LayerRef: layerB.ID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, UserID: f.user, AuthContextType: "local_session",
		AuthContextID: f.session, AuthGeneration: "auth-generation", SecurityGeneration: "security-generation",
		SourceType: "event", State: commandactivation.StateActive, ActivatedAt: now, UpdatedAt: now,
	}
	if err := f.db.Create(&ruleB).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`INSERT INTO command_layer_automation_grants
		(id,user_id,workspace_id,layer_ref_kind,layer_ref,rule_ref_kind,rule_ref,rule_fingerprint,event_name,producer_types_fingerprint,automation_grant_generation,automation_grant_fingerprint,authorization_decision_id,granted_at,granted_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, grantB.ID, grantB.Owner.UserID, nil, grantB.LayerRef.Kind, grantB.LayerRef.Ref, grantB.RuleRef.Kind, grantB.RuleRef.Ref, grantB.RuleFingerprint, grantB.EventName, grantB.ProducerTypesFingerprint, grantB.AutomationGrantGeneration, grantB.AutomationGrantFingerprint, grantB.AuthorizationDecisionID, grantB.GrantedAt, grantB.GrantedBy).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&claimB).Error; err != nil {
		t.Fatal(err)
	}
	before, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	refs := applyImportRefs(t, f)
	input := applyImportLayer(f, "camada substituída", nil)
	if _, err := ApplyPlanImport(context.Background(), f.service, "token", nil, []LayerExport{input}, PlanOptions{Mode: ReplaceMode}, applyImportOwner(f), refs); err != nil {
		t.Fatal(err)
	}
	after, err := f.store.Load(context.Background(), commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.ActivationRules) != len(before.ActivationRules) || len(after.AutomationGrants) != len(before.AutomationGrants) || len(after.ActivationClaims) != len(before.ActivationClaims) {
		t.Fatalf("estado intocado perdeu efeitos: before=%+v after=%+v", before, after)
	}
	beforeRule, beforeRuleOK := findRuleByID(before.ActivationRules, ruleID)
	beforeGrant, beforeGrantOK := findGrantByID(before.AutomationGrants, grantID)
	beforeClaim, beforeClaimOK := findClaimByID(before.ActivationClaims, claimB.ActivationID)
	if !beforeRuleOK || !beforeGrantOK || !beforeClaimOK || !sameRuleByID(after.ActivationRules, ruleID, beforeRule) || !sameGrantByID(after.AutomationGrants, grantID, beforeGrant) || !sameClaimByID(after.ActivationClaims, claimB.ActivationID, beforeClaim) {
		t.Fatalf("grant/claim de regra intocada foram alterados: after=%+v", after)
	}
}

func sameRuleByID(rows []commandactivation.Rule, id string, want commandactivation.Rule) bool {
	for _, row := range rows {
		if row.ID == id {
			return reflect.DeepEqual(row, want)
		}
	}
	return false
}

func findRuleByID(rows []commandactivation.Rule, id string) (commandactivation.Rule, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return commandactivation.Rule{}, false
}

func sameGrantByID(rows []commandautomation.Grant, id string, want commandautomation.Grant) bool {
	for _, row := range rows {
		if row.ID == id {
			return reflect.DeepEqual(row, want)
		}
	}
	return false
}

func findGrantByID(rows []commandautomation.Grant, id string) (commandautomation.Grant, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return commandautomation.Grant{}, false
}

func sameClaimByID(rows []commandactivation.Claim, id string, want commandactivation.Claim) bool {
	for _, row := range rows {
		if row.ActivationID == id {
			return reflect.DeepEqual(row, want)
		}
	}
	return false
}

func findClaimByID(rows []commandactivation.Claim, id string) (commandactivation.Claim, bool) {
	for _, row := range rows {
		if row.ActivationID == id {
			return row, true
		}
	}
	return commandactivation.Claim{}, false
}
