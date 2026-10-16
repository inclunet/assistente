package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
	"assistente/internal/workspace"
)

func TestCommandWorkspaceChatProjectionRejectsOtherMutableTargets(t *testing.T) {
	a := readyCommandProduct(t)
	projection, err := commandProductProjection(a.commandProduct.Load().registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := commandWorkspaceTabCreateRegistration(commandWorkspaceTabChatCreateID)
	if scope, err := projection.ExecutionScope(definition, nil, commandconfig.Scope{}); err != nil || scope != "global" {
		t.Fatalf("chat contextual: scope=%q err=%v", scope, err)
	}
	definition.ID = "workspace.tab.unsupported.create"
	if _, err := projection.ExecutionScope(definition, nil, commandconfig.Scope{}); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("outro alvo mutável deve ser recusado: %v", err)
	}
}

func commandWorkspaceChatTabCount(t *testing.T, a *App) int {
	t.Helper()
	active := a.workspaceMgr.Active()
	if active == nil {
		t.Fatal("workspace ativo ausente")
	}
	return len(active.Tabs.Items)
}

func commandWorkspaceChatAddedEvents(t *testing.T, emitter *testEmitter) []capturedEvent {
	t.Helper()
	return emitter.find("workspace:tab_added")
}

func beginWorkspaceChatCommand(t *testing.T, a *App) (commandui.Reservation, commandui.Handoff) {
	t.Helper()
	reservation := beginUICommand(t, a, commandWorkspaceTabChatCreateID)
	handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceTabChatCreateID)
	return reservation, handoff
}

func beginWorkspaceTabCloseCommand(t *testing.T, a *App) (commandui.Reservation, commandui.Handoff) {
	t.Helper()
	reservation := beginUICommand(t, a, commandWorkspaceTabCloseID)
	handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceTabCloseID)
	return reservation, handoff
}

func TestCommandWorkspaceTabCloseCommitReplacesLastTabAndEmiteSnapshotRemoved(t *testing.T) {
	a := readyCommandProduct(t)
	emitter := wireCommandWorkspaceForTest(a)
	activeBefore := a.workspaceMgr.Active().Tabs.Active
	reservation, handoff := beginWorkspaceTabCloseCommand(t, a)
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("CommitWorkspaceTabCommand close: %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
		t.Fatalf("resultado close: %+v", result)
	}
	active := a.workspaceMgr.Active()
	if len(active.Tabs.Items) != 1 || active.Tabs.Active == activeBefore {
		t.Fatalf("última aba não foi substituída atomicamente: antes=%q depois=%q abas=%d", activeBefore, active.Tabs.Active, len(active.Tabs.Items))
	}
	if tab := active.FindTab(active.Tabs.Active); tab == nil || tab.Type != workspace.TabTypeChat || tab.ConversationID != "" || tab.State != nil {
		t.Fatalf("substituta não é chat: %#v", tab)
	}
	events := emitter.find("workspace:tab_removed")
	if len(events) != 1 {
		t.Fatalf("eventos workspace:tab_removed=%d, esperado 1", len(events))
	}
	if len(emitter.find("workspace:tab_added")) != 0 {
		t.Fatal("close publicou workspace:tab_added")
	}
}

