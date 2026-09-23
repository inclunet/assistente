package app

import (
	"context"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"github.com/google/uuid"
)

func TestCommandDeckOriginRejectsForgedInvocationAndCancelledOccurrence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	instance := uuid.Must(uuid.NewV7()).String()
	p := &commandProductRuntime{deckExecution: &commandDeckExecution{
		occurrences: map[string]commandDeckOccurrence{
			"trusted": {
				ctx: ctx, identity: "streamdeck.key:SERIAL-1:key:2", serial: "SERIAL-1", instanceID: instance,
				versions: commandexecution.Versions{Registry: "r1", GlobalConfig: "g1", ActiveLayers: "l1", Unlocked: true},
			},
		},
	}}

	if _, ok := commandDeckOccurrenceFor(p, "forged"); ok {
		t.Fatal("invocação forjada não deveria localizar uma ocorrência física")
	}
	identity, err := (commandconfig.StreamDeckTriggerPort{}).Normalize(context.Background(), []byte(`{"version":1,"device":"SERIAL-1","key":2}`))
	if err != nil || identity != "streamdeck.key:SERIAL-1:key:2" {
		t.Fatalf("acionador canônico inválido: identity=%q err=%v", identity, err)
	}
	if !validCommandDeckInstanceID(instance) {
		t.Fatal("instanceID físico deveria ser UUIDv7")
	}

	cancel()
	occurrence, ok := commandDeckOccurrenceFor(p, "trusted")
	if !ok || occurrence.ctx.Err() == nil {
		t.Fatal("cancelamento não invalidou o contexto da ocorrência")
	}
}

func TestCommandDeckOriginRestrictsToUICommands(t *testing.T) {
	if !commandDeckUICommand("navigation.help.open") || !commandDeckUICommand(commandProductShortcutsShowID) {
		t.Fatal("comandos de navegação/ajuda deveriam ser aceitos")
	}
	for _, commandID := range []string{"workspace.list", "command.run", "palette.open"} {
		if commandDeckUICommand(commandID) {
			t.Fatalf("comando não-UI foi aceito pela origem Stream Deck: %s", commandID)
		}
	}
}

func TestCommandDeckLedgerGuardRejectsClosedLocalUIAndAcceptsContextual(t *testing.T) {
	a := readyCommandProduct(t)
	registry, _, err := a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	localIDs := append([]string{}, commandWorkspaceTabNavigationIDs...)
	for _, item := range commandProductUINavigation {
		localIDs = append(localIDs, item.id)
	}
	for _, item := range commandProductChatPickers {
		localIDs = append(localIDs, item.id)
	}
	for _, item := range commandProductEditorMenus {
		localIDs = append(localIDs, item.id)
	}
	localIDs = append(localIDs, commandProductShortcutsShowID, commandWorkspacePanelFocusID)
	for _, id := range localIDs {
		definition, ok := registry.Lookup(id)
		if !ok || commandDeckLedgerCommand(definition) {
			t.Fatalf("comando local atravessou guard de ledger Deck: %s %+v", id, definition)
		}
	}
	for _, id := range []string{commandWorkspaceTabChatCreateID, commandWorkspaceTabEditorCreateID, commandWorkspaceTabTasklistCreateID, commandWorkspaceTabTerminalCreateID, commandWorkspaceTabCloseID} {
		definition, ok := registry.Lookup(id)
		if !ok || !commandDeckLedgerCommand(definition) {
			t.Fatalf("mutação contextual foi rejeitada pelo guard de ledger Deck: %s %+v", id, definition)
		}
	}
}

func TestCommandDeckLayerActionsAreMapAndCommonExecutorEligible(t *testing.T) {
	for _, registration := range commandLayerActionRegistrations() {
		definition := registration.Definition
		if !commandDeckDefinitionEligible(definition) || !commandDeckLedgerCommand(definition) {
			t.Fatalf("layer %s não é elegível no mapa/executor Deck", definition.ID)
		}
	}
}
