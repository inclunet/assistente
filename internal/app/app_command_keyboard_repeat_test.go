package app

import (
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandexecution"
)

type localKeyboardRepeatBinding struct {
	commandID string
	shortcut  LocalCommandShortcut
}

func localKeyboardRepeatFixture(t *testing.T, bindings ...localKeyboardRepeatBinding) (*App, LocalCommandKeyboardMap) {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Repetição seletiva", Enabled: true})
	})
	for _, binding := range bindings {
		binding := binding
		settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
			return a.SaveCommandBinding(CommandBindingEdit{
				LayerID: layer.ID, CommandID: binding.commandID, TriggerType: "keyboard.local",
				TriggerSpec: mustJSONShortcut(t, binding.shortcut), Enabled: true,
			})
		})
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayer(layer.ID)
	})
	if _, err := a.SetCommandLayerActive(layer.ID, true); err != nil {
		t.Fatal(err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	return a, view
}

func mustJSONShortcut(t *testing.T, shortcut LocalCommandShortcut) string {
	t.Helper()
	raw, err := json.Marshal(shortcut)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCommandKeyboardRepeatIsRejectedAtBackendIngress(t *testing.T) {
	navigationShortcut := LocalCommandShortcut{Version: 1, Code: "KeyJ", Modifiers: []string{"Control"}}

	t.Run("local navigation is projected and never reaches host", func(t *testing.T) {
		a, view := localKeyboardRepeatFixture(t, localKeyboardRepeatBinding{commandWorkspaceTabNextID, navigationShortcut})
		binding := commandKeyboardBindingFor(t, view, navigationShortcut)
		if binding.Handler != "local_ui" {
			t.Fatalf("navegação não foi projetada como local_ui: %+v", binding)
		}
		if reservation, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, false); !errors.Is(err, commandexecution.ErrDenied) || reservation != nil {
			t.Fatalf("navegação local criou handoff/ledger: reservation=%+v err=%v", reservation, err)
		}
		if reservation, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, true); !errors.Is(err, commandexecution.ErrDenied) || reservation != nil {
			t.Fatalf("repeat local alcançou backend: reservation=%+v err=%v", reservation, err)
		}
		a.ResetLocalCommandKeyboard(view.Generation)
	})

	t.Run("create or close repeat remains ignored", func(t *testing.T) {
		for _, commandID := range []string{commandWorkspaceTabChatCreateID, commandWorkspaceTabCloseID} {
			t.Run(commandID, func(t *testing.T) {
				a, view := localKeyboardRepeatFixture(t, localKeyboardRepeatBinding{commandID, navigationShortcut})
				binding := commandKeyboardBindingFor(t, view, navigationShortcut)
				first, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, false)
				if err != nil || first == nil {
					t.Fatalf("down inicial: reservation=%+v err=%v", first, err)
				}
				repeat, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, true)
			if !errors.Is(err, commandexecution.ErrDenied) || repeat != nil {
				t.Fatalf("repeat de %s foi aceito: reservation=%+v err=%v", commandID, repeat, err)
				}
				a.ResetLocalCommandKeyboard(view.Generation)
			})
		}
	})

	t.Run("local repeat never depends on held identity", func(t *testing.T) {
		changedShortcut := LocalCommandShortcut{Version: 1, Code: navigationShortcut.Code, Modifiers: []string{"Alt"}}
		a, view := localKeyboardRepeatFixture(t,
			localKeyboardRepeatBinding{commandWorkspaceTabNextID, navigationShortcut},
			localKeyboardRepeatBinding{commandWorkspaceTabNextID, changedShortcut},
		)
		changed := commandKeyboardBindingFor(t, view, changedShortcut)
		repeat, err := a.BeginLocalCommandUIKey(view.Generation, changed.Shortcut, true)
		if !errors.Is(err, commandexecution.ErrDenied) || repeat != nil {
			t.Fatalf("repeat local foi aceito: reservation=%+v err=%v", repeat, err)
		}
		a.ResetLocalCommandKeyboard(view.Generation)
	})
}
