package app

import (
	"context"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/commandadapter"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddecision"
	"assistente/internal/commandinput"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/terminal"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

type contextualDeckTestFixture struct {
	a              *App
	controller     *commandDeckController
	events         <-chan CommandDeckContextualUIEvent
	decisions      <-chan map[string]any
	observed       LocalCommandKeyboardContext
	conversationID string
	layerID        string
}

func TestContextualDeckLegacyOccurrenceCleanupBeforePrepare(t *testing.T) {
	for _, id := range []string{commandMessagePinID, commandTerminalSessionCreateID} {
		t.Run(id, func(t *testing.T) {
			kind := workspace.TabTypeChat
			if id == commandTerminalSessionCreateID {
				kind = workspace.TabTypeTerminal
			}
			a, _, _ := surfacePaletteFixture(t, kind)
			if kind == workspace.TabTypeTerminal {
				a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
				t.Cleanup(a.terminalMgr.CloseAll)
			}
			p, _ := deckProfileConfiguration(t, a, id)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			versions, err := p.host.Snapshot(ctx, p.principal)
			if err != nil {
				t.Fatal(err)
			}
			origin, err := p.commandOriginFacts(ctx, commandcatalog.StreamDeck, []commandbindings.Field{commandbindings.Profile})
			if err != nil {
				t.Fatal(err)
			}
			instanceID := uuid.Must(uuid.NewV7()).String()
			r, err := p.beginDeckCommandWithOrigin(ctx, "test-deck", instanceID, 0, versions, id, p.deckInputGeneration(), origin.version, origin)
			if err != nil {
				t.Fatal(err)
			}
			p.mu.Lock()
			run := p.uiRuns[r.Ticket]
			p.mu.Unlock()
			if run == nil {
				t.Fatal("missing legacy reservation")
			}
			if occurrence, exists := commandDeckOccurrenceFor(p, r.InvocationID); !exists || occurrence.identity != deckProfileTrigger {
				t.Fatal("legacy physical occurrence not registered")
			}
			if err := a.CancelUICommand(r.Ticket); err != nil {
				t.Fatal(err)
			}
			select {
			case <-run.done:
			case <-time.After(5 * time.Second):
				t.Fatal("legacy cancelled run did not terminate")
			}
			p.deckExecution.mu.Lock()
			remaining := len(p.deckExecution.occurrences)
			p.deckExecution.mu.Unlock()
			if remaining != 0 {
				t.Fatalf("legacy cancellation leaked %d occurrences", remaining)
			}
			if commandInvocationCount(t, id) != 0 {
				t.Fatal("legacy cancellation before preparation wrote invocation")
			}
			if kind == workspace.TabTypeTerminal && len(a.terminalMgr.List()) != 0 {
				t.Fatal("cancelled legacy reservation started terminal")
			}
		})
	}
}

func TestContextualDeckOccurrenceCleanupAfterReservation(t *testing.T) {
	for _, scenario := range []string{"message-cancel-before-prepare", "terminal-cancel-before-prepare", "message-success"} {
		t.Run(scenario, func(t *testing.T) {
			kind, id := workspace.TabTypeChat, commandMessagePinID
			if scenario == "terminal-cancel-before-prepare" {
				kind, id = workspace.TabTypeTerminal, commandTerminalSessionCreateID
			}
			f := newContextualDeckTestFixture(t, kind, id)
			if kind == workspace.TabTypeTerminal {
				f.a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
				t.Cleanup(f.a.terminalMgr.CloseAll)
			}
			event := f.press(t)
			r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed)
			if err != nil {
				t.Fatal(err)
			}
			p := f.a.commandProduct.Load()
			p.mu.Lock()
			run := p.uiRuns[r.Ticket]
			p.mu.Unlock()
			if run == nil {
				t.Fatal("missing reserved run")
			}
			if _, exists := commandDeckOccurrenceFor(p, r.InvocationID); !exists {
				t.Fatal("test did not register physical occurrence")
			}
			if commandInvocationCount(t, id) != 0 {
				t.Fatal("invocation before preparation")
			}
			if scenario == "message-success" {
				message := messageCommandSeed(t, f.conversationID)
				if err = f.a.PrepareChatMessageCommand(r.Ticket, message.ID); err != nil {
					t.Fatal(err)
				}
				h := takeUICommandFor(t, f.a, r.Ticket, id)
				if err = f.a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err != nil {
					t.Fatal(err)
				}
			} else if err = f.a.CancelUICommand(r.Ticket); err != nil {
				t.Fatal(err)
			}
			select {
			case <-run.done:
			case <-time.After(5 * time.Second):
				t.Fatal("reserved run did not terminate")
			}
			p.deckExecution.mu.Lock()
			remaining := len(p.deckExecution.occurrences)
			p.deckExecution.mu.Unlock()
			if remaining != 0 {
				t.Fatalf("completed reservation leaked %d physical occurrences", remaining)
			}
			if scenario == "message-success" {
				assertContextualDeckLedger(t, f.a, r.Ticket, r.InvocationID, id)
			} else if commandInvocationCount(t, id) != 0 {
				t.Fatal("cancel before preparation wrote invocation")
			}
			if kind == workspace.TabTypeTerminal && len(f.a.terminalMgr.List()) != 0 {
				t.Fatal("cancelled preparation started terminal")
			}
		})
	}
}

