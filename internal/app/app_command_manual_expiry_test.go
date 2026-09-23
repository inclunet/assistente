package app

import (
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/database"
)

func TestCommandManualRuleEditorPreservesIndependentJobLifecycle(t *testing.T) {
	for _, lifecycle := range []string{"persistent", "session"} {
		rule, err := commandSettingsRuleInput(&CommandSettingsRuleInput{LayerID: "layer", Mode: "event", Lifecycle: lifecycle, Enabled: false}, "")
		if err != nil || string(rule.Lifecycle) != lifecycle || rule.Enabled {
			t.Fatalf("ciclo de job foi substituído pelo manual: %+v %v", rule, err)
		}
	}
}

func TestCommandManualTemporaryExpiresWithoutSettingsReload(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Temporária real", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "workspace.tab.next", TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyY","modifiers":["Control","Shift"]}`, Effect: "execute", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "temporary", Enabled: true}})
	result, err := a.ApplyCommandLayerAction("global", rule.ID, "pin", 1)
	if err != nil || !result.Published {
		t.Fatalf("ativação: %+v %v", result, err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || view.ValidUntil <= time.Now().UnixMilli() {
		t.Fatalf("mapa sem prazo: %+v %v", view, err)
	}
	p := a.commandProduct.Load()
	before, _, _, err := p.host.ResolutionSnapshot(a.ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := before.Resolve("keyboard.local:Control+Shift+KeyY", commandbindings.Facts{commandbindings.AppFocused: true}, nil)
	if err != nil || selected.Status != commandbindings.Selected {
		t.Fatalf("camada não executável: %+v %v", selected, err)
	}
	// Observe durable expiry only: no settings getter or rebuild drives it.
	limit := time.Now().Add(5 * time.Second)
	for {
		var claim commandactivation.Claim
		if err := database.DB().Where("rule_ref = ?", rule.ID).First(&claim).Error; err != nil {
			t.Fatal(err)
		}
		if claim.State == commandactivation.StateExpired {
			break
		}
		if time.Now().After(limit) {
			t.Fatal("scheduler não expirou claim")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		current, _, _, err := p.host.ResolutionSnapshot(a.ctx, p.principal)
		if err == nil && current != before {
			resolved, resolveErr := current.Resolve("keyboard.local:Control+Shift+KeyY", commandbindings.Facts{commandbindings.AppFocused: true}, nil)
			if resolveErr != nil || resolved.Status != commandbindings.NoMatch {
				t.Fatalf("binding vencido continua ativo: %+v %v", resolved, resolveErr)
			}
			break
		}
		if time.Now().After(limit) {
			t.Fatalf("scheduler não republicou: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
		t.Fatal(err)
	}
	var claim commandactivation.Claim
	if err := database.DB().Where("rule_ref = ?", rule.ID).First(&claim).Error; err != nil || claim.State != commandactivation.StateExpired {
		t.Fatalf("restart reviveu claim: %+v %v", claim, err)
	}
}

func TestCommandManualSessionSurvivesRebuildButNotRestart(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Só nesta sessão", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "session", Enabled: true}})
	if out, err := a.ApplyCommandLayerAction("global", rule.ID, "pin", 0); err != nil || !out.Published {
		t.Fatalf("ativação: %+v %v", out, err)
	}
	if err := a.rebuildCommandLifecycleProjection(a.ctx, false); err != nil {
		t.Fatal(err)
	}
	other, err := a.workspaceMgr.Create("Outra área de trabalho")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceMgr.Switch(other.ID); err != nil {
		t.Fatal(err)
	}
	a.reloadCommandsAfterWorkspaceSwitch()
	var claim commandactivation.Claim
	if err := database.DB().Where("rule_ref = ?", rule.ID).First(&claim).Error; err != nil || claim.State != commandactivation.StateActive {
		t.Fatalf("rebuild perdeu sessão: %+v %v", claim, err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Where("rule_ref = ?", rule.ID).First(&claim).Error; err != nil || claim.State == commandactivation.StateActive {
		t.Fatalf("restart reviveu sessão: %+v %v", claim, err)
	}
}

func TestCommandManualDeadlineSurvivesConfigurationEdit(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Prazo preservado", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "temporary", Enabled: true}})
	if out, err := a.ApplyCommandLayerAction("global", rule.ID, "pin", 30); err != nil || !out.Published {
		t.Fatalf("ativação: %+v %v", out, err)
	}
	before, err := a.GetLocalCommandKeyboardMap()
	if err != nil || before.ValidUntil == 0 {
		t.Fatalf("prazo inicial: %+v %v", before, err)
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_update", ID: layer.ID, Layer: &CommandSettingsLayerInput{ID: layer.ID, Name: "Nome editado", Enabled: true}})
	after, err := a.GetLocalCommandKeyboardMap()
	if err != nil || after.ValidUntil != before.ValidUntil {
		t.Fatalf("edição perdeu prazo: antes=%d depois=%d err=%v", before.ValidUntil, after.ValidUntil, err)
	}
	settings, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range settings.Rules {
		if row.ID == rule.ID {
			found = row.ManualActive && row.ManualExpiresAt == before.ValidUntil
		}
	}
	if !found {
		t.Fatal("UI não recebeu estado/prazo da regra ativa")
	}
}

func TestCommandManualSecurityEpochCannotBeRepublishedWithoutRestore(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Epoch vinculado", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true}})
	if out, err := a.ApplyCommandLayerAction("global", rule.ID, "pin", 0); err != nil || !out.Published {
		t.Fatalf("pin: %+v %v", out, err)
	}
	p := a.commandProduct.Load()
	if err := a.commandEpochs.InvalidateSecurity(a.ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := p.host.ResolutionSnapshot(a.ctx, p.principal); err == nil {
		t.Fatal("mapa antigo sobreviveu à invalidação de segurança")
	}
	if err := a.rebuildCommandLifecycleProjection(a.ctx, false); err != nil {
		t.Fatal(err)
	}
	_, active, _, err := p.host.ResolutionSnapshot(a.ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range active {
		if id == layer.ID {
			t.Fatal("rebuild simples reativou claim com epoch antigo")
		}
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
		t.Fatal(err)
	}
	_, active, _, err = p.host.ResolutionSnapshot(a.ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range active {
		found = found || id == layer.ID
	}
	if !found {
		t.Fatal("restore autenticado não restaurou persistente válida")
	}
}