func TestCommandWorkspaceTabCloseGuardsCancelReplayForgedAndStale(t *testing.T) {
	t.Run("complete falso e cancelamento", func(t *testing.T) {
		a := readyCommandProduct(t)
		before := a.workspaceMgr.Active()
		reservation, handoff := beginWorkspaceTabCloseCommand(t, a)
		if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "succeeded"); !errors.Is(err, commandui.ErrCommitRequired) {
			t.Fatalf("Complete forjado=%v", err)
		}
		if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "cancelled"); err != nil {
			t.Fatalf("cancelamento após Take=%v", err)
		}
		after := a.workspaceMgr.Active()
		if after.Tabs.Active != before.Tabs.Active || len(after.Tabs.Items) != len(before.Tabs.Items) {
			t.Fatalf("cancelamento teve efeito: antes=%q/%d depois=%q/%d", before.Tabs.Active, len(before.Tabs.Items), after.Tabs.Active, len(after.Tabs.Items))
		}
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
			t.Fatal("commit após cancelamento aceito")
		}
		after = a.workspaceMgr.Active()
		if after.Tabs.Active != before.Tabs.Active || len(after.Tabs.Items) != len(before.Tabs.Items) {
			t.Fatal("commit após cancelamento alterou workspace")
		}
	})
	t.Run("handoff forjado e replay", func(t *testing.T) {
		a := readyCommandProduct(t)
		before := a.workspaceMgr.Active()
		reservation, handoff := beginWorkspaceTabCloseCommand(t, a)
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID+"-forged"); err == nil {
			t.Fatal("handoff close forjado aceito")
		}
		after := a.workspaceMgr.Active()
		if after.Tabs.Active != before.Tabs.Active || len(after.Tabs.Items) != len(before.Tabs.Items) {
			t.Fatal("handoff forjado alterou workspace")
		}
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
			t.Fatal(err)
		}
		committed := a.workspaceMgr.Active()
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
			t.Fatal("replay close aceito")
		}
		after = a.workspaceMgr.Active()
		if after.Tabs.Active != committed.Tabs.Active || len(after.Tabs.Items) != len(committed.Tabs.Items) {
			t.Fatalf("replay alterou estado: antes=%q/%d depois=%q/%d", committed.Tabs.Active, len(committed.Tabs.Items), after.Tabs.Active, len(after.Tabs.Items))
		}
	})
	t.Run("snapshot stale", func(t *testing.T) {
		a := readyCommandProduct(t)
		reservation, handoff := beginWorkspaceTabCloseCommand(t, a)
		if err := a.workspaceMgr.AddTab(workspace.Tab{ID: "stale-editor", Type: workspace.TabTypeEditor, Title: "Stale"}); err != nil {
			t.Fatal(err)
		}
		if err := a.workspaceMgr.SetActiveTab("stale-editor"); err != nil {
			t.Fatal(err)
		}
		expected := a.workspaceMgr.Active()
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
			t.Fatal("close aceitou target stale")
		}
		after := a.workspaceMgr.Active()
		if after.Tabs.Active != expected.Tabs.Active || len(after.Tabs.Items) != len(expected.Tabs.Items) {
			t.Fatal("close stale teve efeito")
		}
	})
}

func TestCommandWorkspaceTabCreateCommitDosTresTiposRegistrados(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		tabType workspace.TabType
	}{
		{name: "chat", command: commandWorkspaceTabChatCreateID, tabType: workspace.TabTypeChat},
		{name: "editor", command: commandWorkspaceTabEditorCreateID, tabType: workspace.TabTypeEditor},
		{name: "tasklist", command: commandWorkspaceTabTasklistCreateID, tabType: workspace.TabTypeTasklist},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := readyCommandProduct(t)
			before := commandWorkspaceChatTabCount(t, a)
			reservation := beginUICommand(t, a, tc.command)
			handoff := takeUICommandFor(t, a, reservation.Ticket, tc.command)
			if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
				t.Fatalf("CommitWorkspaceTabCommand: %v", err)
			}
			result := getUIResultEventually(t, a, reservation.Ticket)
			if result.Status != "succeeded" {
				t.Fatalf("resultado: %+v", result)
			}
			active := a.workspaceMgr.Active()
			if len(active.Tabs.Items) != before+1 {
				t.Fatalf("abas=%d, esperado=%d", len(active.Tabs.Items), before+1)
			}
			created := active.FindTab(active.Tabs.Active)
			if created == nil || created.Type != tc.tabType {
				t.Fatalf("aba criada=%#v, esperado tipo=%q", created, tc.tabType)
			}
			if tc.tabType != workspace.TabTypeChat && (created.ConversationID != "" || len(created.State) != 0) {
				t.Fatalf("aba %q recebeu estado de chat: %#v", tc.tabType, created)
			}
		})
	}
}

