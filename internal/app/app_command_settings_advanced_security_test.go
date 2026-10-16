package app

import (
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
)

func advancedSettingsSnapshot(t *testing.T, a *App, scope CommandSettingsScope) CommandSettingsSnapshot {
	t.Helper()
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", string(scope))
	if err != nil {
		t.Fatalf("GetCommandSettingsForScope(%q): %v", scope, err)
	}
	if snapshot.Revision < 1 || snapshot.Fingerprint == "" {
		t.Fatalf("snapshot sem etag utilizável: %+v", snapshot)
	}
	return snapshot
}

func advancedSettingsApply(t *testing.T, a *App, decisions <-chan map[string]any, req CommandSettingsMutationRequest) CommandSettingsMutation {
	t.Helper()
	snapshot := advancedSettingsSnapshot(t, a, req.Scope)
	req.Locale, req.ExpectedRevision, req.ExpectedFingerprint = "pt-BR", snapshot.Revision, snapshot.Fingerprint
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.MutateCommandSettings(req)
	})
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	outcome := settingsSecurityFinish(t, done)
	if outcome.err != nil || !outcome.result.Committed || !outcome.result.Published {
		t.Fatalf("mutação não publicou: req=%+v outcome=%+v", req, outcome)
	}
	return outcome.result
}

func advancedSettingsAssertDeniedWithoutDecision(t *testing.T, a *App, decisions <-chan map[string]any, req CommandSettingsMutationRequest) {
	t.Helper()
	result, err := a.MutateCommandSettings(req)
	if err == nil || result.Committed || result.Published {
		t.Fatalf("mutação indevida aceita: req=%+v result=%+v err=%v", req, result, err)
	}
	select {
	case decision := <-decisions:
		t.Fatalf("mutação inválida chegou à decisão: %#v", decision)
	default:
	}
}

func TestCommandSettingsAdvancedSecurityCancelDoesNotCommit(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	before := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	const marker = "cancelamento sem commit"
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.MutateCommandSettings(CommandSettingsMutationRequest{
			Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.LayerCreate),
			Layer:            &CommandSettingsLayerInput{Name: marker, Enabled: true},
			ExpectedRevision: before.Revision, ExpectedFingerprint: before.Fingerprint,
		})
	})
	var decision map[string]any
	select {
	case decision = <-decisions:
	case outcome := <-done:
		t.Fatalf("mutação terminou antes do cancelamento: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("decisão de cancelamento não foi apresentada")
	}
	finishCommandDecision(t, a.questionnaireMgr, decision, nil, true)
	outcome := settingsSecurityFinish(t, done)
	if outcome.err == nil || outcome.result.Committed || outcome.result.Published {
		t.Fatalf("cancelamento autorizou commit: %+v", outcome)
	}
	after := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	if after.Revision != before.Revision || after.Fingerprint != before.Fingerprint {
		t.Fatalf("cancelamento alterou o snapshot: antes=%+v depois=%+v", before, after)
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Layer{}).Where("name = ?", marker).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("camada cancelada persistida: count=%d err=%v", count, err)
	}
}

func TestCommandSettingsAdvancedSecurityRejectsForeignAndInheritedIDs(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	now := time.Now().UTC()
	foreignID := appCommandPortabilityUUID(t)
	foreignUserID := appCommandPortabilityUUID(t)
	foreign := commandconfig.Layer{ID: foreignID, UserID: foreignUserID, Name: "owner alheio", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now}
	if err := database.DB().Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.DB().Delete(&commandconfig.Layer{}, "id = ?", foreignID).Error })
	global := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	advancedSettingsAssertDeniedWithoutDecision(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.LayerDelete), ID: foreignID,
		ExpectedRevision: global.Revision, ExpectedFingerprint: global.Fingerprint,
	})

	inheritedID := appCommandPortabilityUUID(t)
	inherited := commandconfig.Layer{ID: inheritedID, UserID: a.currentUserID, Name: "global herdada", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now}
	if err := database.DB().Create(&inherited).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.DB().Delete(&commandconfig.Layer{}, "id = ?", inheritedID).Error })
	workspace := advancedSettingsSnapshot(t, a, CommandSettingsScopeWorkspace)
	foundInherited := false
	for _, layer := range workspace.Layers {
		if layer.ID == inheritedID && layer.WorkspaceID == "" {
			foundInherited = true
			break
		}
	}
	if !foundInherited {
		t.Fatalf("fixture não projetou a camada global herdada: %+v", workspace.Layers)
	}
	advancedSettingsAssertDeniedWithoutDecision(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeWorkspace, Operation: string(commandconfig.LayerDelete), ID: inheritedID,
		ExpectedRevision: workspace.Revision, ExpectedFingerprint: workspace.Fingerprint,
	})

	var persisted commandconfig.Layer
	if err := database.DB().First(&persisted, "id = ?", inheritedID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Name != inherited.Name || persisted.UserID != a.currentUserID || persisted.WorkspaceID != nil {
		t.Fatalf("camada herdada foi alterada: %+v", persisted)
	}
}

