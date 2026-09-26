package app

import (
	"context"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/commandautomation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/terminal"
	"assistente/internal/wailsapi"
)

func TestCommandPaletteCatalogIsWiredBeforeBootstrapAndSurvivesKeyboardActivation(t *testing.T) {
	ctx := context.Background()
	a, _ := appLifecycleProductMountFixture(t)
	// A fixture operacional basta para readiness; nenhum PTY é iniciado.
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal("migrar automação de comandos", err)
	}
	catalogAPI := wailsapi.NewCommandCatalog()
	SetCommandCatalogAPI(a, catalogAPI)

	if err := a.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		t.Fatal("montar produto real", err)
	}
	if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal("projetar configuração inicial", err)
	}
	if err := BootstrapCommandLifecycle(ctx, a); err != nil {
		t.Fatal("bootstrap inicial", err)
	}
	assertPaletteCatalog := func(label string) {
		t.Helper()
		items, err := catalogAPI.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Source: string(commandcatalog.Palette)})
		if err != nil {
			t.Fatalf("ListCommands %s: %v", label, err)
		}
		if len(items) != 150 {
			t.Fatalf("catálogo %s vazio/incompleto: len=%d items=%+v", label, len(items), items)
		}
		for _, item := range items {
			if item.ID == commandGlobalJobID || item.ID == commandGlobalVoiceID {
				if item.Available || item.ReadinessReason == "" || len(item.AllowedSources) != 1 || item.AllowedSources[0] != "keyboard.global" {
					t.Fatalf("global exposto como comando de paleta: %+v", item)
				}
				continue
			}
			if !item.Available {
				if item.ID == commandConversationClearID || item.ID == commandTerminalInterruptID || isChatActionCommand(item.ID) || isChatMessageCommand(item.ID) || isPageMutationCommand(item.ID) || isCommandLayerAction(item.ID) || item.ID == commandTerminalSessionCreateID || item.ID == commandTerminalSessionCloseID {
					if item.ReadinessReason == "" {
						t.Fatal("clear indisponível sem motivo")
					}
					continue
				}
				t.Fatalf("item da paleta indisponível %s: %+v", label, item)
			}
		}
	}
	assertPaletteCatalog("antes da ativação")

	decisions := appCommandImportWailsCopyDecisions(t, a)
	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Atalho de regressão da paleta", Enabled: true})
	})
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer.ID, CommandID: commandProductWorkspaceListID, TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`, Enabled: true,
		})
	})
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayer(layer.ID)
	})
	if _, err := a.SetCommandLayerActive(layer.ID, true); err != nil {
		t.Fatal("ativar camada", err)
	}

	assertPaletteCatalog("após ativação")

	keyboard, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(keyboard.Bindings) != 63 {
		t.Fatalf("mapa customizado inesperado: %+v err=%v", keyboard, err)
	}
	custom := commandKeyboardBindingFor(t, keyboard, LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}})
	if custom.CommandID != commandProductWorkspaceListID || custom.Handler != string(commandcatalog.HandlerBackend) {
		t.Fatalf("binding customizado inesperado: %+v", custom)
	}
	result, err := a.DispatchLocalCommandKey(keyboard.Generation, custom.Shortcut, "down", false)
	if err != nil || result == nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("execução do atalho customizado: result=%+v err=%v", result, err)
	}
	assertPaletteCatalog("após dispatch")

	if snapshot, err := CommandLifecycleSnapshot(a); err != nil || !snapshot.Published {
		t.Fatalf("publicação deixou de estar pronta após atalho: snapshot=%+v err=%v", snapshot, err)
	}
}