func TestCommandWorkspaceTabCreateRecusaIDNaoCatalogado(t *testing.T) {
	a := readyCommandProduct(t)
	for _, commandID := range []string{"workspace.tab.invalid.create", "workspace.tab.editor.arbitrary"} {
		if _, err := a.BeginUICommand(commandID); err == nil {
			t.Fatalf("comando não permitido aceito: %s", commandID)
		}
	}
}

func TestCommandWorkspaceTabTerminalCreateIsCatalogedContextual(t *testing.T) {
	a := readyCommandProduct(t)
	definition, ok := a.commandProduct.Load().registry.Lookup(commandWorkspaceTabTerminalCreateID)
	if !ok || definition.Effect != commandcatalog.Write || !definition.HasMutableTarget ||
		definition.HandlerClassification != commandcatalog.HandlerBackend ||
		!definition.AllowsSource(commandcatalog.Palette) || !definition.AllowsSource(commandcatalog.KeyboardLocal) || !definition.AllowsSource(commandcatalog.StreamDeck) ||
		commandExecutionClassForDefinition(definition) != commandExecutionDurable {
		t.Fatalf("terminal não foi registrado como mutação contextual: %+v", definition)
	}
	if definition.Presentation == nil ||
		definition.Presentation.Locales["pt-BR"].Description != "Cria uma nova aba e uma nova sessão de terminal no workspace atual" ||
		definition.Presentation.Locales["en"].Description != "Creates a new terminal tab and terminal session in the current workspace" ||
		definition.Presentation.Locales["es"].Description != "Crea una nueva pestaña y una nueva sesión de terminal en el espacio de trabajo actual" {
		t.Fatalf("metadata do terminal não explicita a nova sessão: %+v", definition.Presentation)
	}
	if a.currentTerminalManager() != nil {
		t.Fatal("fixture de readiness deveria estar sem terminalMgr")
	}
	if reservation, err := a.BeginUICommand(commandWorkspaceTabTerminalCreateID); err == nil || reservation.Ticket != "" {
		t.Fatalf("terminal sem manager não deve iniciar handoff: reservation=%+v err=%v", reservation, err)
	}
}

