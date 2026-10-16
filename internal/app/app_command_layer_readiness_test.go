package app

import (
	"encoding/json"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/commandledger"
	"assistente/internal/wailsapi"
)

func TestCommandLayerPaletteReadinessRequiresEffectiveConfiguredBindingForActivate(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	assertReady := func(want bool) {
		t.Helper()
		items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Source: "palette"})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ID == commandLayerActivateID {
				if item.Available != want || !want && item.ReadinessReason == "" {
					t.Fatalf("readiness de ativar=%+v; disponível esperado=%v", item, want)
				}
				return
			}
		}
		t.Fatal("ação de camada ausente do catálogo")
	}
	assertReady(false)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Controle", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	target := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Alvo manual", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: target.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{
		LayerID: layer.ID, CommandID: commandLayerActivateID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"layer.activate"}`,
		Arguments: map[string]any{"scope": "global", "rule_id": rule.ID, "duration_seconds": 0}, Effect: "execute", Enabled: true,
	}})
	assertReady(true)
	result, err := a.ExecutePaletteCommand(commandLayerActivateID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("ativação real falhou enquanto readiness estava disponível: %+v err=%v", result, err)
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_delete", ID: rule.ID})
	assertReady(false)
}

func TestCommandLayerPaletteReadinessBackRequiresConfiguredBinding(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	assertReady := func(want bool) {
		t.Helper()
		items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Source: "palette"})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ID == commandLayerBackID {
				if item.Available != want || !want && item.ReadinessReason == "" {
					t.Fatalf("readiness de back=%+v; disponível esperado=%v", item, want)
				}
				return
			}
		}
		t.Fatal("ação de back ausente do catálogo")
	}
	assertReady(false)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Controle de retorno", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	binding := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{
		LayerID: layer.ID, CommandID: commandLayerBackID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"layer.back"}`,
		Arguments: map[string]any{"scope": "global", "rule_id": "", "duration_seconds": 0}, Effect: "execute", Enabled: true,
	}})
	assertReady(true)
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_delete", ID: binding.ID})
	assertReady(false)
}

func TestCommandLayerPaletteReadinessSupportsWorkspaceScope(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	available := func() bool {
		t.Helper()
		items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Source: "palette"})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ID == commandLayerActivateID {
				return item.Available
			}
		}
		t.Fatal("ação de ativação ausente do catálogo")
		return false
	}
	if available() {
		t.Fatal("ativação workspace disponível sem binding efetivo")
	}
	control := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Controle workspace", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: control.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	target := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Alvo workspace", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: target.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "binding_create", Binding: &CommandSettingsBindingInput{
		LayerID: control.ID, CommandID: commandLayerActivateID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"layer.activate"}`,
		Arguments: map[string]any{"scope": "workspace", "rule_id": rule.ID, "duration_seconds": 0}, Effect: "execute", Enabled: true,
	}})
	if !available() {
		t.Fatal("ativação workspace permaneceu indisponível com binding e alvo efetivos")
	}
}
