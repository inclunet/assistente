package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commanddeck"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandWorkspaceContextChatCatalogContract(t *testing.T) {
	a := readyCommandProduct(t)
	definition, ok := a.commandProduct.Load().registry.Lookup(commandWorkspaceChatOpenID)
	if !ok {
		t.Fatal("workspace.chat.open ausente do catálogo")
	}
	if definition.Effect != commandcatalog.Write || definition.Decision != commandcatalog.NoDecision ||
		!definition.HasMutableTarget || definition.HandlerClassification != commandcatalog.HandlerBackend {
		t.Fatalf("contrato workspace.chat.open inesperado: %+v", definition)
	}
	if len(definition.AllowedSources) != 3 || !definition.AllowsSource(commandcatalog.Palette) ||
		!definition.AllowsSource(commandcatalog.KeyboardLocal) || !definition.AllowsSource(commandcatalog.StreamDeck) {
		t.Fatalf("origens workspace.chat.open inesperadas: %+v", definition.AllowedSources)
	}
	if definition.Context.None || len(definition.Context.Facts) != 1 || definition.Context.Facts[0].Provider != "workspace" ||
		definition.Context.Facts[0].Fact != "active_tab" || definition.Context.Facts[0].Mode != commandcatalog.ExactVersion {
		t.Fatalf("contexto workspace.chat.open inesperado: %+v", definition.Context)
	}
	if definition.ArgumentsSchema == nil || definition.ArgumentsSchema.Type != commandcatalog.SchemaObject ||
		definition.ResultSchema == nil || definition.ResultSchema.Type != commandcatalog.SchemaObject {
		t.Fatalf("schemas workspace.chat.open inesperados: args=%+v result=%+v", definition.ArgumentsSchema, definition.ResultSchema)
	}
	if definition.Persistence.Arguments != commandcatalog.PersistenceNever || definition.Persistence.Result != commandcatalog.PersistenceSummary || definition.Persistence.Audit != commandcatalog.PersistenceRedacted {
		t.Fatalf("persistência workspace.chat.open inesperada: %+v", definition.Persistence)
	}
	wantNames := map[string]string{"pt-BR": "Abrir chat contextual", "en": "Open contextual chat", "es": "Abrir chat contextual"}
	for locale, want := range wantNames {
		metadata, ok := definition.Presentation.Locales[locale]
		if !ok || metadata.Name != want || metadata.Description == "" || metadata.Category == "" {
			t.Fatalf("metadata %s inesperado: %+v", locale, definition.Presentation)
		}
	}
	if handler, ok := a.commandProduct.Load().registry.Lookup(commandWorkspaceChatOpenID); !ok || handler.HandlerRoute != "contextual/workspace/chat/open" {
		t.Fatalf("rota workspace.chat.open inesperada: %+v", handler)
	}
}

func TestCommandWorkspaceContextChatDefaultSuppressionAndRemapProjection(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var defaultID string
	for _, item := range projection.BuiltinLayers[1].Defaults {
		if item.Candidate.CommandID == commandWorkspaceChatOpenID {
			defaultID = item.Candidate.ID
			if item.Candidate.Trigger != "keyboard.local:Control+Shift+KeyI" || item.Version != "1" {
				t.Fatalf("default Ctrl+Shift+I inesperado: %+v", item)
			}
		}
	}
	if defaultID == "" {
		t.Fatal("default workspace.chat.open ausente")
	}
	initial, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(initial.Bindings) != 62 {
		t.Fatalf("mapa inicial: %+v err=%v", initial, err)
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(defaultID, true)
	})
	suppressed, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(suppressed.Bindings) != 61 {
		t.Fatalf("mapa suprimido: %+v err=%v", suppressed, err)
	}
	for _, binding := range suppressed.Bindings {
		if binding.CommandID == commandWorkspaceChatOpenID {
			t.Fatalf("workspace.chat.open continuou publicado após supressão: %+v", binding)
		}
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(defaultID, false)
	})
	restored, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(restored.Bindings) != 62 {
		t.Fatalf("mapa restaurado: %+v err=%v", restored, err)
	}
	commandKeyboardBindingFor(t, restored, LocalCommandShortcut{Version: 1, Code: "KeyI", Modifiers: []string{"Control", "Shift"}})
}

func TestCommandWorkspaceContextChatDefaultCtrlShiftICommitAuditaKeyboardLocal(t *testing.T) {
	a := contextChatFixture(t, workspace.TabTypeEditor)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyI", Modifiers: []string{"Control", "Shift"}}
	binding := commandKeyboardBindingFor(t, view, shortcut)
	if binding.CommandID != commandWorkspaceChatOpenID || binding.Handler != "contextual" {
		t.Fatalf("binding Ctrl+Shift+I inesperado: %+v", binding)
	}
	reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false)
	if err != nil || reservation == nil {
		t.Fatalf("Begin Ctrl+Shift+I: reservation=%+v err=%v", reservation, err)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit Ctrl+Shift+I: %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
		t.Fatalf("resultado Ctrl+Shift+I: %+v", result)
	}
	active := a.workspaceMgr.Active().FindTab(a.workspaceMgr.Active().Tabs.Active)
	if active == nil {
		t.Fatal("aba ativa ausente após Ctrl+Shift+I")
	}
	if _, err := uuid.Parse(active.ConversationID); err != nil {
		t.Fatalf("conversation vinculada: %v", err)
	}
	var source struct{ SourceType string }
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", reservation.InvocationID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.SourceType != "keyboard.local" {
		t.Fatalf("origem Ctrl+Shift+I=%q, esperado keyboard.local", source.SourceType)
	}
}

func TestCommandWorkspaceContextChatStreamDeckNativeCommitAuditaStreamDeck(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	if err := database.DB().AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	tabID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		t.Fatal(err)
	}
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandWorkspaceChatOpenID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true})
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
		t.Fatal("reserva native não abriu o dispositivo configurado")
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
	if reservation.CommandID != commandWorkspaceChatOpenID {
		t.Fatalf("comando da reserva native=%q", reservation.CommandID)
	}
	handoff, err := a.TakeUICommand(reservation.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatalf("Commit Stream Deck: %v", err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
		t.Fatalf("resultado Stream Deck: %+v", result)
	}
	active := a.workspaceMgr.Active().FindTab(a.workspaceMgr.Active().Tabs.Active)
	if active == nil {
		t.Fatal("aba ativa ausente após Stream Deck")
	}
	if _, err := uuid.Parse(active.ConversationID); err != nil {
		t.Fatalf("conversation vinculada pelo Deck: %v", err)
	}
	var source struct{ SourceType string }
	if err := database.DB().Table("command_invocations").Select("source_type").Where("invocation_id = ?", reservation.InvocationID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.SourceType != "streamdeck.key" {
		t.Fatalf("origem Stream Deck=%q, esperado streamdeck.key", source.SourceType)
	}
}
