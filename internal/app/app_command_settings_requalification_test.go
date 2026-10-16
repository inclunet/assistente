package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
)

// Esta prova percorre a fachada de settings, e não somente o applier interno:
// uma personalização existente passa por upgrade, fica needs_review quando o
// default mudou e só volta a active após um rebase com o trio atual.
func TestCommandSettingsRequalificationDefaultUpgradeAndRebaseViaSettingsAPI(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	const defaultID = "builtin.palette.workspace.list"
	global, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	var target CommandSettingsBinding
	for _, item := range global.Bindings {
		if item.DefaultID == defaultID {
			target = item
			break
		}
	}
	if target.DefaultID != defaultID || target.LayerID == "" || target.CurrentDefaultVersion == "" || target.CurrentDefaultFingerprint == "" {
		t.Fatalf("default global não encontrado na projeção: %+v", global.Bindings)
	}
	created := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: &CommandSettingsBindingInput{
			LayerID: target.LayerID, TriggerType: target.TriggerType, TriggerSpec: target.TriggerSpec,
			Arguments: map[string]any{}, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{}},
			Effect: "suppress", Enabled: true, ResolutionPriority: target.ResolutionPriority,
			ReplacesDefaultID: target.DefaultID, ReplacesDefaultVersion: target.CurrentDefaultVersion,
			ReplacesDefaultFingerprint: target.CurrentDefaultFingerprint,
		},
	})

	var row commandconfig.Binding
	if err := database.DB().Where("replaces_default_id = ? AND user_id = ?", defaultID, a.currentUserID).First(&row).Error; err != nil {
		t.Fatalf("delta criado pelo caminho real de settings não encontrado: %v", err)
	}
	if created.ID != row.ID || row.Effect != "suppress" || row.ReviewStatus != string(commandbindings.Active) {
		t.Fatalf("personalização inicial inesperada: %+v", row)
	}

	if err := database.DB().Model(&commandconfig.Binding{}).Where("id = ?", row.ID).Updates(map[string]any{
		"replaces_default_version":     "0",
		"replaces_default_fingerprint": "obsolete-fingerprint",
		"review_status":                string(commandbindings.Active),
	}).Error; err != nil {
		t.Fatal(err)
	}

	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "default_upgrade",
	})
	afterUpgrade, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	var upgraded CommandSettingsBinding
	for _, binding := range afterUpgrade.Bindings {
		if binding.ID == row.ID {
			upgraded = binding
			break
		}
	}
	if upgraded.ID == "" || upgraded.ReviewStatus != string(commandbindings.NeedsReview) {
		t.Fatalf("upgrade pela API não marcou needs_review: %+v", upgraded)
	}
	if upgraded.ReplacesDefaultVersion != "0" || upgraded.ReplacesDefaultFingerprint != "obsolete-fingerprint" {
		t.Fatalf("upgrade alterou o trio obsoleto antes do rebase: %+v", upgraded)
	}

	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "default_rebase",
		Default: &CommandSettingsDefaultInput{
			BindingID: row.ID,
			Default: CommandSettingsDefaultRef{
				ID:          defaultID,
				Version:     "1",
				Fingerprint: currentDefaultFingerprint(t, a, defaultID),
			},
			Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{}},
		},
	})
	afterRebase, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	var rebased CommandSettingsBinding
	for _, binding := range afterRebase.Bindings {
		if binding.ID == row.ID {
			rebased = binding
			break
		}
	}
	if rebased.ID == "" || rebased.ReviewStatus != string(commandbindings.Active) || rebased.ReplacesDefaultVersion != "1" {
		t.Fatalf("rebase pela API não restaurou o binding: %+v", rebased)
	}
	if rebased.ReplacesDefaultFingerprint != currentDefaultFingerprint(t, a, defaultID) {
		t.Fatalf("rebase não atualizou fingerprint: %+v", rebased)
	}

	// O efeito de supressão continua publicado: rebase atualiza a referência do
	// delta, não transforma a intenção do usuário em execução do default.
	result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil)
	if err != nil || result.Status != "suppressed" {
		t.Fatalf("rebase perdeu a mudança efetiva do mapa: %+v err=%v", result, err)
	}
}

