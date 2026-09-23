package app

import (
	"context"
	"testing"

	"assistente/internal/commandadapter"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandinput"
	"assistente/internal/database"
	"assistente/internal/workspace"
)

// Starts at the trusted adapter boundary, not a real HID device. Frontend
// tests separately exercise the emitted event through the shared UI dispatcher.
func TestCommandLocalFamiliesDeckProjectionAndSingleDispatch(t *testing.T) {
	ids := []string{
		"navigation.workspace.open", "navigation.history.open", "navigation.memories.open",
		"navigation.tasklists.open", "navigation.jobs.open", "navigation.profiles.open",
		"navigation.settings.open", "navigation.help.open", "navigation.about.open",
		"chat.focus.input", "chat.focus.messages", "chat.message.read.open",
		"chat.message.menu.open", "chat.message.reasoning.toggle",
		"chat.message.thread.expand", "chat.message.thread.collapse",
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			a := deckChatPickerFixture(t, id)
			view, err := a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			if !containsString(view.LocalPaletteCommands, id) {
				t.Fatal("same definition missing from local palette projection")
			}
			p := a.commandProduct.Load()
			definition, ok := p.registry.Lookup(id)
			if !ok || commandExecutionClassForDefinition(definition) != commandExecutionLocalUI {
				t.Fatal("presentation classification changed")
			}
			for _, source := range []commandcatalog.Source{commandcatalog.KeyboardLocal, commandcatalog.Palette, commandcatalog.StreamDeck} {
				if !definition.AllowsSource(source) {
					t.Fatalf("missing allowed source %s", source)
				}
			}
			for _, source := range []commandcatalog.Source{commandcatalog.KeyboardGlobal, commandcatalog.Chat, commandcatalog.CLI} {
				if definition.AllowsSource(source) {
					t.Fatalf("unexpected source expansion: %s", source)
				}
			}
			identities, versions, err := p.deckTriggerMap(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var emitted []CommandDeckLocalUIEvent
			a.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
				if name == "command:deck-local-ui" {
					emitted = append(emitted, payload.(CommandDeckLocalUIEvent))
				} else if name == "command:deck-ui-reservation" || name == "command:deck-contextual-ui" {
					t.Errorf("local presentation emitted durable reservation: %s", name)
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			controller := &commandDeckController{p: p, ctx: ctx, versions: versions, identities: identities,
				pressed: map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{}, generation: p.deckInputGeneration()}
			controller.opened("test-deck")
			t.Cleanup(func() { controller.reset("") })
			down := commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:0", Kind: commandinput.KeyDown}
			ack, err := controller.Input(ctx, down)
			if err != nil || !ack.Accepted || ack.InvocationID != "" || len(emitted) != 1 {
				t.Fatalf("first presentation: %+v, %v, events=%d", ack, err, len(emitted))
			}
			event := emitted[0]
			if event.CommandID != id || event.Generation != view.Generation || event.UserID != p.principal.UserID || event.SessionID != p.principal.SessionID || event.WorkspaceID != p.workspaceID {
				t.Fatalf("wrong command or authority in UI event: %+v", event)
			}
			for _, repeat := range []bool{false, true} {
				down.Repeat = repeat
				ack, err := controller.Input(ctx, down)
				if err != nil || ack.Accepted || len(emitted) != 1 {
					t.Fatalf("held/repeated key emitted duplicate: %+v %v", ack, err)
				}
			}
			up := down
			up.Kind, up.Repeat = commandinput.KeyUp, false
			if _, err := controller.Input(ctx, up); err != nil {
				t.Fatal(err)
			}
			down.Repeat = false
			if ack, err := controller.Input(ctx, down); err != nil || !ack.Accepted || len(emitted) != 2 {
				t.Fatalf("fresh press not delivered exactly once: %+v %v", ack, err)
			}
			if _, err := controller.Input(ctx, up); err != nil {
				t.Fatal(err)
			}
			controller.reset("test-deck")
			if ack, err := controller.Input(ctx, down); err == nil || ack.Accepted || len(emitted) != 2 {
				t.Fatalf("disconnected device delivered action: %+v %v", ack, err)
			}
			if commandInvocationCount(t, id) != 0 || commandLedgerCount(t, id) != 0 {
				t.Fatal("presentation created persistent invocation/audit")
			}
		})
	}
}

