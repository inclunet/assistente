package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func commandKeyboardBindingFor(t *testing.T, view LocalCommandKeyboardMap, shortcut LocalCommandShortcut) LocalCommandKeyboardBinding {
	t.Helper()
	for _, binding := range view.Bindings {
		if reflect.DeepEqual(binding.Shortcut, shortcut) {
			return binding
		}
	}
	t.Fatalf("atalho não publicado: %+v em %+v", shortcut, view)
	return LocalCommandKeyboardBinding{}
}

func TestCommandKeyboardInactiveDoesNotPublishAndActiveDoes(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}}

	inactive, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(inactive.Bindings) != 62 {
		t.Fatalf("defaults de teclado não publicados: %+v", inactive)
	}
	if inactive.OwnerID != a.commandProduct.Load().principal.UserID || inactive.SessionID != a.commandProduct.Load().principal.SessionID || inactive.WorkspaceID != a.commandProduct.Load().workspaceID {
		t.Fatalf("identidade do mapa local incorreta: %+v", inactive)
	}

	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Teclado local", Enabled: true})
	})
	binding := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer.ID, CommandID: commandProductWorkspaceListID,
			TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`,
			Enabled:     true,
		})
	})
	if binding.ID == "" {
		t.Fatal("binding não recebeu ID")
	}
	for _, spec := range []string{
		`{"version":1,"code":"KeyP","modifiers":[]}`,
		`{"version":1,"code":"KeyS","modifiers":["Shift"]}`,
	} {
		settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
			return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer.ID, CommandID: commandProductWorkspaceListID,
				TriggerType: "keyboard.local", TriggerSpec: spec, Enabled: true})
		})
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayer(layer.ID)
	})

	prepared, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Bindings) != 62 {
		t.Fatalf("Prepare alterou defaults builtin: %+v", prepared)
	}
	if _, err := a.SetCommandLayerActive(layer.ID, true); err != nil {
		t.Fatal(err)
	}
	active, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	custom := commandKeyboardBindingFor(t, active, shortcut)
	if len(active.Bindings) != 63 || custom.CommandID != commandProductWorkspaceListID || custom.Handler != "backend" {
		t.Fatalf("teclado ativo não publicou KeyK: %+v", active)
	}
	settings, err := a.GetCommandSettings("pt-BR")
	if err != nil || !settings.KeyboardOperational {
		t.Fatalf("KeyboardOperational não confirmou teclado ativo: %+v err=%v", settings, err)
	}
}

func TestCommandKeyboardDispatchesOnceWithProvenanceAndReleasesOnKeyup(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	binding := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer, CommandID: commandProductWorkspaceListID,
			TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`,
			Enabled:     true,
		})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	mapView, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(mapView.Bindings) != 63 {
		t.Fatalf("mapa ativo inesperado: %+v", mapView)
	}
	custom := commandKeyboardBindingFor(t, mapView, LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}})

	first, err := a.DispatchLocalCommandKey(mapView.Generation, custom.Shortcut, "down", false)
	if err != nil || first == nil || first.Status != string(commandledger.Succeeded) {
		t.Fatalf("keydown não executou workspace.list: %+v err=%v", first, err)
	}
	for _, repeat := range []bool{true, false} {
		result, dispatchErr := a.DispatchLocalCommandKey(mapView.Generation, custom.Shortcut, "down", repeat)
		if repeat {
			if !errors.Is(dispatchErr, commandexecution.ErrDenied) || result != nil {
				t.Fatalf("repeat alcançou host ou executou novamente: result=%+v err=%v", result, dispatchErr)
			}
		} else if dispatchErr != nil || result != nil {
			t.Fatalf("down repetido executou novamente: result=%+v err=%v", result, dispatchErr)
		}
	}
	if result, err := a.DispatchLocalCommandKey(mapView.Generation, custom.Shortcut, "up", false); err != nil || result != nil {
		t.Fatalf("keyup não liberou: result=%+v err=%v", result, err)
	}
	second, err := a.DispatchLocalCommandKey(mapView.Generation, custom.Shortcut, "down", false)
	if err != nil || second == nil || second.Status != string(commandledger.Succeeded) {
		t.Fatalf("keydown após keyup não executou: %+v err=%v", second, err)
	}

	var rows []struct {
		InvocationID        string
		SourceType          *string
		ObserverType        *string
		SourceInstanceID    *string
		SourceEventID       *string
		ObservedTriggerType *string
		BindingIDs          string
	}
	if err := database.DB().Table("command_invocations").Select("invocation_id, source_type, observer_type, source_instance_id, source_event_id, observed_trigger_type, binding_ids").Where("invocation_id IN ?", []string{first.InvocationID, second.InvocationID}).Order("invocation_id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("invocações persistidas = %d, queria 2: %+v", len(rows), rows)
	}
	for _, row := range rows {
		if row.SourceType == nil || *row.SourceType != "keyboard.local" || row.ObserverType == nil || *row.ObserverType != "keyboard.local" || row.SourceInstanceID == nil || *row.SourceInstanceID != mapView.Generation || row.SourceEventID == nil || *row.SourceEventID != row.InvocationID || row.ObservedTriggerType == nil || *row.ObservedTriggerType != "keyboard.local" {
			t.Fatalf("proveniência de teclado incompleta: %+v", row)
		}
		var ids []string
		if err := json.Unmarshal([]byte(row.BindingIDs), &ids); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(ids, binding.ID) {
			t.Fatalf("binding não auditado: %+v", ids)
		}
	}
}

