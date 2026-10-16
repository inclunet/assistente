package app

import (
	"errors"
	"testing"

	"assistente/internal/commandexecution"
	"assistente/internal/database"
)

func commandKeyboardNavigationFixture(t *testing.T) (*App, <-chan map[string]any, string, CommandSettingsMutation, LocalCommandKeyboardMap, LocalCommandKeyboardBinding) {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Navegação por teclado", Enabled: true})
	})
	binding := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer.ID, CommandID: "navigation.history.open", TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyH","modifiers":["Control","Shift"]}`, Enabled: true,
		})
	})
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
	for _, candidate := range view.Bindings {
		if candidate.CommandID == "navigation.history.open" && candidate.Shortcut.Code == "KeyH" && len(candidate.Shortcut.Modifiers) == 2 && candidate.Shortcut.Modifiers[0] == "Control" && candidate.Shortcut.Modifiers[1] == "Shift" {
			return a, decisions, layer.ID, binding, view, candidate
		}
	}
	t.Fatalf("navigation.history.open não entrou no mapa: %+v", view)
	return nil, nil, "", CommandSettingsMutation{}, LocalCommandKeyboardMap{}, LocalCommandKeyboardBinding{}
}

func commandKeyboardDurableFixture(t *testing.T, commandID string) (*App, <-chan map[string]any, string, CommandSettingsMutation, LocalCommandKeyboardMap, LocalCommandKeyboardBinding) {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Comando durável por teclado", Enabled: true})
	})
	binding := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer.ID, CommandID: commandID, TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyD","modifiers":["Control"]}`, Enabled: true,
		})
	})
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
	return a, decisions, layer.ID, binding, view, commandKeyboardBindingFor(t, view, LocalCommandShortcut{Version: 1, Code: "KeyD", Modifiers: []string{"Control"}})
}

func TestCommandKeyboardNavigationIsLocalAndDoesNotCreateHandoff(t *testing.T) {
	a, _, _, binding, view, keyboardBinding := commandKeyboardNavigationFixture(t)
	if keyboardBinding.Handler != "local_ui" || keyboardBinding.Shortcut.Code != "KeyH" {
		t.Fatalf("binding UI inesperado: %+v", keyboardBinding)
	}

	if result, err := a.DispatchLocalCommandKey(view.Generation, keyboardBinding.Shortcut, "down", false); err == nil || result != nil {
		t.Fatalf("rota backend aceitou binding UI: result=%+v err=%v", result, err)
	}
	if reservation, err := a.BeginLocalCommandUIKey(view.Generation, keyboardBinding.Shortcut, false); !errors.Is(err, commandexecution.ErrDenied) || reservation != nil {
		t.Fatalf("local_ui criou handoff/ledger: reservation=%+v err=%v", reservation, err)
	}
	if repeated, err := a.BeginLocalCommandUIKey(view.Generation, keyboardBinding.Shortcut, true); !errors.Is(err, commandexecution.ErrDenied) || repeated != nil {
		t.Fatalf("repeat local_ui alcançou host: reservation=%+v err=%v", repeated, err)
	}
	if result, err := a.DispatchLocalCommandKey(view.Generation, keyboardBinding.Shortcut, "down", false); err == nil || result != nil {
		t.Fatalf("rota backend aceitou binding UI pressionado: result=%+v err=%v", result, err)
	}
	if result, err := a.DispatchLocalCommandKey(view.Generation, keyboardBinding.Shortcut, "up", false); err != nil || result != nil {
		t.Fatalf("keyup UI não liberou: result=%+v err=%v", result, err)
	}
	a.ResetLocalCommandKeyboard(view.Generation)
	if binding.ID == "" {
		t.Fatal("binding não recebeu identidade")
	}
}

func TestCommandKeyboardNavigationStaleAndAdmissionGuards(t *testing.T) {
	t.Run("stale generation after reset", func(t *testing.T) {
		a, _, _, _, view, binding := commandKeyboardNavigationFixture(t)
		a.ResetLocalCommandKeyboard(view.Generation)
		if reservation, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, false); !errors.Is(err, commandexecution.ErrStale) || reservation != nil {
			t.Fatalf("Begin stale: reservation=%+v err=%v", reservation, err)
		}
	})

	t.Run("reset between begin and take", func(t *testing.T) {
		a, _, _, _, view, binding := commandKeyboardDurableFixture(t, commandWorkspaceTabChatCreateID)
		reservation, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, false)
		if err != nil {
			t.Fatal(err)
		}
		a.ResetLocalCommandKeyboard(view.Generation)
		if _, err := a.TakeUICommand(reservation.Ticket); !errors.Is(err, commandexecution.ErrStale) {
			t.Fatalf("Take após reset: %v", err)
		}
	})

	t.Run("configuration mutation before take", func(t *testing.T) {
		a, decisions, layerID, binding, view, keyboardBinding := commandKeyboardDurableFixture(t, commandWorkspaceTabChatCreateID)
		reservation, err := a.BeginLocalCommandUIKey(view.Generation, keyboardBinding.Shortcut, false)
		if err != nil {
			t.Fatal(err)
		}
		settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
			return a.SaveCommandBinding(CommandBindingEdit{ID: binding.ID, LayerID: layerID, CommandID: commandWorkspaceTabChatCreateID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyD","modifiers":["Control"]}`, Enabled: false})
		})
		if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
			t.Fatal("Take aceitou mutação de configuração anterior")
		}
	})

	t.Run("session revoked before take", func(t *testing.T) {
		a, _, _, _, view, binding := commandKeyboardDurableFixture(t, commandWorkspaceTabCloseID)
		reservation, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, false)
		if err != nil {
			t.Fatal(err)
		}
		principal := a.commandProduct.Load().principal
		if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", principal.SessionID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
			t.Fatal("Take aceitou sessão revogada")
		}
	})
}