// Complements the existing palette Begin/Take/Complete matrix for all format
// commands. Success here is the UI acknowledgement, not proof of a DOM edit;
// final formatting and context guards are exercised by frontend integration.
func TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff(t *testing.T) {
	for _, id := range commandEditorFormatIDs {
		t.Run(id, func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeEditor, id)
			event := f.press(t)
			if commandInvocationCount(t, id) != 0 {
				t.Fatal("offer already created invocation")
			}
			reservation, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed)
			if err != nil || reservation.CommandID != id {
				t.Fatalf("wrong admission: %+v %v", reservation, err)
			}
			handoff := takeUICommandFor(t, f.a, reservation.Ticket, id)
			if replay, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed); err == nil || replay.Ticket != "" {
				t.Fatal("physical offer replay admitted")
			}
			if _, err := f.a.TakeUICommand(reservation.Ticket); err == nil {
				t.Fatal("UI handoff replay admitted")
			}
			if err := f.a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "succeeded"); err != nil {
				t.Fatal(err)
			}
			assertContextualDeckLedger(t, f.a, reservation.Ticket, reservation.InvocationID, id)
			var rows int64
			if err := database.DB().Table("command_invocations").Where("command_id = ?", id).Count(&rows).Error; err != nil || rows != 1 {
				t.Fatalf("one action created %d invocations: %v", rows, err)
			}
		})
	}
}

func TestCommandEditorFormatDefaultsUseCommonHandoffWithoutRepeat(t *testing.T) {
	a := readyCommandProduct(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	codeBlock := commandKeyboardBindingFor(t, view, LocalCommandShortcut{Version: 1, Code: "KeyC", Modifiers: []string{"Control", "Alt"}})
	if codeBlock.CommandID != commandEditorFormatCodeBlockID || codeBlock.Handler != "ui" {
		t.Fatalf("Ctrl+Alt+C changed: %+v", codeBlock)
	}
	covered := 0
	for _, binding := range view.Bindings {
		if !isEditorFormatCommand(binding.CommandID) {
			continue
		}
		covered++
		t.Run(binding.CommandID, func(t *testing.T) {
			if binding.Handler != "ui" {
				t.Fatal("format mutation escaped audited UI")
			}
			reservation, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, false)
			if err != nil || reservation == nil || reservation.CommandID != binding.CommandID {
				t.Fatalf("keyboard admission: %+v %v", reservation, err)
			}
			if repeated, err := a.BeginLocalCommandUIKey(view.Generation, binding.Shortcut, true); err == nil || repeated != nil {
				t.Fatal("auto-repeat admitted a second mutation")
			}
			handoff := takeUICommandFor(t, a, reservation.Ticket, binding.CommandID)
			if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
				t.Fatal("handoff delivered twice")
			}
			if err := a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "succeeded"); err != nil {
				t.Fatal(err)
			}
			if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
				t.Fatalf("result: %+v", result)
			}
			var count int64
			if err := database.DB().Table("command_invocations").Where("command_id = ? AND source_type = ? AND status = ?", binding.CommandID, "keyboard.local", "succeeded").Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("keyboard provenance/count: %d %v", count, err)
			}
			if _, err := a.DispatchLocalCommandKey(view.Generation, binding.Shortcut, "up", false); err != nil {
				t.Fatal(err)
			}
		})
	}
	if covered != 14 {
		t.Fatalf("format defaults coverage changed: got %d, want 14", covered)
	}
}
