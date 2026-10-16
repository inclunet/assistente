package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandinput"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func contextualDeckLayerFixture(t *testing.T) contextualLayerPaletteFixture {
	t.Helper()
	f := newContextualLayerPaletteFixture(t)
	layer := settingsContractApply(t, f.a, f.events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Deck layer controller", Enabled: true}})
	settingsContractApply(t, f.a, f.events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	for key, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		ruleID := f.ruleID
		if id == commandLayerBackID {
			ruleID = ""
		}
		binding := settingsContractApply(t, f.a, f.events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: id, TriggerType: "streamdeck.key", TriggerSpec: fmt.Sprintf(`{"version":1,"device":"test-deck","key":%d}`, key), Arguments: map[string]any{"scope": "global", "rule_id": ruleID, "duration_seconds": 0}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{
			{Field: "app.focused", Value: true}, {Field: "surface.type", Value: f.observed.SurfaceType}, {Field: "surface.id", Value: f.observed.SurfaceID}, {Field: "profile", Value: f.observed.Profile},
		}}}})
		f.bindings[id] = binding.ID
	}
	var err error
	f.view, err = f.a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func physicalDeckLayerOffer(t *testing.T, f contextualLayerPaletteFixture, key int) (CommandDeckContextualUIEvent, *commandDeckController) {
	t.Helper()
	p := f.a.commandProduct.Load()
	identities, versions, err := p.deckTriggerMap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan CommandDeckContextualUIEvent, 1)
	previous := f.a.emitter
	f.a.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
		if name == "command:deck-contextual-ui" {
			events <- payload.(CommandDeckContextualUIEvent)
			return
		}
		if previous != nil {
			previous.Emit(name, payload)
		}
	})
	defer func() { f.a.emitter = previous }()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &commandDeckController{p: p, ctx: ctx, versions: versions, identities: identities, pressed: map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{}, generation: p.deckInputGeneration()}
	c.opened("test-deck")
	t.Cleanup(func() { c.reset("") })
	ack, err := c.Input(ctx, commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: fmt.Sprintf("key:%d", key), Kind: commandinput.KeyDown})
	if err != nil || !ack.Accepted || ack.InvocationID != "" {
		t.Fatalf("layer physical offer: %+v %v", ack, err)
	}
	select {
	case event := <-events:
		state := p.deckExecution
		state.mu.Lock()
		offer, exists := state.offers[event.OfferID]
		state.mu.Unlock()
		if !exists || offer.serial != "test-deck" || offer.key != key || event.Generation == "" || event.UserID != p.principal.UserID || event.SessionID != p.principal.SessionID {
			t.Fatalf("offer lost physical identity: %+v %+v", event, offer)
		}
		if _, err := c.Input(ctx, commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: fmt.Sprintf("key:%d", key), Kind: commandinput.KeyUp}); err != nil {
			t.Fatal(err)
		}
		return event, c
	case <-time.After(time.Second):
		t.Fatal("no physical layer offer")
	}
	return CommandDeckContextualUIEvent{}, c
}

func assertDeckLayerNoUIReservation(t *testing.T, a *App) {
	t.Helper()
	p := a.commandProduct.Load()
	p.mu.Lock()
	n := len(p.uiRuns)
	p.mu.Unlock()
	if n != 0 {
		t.Fatalf("layer created %d UI reservations", n)
	}
}

