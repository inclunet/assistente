package app

import (
	"context"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/commandcatalog"
	"assistente/internal/database"
	"assistente/internal/terminal"
	"assistente/internal/wailsapi"
)

func TestCommandProductCatalogReflectsOperationalReadiness(t *testing.T) {
	for _, scenario := range []string{"ready", "keyboard.local", "ui", "vault_locked", "session_revoked", "shutdown", "workspace_changed"} {
		t.Run(scenario, func(t *testing.T) {
			a := readyCommandProduct(t)
			// O cenário operacional inclui o manager, sem iniciar nenhum PTY.
			a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
			a.commandCatalogAPI = wailsapi.NewCommandCatalog()
			a.wireCommandCatalog()
			ctx := context.Background()
			source := "palette"
			switch scenario {
			case "keyboard.local":
				source = scenario
			case "ui":
				source = string(commandcatalog.UI)
			case "vault_locked":
				if err := a.commandHost.SetVaultUnlocked(ctx, false); err != nil {
					t.Fatal(err)
				}
			case "session_revoked":
				if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				if err := a.commandProduct.Load().Shutdown(ctx); err != nil {
					t.Fatal(err)
				}
			case "workspace_changed":
				other, err := a.workspaceMgr.Create("Outro workspace")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := a.workspaceMgr.Switch(other.ID); err != nil {
					t.Fatal(err)
				}
			}
			items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Source: source, Locale: "pt-BR"})
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 151 {
				t.Fatalf("catálogo inesperado: %+v", items)
			}
			want := scenario == "ready"
			wantIDs := map[string]bool{
				commandProductWorkspaceListID: true, commandProductShortcutsShowID: true,
				commandWorkspacePanelFocusID:        true,
				commandWorkspaceTabChatCreateID:     true,
				commandWorkspaceTabEditorCreateID:   true,
				commandWorkspaceTabTasklistCreateID: true,
				commandWorkspaceTabTerminalCreateID: true,
				commandWorkspaceTabCloseID:          true,
				commandWorkspaceCreateID:            true,
				commandWorkspaceChatOpenID:          true,
				commandEditorModeMarkdownID:         true,
				commandEditorModeRichID:             true,
				commandEditorModeViewID:             true,
				commandEditorFileOpenID:             true,
				commandEditorFileSaveID:             true,
				commandEditorFileSaveCopyID:         true,
				commandLayerActivateID:              true,
				commandLayerToggleID:                true,
				commandLayerBackID:                  true,
			}
			for _, formatID := range commandEditorFormatIDs {
				wantIDs[formatID] = true
			}
			for _, navigationID := range commandWorkspaceTabNavigationIDs {
				wantIDs[navigationID] = true
			}
			for _, navigation := range commandProductUINavigation {
				wantIDs[navigation.id] = true
			}
			for _, picker := range commandProductChatPickers {
				wantIDs[picker.id] = true
			}
			for _, editorCommand := range commandProductEditorMenus {
				wantIDs[editorCommand.id] = true
			}
			for _, pagePresentation := range commandProductPagePresentation {
				wantIDs[pagePresentation.id] = true
			}
			wantIDs[commandGlobalVoiceID] = true
			wantIDs[commandGlobalJobID] = true
			globalOnly := map[string]bool{commandGlobalVoiceID: true, commandGlobalJobID: true}
			seen := make(map[string]bool)
			for _, item := range items {
				if item.ID == commandConversationClearID || item.ID == commandTerminalInterruptID || isChatActionCommand(item.ID) || isChatMessageCommand(item.ID) || isPageMutationCommand(item.ID) || item.ID == commandTerminalSessionCreateID || item.ID == commandTerminalSessionCloseID {
					if item.Available || item.ReadinessReason == "" {
						t.Fatalf("clear without bound conversation: %+v", item)
					}
					continue
				}
				if isCommandLayerAction(item.ID) {
					if item.Available || item.ReadinessReason == "" {
						t.Fatalf("ação de camada com readiness inválida: %+v", item)
					}
					seen[item.ID] = true
					continue
				}
				if !wantIDs[item.ID] {
					t.Fatalf("comando inesperado: %s", item.ID)
				}
				seen[item.ID] = true
				expectedAvailable := want
				if globalOnly[item.ID] {
					// Estes comandos só têm binding na origem keyboard.global;
					// catalogá-los não os torna executáveis por palette, teclado
					// local ou UI.
					expectedAvailable = false
				}
				if item.Available != expectedAvailable {
					t.Fatalf("disponível=%v, esperado=%v: %+v", item.Available, expectedAvailable, item)
				}
				if !expectedAvailable && item.ReadinessReason == "" {
					t.Fatal("indisponibilidade sem motivo")
				}
			}
			if len(seen) != len(wantIDs) {
				t.Fatal("catálogo com IDs duplicados")
			}
		})
	}
}
