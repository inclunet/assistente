package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func prepareKeyboardLayerActionBindings(t *testing.T, a *App, decisions <-chan map[string]any) (string, LocalCommandShortcut, LocalCommandShortcut) {
	t.Helper()
	control := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "layer_create",
		Layer:     &CommandSettingsLayerInput{Name: "Keyboard control always", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "rule_create",
		Rule:      &CommandSettingsRuleInput{LayerID: control.ID, Mode: "always", Lifecycle: "persistent", Enabled: true},
	})
	target := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "layer_create",
		Layer:     &CommandSettingsLayerInput{Name: "Keyboard manual target", Enabled: true},
	})
	prepared := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "rule_create",
		Rule:      &CommandSettingsRuleInput{LayerID: target.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true},
	})
	shortcutActivate := LocalCommandShortcut{Version: 1, Code: "KeyL", Modifiers: []string{"Control", "Shift"}}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: &CommandSettingsBindingInput{LayerID: target.ID, CommandID: commandProductWorkspaceListID,
			TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"F12","modifiers":["Control","Alt"]}`,
			Arguments: map[string]any{}, Effect: "execute", Enabled: true},
	})
	shortcutBack := LocalCommandShortcut{Version: 1, Code: "KeyB", Modifiers: []string{"Control", "Shift"}}
	for id, binding := range map[string]struct {
		shortcut LocalCommandShortcut
		args     map[string]any
	}{
		commandLayerActivateID: {shortcut: shortcutActivate, args: map[string]any{"scope": "global", "rule_id": prepared.ID, "duration_seconds": 0}},
		commandLayerBackID:     {shortcut: shortcutBack, args: map[string]any{"scope": "global", "rule_id": "", "duration_seconds": 0}},
	} {
		spec, err := json.Marshal(map[string]any{"version": 1, "code": binding.shortcut.Code, "modifiers": binding.shortcut.Modifiers})
		if err != nil {
			t.Fatal(err)
		}
		settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
			Scope:     CommandSettingsScopeGlobal,
			Operation: "binding_create",
			Binding: &CommandSettingsBindingInput{
				LayerID:     control.ID,
				CommandID:   id,
				TriggerType: "keyboard.local",
				TriggerSpec: string(spec),
				Arguments:   binding.args,
				Effect:      "execute",
				Enabled:     true,
			},
		})
	}
	return prepared.ID, shortcutActivate, shortcutBack
}

func TestCommandLayerKeyboardActivatesAndBacksAcrossPublishedGenerations(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	ruleID, activate, back := prepareKeyboardLayerActionBindings(t, a, decisions)
	first, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	assertTargetBinding := func(view LocalCommandKeyboardMap, want bool) {
		t.Helper()
		found := false
		for _, binding := range view.Bindings {
			if binding.Shortcut.Code == "F12" && binding.CommandID == commandProductWorkspaceListID {
				found = true
			}
		}
		if found != want {
			t.Fatalf("binding da camada alvo no mapa=%v, esperado=%v", found, want)
		}
	}
	assertTargetBinding(first, false)
	firstResult, err := a.DispatchLocalCommandKey(first.Generation, activate, "down", false)
	if err != nil || firstResult == nil || firstResult.Status != string(commandledger.Succeeded) {
		var summary, code string
		if firstResult != nil {
			if firstResult.ResultSummary != nil {
				summary = *firstResult.ResultSummary
			}
			if firstResult.ErrorCode != nil {
				code = *firstResult.ErrorCode
			}
		}
		status := "<nil>"
		if firstResult != nil {
			status = firstResult.Status
		}
		t.Fatalf("keyboard activate não concluiu: status=%q summary=%q error_code=%q result=%+v err=%v", status, summary, code, firstResult, err)
	}
	if firstResult.Output != nil {
		t.Fatalf("ação durável expôs output visual: %+v", firstResult.Output)
	}
	second, err := a.GetLocalCommandKeyboardMap()
	if err != nil || second.Generation == first.Generation {
		t.Fatalf("ativação não publicou mapa novo: first=%+v second=%+v err=%v", first, second, err)
	}
	assertTargetBinding(second, true)
	targetKey := LocalCommandShortcut{Version: 1, Code: "F12", Modifiers: []string{"Control", "Alt"}}
	targetResult, err := a.DispatchLocalCommandKey(second.Generation, targetKey, "down", false)
	if err != nil || targetResult == nil || targetResult.Status != string(commandledger.Succeeded) || targetResult.Output == nil {
		t.Fatalf("binding recém-ativado não executou a leitura real: %+v err=%v", targetResult, err)
	}
	if _, err := a.DispatchLocalCommandKey(second.Generation, targetKey, "up", false); err != nil {
		t.Fatal(err)
	}
	var active int64
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", ruleID, "active").Count(&active).Error; err != nil || active != 1 {
		t.Fatalf("claim manual não ficou ativa: count=%d err=%v", active, err)
	}
	if _, err := a.DispatchLocalCommandKey(first.Generation, activate, "up", false); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("mapa antigo aceitou keyup: err=%v", err)
	}
	backResult, err := a.DispatchLocalCommandKey(second.Generation, back, "down", false)
	if err != nil || backResult == nil || backResult.Status != string(commandledger.Succeeded) || backResult.Output != nil {
		t.Fatalf("keyboard back não concluiu sem output: result=%+v err=%v", backResult, err)
	}
	third, err := a.GetLocalCommandKeyboardMap()
	if err != nil || third.Generation == second.Generation {
		t.Fatalf("back não publicou mapa novo: second=%+v third=%+v err=%v", second, third, err)
	}
	assertTargetBinding(third, false)
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", ruleID, "active").Count(&active).Error; err != nil || active != 0 {
		t.Fatalf("back não removeu claim: count=%d err=%v", active, err)
	}
}

func TestCommandLayerKeyboardCancelAndLogoutBeforeCommitDoNotActivate(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	ruleID, _, _ := prepareKeyboardLayerActionBindings(t, a, decisions)
	beforeLogout, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	product := a.commandProduct.Load()
	if product == nil {
		t.Fatal("produto de comandos ausente")
	}
	args := json.RawMessage(`{"scope":"global","rule_id":"` + ruleID + `","duration_seconds":0}`)
	commandID := commandLayerActivateID
	trigger := json.RawMessage(`{"version":1,"code":"KeyL","modifiers":["Control","Shift"]}`)
	invocation := commandexecution.Invocation{
		ID:        "cancel-before-commit",
		CommandID: commandID,
		Principal: product.principal,
		Source:    commandcatalog.KeyboardLocal,
		Envelope:  &commandcontract.Envelope{Arguments: &args, TriggerSpec: &trigger},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handle, err := a.startCommandLayerAction(ctx, invocation)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status == commandledger.Succeeded {
			t.Fatalf("cancelamento pré-commit virou sucesso: %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler cancelado não terminou")
	}
	_ = a.Logout(LogoutRequest{})
	if result, err := a.DispatchLocalCommandKey(beforeLogout.Generation, LocalCommandShortcut{Version: 1, Code: "KeyL", Modifiers: []string{"Control", "Shift"}}, "down", false); err == nil || result != nil {
		t.Fatalf("logout não recusou keyboard layer: result=%+v err=%v", result, err)
	}
	var active int64
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", ruleID, "active").Count(&active).Error; err != nil || active != 0 {
		t.Fatalf("cancel/logout deixou claim ativa: count=%d err=%v", active, err)
	}
}