func newContextualDeckTestFixture(t *testing.T, kind workspace.TabType, id string) contextualDeckTestFixture {
	t.Helper()
	a, decisions, cid := surfacePaletteFixture(t, kind)
	a.chatCtrl = controllers.NewChatController(controllers.ChatControllerConfig{})
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	observed := LocalCommandKeyboardContext{SurfaceType: string(snapshot.Tab.Type), SurfaceID: snapshot.Tab.ID, Profile: "dev"}
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Contextual physical Deck", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: id, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":1}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{
		{Field: "app.focused", Value: true}, {Field: "surface.type", Value: observed.SurfaceType}, {Field: "surface.id", Value: observed.SurfaceID}, {Field: "profile", Value: observed.Profile},
	}}}})
	// Same physical key, disjoint local-UI branch: the offer must never turn
	// this branch into a durable invocation or let the UI choose a command ID.
	otherType := "editor"
	if kind == workspace.TabTypeEditor {
		otherType = "chat"
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":1}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: otherType}}}}})
	if _, err = a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	identities, versions, err := p.deckTriggerMap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan CommandDeckContextualUIEvent, 8)
	emitter := a.emitter
	a.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
		if name == "command:deck-contextual-ui" {
			events <- payload.(CommandDeckContextualUIEvent)
			return
		}
		if emitter != nil {
			emitter.Emit(name, payload)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &commandDeckController{p: p, ctx: ctx, versions: versions, identities: identities, pressed: map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{}, generation: p.deckInputGeneration()}
	c.opened("test-deck")
	t.Cleanup(func() { c.reset("") })
	return contextualDeckTestFixture{a: a, controller: c, events: events, decisions: decisions, observed: observed, conversationID: cid, layerID: layer.ID}
}