func TestCommandKeyboardGenerationCopyResetConfigurationAndAuthGuards(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	binding := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer, CommandID: commandProductWorkspaceListID,
			TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`,
			Enabled:     true,
		})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	first, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(first.Bindings) != 63 {
		t.Fatalf("mapa inicial: %+v err=%v", first, err)
	}
	second, err := a.GetLocalCommandKeyboardMap()
	if err != nil || second.Generation != first.Generation || !reflect.DeepEqual(second, first) {
		t.Fatalf("releitura não foi estável: first=%+v second=%+v err=%v", first, second, err)
	}
	custom := commandKeyboardBindingFor(t, second, LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}})
	for i := range second.Bindings {
		if reflect.DeepEqual(second.Bindings[i].Shortcut, custom.Shortcut) {
			second.Bindings[i].Shortcut.Modifiers[0] = "Meta"
		}
	}
	second.Bindings = append(second.Bindings, LocalCommandKeyboardBinding{Shortcut: LocalCommandShortcut{Version: 1, Code: "KeyX", Modifiers: []string{"Control"}}, CommandID: commandProductWorkspaceListID, Handler: "backend"})
	third, err := a.GetLocalCommandKeyboardMap()
	if err != nil || third.Generation != first.Generation || !reflect.DeepEqual(third, first) {
		t.Fatalf("retorno não é cópia profunda: first=%+v third=%+v err=%v", first, third, err)
	}

	a.ResetLocalCommandKeyboard(first.Generation)
	if result, err := a.DispatchLocalCommandKey(first.Generation, custom.Shortcut, "down", false); !errors.Is(err, commandexecution.ErrStale) || result != nil {
		t.Fatalf("mapa resetado não ficou stale: result=%+v err=%v", result, err)
	}
	old, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(old.Bindings) != 63 {
		t.Fatalf("mapa após reset: %+v err=%v", old, err)
	}

	a.setCurrentUserID("foreign-user")
	if result, err := a.DispatchLocalCommandKey(old.Generation, custom.Shortcut, "down", false); err == nil || result != nil {
		t.Fatalf("autenticação foreign aceitou teclado: result=%+v err=%v", result, err)
	}
	a.setCurrentUserID(a.commandProduct.Load().principal.UserID)

	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{ID: binding.ID, LayerID: layer, CommandID: commandProductWorkspaceListID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`, Enabled: false})
	})
	changed, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if changed.Generation == old.Generation || len(changed.Bindings) != 62 {
		t.Fatalf("mudança de configuração não publicou novo mapa: old=%+v changed=%+v", old, changed)
	}
	if result, err := a.DispatchLocalCommandKey(old.Generation, custom.Shortcut, "down", false); !errors.Is(err, commandexecution.ErrStale) || result != nil {
		t.Fatalf("geração antiga executou após mudança: result=%+v err=%v", result, err)
	}
	// A fixture não monta os serviços de autenticação de UI; Logout ainda
	// executa sua limpeza de comandos antes de retornar esse erro de fixture.
	_ = a.Logout(LogoutRequest{})
	if result, err := a.DispatchLocalCommandKey(changed.Generation, LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}}, "down", false); err == nil || result != nil {
		t.Fatalf("logout não recusou teclado: result=%+v err=%v", result, err)
	}
}

func TestCommandKeyboardRejectsUnsupportedSourceAndUnmappedCombination(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	shortcutSpec := `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandProductWorkspaceListID,
			TriggerType: "keyboard.local", TriggerSpec: shortcutSpec, Enabled: true})
	})
	unsupported, err := a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: "navigation.not-registered",
		TriggerType: "keyboard.local", TriggerSpec: shortcutSpec, Enabled: true})
	if err == nil || unsupported.Committed || unsupported.Published {
		t.Fatalf("SaveCommandBinding aceitou teclado em comando somente de paleta: result=%+v err=%v", unsupported, err)
	}
	select {
	case decision := <-decisions:
		t.Fatalf("origem não admitida chegou à confirmação: %+v", decision)
	default:
	}
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Bindings) != 63 || len(view.ContextualBindings) != 2 {
		t.Fatalf("mapa ativo inesperado: bindings=%d contextual=%d view=%+v", len(view.Bindings), len(view.ContextualBindings), view)
	}
	for _, shortcut := range []LocalCommandShortcut{
		{Version: 1, Code: "KeyP", Modifiers: nil},
		{Version: 1, Code: "KeyS", Modifiers: []string{"Shift"}},
		{Version: 1, Code: "KeyX", Modifiers: []string{"Control"}},
	} {
		result, dispatchErr := a.DispatchLocalCommandKey(view.Generation, shortcut, "down", false)
		if dispatchErr == nil || result != nil {
			t.Fatalf("combinação não mapeada despachou: shortcut=%+v result=%+v err=%v", shortcut, result, dispatchErr)
		}
	}
	var count int64
	if err := database.DB().Table("command_invocations").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("combinação plain/Shift/não mapeada criou invocações: %d", count)
	}
}

func TestCommandKeyboardRevokedSessionCannotDispatch(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandProductWorkspaceListID,
			TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(view.Bindings) != 63 || len(view.ContextualBindings) != 2 {
		t.Fatalf("mapa antes da revogação: %+v err=%v", view, err)
	}
	principal := a.commandProduct.Load().principal
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	result, dispatchErr := a.DispatchLocalCommandKey(view.Generation, LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}}, "down", false)
	if dispatchErr == nil || (result != nil && result.InvocationID != "") {
		t.Fatalf("sessão real revogada executou teclado: result=%+v err=%v", result, dispatchErr)
	}
	var count int64
	if err := database.DB().Table("command_invocations").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("revogação real deixou invocação persistida: %d", count)
	}
}
