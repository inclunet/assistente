package app

import (
	"errors"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
)

func TestApplyCommandLayerActionScopedCycleAndBack(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	_, ruleID := settingsActivationSecurityLayerAndRule(t, a, decisions)

	apply := func(scope, rule, action string, duration int) CommandSettingsMutation {
		t.Helper()
		result, err := a.ApplyCommandLayerAction(scope, rule, action, duration)
		if err != nil || !result.Committed || !result.Published {
			t.Fatalf("ação %s não publicou: %+v err=%v", action, result, err)
		}
		return result
	}
	if result := apply("global", ruleID, "pin", 0); result.ID != ruleID {
		t.Fatalf("pin retornou ID incorreto: %+v", result)
	}
	if result := apply("global", ruleID, "toggle", 0); result.ID != ruleID {
		t.Fatalf("toggle retornou ID incorreto: %+v", result)
	}
	if result := apply("global", ruleID, "pin", 0); result.ID != ruleID {
		t.Fatalf("segundo pin retornou ID incorreto: %+v", result)
	}
	if result := apply("global", ruleID, "deactivate", 0); result.ID != ruleID {
		t.Fatalf("deactivate retornou ID incorreto: %+v", result)
	}
	apply("global", ruleID, "pin", 0)
	if result := apply("global", "", "back", 0); result.ID != ruleID {
		t.Fatalf("back não retornou o RuleRef da claim: %+v", result)
	}

	var active int64
	if err := database.DB().Model(&commandactivation.Claim{}).Where("rule_ref = ? AND state = ?", ruleID, commandactivation.StateActive).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("ciclo deixou claims ativas: %d", active)
	}
	if _, err := a.ApplyCommandLayerAction("workspace", ruleID, "pin", 0); !errors.Is(err, commandexecution.ErrDenied) && !errors.Is(err, commandexecution.ErrInvalidRequest) {
		t.Fatalf("ação fora do escopo não falhou fechado: %v", err)
	}
}

func TestApplyCommandLayerActionRevokedSessionCannotMutate(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	_, ruleID := settingsActivationSecurityLayerAndRule(t, a, decisions)
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	result, err := a.ApplyCommandLayerAction("global", ruleID, "pin", 0)
	if err == nil || result.Committed || result.Published {
		t.Fatalf("sessão revogada autorizou ação: %+v err=%v", result, err)
	}
	var claims int64
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ?", ruleID).Count(&claims).Error; err != nil {
		t.Fatal(err)
	}
	if claims != 0 {
		t.Fatalf("ação revogada criou claim: %d", claims)
	}
}

func TestApplyCommandLayerActionBackAcrossLayersAndDisabledLayer(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layerOne, ruleOne := settingsActivationSecurityLayerAndRule(t, a, decisions)
	layerTwo := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Camada dois", Enabled: true})
	})
	ruleTwo := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayer(layerTwo.ID)
	})

	apply := func(ruleID, action string) CommandSettingsMutation {
		t.Helper()
		result, err := a.ApplyCommandLayerAction("global", ruleID, action, 0)
		if err != nil || !result.Committed || !result.Published {
			t.Fatalf("ação %s falhou: %+v err=%v", action, result, err)
		}
		return result
	}
	apply(ruleOne, "pin")
	apply(ruleTwo.ID, "pin")
	if result := apply("", "back"); result.ID != ruleTwo.ID {
		t.Fatalf("back não removeu a claim da camada mais recente: %+v", result)
	}
	if result := apply("", "back"); result.ID != ruleOne {
		t.Fatalf("back não removeu a claim anterior: %+v", result)
	}

	apply(ruleOne, "pin")
	if err := database.DB().Table("command_layers").Where("id = ?", layerOne).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"pin", "toggle"} {
		result, err := a.ApplyCommandLayerAction("global", ruleOne, action, 0)
		if err == nil || result.Committed || result.Published {
			t.Fatalf("disabled layer accepted %s: %+v %v", action, result, err)
		}
	}
	if result := apply(ruleOne, "deactivate"); result.ID != ruleOne {
		t.Fatalf("deactivate não removeu claim de camada desabilitada: %+v", result)
	}
	var claim commandactivation.Claim
	if err := database.DB().Where("rule_ref = ?", ruleOne).Order("activated_at DESC").First(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.State != commandactivation.StateDeactivated {
		t.Fatalf("claim da camada desabilitada não foi encerrada: %+v", claim)
	}
}

func TestApplyCommandLayerActionRejectsInvalidDurationsAndAmbiguousSelection(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Duração", Enabled: true}})
	persistent := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true}})
	temporary := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "temporary", Enabled: true}})

	for _, tc := range []struct {
		name     string
		ruleID   string
		action   string
		duration int
	}{
		{"persistent com prazo", persistent.ID, "pin", 1},
		{"temporária sem prazo", temporary.ID, "pin", 0},
		{"prazo negativo", persistent.ID, "pin", -1},
		{"prazo acima do limite", temporary.ID, "pin", 86401},
		{"pin sem regra", "", "pin", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := a.ApplyCommandLayerAction("global", tc.ruleID, tc.action, tc.duration)
			if err == nil || result.Committed || result.Published {
				t.Fatalf("entrada inválida foi aceita: %+v err=%v", result, err)
			}
		})
	}
}
