package app

import (
	"testing"

	"assistente/internal/commandledger"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandKeyboardSpecificTabExecutesAndRejectsSiblingBeforeCommit(t *testing.T) {
	for _, command := range []string{commandProductWorkspaceListID, commandWorkspaceTabChatCreateID} {
		t.Run(command, func(t *testing.T) {
			a, decisions := contextualKeyboardChatFixture(t)
			observed := contextualKeyboardContext(t, a)
			layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Uma aba", Enabled: true}})
			settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: command, TriggerType: "keyboard.local", TriggerSpec: contextualKeyboardV1Spec, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: observed.SurfaceType}, {Field: "surface.id", Value: observed.SurfaceID}}}}})
			settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
			view, err := a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			shortcut := LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}}
			if command == commandProductWorkspaceListID {
				result, err := a.DispatchContextualLocalCommandKey(view.Generation, shortcut, "down", false, observed)
				if err != nil || result == nil || result.Status != string(commandledger.Succeeded) {
					t.Fatalf("specific backend execution: %+v %v", result, err)
				}
				if _, err := a.DispatchContextualLocalCommandKey(view.Generation, shortcut, "up", false, observed); err != nil {
					t.Fatal(err)
				}
			} else {
				reservation, err := a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, observed)
				if err != nil || reservation == nil {
					t.Fatalf("specific UI reservation: %+v %v", reservation, err)
				}
				handoff := takeUICommandFor(t, a, reservation.Ticket, command)
				before := len(a.workspaceMgr.Active().Tabs.Items)
				if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
					t.Fatal(err)
				}
				if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != string(commandledger.Succeeded) || len(a.workspaceMgr.Active().Tabs.Items) != before+1 {
					t.Fatalf("specific UI effect: %+v", result)
				}
				a.ResetLocalCommandKeyboard(view.Generation)
				if err := a.workspaceMgr.SetActiveTab(observed.SurfaceID); err != nil {
					t.Fatal(err)
				}
				view, err = a.GetLocalCommandKeyboardMap()
				if err != nil {
					t.Fatal(err)
				}
				reservation, err = a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, observed)
				if err != nil || reservation == nil {
					t.Fatalf("second reservation: %+v %v", reservation, err)
				}
				sibling := uuid.NewString()
				if err := a.workspaceMgr.AddTab(workspace.Tab{ID: sibling, Type: workspace.TabTypeChat}); err != nil {
					t.Fatal(err)
				}
				if err := a.workspaceMgr.SetActiveTab(sibling); err != nil {
					t.Fatal(err)
				}
				if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
					t.Fatal("same type but different ID accepted before effect")
				}
			}
			sibling := uuid.NewString()
			if err := a.workspaceMgr.AddTab(workspace.Tab{ID: sibling, Type: workspace.TabTypeChat}); err != nil {
				t.Fatal(err)
			}
			if err := a.workspaceMgr.SetActiveTab(sibling); err != nil {
				t.Fatal(err)
			}
			current := contextualKeyboardContext(t, a)
			if result, err := a.DispatchContextualLocalCommandKey(view.Generation, shortcut, "down", false, current); result != nil || err == nil {
				t.Fatalf("sibling bypassed ID restriction: %+v %v", result, err)
			}
		})
	}
}
