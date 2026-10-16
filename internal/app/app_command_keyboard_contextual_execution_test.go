package app

import (
	"encoding/json"
	"testing"

	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

const (
	contextualKeyboardV1Spec = `{"version":1,"code":"KeyZ","modifiers":["Control","Shift"]}`
	contextualKeyboardV2Spec = `{"version":2,"steps":[{"code":"KeyZ","modifiers":["Control","Shift"]},{"code":"KeyL","modifiers":[]}]}`
)

func contextualKeyboardChatFixture(t *testing.T) (*App, <-chan map[string]any) {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	tabID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: workspace.TabTypeChat}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		t.Fatal(err)
	}
	return a, decisions
}

func contextualKeyboardInstall(t *testing.T, a *App, decisions <-chan map[string]any, commandID string, triggerSpecs ...string) string {
	t.Helper()
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:    CommandSettingsScopeGlobal,
		Operation: "layer_create",
		Layer: &CommandSettingsLayerInput{Name: "Atalhos contextuais de chat", Enabled: true},
	})
	for _, triggerSpec := range triggerSpecs {
		settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
			Scope:    CommandSettingsScopeGlobal,
			Operation: "binding_create",
			Binding: &CommandSettingsBindingInput{
				LayerID:     layer.ID,
				CommandID:   commandID,
				TriggerType: "keyboard.local",
				TriggerSpec: triggerSpec,
				Effect:      "execute",
				Enabled:     true,
			},
		})
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:    CommandSettingsScopeGlobal,
		Operation: "rule_create",
		Rule: &CommandSettingsRuleInput{
			LayerID:    layer.ID,
			Mode:       "condition",
			Lifecycle:  "persistent",
			Enabled:    true,
			Condition:  &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Op: "eq", Value: "chat"}}},
		},
	})
	return layer.ID
}

func contextualKeyboardContext(t *testing.T, a *App) LocalCommandKeyboardContext {
	t.Helper()
	snapshot, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Tab.ID == "" || snapshot.Tab.Type == "" {
		t.Fatalf("snapshot canônico sem aba ativa: %+v", snapshot)
	}
	return LocalCommandKeyboardContext{SurfaceID: snapshot.Tab.ID, SurfaceType: string(snapshot.Tab.Type)}
}

func TestCommandKeyboardContextualCanonicalSurfaceTypeMatrix(t *testing.T) {
	a, _ := contextualKeyboardChatFixture(t)
	for _, tabType := range []workspace.TabType{workspace.TabTypeChat, workspace.TabTypeEditor, workspace.TabTypeTerminal, workspace.TabTypeTasklist} {
		t.Run(string(tabType), func(t *testing.T) {
			tabID := uuid.NewString()
			if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: tabType}); err != nil {
				t.Fatal(err)
			}
			if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
				t.Fatal(err)
			}
			observed := contextualKeyboardContext(t, a)
			proof, err := a.captureLocalKeyboardContext(observed)
			if err != nil || proof == nil || proof.snapshot.Tab.ID != tabID || string(proof.snapshot.Tab.Type) != string(tabType) {
				t.Fatalf("surface canônica não capturada: observed=%+v proof=%+v err=%v", observed, proof, err)
			}
		})
	}
}

func contextualKeyboardEntryFor(t *testing.T, view LocalCommandKeyboardMap, shortcut LocalCommandShortcut) LocalCommandKeyboardContextualBinding {
	t.Helper()
	for _, entry := range view.ContextualBindings {
		if entry.Shortcut.Version != shortcut.Version || entry.Shortcut.Code != shortcut.Code || len(entry.Shortcut.Modifiers) != len(shortcut.Modifiers) || len(entry.Shortcut.Steps) != len(shortcut.Steps) {
			continue
		}
		match := true
		for i := range shortcut.Modifiers {
			if entry.Shortcut.Modifiers[i] != shortcut.Modifiers[i] {
				match = false
				break
			}
		}
		for i := range shortcut.Steps {
			if entry.Shortcut.Steps[i].Code != shortcut.Steps[i].Code || len(entry.Shortcut.Steps[i].Modifiers) != len(shortcut.Steps[i].Modifiers) {
				match = false
				break
			}
			for j := range shortcut.Steps[i].Modifiers {
				if entry.Shortcut.Steps[i].Modifiers[j] != shortcut.Steps[i].Modifiers[j] {
					match = false
				}
			}
		}
		if match {
			return entry
		}
	}
	t.Fatalf("atalho contextual não publicado: %+v", shortcut)
	return LocalCommandKeyboardContextualBinding{}
}