func TestCommandWorkspaceTabCreateKeyboardPessoalAceitaEditorETasklist(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	for _, binding := range []struct {
		command string
		code    string
	}{
		{commandWorkspaceTabEditorCreateID, "KeyE"},
		{commandWorkspaceTabTasklistCreateID, "KeyL"},
	} {
		binding := binding
		settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
			return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: binding.command, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"` + binding.code + `","modifiers":["Control"]}`, Enabled: true})
		})
	}
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		command string
		code    string
	}{
		{commandWorkspaceTabEditorCreateID, "KeyE"},
		{commandWorkspaceTabTasklistCreateID, "KeyL"},
	} {
		shortcut := LocalCommandShortcut{Version: 1, Code: tc.code, Modifiers: []string{"Control"}}
		binding := commandKeyboardBindingFor(t, view, shortcut)
		if binding.CommandID != tc.command || binding.Handler != "contextual" {
			t.Fatalf("binding pessoal inesperado: %+v", binding)
		}
		reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false)
		if err != nil || reservation == nil || reservation.CommandID != tc.command {
			t.Fatalf("Begin pessoal: reservation=%+v err=%v", reservation, err)
		}
		handoff, err := a.TakeUICommand(reservation.Ticket)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
			t.Fatalf("commit pessoal: %v", err)
		}
		if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
			t.Fatalf("resultado pessoal: %+v", result)
		}
	}
}

func TestCommandWorkspaceChatBeginTakeCommitCriaUmaAbaESnapshot(t *testing.T) {
	a := readyCommandProduct(t)
	emitter := wireCommandWorkspaceForTest(a)
	before := commandWorkspaceChatTabCount(t, a)
	reservation, handoff := beginWorkspaceChatCommand(t, a)

	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("CommitWorkspaceTabCommand: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "succeeded" || result.InvocationID != reservation.InvocationID {
		t.Fatalf("resultado do commit: %+v", result)
	}
	if got := commandWorkspaceChatTabCount(t, a); got != before+1 {
		t.Fatalf("abas após commit = %d, esperado exatamente %d", got, before+1)
	}
	assertUICommandLedgerStatus(t, reservation.InvocationID, "succeeded")

	events := commandWorkspaceChatAddedEvents(t, emitter)
	if len(events) != 1 {
		t.Fatalf("eventos workspace:tab_added = %d, esperado 1", len(events))
	}
	var snapshot *workspace.Workspace
	switch value := events[0].data.(type) {
	case *workspace.Workspace:
		snapshot = value
	case workspace.Workspace:
		copy := value
		snapshot = &copy
	default:
		t.Fatalf("snapshot do evento tem tipo %T", events[0].data)
	}
	if snapshot.ID != a.workspaceMgr.ActiveID() || len(snapshot.Tabs.Items) != before+1 {
		t.Fatalf("snapshot do evento não reflete a aba criada: workspace=%q tabs=%d", snapshot.ID, len(snapshot.Tabs.Items))
	}
}

func TestCommandWorkspaceChatCompleteSucceededFailedForgedSaoRejeitados(t *testing.T) {
	for _, status := range []string{"succeeded", "failed"} {
		t.Run(status, func(t *testing.T) {
			a := readyCommandProduct(t)
			before := commandWorkspaceChatTabCount(t, a)
			reservation, handoff := beginWorkspaceChatCommand(t, a)
			if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, status); !errors.Is(err, commandui.ErrCommitRequired) {
				t.Fatalf("CompleteUICommand(%s) = %v, esperado ErrCommitRequired", status, err)
			}
			if got := commandWorkspaceChatTabCount(t, a); got != before {
				t.Fatalf("Complete forjado criou aba: %d antes=%d", got, before)
			}
			if err := a.CancelUICommand(reservation.Ticket); err != nil {
				t.Fatalf("limpeza da reserva: %v", err)
			}
		})
	}
}

func TestCommandWorkspaceChatCancelledPrecommitNaoCriaAba(t *testing.T) {
	a := readyCommandProduct(t)
	before := commandWorkspaceChatTabCount(t, a)
	reservation, err := a.BeginUICommand(commandWorkspaceTabChatCreateID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CancelUICommand(reservation.Ticket); err != nil {
		t.Fatalf("CancelUICommand antes do Take: %v", err)
	}
	if got := commandWorkspaceChatTabCount(t, a); got != before {
		t.Fatalf("cancelamento precommit criou aba: %d antes=%d", got, before)
	}
}

func TestCommandWorkspaceChatCancelledDepoisDoTakeNaoCriaAba(t *testing.T) {
	a := readyCommandProduct(t)
	before := commandWorkspaceChatTabCount(t, a)
	reservation, handoff := beginWorkspaceChatCommand(t, a)
	if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "cancelled"); err != nil {
		t.Fatalf("CompleteUICommand cancelado após Take: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "cancelled" {
		t.Fatalf("resultado do cancelamento após Take = %+v", result)
	}
	if got := commandWorkspaceChatTabCount(t, a); got != before {
		t.Fatalf("cancelamento após Take criou aba: %d antes=%d", got, before)
	}
}

func TestCommandWorkspaceChatReplayEForjadoNaoCriamOutraAba(t *testing.T) {
	a := readyCommandProduct(t)
	before := commandWorkspaceChatTabCount(t, a)
	reservation, handoff := beginWorkspaceChatCommand(t, a)
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID+"-forged"); err == nil {
		t.Fatal("handoff forjado aceito")
	}
	if got := commandWorkspaceChatTabCount(t, a); got != before {
		t.Fatalf("handoff forjado criou aba: %d antes=%d", got, before)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("commit legítimo: %v", err)
	}
	if got := commandWorkspaceChatTabCount(t, a); got != before+1 {
		t.Fatalf("commit legítimo criou %d abas, esperado %d", got, before+1)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("replay de commit aceito")
	}
	if got := commandWorkspaceChatTabCount(t, a); got != before+1 {
		t.Fatalf("replay criou aba adicional: %d", got)
	}
}

func TestCommandWorkspaceChatStaleActiveTabAntesDoCommitNaoCriaAba(t *testing.T) {
	a := readyCommandProduct(t)
	before := commandWorkspaceChatTabCount(t, a)
	reservation, handoff := beginWorkspaceChatCommand(t, a)
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: "stale-editor", Type: workspace.TabTypeEditor, Title: "Stale"}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab("stale-editor"); err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("commit aceitou aba ativa stale")
	}
	if got := commandWorkspaceChatTabCount(t, a); got != before+1 {
		t.Fatalf("commit stale alterou workspace: %d antes=%d", got, before)
	}
}

func TestCommandWorkspaceChatStaleWorkspaceAntesDoCommitNaoCriaAba(t *testing.T) {
	a := readyCommandProduct(t)
	wireCommandWorkspaceForTest(a)
	reservation, handoff := beginWorkspaceChatCommand(t, a)
	other, err := a.workspaceMgr.Create("outro workspace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceCtrl.SwitchWorkspace(other.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err == nil {
		t.Fatal("commit aceitou workspace stale")
	}
	if got := commandWorkspaceChatTabCount(t, a); got != 1 {
		t.Fatalf("workspace novo foi mutado pelo commit stale: %d abas", got)
	}
}

func TestCommandWorkspaceChatCommandCatalogIncluiRegistro(t *testing.T) {
	a := readyCommandProduct(t)
	registry, _, err := a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, commandID := range []string{commandWorkspaceTabChatCreateID, commandWorkspaceTabEditorCreateID, commandWorkspaceTabTasklistCreateID, commandWorkspaceTabCloseID} {
		if _, ok := registry.Lookup(commandID); !ok {
			t.Fatalf("catálogo não registrou %s", commandID)
		}
	}
}

func TestCommandWorkspaceTabNavigationCatalogIsClosedLocalUI(t *testing.T) {
	a := readyCommandProduct(t)
	registry, _, err := a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, commandID := range commandWorkspaceTabNavigationIDs {
		definition, ok := registry.Lookup(commandID)
		if !ok {
			t.Fatalf("navegação ausente do catálogo: %s", commandID)
		}
		if definition.Effect != commandcatalog.Read || definition.HasMutableTarget || definition.HandlerRoute != "ui/workspace/tab/navigate" || definition.HandlerClassification != commandcatalog.HandlerUI || commandExecutionClassForDefinition(definition) != commandExecutionLocalUI {
			t.Fatalf("contrato de navegação inesperado para %s: %+v", commandID, definition)
		}
		if definition.Persistence.Result != commandcatalog.PersistenceNever || definition.Persistence.Audit != commandcatalog.PersistenceNever {
			t.Fatalf("persistência local inesperada para %s: %+v", commandID, definition.Persistence)
		}
		if !definition.Context.None || len(definition.Context.Facts) != 0 {
			t.Fatalf("contexto de navegação inesperado para %s: %+v", commandID, definition.Context)
		}
	}
}

func TestCommandWorkspaceTabNavigationGenericPathIsRejectedWithoutLedger(t *testing.T) {
	a := readyCommandProduct(t)
	before := a.workspaceMgr.Active()
	if _, err := a.BeginUICommand(commandWorkspaceTabNextID); err == nil {
		t.Fatal("BeginUICommand aceitou navegação local")
	}
	if after := a.workspaceMgr.Active(); after.Tabs.Active != before.Tabs.Active || len(after.Tabs.Items) != len(before.Tabs.Items) {
		t.Fatal("ingresso local alterou workspace")
	}
}

func TestCommandWorkspaceTabNavigationNoopIsLocalWithoutMutation(t *testing.T) {
	a := readyCommandProduct(t)
	firstTabID := a.workspaceMgr.Active().Tabs.Items[0].ID
	if err := a.workspaceMgr.SetActiveTab(firstTabID); err != nil {
		t.Fatal(err)
	}
	before, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.BeginUICommand(commandWorkspaceTabFirstID); err == nil {
		t.Fatal("navegação local entrou no handoff auditado")
	}
	after, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.WorkspaceID != before.WorkspaceID || after.ActiveTabID != before.ActiveTabID || after.Version != before.Version {
		t.Fatalf("no-op alterou snapshot: antes=%+v depois=%+v", before, after)
	}
}

func TestCommandWorkspaceTabNavigationAllClosedIDsAreLocal(t *testing.T) {
	for _, commandID := range commandWorkspaceTabNavigationIDs {
		t.Run(commandID, func(t *testing.T) {
			a := readyCommandProduct(t)
			if _, err := a.BeginUICommand(commandID); err == nil {
				t.Fatal("navegação local entrou no handoff auditado")
			}
		})
	}
}

func TestCommandWorkspaceTabNavigationKeyboardPessoal(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandWorkspaceTabNextID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyL","modifiers":["Control","Shift"]}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyL", Modifiers: []string{"Control", "Shift"}}
	binding := commandKeyboardBindingFor(t, view, shortcut)
	if binding.CommandID != commandWorkspaceTabNextID || binding.Handler != "local_ui" {
		t.Fatalf("binding pessoal inesperado: %+v", binding)
	}
	if reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false); err == nil || reservation != nil {
		t.Fatalf("UI local entrou no handoff auditado: reservation=%+v err=%v", reservation, err)
	}
}

func TestCommandWorkspaceChatListCommandsRefleteAlvoESessao(t *testing.T) {
	for _, scenario := range []string{"session_revoked", "workspace_stale"} {
		t.Run(scenario, func(t *testing.T) {
			a := readyCommandProduct(t)
			a.commandCatalogAPI = wailsapi.NewCommandCatalog()
			a.wireCommandCatalog()
			filter := apidto.CommandCatalogFilter{Source: "palette", Locale: "pt-BR"}

			items, err := a.commandCatalogAPI.ListCommands(filter)
			if err != nil {
				t.Fatalf("ListCommands antes do Begin: %v", err)
			}
			var before *apidto.CommandCatalogItem
			for i := range items {
				if items[i].ID == commandWorkspaceTabChatCreateID {
					before = &items[i]
					break
				}
			}
			if before == nil || !before.Available {
				t.Fatalf("chat create não disponível antes do Begin: %+v", before)
			}

			switch scenario {
			case "session_revoked":
				principal := a.commandProduct.Load().principal
				if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", principal.SessionID).Error; err != nil {
					t.Fatal(err)
				}
			case "workspace_stale":
				other, err := a.workspaceMgr.Create("workspace stale")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := a.workspaceMgr.Switch(other.ID); err != nil {
					t.Fatal(err)
				}
			}

			items, err = a.commandCatalogAPI.ListCommands(filter)
			if err != nil {
				t.Fatalf("ListCommands após %s: %v", scenario, err)
			}
			for i := range items {
				if items[i].ID == commandWorkspaceTabChatCreateID {
					if items[i].Available || items[i].ReadinessReason == "" {
						t.Fatalf("chat create permaneceu disponível após %s: %+v", scenario, items[i])
					}
					return
				}
			}
			t.Fatalf("chat create ausente após %s", scenario)
		})
	}
}

func TestCommandWorkspaceChatDefaultCtrlTCommitAuditaKeyboardLocal(t *testing.T) {
	a := readyCommandProduct(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyT", Modifiers: []string{"Control"}}
	binding := commandKeyboardBindingFor(t, view, shortcut)
	if binding.CommandID != commandWorkspaceTabChatCreateID || binding.Handler != "contextual" {
		t.Fatalf("binding Ctrl+T inesperado: %+v", binding)
	}
	reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false)
	if err != nil || reservation == nil {
		t.Fatalf("Begin Ctrl+T: reservation=%+v err=%v", reservation, err)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit Ctrl+T: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "succeeded" {
		t.Fatalf("resultado Ctrl+T: %+v", result)
	}
	var source struct{ SourceType string }
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", reservation.InvocationID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.SourceType != "keyboard.local" {
		t.Fatalf("origem Ctrl+T = %q, esperado keyboard.local", source.SourceType)
	}
}

func TestCommandWorkspaceTabCloseDefaultCtrlWCommitAuditaKeyboardLocal(t *testing.T) {
	a := readyCommandProduct(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyW", Modifiers: []string{"Control"}}
	binding := commandKeyboardBindingFor(t, view, shortcut)
	if binding.CommandID != commandWorkspaceTabCloseID || binding.Handler != "contextual" {
		t.Fatalf("binding Ctrl+W inesperado: %+v", binding)
	}
	reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false)
	if err != nil || reservation == nil {
		t.Fatalf("Begin Ctrl+W: reservation=%+v err=%v", reservation, err)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit Ctrl+W: %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
		t.Fatalf("resultado Ctrl+W: %+v", result)
	}
	var source struct{ SourceType string }
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", reservation.InvocationID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.SourceType != "keyboard.local" {
		t.Fatalf("origem Ctrl+W=%q", source.SourceType)
	}
	ctrlF4 := commandKeyboardBindingFor(t, view, LocalCommandShortcut{Version: 1, Code: "F4", Modifiers: []string{"Control"}})
	if ctrlF4.CommandID != commandWorkspaceTabCloseID || ctrlF4.Handler != "contextual" {
		t.Fatalf("binding Ctrl+F4 inesperado: %+v", ctrlF4)
	}
}

func TestCommandWorkspaceTabCloseDefaultsSuppressRestoreSemFallback(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	projection, err := commandProductProjection(a.commandProduct.Load().registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var closeDefaultID string
	for _, item := range projection.BuiltinLayers[1].Defaults {
		if item.Candidate.Trigger == "keyboard.local:Control+KeyW" {
			closeDefaultID = item.Candidate.ID
		}
	}
	if closeDefaultID == "" {
		t.Fatal("default Ctrl+W de close ausente")
	}
	initial, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	commandKeyboardBindingFor(t, initial, LocalCommandShortcut{Version: 1, Code: "KeyW", Modifiers: []string{"Control"}})
	commandKeyboardBindingFor(t, initial, LocalCommandShortcut{Version: 1, Code: "F4", Modifiers: []string{"Control"}})
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(closeDefaultID, true)
	})
	suppressed, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, shortcut := range []LocalCommandShortcut{{Version: 1, Code: "KeyW", Modifiers: []string{"Control"}}} {
		for _, binding := range suppressed.Bindings {
			if binding.Shortcut.Code == shortcut.Code && len(binding.Shortcut.Modifiers) == 1 && binding.Shortcut.Modifiers[0] == "Control" {
				t.Fatalf("Ctrl+W continuou publicado após supressão: %+v", suppressed)
			}
		}
	}
	commandKeyboardBindingFor(t, suppressed, LocalCommandShortcut{Version: 1, Code: "F4", Modifiers: []string{"Control"}})
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(closeDefaultID, false)
	})
	restored, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	commandKeyboardBindingFor(t, restored, LocalCommandShortcut{Version: 1, Code: "KeyW", Modifiers: []string{"Control"}})
	commandKeyboardBindingFor(t, restored, LocalCommandShortcut{Version: 1, Code: "F4", Modifiers: []string{"Control"}})
}

func TestCommandWorkspaceChatStreamDeckNativeCommitAuditaStreamDeck(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer, CommandID: commandWorkspaceTabChatCreateID, TriggerType: "streamdeck.key",
			TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true,
		})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	reservations := make(chan commandui.Reservation, 1)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-ui-reservation" {
			reservations <- value.(commandui.Reservation)
		}
	})
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(4 * time.Second):
		t.Fatal("native adapter não abriu o dispositivo configurado")
	}
	if err := drainDeckStartupFrames(handle); err != nil {
		t.Fatal(err)
	}
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var reservation commandui.Reservation
	select {
	case reservation = <-reservations:
	case <-time.After(4 * time.Second):
		t.Fatal("reserva native ausente")
	}
	if reservation.CommandID != commandWorkspaceTabChatCreateID {
		t.Fatalf("comando da reserva native = %q", reservation.CommandID)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit Stream Deck: %v", err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != "succeeded" {
		t.Fatalf("resultado Stream Deck: %+v", result)
	}
	var source struct{ SourceType string }
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", reservation.InvocationID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.SourceType != "streamdeck.key" {
		t.Fatalf("origem Stream Deck = %q, esperado streamdeck.key", source.SourceType)
	}
}

func TestCommandWorkspaceTabEditorStreamDeckNativeCommit(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandWorkspaceTabEditorCreateID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	reservations := make(chan commandui.Reservation, 1)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-ui-reservation" {
			reservations <- value.(commandui.Reservation)
		}
	})
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(4 * time.Second):
		t.Fatal("native adapter não abriu o dispositivo configurado")
	}
	if err := drainDeckStartupFrames(handle); err != nil {
		t.Fatal(err)
	}
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var reservation commandui.Reservation
	select {
	case reservation = <-reservations:
	case <-time.After(4 * time.Second):
		t.Fatal("reserva native ausente")
	}
	if reservation.CommandID != commandWorkspaceTabEditorCreateID {
		t.Fatalf("comando da reserva native = %q", reservation.CommandID)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit Stream Deck editor: %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
		t.Fatalf("resultado Stream Deck editor: %+v", result)
	}
}

func TestCommandWorkspaceTabCloseStreamDeckNativeCommit(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandWorkspaceTabCloseID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	reservations := make(chan commandui.Reservation, 1)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-ui-reservation" {
			reservations <- value.(commandui.Reservation)
		}
	})
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(4 * time.Second):
		t.Fatal("native adapter não abriu o dispositivo configurado")
	}
	if err := drainDeckStartupFrames(handle); err != nil {
		t.Fatal(err)
	}
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var reservation commandui.Reservation
	select {
	case reservation = <-reservations:
	case <-time.After(4 * time.Second):
		t.Fatal("reserva native ausente")
	}
	if reservation.CommandID != commandWorkspaceTabCloseID {
		t.Fatalf("comando da reserva native=%q", reservation.CommandID)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit Stream Deck close: %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
		t.Fatalf("resultado Stream Deck close: %+v", result)
	}
}

func TestCommandWorkspaceTabNavigationStreamDeckNativeCommit(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandWorkspaceTabNextID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	for _, commandID := range commandWorkspaceTabNavigationIDs {
		definition, exists := p.registry.Lookup(commandID)
		if !exists || !commandDeckDefinitionEligible(definition) {
			t.Fatalf("navegação registrada não elegível no Deck: %s", commandID)
		}
	}
	preview, _, err := p.deckMap(context.Background())
	if err != nil || preview["test-deck"][0].commandID != commandWorkspaceTabNextID || preview["test-deck"][0].title == "" {
		t.Fatalf("mapa Deck perdeu navegação antes de abrir o driver: %+v err=%v", preview, err)
	}
	events := make(chan CommandDeckLocalUIEvent, 1)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-local-ui" {
			events <- value.(CommandDeckLocalUIEvent)
		}
	})
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(4 * time.Second):
		t.Fatal("native adapter não abriu o dispositivo configurado")
	}
	if err := drainDeckStartupFrames(handle); err != nil {
		t.Fatal(err)
	}
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var event CommandDeckLocalUIEvent
	select {
	case event = <-events:
	case <-time.After(4 * time.Second):
		t.Fatal("evento local do Stream Deck ausente")
	}
	if event.CommandID != commandWorkspaceTabNextID || event.Generation == "" || event.UserID != p.principal.UserID || event.SessionID != p.principal.SessionID || event.WorkspaceID != p.workspaceID {
		t.Fatalf("evento local Stream Deck inesperado: %+v", event)
	}
}

func drainDeckStartupFrames(handle *appDeckHandle) error {
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	select {
	case <-handle.writes:
	case <-timer.C:
		return errors.New("native adapter não publicou frame inicial")
	}
	select {
	case <-handle.writes:
		return nil
	case <-timer.C:
		return errors.New("native adapter não publicou frame de usuário")
	}
}
