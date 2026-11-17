package app

import (
	"encoding/json"
	"testing"

	"assistente/internal/commandledger"
)

func TestCommandSettingsPaletteProfileRuleUsesCurrentWorkspaceWithoutRebuild(t *testing.T) {
	a, decisions := contextualKeyboardChatFixture(t)
	a.workspaceMgr.SetDefaultProfile("profile-a")
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed("builtin.palette.workspace.list", true)
	})
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Perfil A", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandProductWorkspaceListID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.list"}`, Effect: "execute", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "profile", Value: "profile-a"}}}}})
	for _, profile := range []string{"profile-a", "profile-b", "profile-a"} {
		a.workspaceMgr.SetDefaultProfile(profile)
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		want := string(commandledger.Succeeded)
		if profile == "profile-b" {
			want = string(commandledger.Suppressed)
		}
		if err != nil || result.Status != want {
			t.Fatalf("profile=%s: %+v %v", profile, result, err)
		}
	}
}