func TestContextualDeckLayerActionsCommitAcrossGenerations(t *testing.T) {
	f := contextualDeckLayerFixture(t)
	assertContextualLayerState(t, f, f.view, false)
	for _, step := range []struct {
		key    int
		id     string
		active bool
	}{{0, commandLayerActivateID, true}, {1, commandLayerToggleID, false}, {1, commandLayerToggleID, true}, {2, commandLayerBackID, false}} {
		event, controller := physicalDeckLayerOffer(t, f, step.key)
		controller.mu.Lock()
		instanceID := controller.instances["test-deck"].id
		controller.mu.Unlock()
		result, err := f.a.ExecuteContextualDeckLayerCommand(event.OfferID, event.Generation, f.observed)
		if err != nil || result.Status != "succeeded" || result.InvocationID == "" || result.Output != nil {
			t.Fatalf("%s result: %+v %v", step.id, result, err)
		}
		var count int64
		if err = database.DB().Table("command_invocations").Where("invocation_id = ? AND command_id = ? AND source_type = ? AND status = ?", result.InvocationID, step.id, "streamdeck.key", "succeeded").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("Deck/key-selected ledger=%d %v", count, err)
		}
		var audit struct{ TriggerType, ObservedTriggerType, ObserverType, SourceInstanceID, SourceEventID, BindingIDs, TriggerSpecSnapshot string }
		if err = database.DB().Table("command_invocations").Where("invocation_id = ?", result.InvocationID).Take(&audit).Error; err != nil {
			t.Fatal(err)
		}
		if audit.TriggerType != "streamdeck.key" || audit.ObservedTriggerType != "streamdeck.key" || audit.ObserverType != "streamdeck.key" || audit.SourceInstanceID != instanceID || audit.SourceEventID != result.InvocationID {
			t.Fatalf("ledger lost native provenance: %+v; instance=%s", audit, instanceID)
		}
		// Trigger snapshots are deliberately redacted. Verify the exact persisted
		// device/key binding, rather than requiring sensitive raw trigger storage.
		var bindingIDs []string
		if err = json.Unmarshal([]byte(audit.BindingIDs), &bindingIDs); err != nil || len(bindingIDs) != 1 || bindingIDs[0] != f.bindings[step.id] {
			t.Fatalf("wrong physical key binding: %+v %v", bindingIDs, err)
		}
		if audit.TriggerSpecSnapshot != `{"version":1,"redacted":true}` {
			t.Fatalf("trigger redaction changed: %s", audit.TriggerSpecSnapshot)
		}
		p := f.a.commandProduct.Load()
		p.deckExecution.mu.Lock()
		remaining := len(p.deckExecution.occurrences)
		p.deckExecution.mu.Unlock()
		if remaining != 0 {
			t.Fatalf("returned layer invocation retained %d occurrences", remaining)
		}
		fresh, err := f.a.GetLocalCommandKeyboardMap()
		if err != nil || fresh.Generation == event.Generation {
			t.Fatalf("own mutation did not republish: %+v %v", fresh, err)
		}
		assertContextualLayerState(t, f, fresh, step.active)
		assertDeckLayerNoUIReservation(t, f.a)
		if replay, err := f.a.ExecuteContextualDeckLayerCommand(event.OfferID, fresh.Generation, f.observed); err == nil || replay.Status == "succeeded" {
			t.Fatalf("offer replay succeeded: %+v %v", replay, err)
		}
		f.view = fresh
	}
}

func TestContextualDeckLayerRejectsInvalidOffers(t *testing.T) {
	for _, scenario := range []string{"forged", "TTL", "old-map", "map-expiry", "session", "disconnect", "ABA", "wrong-profile", "wrong-surface"} {
		t.Run(scenario, func(t *testing.T) {
			f := contextualDeckLayerFixture(t)
			event, c := physicalDeckLayerOffer(t, f, 0)
			observed := f.observed
			p := f.a.commandProduct.Load()
			switch scenario {
			case "forged":
				event.OfferID = "forged"
			case "TTL":
				p.deckExecution.mu.Lock()
				offer := p.deckExecution.offers[event.OfferID]
				offer.expiresAt = time.Now().Add(-time.Second)
				p.deckExecution.offers[event.OfferID] = offer
				p.deckExecution.mu.Unlock()
			case "old-map":
				f.a.ResetLocalCommandKeyboard(event.Generation)
				if _, err := f.a.GetLocalCommandKeyboardMap(); err != nil {
					t.Fatal(err)
				}
			case "map-expiry":
				p.keyboardMu.Lock()
				p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.keyboardMu.Unlock()
			case "session":
				if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", p.principal.SessionID).Error; err != nil {
					t.Fatal(err)
				}
			case "disconnect":
				c.reset("test-deck")
			case "ABA":
				if err := f.a.workspaceMgr.SetProfile("other"); err != nil {
					t.Fatal(err)
				}
				if err := f.a.workspaceMgr.SetProfile("dev"); err != nil {
					t.Fatal(err)
				}
			case "wrong-profile":
				observed.Profile = "other"
			case "wrong-surface":
				observed.SurfaceID = "foreign-tab"
			}
			if result, err := f.a.ExecuteContextualDeckLayerCommand(event.OfferID, event.Generation, observed); err == nil || result.Status == "succeeded" {
				t.Fatalf("invalid offer executed: %+v %v", result, err)
			}
			assertContextualLayerState(t, f, f.view, false)
			assertDeckLayerNoUIReservation(t, f.a)
			if commandInvocationCount(t, commandLayerActivateID) != 0 {
				t.Fatal("invalid offer wrote invocation")
			}
		})
	}
}