func TestCommandSettingsRequalificationWorkspaceDefaultUpgradeAndRebaseStayScoped(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	const defaultID = "builtin.palette.workspace.list"
	workspace, err := a.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	var target CommandSettingsBinding
	for _, item := range workspace.Bindings {
		if item.DefaultID == defaultID {
			target = item
			break
		}
	}
	if target.DefaultID != defaultID || target.LayerID == "" || target.CurrentDefaultVersion == "" || target.CurrentDefaultFingerprint == "" {
		t.Fatalf("default workspace não encontrado na projeção: %+v", workspace.Bindings)
	}
	fingerprint := target.CurrentDefaultFingerprint
	created := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeWorkspace, Operation: "binding_create",
		Binding: &CommandSettingsBindingInput{
			LayerID: target.LayerID, TriggerType: target.TriggerType, TriggerSpec: target.TriggerSpec,
			Arguments: map[string]any{}, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{}},
			Effect: "suppress", Enabled: true, ResolutionPriority: target.ResolutionPriority,
			ReplacesDefaultID: target.DefaultID, ReplacesDefaultVersion: target.CurrentDefaultVersion,
			ReplacesDefaultFingerprint: target.CurrentDefaultFingerprint,
		},
	})
	if created.ID == "" {
		t.Fatalf("binding_create workspace não retornou ID: %+v", created)
	}
	// A alteração de versão é somente o evento externo que torna o binding
	// obsoleto; a criação do override acima segue o caminho settings/UI real.
	if err := database.DB().Model(&commandconfig.Binding{}).Where("id = ?", created.ID).Updates(map[string]any{
		"replaces_default_version":     "0",
		"replaces_default_fingerprint": "obsolete-workspace-fingerprint",
		"review_status":                string(commandbindings.Active),
	}).Error; err != nil {
		t.Fatal(err)
	}

	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "default_upgrade"})
	workspace, err = a.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	var pending CommandSettingsBinding
	for _, item := range workspace.Bindings {
		if item.ID == created.ID {
			pending = item
		}
	}
	if pending.ID == "" || pending.ReviewStatus != string(commandbindings.NeedsReview) {
		t.Fatalf("upgrade workspace não marcou needs_review: %+v", pending)
	}

	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeWorkspace,
		Operation: "default_rebase",
		Default: &CommandSettingsDefaultInput{
			BindingID: created.ID,
			Default:   CommandSettingsDefaultRef{ID: defaultID, Version: "1", Fingerprint: fingerprint},
			Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{}},
		},
	})
	workspace, err = a.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range workspace.Bindings {
		if item.ID == created.ID && item.ReviewStatus != string(commandbindings.Active) {
			t.Fatalf("rebase workspace não restaurou active: %+v", item)
		}
	}
	global, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range global.Bindings {
		if item.ID == created.ID {
			t.Fatalf("override workspace vazou para o escopo global: %+v", item)
		}
	}
}

func TestCommandSettingsDefaultRestoreWorkspaceKeepsInheritedGlobalOverride(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	const defaultID = "builtin.palette.workspace.list"
	target := settingsDefaultBinding(t, a, CommandSettingsScopeGlobal, defaultID)
	global := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: settingsDefaultOverrideInput(target),
	})

	workspaceBefore, err := a.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if row := settingsFindDefaultBinding(workspaceBefore, defaultID); row.ID != global.ID || !row.Customized || !row.ReadOnly {
		t.Fatalf("workspace não projetou override global herdado: %+v", row)
	}
	local := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeWorkspace, Operation: "layer_create",
		Layer: &CommandSettingsLayerInput{Name: "Restore local default test", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "config_restore"})
	workspaceAfter, err := a.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if row := settingsFindDefaultBinding(workspaceAfter, defaultID); row.ID != global.ID || !row.Customized || !row.ReadOnly {
		t.Fatalf("restore workspace removeu ou alterou override global herdado: %+v", row)
	}
	globalAfter, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	if row := settingsFindDefaultBinding(globalAfter, defaultID); row.ID != global.ID || !row.Customized {
		t.Fatalf("restore workspace alterou o escopo global: %+v", row)
	}
	for _, layer := range workspaceAfter.Layers {
		if layer.ID == local.ID {
			t.Fatalf("restore workspace reteve camada local: %+v", layer)
		}
	}
}

