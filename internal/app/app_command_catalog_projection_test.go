package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/commandexecution"
	"assistente/internal/terminal"
	"assistente/internal/wailsapi"
)

func TestCommandCatalogRefreshesStaleWorkspaceProjection(t *testing.T) {
	a := commandJobPublicationApp(t)
	// O catálogo avalia readiness operacional; não inicia nenhuma sessão/PTY.
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("palette-projection-regression"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.host.Snapshot(context.Background(), p.principal); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("mudança real não invalidou a projeção: %v", err)
	}
	items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Source: "palette", Locale: "pt-BR"})
	if err != nil || len(items) != 150 {
		t.Fatalf("catálogo: count=%d err=%v", len(items), err)
	}
	// Refresh repairs projection freshness, not missing user configuration.
	// These actions deliberately have no default binding/target. Their positive
	// configured path is covered by app_command_layer_readiness_test.go.
	unconfiguredLayers := map[string]bool{
		commandLayerActivateID: false, commandLayerToggleID: false, commandLayerBackID: false,
	}
	for _, item := range items {
		if _, expected := unconfiguredLayers[item.ID]; expected {
			if item.Available || item.ReadinessReason == "" {
				t.Fatalf("refresh liberou ação de camada sem binding configurado: %+v", item)
			}
			unconfiguredLayers[item.ID] = true
			continue
		}
		if item.ID == commandGlobalJobID || item.ID == commandGlobalVoiceID {
			if item.Available || item.ReadinessReason == "" || len(item.AllowedSources) != 1 || item.AllowedSources[0] != "keyboard.global" {
				t.Fatalf("global exposto como comando de paleta: %+v", item)
			}
			continue
		}
		if !item.Available {
			if item.ID == commandConversationClearID || item.ID == commandTerminalInterruptID || isChatActionCommand(item.ID) || isChatMessageCommand(item.ID) || isPageMutationCommand(item.ID) || item.ID == commandTerminalSessionCreateID || item.ID == commandTerminalSessionCloseID {
				if item.ReadinessReason == "" {
					t.Fatal("clear indisponível sem motivo")
				}
				continue
			}
			t.Fatalf("comando bloqueado por projeção recuperável: %s reason=%s", item.ID, item.ReadinessReason)
		}
	}
	for id, seen := range unconfiguredLayers {
		if !seen {
			t.Fatalf("ação de camada ausente do catálogo após refresh: %s", id)
		}
	}
	reservation, err := a.BeginUICommand(commandWorkspaceTabChatCreateID)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	result, err := a.GetUICommandResult(reservation.Ticket)
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("execução após refresh: %+v err=%v", result, err)
	}
}

func TestCommandCatalogProjectionRefreshDoesNotUnlockVault(t *testing.T) {
	a := commandJobPublicationApp(t)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("locked-palette-projection"); err != nil {
		t.Fatal(err)
	}
	if err := p.host.SetVaultUnlocked(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Source: "palette", Locale: "pt-BR"})
	if err != nil || len(items) != 150 {
		t.Fatalf("catálogo: count=%d err=%v", len(items), err)
	}
	for _, item := range items {
		if item.Available || item.ReadinessReason == "" {
			t.Fatalf("refresh liberou comando com cofre bloqueado: %+v", item)
		}
	}
	if _, err := a.BeginUICommand("navigation.history.open"); err == nil {
		t.Fatal("execução liberada com cofre bloqueado")
	}
}