func (f contextualDeckTestFixture) press(t *testing.T) CommandDeckContextualUIEvent {
	t.Helper()
	ack, err := f.controller.Input(context.Background(), commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:1", Kind: commandinput.KeyDown})
	if err != nil || !ack.Accepted || ack.InvocationID != "" {
		t.Fatalf("physical offer: %+v %v", ack, err)
	}
	select {
	case event := <-f.events:
		p := f.a.commandProduct.Load()
		if event.OfferID == "" || event.Generation == "" || event.UserID != p.principal.UserID || event.SessionID != p.principal.SessionID || event.WorkspaceID != p.workspaceID || len(event.Conditions) != 2 {
			t.Fatalf("invalid offer: %+v", event)
		}
		state := p.deckExecution
		state.mu.Lock()
		offer, exists := state.offers[event.OfferID]
		state.mu.Unlock()
		remaining := time.Until(offer.expiresAt)
		if !exists || remaining <= 0 || remaining > 10*time.Second {
			t.Fatalf("invalid offer TTL: %v", remaining)
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("no contextual offer")
	}
	return CommandDeckContextualUIEvent{}
}

func assertContextualDeckLedger(t *testing.T, a *App, ticket, invocationID, id string) {
	t.Helper()
	if result := getUIResultEventually(t, a, ticket); result.Status != "succeeded" {
		t.Fatalf("result: %+v", result)
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ? AND command_id = ? AND source_type = ? AND status = ?", invocationID, id, "streamdeck.key", "succeeded").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("physical ledger=%d %v", count, err)
	}
}

func TestContextualDeckPhysicalOfferCommitsRepresentativeClasses(t *testing.T) {
	for _, id := range []string{commandChatCancelID, commandMessagePinID, commandMessageCopyID, commandEditorFormatBoldID} {
		t.Run(id, func(t *testing.T) {
			kind := workspace.TabTypeChat
			if id == commandEditorFormatBoldID {
				kind = workspace.TabTypeEditor
			}
			f := newContextualDeckTestFixture(t, kind, id)
			message := messageCommandSeed(t, f.conversationID)
			cancelled := false
			if id == commandChatCancelID {
				f.a.streamMgr.Register(f.conversationID, func() { cancelled = true })
			}
			event := f.press(t)
			if commandInvocationCount(t, id) != 0 {
				t.Fatal("offer wrote invocation")
			}
			r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed)
			if err != nil || r.CommandID != id {
				t.Fatalf("Begin: %+v %v", r, err)
			}
			if cancelled {
				t.Fatal("Begin executed backend effect")
			}
			if isChatMessageCommand(id) {
				if err = f.a.PrepareChatMessageCommand(r.Ticket, message.ID); err != nil {
					t.Fatal(err)
				}
			}
			h := takeUICommandFor(t, f.a, r.Ticket, id)
			occurrence, ok := commandDeckOccurrenceFor(f.a.commandProduct.Load(), r.InvocationID)
			if !ok || occurrence.identity != "streamdeck.key:test-deck:key:1" || occurrence.serial != "test-deck" {
				t.Fatalf("lost exact physical key: %+v", occurrence)
			}
			if replay, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed); err == nil || replay.Ticket != "" {
				t.Fatalf("offer reused: %+v %v", replay, err)
			}
			switch id {
			case commandChatCancelID:
				err = f.a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID)
			case commandMessagePinID:
				err = f.a.CommitChatMessageCommand(r.Ticket, h.HandoffID)
			default:
				err = f.a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded")
			}
			if err != nil {
				t.Fatal(err)
			}
			assertContextualDeckLedger(t, f.a, r.Ticket, r.InvocationID, id)
			if id == commandChatCancelID && !cancelled {
				t.Fatal("stream not cancelled")
			}
			if id == commandMessagePinID {
				var stored database.ChatMessage
				if err = database.DB().First(&stored, "id = ?", message.ID).Error; err != nil || !stored.Pinned {
					t.Fatalf("message not pinned: %+v %v", stored, err)
				}
			}
		})
	}
}