func TestCommandSettingsDefaultCreateRollbackOnDeniedDecision(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	target := settingsDefaultBinding(t, a, CommandSettingsScopeGlobal, "builtin.palette.workspace.list")
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		snapshot, err := a.GetCommandSettingsForScope("pt-BR", "global")
		if err != nil {
			return CommandSettingsMutation{}, err
		}
		return a.MutateCommandSettings(CommandSettingsMutationRequest{
			Locale: "pt-BR", Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
			ExpectedRevision: snapshot.Revision, ExpectedFingerprint: snapshot.Fingerprint,
			Binding: settingsDefaultOverrideInput(target),
		})
	})
	select {
	case payload := <-decisions:
		finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.DenyAction}, false)
	case early := <-done:
		t.Fatalf("binding_create terminou antes da decisão: %+v", early)
	}
	outcome := settingsSecurityFinish(t, done)
	if outcome.err == nil || outcome.result.Committed || outcome.result.Published {
		t.Fatalf("rollback da decisão negada não ocorreu: %+v", outcome)
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Binding{}).Where("replaces_default_id = ? AND user_id = ?", target.DefaultID, a.currentUserID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("binding default persistiu após negação: %d", count)
	}
}

func TestCommandSettingsDefaultOverridesSurviveRestartInGlobalAndWorkspaceScopes(t *testing.T) {
	ctx := context.Background()
	a, decisions := settingsSecurityFixture(t)
	const defaultID = "builtin.palette.workspace.list"
	target := settingsDefaultBinding(t, a, CommandSettingsScopeGlobal, defaultID)
	global := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: settingsDefaultOverrideInput(target),
	})
	workspaceTarget := settingsDefaultBinding(t, a, CommandSettingsScopeWorkspace, defaultID)
	workspace := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeWorkspace, Operation: "binding_create",
		Binding: settingsDefaultOverrideInput(workspaceTarget),
	})

	fresh := restartCommandApp(t, a)
	if _, err := a.commandEpochs.CloseAndDrain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.commandEpochs.ReleaseInstance(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.mountCommandProduct(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	globalAfter, err := fresh.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	if row := settingsFindDefaultBinding(globalAfter, defaultID); row.ID != global.ID || !row.Customized {
		t.Fatalf("override global não sobreviveu ao restart: %+v", row)
	}
	workspaceAfter, err := fresh.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if row := settingsFindDefaultBinding(workspaceAfter, defaultID); row.ID != workspace.ID || !row.Customized || row.WorkspaceID == "" {
		t.Fatalf("override workspace não sobreviveu ao restart: %+v", row)
	}
	result, err := fresh.ExecutePaletteCommand(commandProductWorkspaceListID, nil)
	if err != nil || result.Status != "suppressed" {
		t.Fatalf("mapa publicado reanimou default após restart: %+v err=%v", result, err)
	}
}

func settingsDefaultBinding(t *testing.T, a *App, scope CommandSettingsScope, defaultID string) CommandSettingsBinding {
	t.Helper()
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", string(scope))
	if err != nil {
		t.Fatal(err)
	}
	row := settingsFindDefaultBinding(snapshot, defaultID)
	if row.DefaultID != defaultID || row.LayerID == "" || row.CurrentDefaultVersion == "" || row.CurrentDefaultFingerprint == "" {
		t.Fatalf("default não encontrado no escopo %s: %+v", scope, row)
	}
	return row
}

func settingsFindDefaultBinding(snapshot CommandSettingsSnapshot, defaultID string) CommandSettingsBinding {
	for _, row := range snapshot.Bindings {
		if row.DefaultID == defaultID {
			return row
		}
	}
	return CommandSettingsBinding{}
}

func settingsDefaultOverrideInput(target CommandSettingsBinding) *CommandSettingsBindingInput {
	return &CommandSettingsBindingInput{
		LayerID: target.LayerID, TriggerType: target.TriggerType, TriggerSpec: target.TriggerSpec,
		Arguments: map[string]any{}, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{}},
		Effect: "suppress", Enabled: true, ResolutionPriority: target.ResolutionPriority,
		ReplacesDefaultID: target.DefaultID, ReplacesDefaultVersion: target.CurrentDefaultVersion,
		ReplacesDefaultFingerprint: target.CurrentDefaultFingerprint,
	}
}

func currentDefaultFingerprint(t *testing.T, a *App, defaultID string) string {
	t.Helper()
	projection, err := commandProductProjection(a.commandProduct.Load().registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range projection.BuiltinLayers {
		for _, item := range layer.Defaults {
			if item.Candidate.ID == defaultID {
				return item.Fingerprint
			}
		}
	}
	t.Fatalf("default não encontrado na projeção: %s", defaultID)
	return ""
}