func TestContextualDeckLayerAndWorkspaceRoutesDoNotCross(t *testing.T) {
	t.Run("layer-to-workspace", func(t *testing.T) {
		f := contextualDeckLayerFixture(t)
		event, _ := physicalDeckLayerOffer(t, f, 0)
		if r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed); err == nil || r.Ticket != "" {
			t.Fatalf("layer became UI reservation: %+v %v", r, err)
		}
		if r, err := f.a.ExecuteContextualDeckLayerCommand(event.OfferID, event.Generation, f.observed); err == nil || r.Status == "succeeded" {
			t.Fatalf("wrong ingress did not consume offer: %+v %v", r, err)
		}
		assertContextualLayerState(t, f, f.view, false)
		assertDeckLayerNoUIReservation(t, f.a)
	})
	t.Run("workspace-to-layer", func(t *testing.T) {
		f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandChatCancelID)
		event := f.press(t)
		if r, err := f.a.ExecuteContextualDeckLayerCommand(event.OfferID, event.Generation, f.observed); err == nil || r.Status == "succeeded" {
			t.Fatalf("workspace became layer: %+v %v", r, err)
		}
		if r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed); err == nil || r.Ticket != "" {
			t.Fatalf("wrong layer ingress did not consume: %+v %v", r, err)
		}
		assertDeckLayerNoUIReservation(t, f.a)
		if commandInvocationCount(t, commandChatCancelID) != 0 {
			t.Fatal("cross-route wrote invocation")
		}
	})
}

func TestContextualDeckLayerClaimRechecksProfileABA(t *testing.T) {
	for _, scenario := range []string{"unchanged", "ABA-before-claim", "ABA-after-validation", "reset-after-validation", "expiry-after-validation"} {
		t.Run(scenario, func(t *testing.T) {
			f := contextualDeckLayerFixture(t)
			p := f.a.commandProduct.Load()
			proof, err := f.a.captureLocalKeyboardContext(f.observed)
			if err != nil {
				t.Fatal(err)
			}
			versions, err := p.host.Snapshot(context.Background(), p.principal)
			if err != nil {
				t.Fatal(err)
			}
			id := uuid.Must(uuid.NewV7()).String()
			p.keyboardMu.Lock()
			keyboard := p.keyboardMap
			p.keyboardMu.Unlock()
			if keyboard == nil {
				t.Fatal("fixture has no current keyboard map")
			}
			occurrence := &contextualDeckLayerOccurrence{product: p, invocationID: id, commandID: commandLayerActivateID, proof: proof, versions: versions, keyboard: keyboard, valid: func() bool { return p.localKeyboardContextCurrent(proof) }}
			if scenario == "ABA-after-validation" {
				occurrence.valid = func() bool {
					current := p.localKeyboardContextCurrent(proof)
					if f.a.workspaceMgr.SetProfile("other") != nil || f.a.workspaceMgr.SetProfile("dev") != nil {
						return false
					}
					return current
				}
			}
			optimisticValidated := false
			if scenario == "reset-after-validation" || scenario == "expiry-after-validation" {
				occurrence.valid = func() bool {
					current := p.localKeyboardContextCurrent(proof)
					optimisticValidated = current
					if scenario == "reset-after-validation" {
						p.clearLocalCommandKeyboard(keyboard.view.Generation)
					} else {
						p.keyboardMu.Lock()
						keyboard.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
						p.keyboardMu.Unlock()
					}
					return current
				}
			}
			// Intentionally no keyboard cancellation watcher: the authoritative
			// map guard must reject even before asynchronous cancellation arrives.
			ctx := context.WithValue(context.Background(), contextualDeckLayerKey{}, occurrence)
			args := json.RawMessage(`{"scope":"global","rule_id":"` + f.ruleID + `","duration_seconds":0}`)
			trigger := json.RawMessage(`{"version":1,"device":"test-deck","key":0}`)
			var handle commandexecution.ExecutionHandle
			err = f.a.commandGate.WithMutation(context.Background(), func() error {
				var err error
				handle, err = f.a.startCommandLayerAction(ctx, commandexecution.Invocation{ID: id, CommandID: commandLayerActivateID, Principal: p.principal, Source: commandcatalog.StreamDeck, Envelope: &commandcontract.Envelope{Arguments: &args, TriggerSpec: &trigger}})
				if err != nil || scenario != "ABA-before-claim" {
					return err
				}
				if err = f.a.workspaceMgr.SetProfile("other"); err != nil {
					return err
				}
				return f.a.workspaceMgr.SetProfile("dev")
			})
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Cancel()
			select {
			case outcome := <-handle.Done:
				if (outcome.Status == "succeeded") != (scenario == "unchanged") {
					t.Fatalf("claim %s: %+v", scenario, outcome)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("claim did not finish")
			}
			if (scenario == "reset-after-validation" || scenario == "expiry-after-validation") && !optimisticValidated {
				t.Fatal("negative did not reach a successful optimistic validation")
			}
			view, err := f.a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			assertContextualLayerState(t, f, view, scenario == "unchanged")
		})
	}
}
