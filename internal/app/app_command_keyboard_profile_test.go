package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandledger"
)

func TestContextualFallbackProfileNaoColideComSlugListado(t *testing.T) {
	values := []string{"dev", "__keyboard_local_profile_fallback__", "__keyboard_local_profile_fallback___1"}
	got := contextualFallbackProfile(values)
	if got == "" {
		t.Fatal("sentinela vazia")
	}
	for _, value := range values {
		if got == value {
			t.Fatalf("sentinela colidiu com perfil listado: %q", got)
		}
	}
}

func TestContextualFallbackProfileRepresentaPerfilNaoListado(t *testing.T) {
	values := []string{"a", "b"}
	if got := contextualFallbackProfile(values); got == "a" || got == "b" {
		t.Fatalf("raiz não representa fallback: %q", got)
	}
}

func TestContextualProfileSurfaceProjetaSomenteTiposMencionadosERaizFallback(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	trigger := "keyboard.local:Alt+KeyI"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyI", Modifiers: []string{"Alt"}}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		{ID: "fallback", Trigger: trigger, CommandID: "navigation.data.import.open", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true},
		{ID: "profile-chat", Trigger: trigger, CommandID: "editor.menu.insert.open", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Surface, Condition: commandbindings.Facts{commandbindings.Profile: "dev", commandbindings.SurfaceType: "chat"}, Enabled: true, LayerActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := contextualKeyboardBinding(context.Background(), configuration, a.commandProduct.Load().registry, trigger, shortcut)
	if !ok || len(entry.BySurface) != 1 || entry.BySurface["chat"] == nil || entry.BySurface["editor"] != nil {
		t.Fatalf("raiz não respeitou subset de surface: %+v", entry)
	}
	leaf := entry.ByProfile["dev"]
	if leaf == nil || len(leaf.BySurface) != 1 || leaf.BySurface["chat"] == nil || leaf.BySurface["editor"] != nil {
		t.Fatalf("folha profile+surface inesperada: %+v", entry.ByProfile)
	}
	if entry.BySurface["chat"].CommandID != "navigation.data.import.open" || leaf.BySurface["chat"].CommandID != "editor.menu.insert.open" {
		t.Fatalf("raiz não é fallback ou folha não resolveu perfil: raiz=%+v folha=%+v", entry.BySurface["chat"], leaf.BySurface["chat"])
	}
}

func TestContextualProfileOnlyProjetaAsQuatroSuperficiesWorkspace(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	trigger := "keyboard.local:Alt+KeyI"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyI", Modifiers: []string{"Alt"}}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "profile-only", Trigger: trigger, CommandID: "navigation.data.import.open", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Application,
		Condition: commandbindings.Facts{commandbindings.Profile: "dev"}, Enabled: true, LayerActive: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := contextualKeyboardBinding(context.Background(), configuration, a.commandProduct.Load().registry, trigger, shortcut)
	if !ok || entry.ByProfile["dev"] == nil {
		t.Fatalf("profile-only não foi projetado: %+v", entry)
	}
	leaf := entry.ByProfile["dev"]
	for _, surface := range []string{"chat", "editor", "terminal", "tasklist"} {
		if leaf.BySurface[surface] == nil || leaf.BySurface[surface].CommandID != "navigation.data.import.open" {
			t.Fatalf("superfície workspace ausente em profile-only: %s: %+v", surface, leaf.BySurface)
		}
	}
}

func TestCommandKeyboardProfileAPIRrealBindingRegraMapaEExecucaoBackend(t *testing.T) {
	a, decisions := contextualKeyboardChatFixture(t)
	if err := a.workspaceMgr.SetProfile("workspace-profile"); err != nil {
		t.Fatal(err)
	}
	observed := contextualKeyboardContext(t, a)
	if err := a.workspaceMgr.UpdateTab(observed.SurfaceID, map[string]any{"profile_override": map[string]any{"slug": "profile-a"}}); err != nil {
		t.Fatal(err)
	}
	observed.Profile = "profile-a"
	proof, err := a.captureLocalKeyboardContext(observed)
	if err != nil || proof == nil || proof.snapshot.WorkspaceProfile != "workspace-profile" || proof.snapshot.Tab.ProfileOverrideSlug != "profile-a" || localKeyboardEffectiveProfile(proof.snapshot) != "profile-a" {
		t.Fatalf("override canônico não venceu perfil do workspace: proof=%+v err=%v", proof, err)
	}
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Perfil teclado", Enabled: true}})
	condition := &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "profile", Op: "eq", Value: "profile-a"}, {Field: "surface.type", Op: "eq", Value: observed.SurfaceType}}}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandProductWorkspaceListID, TriggerType: "keyboard.local", TriggerSpec: contextualKeyboardV1Spec, Effect: "execute", Enabled: true, Condition: condition}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	entry := contextualKeyboardEntryFor(t, view, LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}})
	_, fallbackSurface := entry.BySurface["chat"]
	if entry.ByProfile["profile-a"] == nil || entry.ByProfile["profile-a"].BySurface["chat"] == nil || !fallbackSurface {
		t.Fatalf("binding profile não publicado pela API real: raiz=%+v folha=%+v", entry, entry.ByProfile["profile-a"])
	}
	result, err := a.DispatchContextualLocalCommandKey(view.Generation, entry.Shortcut, "down", false, observed)
	if err != nil || result == nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("execução backend com perfil canônico falhou: %+v %v", result, err)
	}
	if _, err := a.DispatchContextualLocalCommandKey(view.Generation, entry.Shortcut, "up", false, observed); err != nil {
		t.Fatal(err)
	}
	for name, forged := range map[string]LocalCommandKeyboardContext{
		"forjado":                {SurfaceID: observed.SurfaceID, SurfaceType: observed.SurfaceType, Profile: "profile-b"},
		"workspace-sem-override": {SurfaceID: observed.SurfaceID, SurfaceType: observed.SurfaceType, Profile: "workspace-profile"},
		"ausente":                {SurfaceID: observed.SurfaceID, SurfaceType: observed.SurfaceType},
	} {
		t.Run(name, func(t *testing.T) {
			if result, err := a.DispatchContextualLocalCommandKey(view.Generation, entry.Shortcut, "down", false, forged); result != nil || err == nil {
				t.Fatalf("prova de perfil %s aceita: %+v %v", name, result, err)
			}
		})
	}
}

func TestCommandKeyboardProfileBeginCommitAndABAAfterReservation(t *testing.T) {
	a, decisions := contextualKeyboardChatFixture(t)
	if err := a.workspaceMgr.SetProfile("profile-a"); err != nil {
		t.Fatal(err)
	}
	observed := contextualKeyboardContext(t, a)
	observed.Profile = "profile-a"
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Perfil UI", Enabled: true}})
	condition := &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "profile", Op: "eq", Value: "profile-a"}, {Field: "surface.type", Op: "eq", Value: observed.SurfaceType}}}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandWorkspaceTabChatCreateID, TriggerType: "keyboard.local", TriggerSpec: contextualKeyboardV1Spec, Effect: "execute", Enabled: true, Condition: condition}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}}
	reservation, err := a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, observed)
	if err != nil || reservation == nil {
		t.Fatalf("Begin contextual profile recusado: %+v %v", reservation, err)
	}
	handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceTabChatCreateID)
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != string(commandledger.Succeeded) {
		t.Fatalf("Commit contextual profile falhou: %+v", result)
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
		t.Fatalf("segunda reserva profile recusada: %+v %v", reservation, err)
	}
	if err := a.workspaceMgr.SetProfile("profile-b"); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("profile-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("A-B-A revalidou reserva contextual de perfil")
	}
}