func contextualKeyboardSurfaceBinding(t *testing.T, entry LocalCommandKeyboardContextualBinding, surface string) LocalCommandKeyboardBinding {
	t.Helper()
	binding, ok := entry.BySurface[surface]
	if !ok || binding == nil {
		t.Fatalf("superfície %q não publicou binding: %+v", surface, entry)
	}
	return *binding
}

func contextualKeyboardInvocationCount(t *testing.T, commandID string) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Table("command_invocations").Where("command_id = ? AND source_type = ?", commandID, "keyboard.local").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCommandKeyboardContextualBackendResolvesCanonicalChatAndSequenceV2(t *testing.T) {
	a, decisions := contextualKeyboardChatFixture(t)
	contextualKeyboardInstall(t, a, decisions, commandProductWorkspaceListID, contextualKeyboardV1Spec, contextualKeyboardV2Spec)

	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	v1 := contextualKeyboardEntryFor(t, view, LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}})
	v2 := contextualKeyboardEntryFor(t, view, LocalCommandShortcut{Version: 2, Steps: []LocalCommandShortcutStep{{Code: "KeyZ", Modifiers: []string{"Control", "Shift"}}, {Code: "KeyL", Modifiers: []string{}}}})
	if v2.Shortcut.Modifiers != nil {
		t.Fatalf("clone do v2 materializou modifiers nulo como slice vazia: %+v", v2.Shortcut)
	}
	if _, err := json.Marshal(v2.Shortcut); err != nil {
		t.Fatalf("round-trip do v2 publicado falhou: %v", err)
	}
	if got := contextualKeyboardSurfaceBinding(t, v1, "chat"); got.CommandID != commandProductWorkspaceListID || got.Handler != "backend" {
		t.Fatalf("binding v1 contextual inesperado: %+v", got)
	}
	if got := contextualKeyboardSurfaceBinding(t, v2, "chat"); got.CommandID != commandProductWorkspaceListID || got.Handler != "backend" || got.Shortcut.Version != 2 {
		t.Fatalf("sequência v2 contextual inesperada: %+v", got)
	}

	chatContext := contextualKeyboardContext(t, a)
	before := contextualKeyboardInvocationCount(t, commandProductWorkspaceListID)
	first, err := a.DispatchContextualLocalCommandKey(view.Generation, v1.Shortcut, "down", false, chatContext)
	if err != nil || first == nil || first.Status != string(commandledger.Succeeded) {
		t.Fatalf("workspace.list contextual não executou: result=%+v err=%v", first, err)
	}
	afterFirst := contextualKeyboardInvocationCount(t, commandProductWorkspaceListID)
	if afterFirst != before+1 {
		t.Fatalf("primeira ocorrência criou %d invocações, antes=%d depois=%d", afterFirst-before, before, afterFirst)
	}
	duplicate, duplicateErr := a.DispatchContextualLocalCommandKey(view.Generation, v1.Shortcut, "down", false, chatContext)
	afterDuplicate := contextualKeyboardInvocationCount(t, commandProductWorkspaceListID)
	if duplicate != nil || afterDuplicate != afterFirst {
		t.Fatalf("keydown duplicado reexecutou: result=%+v err=%v count=%d/%d", duplicate, duplicateErr, afterDuplicate, afterFirst)
	}
	if _, err := a.DispatchContextualLocalCommandKey(view.Generation, v1.Shortcut, "up", false, chatContext); err != nil {
		t.Fatalf("keyup contextual: %v", err)
	}
	sequence, err := a.DispatchContextualLocalCommandKey(view.Generation, v2.Shortcut, "down", false, chatContext)
	if err != nil || sequence == nil || sequence.Status != string(commandledger.Succeeded) {
		t.Fatalf("workspace.list sequência v2 não executou: result=%+v err=%v", sequence, err)
	}
	if _, err := a.DispatchContextualLocalCommandKey(view.Generation, v2.Shortcut, "up", false, chatContext); err != nil {
		t.Fatalf("keyup sequência v2: %v", err)
	}

	forged := chatContext
	forged.SurfaceType = string(workspace.TabTypeEditor)
	if result, err := a.DispatchContextualLocalCommandKey(view.Generation, v1.Shortcut, "down", false, forged); result != nil || err == nil {
		t.Fatalf("contexto surface forjado foi aceito: result=%+v err=%v", result, err)
	}
	if got := contextualKeyboardInvocationCount(t, commandProductWorkspaceListID); got != afterFirst+1 {
		t.Fatalf("contexto forjado criou invocação: %d", got)
	}

	editorID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: editorID, Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(editorID); err != nil {
		t.Fatal(err)
	}
	editorContext := contextualKeyboardContext(t, a)
	if result, err := a.DispatchContextualLocalCommandKey(view.Generation, v1.Shortcut, "down", false, editorContext); result != nil || err == nil {
		t.Fatalf("binding chat executou fora do chat: result=%+v err=%v", result, err)
	}
	if got := contextualKeyboardInvocationCount(t, commandProductWorkspaceListID); got != afterFirst+1 {
		t.Fatalf("contexto editor criou invocação: %d", got)
	}

	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "config_restore"})
	restored, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range restored.ContextualBindings {
		if entry.Shortcut.Code == "KeyZ" {
			t.Fatalf("restore manteve personalização contextual: %+v", entry)
		}
	}
}

