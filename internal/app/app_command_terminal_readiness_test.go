package app

import (
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/terminal"
	"assistente/internal/wailsapi"
)

func TestCommandCatalogTerminalCreateRequiresTerminalManager(t *testing.T) {
	a := readyCommandProduct(t)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	filter := apidto.CommandCatalogFilter{Source: "palette", Locale: "pt-BR"}

	item := func(commandID string) apidto.CommandCatalogItem {
		t.Helper()
		items, err := a.commandCatalogAPI.ListCommands(filter)
		if err != nil {
			t.Fatalf("ListCommands: %v", err)
		}
		for _, candidate := range items {
			if candidate.ID == commandID {
				return candidate
			}
		}
		t.Fatalf("comando terminal ausente do catálogo")
		return apidto.CommandCatalogItem{}
	}

	withoutManager := item(commandWorkspaceTabTerminalCreateID)
	if withoutManager.Available || withoutManager.ReadinessReason == "" {
		t.Fatalf("terminal deveria estar indisponível sem manager: %+v", withoutManager)
	}
	if chat := item(commandWorkspaceTabChatCreateID); !chat.Available {
		t.Fatalf("manager ausente não deve bloquear chat: %+v", chat)
	}

	a.authMu.Lock()
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	a.authMu.Unlock()

	withManager := item(commandWorkspaceTabTerminalCreateID)
	if !withManager.Available || withManager.ReadinessReason != "" {
		t.Fatalf("manager vazio deve bastar para readiness: %+v", withManager)
	}
}