func TestCommandSettingsAdvancedSecurityWorkspaceSwitchDuringDecisionDoesNotCommit(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	if a.workspaceCtrl == nil {
		a.workspaceCtrl = controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{
			WorkspaceMgr: a.workspaceMgr,
			Emitter:      globalJobTestEmitter(func(string, any) {}),
		})
	}
	original := a.workspaceMgr.ActiveID()
	other, err := a.workspaceMgr.Create("settings-security-outro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if a.workspaceMgr.ActiveID() != original {
			_, _ = a.workspaceCtrl.SwitchWorkspace(original)
		}
	})
	before := advancedSettingsSnapshot(t, a, CommandSettingsScopeWorkspace)
	const marker = "workspace trocado durante decisão"
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.MutateCommandSettings(CommandSettingsMutationRequest{
			Scope: CommandSettingsScopeWorkspace, Operation: string(commandconfig.LayerCreate),
			Layer:            &CommandSettingsLayerInput{Name: marker, Enabled: true},
			ExpectedRevision: before.Revision, ExpectedFingerprint: before.Fingerprint,
		})
	})
	var decision map[string]any
	select {
	case decision = <-decisions:
	case outcome := <-done:
		t.Fatalf("mutação terminou antes da troca de workspace: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("decisão de workspace não foi apresentada")
	}
	if _, err := a.workspaceCtrl.SwitchWorkspace(other.ID); err != nil {
		t.Fatal(err)
	}
	finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	outcome := settingsSecurityFinish(t, done)
	if outcome.err == nil || outcome.result.Committed || outcome.result.Published {
		t.Fatalf("troca de workspace permitiu commit: %+v", outcome)
	}
	if count := advancedSettingsLayerCount(t, marker); count != 0 {
		t.Fatalf("camada stale persistida em algum workspace: count=%d", count)
	}
}
func advancedSettingsLayerCount(t *testing.T, name string) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Model(&commandconfig.Layer{}).Where("name = ?", name).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCommandSettingsAdvancedSecurityDefaultOverrideAndRestore(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	const defaultID = "builtin.palette.workspace.list"
	initial := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	var target CommandSettingsBinding
	for _, binding := range initial.Bindings {
		if binding.DefaultID == defaultID {
			target = binding
			break
		}
	}
	if target.DefaultID != defaultID || target.LayerID == "" || target.CurrentDefaultVersion == "" || target.CurrentDefaultFingerprint == "" {
		t.Fatalf("default de override não encontrado na projeção: %+v", initial.Bindings)
	}
	advancedSettingsApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.BindingCreate),
		Binding: &CommandSettingsBindingInput{
			LayerID: target.LayerID, TriggerType: target.TriggerType, TriggerSpec: target.TriggerSpec,
			Arguments: map[string]any{}, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Op: "eq", Value: "chat"}}},
			Effect: "suppress", Enabled: true, ResolutionPriority: 41, ReplacesDefaultID: target.DefaultID,
			ReplacesDefaultVersion: target.CurrentDefaultVersion, ReplacesDefaultFingerprint: target.CurrentDefaultFingerprint,
		},
	})
	overridden := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	var override commandconfig.Binding
	if err := database.DB().Where("replaces_default_id = ?", defaultID).First(&override).Error; err != nil {
		t.Fatalf("override não persistido: %v", err)
	}
	var overriddenView CommandSettingsBinding
	for _, binding := range overridden.Bindings {
		if binding.DefaultID == defaultID {
			overriddenView = binding
			break
		}
	}
	if !overriddenView.Customized || overriddenView.Enabled || overriddenView.ID != override.ID || overriddenView.WorkspaceID != "" || overriddenView.ResolutionPriority != 41 || len(overriddenView.Arguments) != 0 || len(overriddenView.Condition.Clauses) != 1 || overriddenView.Condition.Clauses[0].Field != "surface.type" || overriddenView.Condition.Clauses[0].Value != "chat" || overriddenView.ReplacesDefaultVersion != target.CurrentDefaultVersion || overriddenView.ReplacesDefaultFingerprint != target.CurrentDefaultFingerprint {
		t.Fatalf("override não suprimiu default na projeção: %+v", overriddenView)
	}
	advancedSettingsApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.BindingRestore), ID: override.ID,
	})
	restored := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	var restoredView CommandSettingsBinding
	for _, binding := range restored.Bindings {
		if binding.DefaultID == defaultID {
			restoredView = binding
			break
		}
	}
	if restoredView.Customized || !restoredView.Enabled {
		t.Fatalf("restore não devolveu default: %+v", restoredView)
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Binding{}).Where("replaces_default_id = ?", defaultID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("restore deixou override: count=%d err=%v", count, err)
	}
}