func TestContextualDeckOfferRejectsStaleAndForgedAdmission(t *testing.T) {
	for _, scenario := range []string{"forged", "generation", "map", "map-expiry", "disconnect", "ABA", "wrong-type", "wrong-id", "wrong-profile", "session", "TTL"} {
		t.Run(scenario, func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandChatCancelID)
			event := f.press(t)
			observed := f.observed
			switch scenario {
			case "forged":
				event.OfferID = "forged-offer"
			case "generation":
				event.Generation = "old-map"
			case "map":
				settingsContractApply(t, f.a, f.decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_disable", ID: f.layerID})
			case "map-expiry":
				p := f.a.commandProduct.Load()
				p.keyboardMu.Lock()
				p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.keyboardMu.Unlock()
				p.deckExecution.mu.Lock()
				offer := p.deckExecution.offers[event.OfferID]
				p.deckExecution.mu.Unlock()
				if offer.ctx.Err() != nil || !time.Now().Before(offer.expiresAt) {
					t.Fatal("physical offer expired instead of keyboard map")
				}
			case "disconnect":
				f.controller.reset("test-deck")
			case "ABA":
				if err := f.a.workspaceMgr.SetProfile("other"); err != nil {
					t.Fatal(err)
				}
				if err := f.a.workspaceMgr.SetProfile("dev"); err != nil {
					t.Fatal(err)
				}
			case "wrong-type":
				observed.SurfaceType = "editor"
			case "wrong-id":
				observed.SurfaceID = "foreign-tab"
			case "wrong-profile":
				observed.Profile = "other"
			case "session":
				if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", f.a.commandProduct.Load().principal.SessionID).Error; err != nil {
					t.Fatal(err)
				}
			case "TTL":
				state := f.a.commandProduct.Load().deckExecution
				state.mu.Lock()
				offer := state.offers[event.OfferID]
				offer.expiresAt = time.Now().Add(-time.Second)
				state.offers[event.OfferID] = offer
				state.mu.Unlock()
			}
			if r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, observed); err == nil || r.Ticket != "" {
				t.Fatalf("invalid offer admitted: %+v %v", r, err)
			}
			if commandInvocationCount(t, commandChatCancelID) != 0 {
				t.Fatal("invalid offer wrote invocation")
			}
		})
	}
}

func TestContextualDeckKeyboardBlurAfterBeginPreservesPhysicalReservation(t *testing.T) {
	for _, phase := range []string{"before-take", "before-commit"} {
		t.Run(phase, func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandChatCancelID)
			cancelled := false
			f.a.streamMgr.Register(f.conversationID, func() { cancelled = true })
			event := f.press(t)
			r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed)
			if err != nil {
				t.Fatal(err)
			}
			handoffID := ""
			if phase == "before-commit" {
				handoffID = takeUICommandFor(t, f.a, r.Ticket, r.CommandID).HandoffID
			}
			// Native dialog blur retires only the keyboard lease, not the
			// physical connection or the already captured Deck authority.
			f.a.ResetLocalCommandKeyboard(event.Generation)
			if _, _, ready := f.a.commandProduct.Load().localKeyboardState(); ready {
				t.Fatal("blur did not release keyboard map")
			}
			if phase == "before-take" {
				handoffID = takeUICommandFor(t, f.a, r.Ticket, r.CommandID).HandoffID
			}
			if cancelled {
				t.Fatal("effect occurred before commit")
			}
			if err = f.a.CommitWorkspaceTabCommand(r.Ticket, handoffID); err != nil {
				t.Fatal(err)
			}
			assertContextualDeckLedger(t, f.a, r.Ticket, r.InvocationID, commandChatCancelID)
			if !cancelled {
				t.Fatal("physical command lost after keyboard blur")
			}
		})
	}
}

func TestContextualDeckHeldKeyDoesNotMintTwice(t *testing.T) {
	f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandChatCancelID)
	first := f.press(t)
	press := commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:1", Kind: commandinput.KeyDown}
	for _, repeat := range []bool{false, true} {
		press.Repeat = repeat
		if ack, err := f.controller.Input(context.Background(), press); err != nil || ack.Accepted {
			t.Fatalf("duplicate press: %+v %v", ack, err)
		}
	}
	select {
	case <-f.events:
		t.Fatal("duplicate offer")
	default:
	}
	if _, err := f.controller.Input(context.Background(), commandadapter.Event{SourceInstance: press.SourceInstance, Key: press.Key, Kind: commandinput.KeyUp}); err != nil {
		t.Fatal(err)
	}
	second := f.press(t)
	if second.OfferID == first.OfferID {
		t.Fatal("new physical edge reused offer")
	}
	if commandInvocationCount(t, commandChatCancelID) != 0 {
		t.Fatal("offers wrote ledger")
	}
}