func TestCommandKeyboardContextualWorkspaceMutationUsesUIHandoffAndRejectsReplay(t *testing.T) {
	a, decisions := contextualKeyboardChatFixture(t)
	contextualKeyboardInstall(t, a, decisions, commandWorkspaceTabChatCreateID, contextualKeyboardV1Spec)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}}
	entry := contextualKeyboardEntryFor(t, view, shortcut)
	binding := contextualKeyboardSurfaceBinding(t, entry, "chat")
	if binding.CommandID != commandWorkspaceTabChatCreateID || binding.Handler != "contextual" {
		t.Fatalf("mutação contextual não foi publicada como UI: %+v", binding)
	}

	chatContext := contextualKeyboardContext(t, a)
	beforeTabs := len(a.workspaceMgr.Active().Tabs.Items)
	reservation, err := a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, chatContext)
	if err != nil || reservation == nil || reservation.CommandID != commandWorkspaceTabChatCreateID {
		t.Fatalf("Begin contextual UI inesperado: reservation=%+v err=%v", reservation, err)
	}
	duplicate, duplicateErr := a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, chatContext)
	if duplicate != nil {
		t.Fatalf("segunda reserva contextual criada: %+v err=%v", duplicate, duplicateErr)
	}
	handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceTabChatCreateID)
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("commit UI contextual: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != string(commandledger.Succeeded) || len(a.workspaceMgr.Active().Tabs.Items) != beforeTabs+1 {
		t.Fatalf("mutação contextual não aplicada uma vez: result=%+v tabs=%d/%d", result, len(a.workspaceMgr.Active().Tabs.Items), beforeTabs)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("replay do commit UI contextual aceito")
	}
	if len(a.workspaceMgr.Active().Tabs.Items) != beforeTabs+1 {
		t.Fatal("replay alterou novamente as abas")
	}

	forged := chatContext
	forged.SurfaceID = uuid.NewString()
	if result, err := a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, forged); result != nil || err == nil {
		t.Fatalf("surface ID forjado aceito: reservation=%+v err=%v", result, err)
	}

	if err := a.workspaceMgr.SetActiveTab(chatContext.SurfaceID); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(a.workspaceMgr.Active().Tabs.Items[len(a.workspaceMgr.Active().Tabs.Items)-1].ID); err != nil {
		t.Fatal(err)
	}
	editorContext := contextualKeyboardContext(t, a)
	if result, err := a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, editorContext); result != nil || err == nil {
		t.Fatalf("mutação contextual aceita no editor: reservation=%+v err=%v", result, err)
	}

	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "config_restore"})
	restored, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range restored.ContextualBindings {
		if entry.Shortcut.Code == "KeyZ" {
			t.Fatalf("restore manteve mutação UI contextual: %+v", entry)
		}
	}
}

func TestCommandKeyboardContextualUIRejectsSnapshotChangedBeforeTake(t *testing.T) {
	a, decisions := contextualKeyboardChatFixture(t)
	contextualKeyboardInstall(t, a, decisions, commandWorkspaceTabChatCreateID, contextualKeyboardV1Spec)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyZ", Modifiers: []string{"Control", "Shift"}}
	chatContext := contextualKeyboardContext(t, a)
	reservation, err := a.BeginContextualLocalCommandUIKey(view.Generation, shortcut, false, chatContext)
	if err != nil || reservation == nil {
		t.Fatalf("reserva contextual inicial inesperada: reservation=%+v err=%v", reservation, err)
	}

	beforeTabs := len(a.workspaceMgr.Active().Tabs.Items)
	editorID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: editorID, Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(editorID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("TakeUICommand aceitou ocorrência após mudança do snapshot canônico")
	}
	if len(a.workspaceMgr.Active().Tabs.Items) != beforeTabs+1 {
		t.Fatalf("snapshot alterado produziu efeito inesperado: tabs=%d", len(a.workspaceMgr.Active().Tabs.Items))
	}

	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "config_restore"})
}