func TestCommandSettingsAdvancedSecurityManualWorkspaceScopePinsAndBacks(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := advancedSettingsApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeWorkspace, Operation: string(commandconfig.LayerCreate),
		Layer: &CommandSettingsLayerInput{Name: "manual somente workspace", Enabled: true, ResolutionPriority: 13},
	})
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayerForScope("workspace", layer.ID)
	})
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	prepared := settingsSecurityFinish(t, done)
	if prepared.err != nil || !prepared.result.Committed || !prepared.result.Published || prepared.result.ID == "" {
		t.Fatalf("prepare manual scoped falhou: %+v", prepared)
	}
	workspace := advancedSettingsSnapshot(t, a, CommandSettingsScopeWorkspace)
	var ruleID string
	for _, rule := range workspace.Rules {
		if rule.LayerID == layer.ID && rule.WorkspaceID != "" && rule.Mode == "manual" {
			ruleID = rule.ID
			break
		}
	}
	if ruleID == "" {
		t.Fatalf("regra manual scoped não apareceu: %+v", workspace.Rules)
	}
	for _, row := range workspace.Layers {
		if row.ID == layer.ID {
			if !row.ManualReady || row.ManualActive || row.Active {
				t.Fatalf("estado manual inicial incorreto: %+v", row)
			}
		}
	}
	activated, err := a.SetCommandLayerActiveForScope("workspace", layer.ID, true)
	if err != nil || !activated.Committed || !activated.Published || activated.ID != ruleID {
		t.Fatalf("pin manual scoped falhou: %+v err=%v", activated, err)
	}
	workspace = advancedSettingsSnapshot(t, a, CommandSettingsScopeWorkspace)
	for _, row := range workspace.Layers {
		if row.ID == layer.ID && (!row.ManualReady || !row.ManualActive || !row.Active) {
			t.Fatalf("estado manual após pin incorreto: %+v", row)
		}
	}
	backed, err := a.SetCommandLayerActiveForScope("workspace", layer.ID, false)
	if err != nil || !backed.Committed || !backed.Published || backed.ID != ruleID {
		t.Fatalf("back manual scoped falhou: %+v err=%v", backed, err)
	}
	workspace = advancedSettingsSnapshot(t, a, CommandSettingsScopeWorkspace)
	for _, row := range workspace.Layers {
		if row.ID == layer.ID && (row.ManualActive || row.Active) {
			t.Fatalf("estado manual após back incorreto: %+v", row)
		}
	}
	global := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	for _, row := range global.Layers {
		if row.ID == layer.ID {
			t.Fatalf("camada workspace vazou para escopo global: %+v", row)
		}
	}
}

func TestCommandSettingsAdvancedSecurityEventCreateEditAndRegrant(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := advancedSettingsApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.LayerCreate),
		Layer: &CommandSettingsLayerInput{Name: "event security", Enabled: true},
	})
	before := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	advancedSettingsAssertDeniedWithoutDecision(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.RuleCreate),
		ExpectedRevision: before.Revision, ExpectedFingerprint: before.Fingerprint,
		Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "event", Lifecycle: "persistent", Enabled: true},
	})
	rule := advancedSettingsApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.RuleCreate),
		Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "event", Lifecycle: "persistent", Enabled: false},
	})
	snapshot := advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	var created CommandSettingsRule
	for _, row := range snapshot.Rules {
		if row.ID == rule.ID {
			created = row
			break
		}
	}
	if created.ID == "" || created.Mode != "event" || created.Enabled || created.EventName == "" || created.AllowedInternalProducerTypes == "" || created.GrantGeneration != 0 || created.GrantFingerprint != "" {
		t.Fatalf("evento criado fora do estado pending esperado: %+v", created)
	}
	advancedSettingsApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.RuleUpdate), ID: rule.ID,
		Rule: &CommandSettingsRuleInput{ID: rule.ID, LayerID: layer.ID, Mode: "event", Lifecycle: "session", Enabled: false},
	})
	snapshot = advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	var edited CommandSettingsRule
	for _, row := range snapshot.Rules {
		if row.ID == rule.ID {
			edited = row
			break
		}
	}
	if edited.ID == "" || edited.Lifecycle != "session" || edited.Enabled || edited.GrantGeneration != 0 || edited.GrantFingerprint != "" {
		t.Fatalf("edição de evento alterou grant/estado indevidamente: %+v", edited)
	}
	advancedSettingsApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: string(commandconfig.RuleEnable), ID: rule.ID,
	})
	snapshot = advancedSettingsSnapshot(t, a, CommandSettingsScopeGlobal)
	var granted CommandSettingsRule
	for _, row := range snapshot.Rules {
		if row.ID == rule.ID {
			granted = row
			break
		}
	}
	if granted.ID == "" || !granted.Enabled || granted.GrantGeneration <= 0 || granted.GrantFingerprint == "" {
		t.Fatalf("regrant de evento não apareceu no snapshot: %+v", granted)
	}
}