func TestContextualDeckMixedLocalBranchCannotBecomeLedgerCommand(t *testing.T) {
	f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandChatCancelID)
	if err := f.a.workspaceMgr.AddTab(workspace.Tab{ID: "deck-local-editor", Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := f.a.workspaceMgr.SetActiveTab("deck-local-editor"); err != nil {
		t.Fatal(err)
	}
	event := f.press(t)
	observed := LocalCommandKeyboardContext{SurfaceType: "editor", SurfaceID: "deck-local-editor", Profile: "dev"}
	localSelected := false
	for _, condition := range event.Conditions {
		if condition.CommandID == "navigation.settings.open" && condition.ByProfile["dev"].BySurface["editor"] {
			localSelected = true
		}
	}
	if !localSelected {
		t.Fatalf("local alternative missing: %+v", event.Conditions)
	}
	if r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, observed); err == nil || r.Ticket != "" {
		t.Fatalf("local branch admitted as durable: %+v %v", r, err)
	}
	if commandInvocationCount(t, commandChatCancelID) != 0 || commandInvocationCount(t, "navigation.settings.open") != 0 {
		t.Fatal("local choice wrote invocation")
	}
}

func TestContextualDeckContextChangeBeforeTakeAndCommit(t *testing.T) {
	for _, phase := range []string{"take", "commit"} {
		t.Run(phase, func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandChatCancelID)
			cancelled := false
			f.a.streamMgr.Register(f.conversationID, func() { cancelled = true })
			event := f.press(t)
			r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed)
			if err != nil {
				t.Fatal(err)
			}
			handoff := ""
			if phase == "commit" {
				handoff = takeUICommandFor(t, f.a, r.Ticket, r.CommandID).HandoffID
			}
			if err = f.a.workspaceMgr.SetProfile("other"); err != nil {
				t.Fatal(err)
			}
			if err = f.a.workspaceMgr.SetProfile("dev"); err != nil {
				t.Fatal(err)
			}
			if phase == "take" {
				_, err = f.a.TakeUICommand(r.Ticket)
			} else {
				err = f.a.CommitWorkspaceTabCommand(r.Ticket, handoff)
			}
			if err == nil || cancelled {
				t.Fatalf("stale %s performed effect: %v", phase, err)
			}
		})
	}
}

func TestContextualDeckDestructiveDecisionPreserved(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirm", true: "cancel"}[cancel], func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandConversationClearID)
			event := f.press(t)
			r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed)
			if err != nil {
				t.Fatal(err)
			}
			decision := receiveCommandDecisionEvent(t, f.decisions)
			var before database.Conversation
			if err = database.DB().First(&before, "id = ?", f.conversationID).Error; err != nil || before.Summary != "original" {
				t.Fatal("effect before decision", err)
			}
			if cancel {
				finishCommandDecision(t, f.a.questionnaireMgr, decision, nil, true)
				if _, err = f.a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("cancelled decision handed off")
				}
				return
			}
			finishCommandDecision(t, f.a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
			h := takeUICommandFor(t, f.a, r.Ticket, r.CommandID)
			if err = f.a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			assertContextualDeckLedger(t, f.a, r.Ticket, r.InvocationID, r.CommandID)
			var after database.Conversation
			if err = database.DB().First(&after, "id = ?", f.conversationID).Error; err != nil || after.Summary != "" {
				t.Fatalf("clear not applied: %+v %v", after, err)
			}
		})
	}
}
