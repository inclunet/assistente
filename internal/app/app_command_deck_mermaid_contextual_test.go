package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandinput"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/workspace"
)

func beginDeckMermaidTest(t *testing.T, f contextualDeckTestFixture, event CommandDeckContextualUIEvent, id string) commandui.Reservation {
	t.Helper()
	r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed)
	if err != nil || r.Ticket == "" || r.CommandID != id {
		t.Fatalf("Mermaid physical Begin: %+v %v", r, err)
	}
	p := f.a.commandProduct.Load()
	p.deckExecution.mu.Lock()
	_, retained := p.deckExecution.offers[event.OfferID]
	p.deckExecution.mu.Unlock()
	if retained {
		t.Fatal("Begin retained single-use offer")
	}
	occurrence, exists := commandDeckOccurrenceFor(p, r.InvocationID)
	identity, err := (commandconfig.StreamDeckTriggerPort{}).Normalize(context.Background(), json.RawMessage(`{"version":1,"device":"test-deck","key":1}`))
	if err != nil || !exists || occurrence.identity != identity || occurrence.serial != "test-deck" || occurrence.visual == nil || occurrence.visual.observed != f.observed {
		t.Fatalf("Mermaid lost canonical physical/editor proof: %+v %v", occurrence, err)
	}
	if replay, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, f.observed); err == nil || replay.Ticket != "" {
		t.Fatalf("Mermaid offer replay admitted: %+v %v", replay, err)
	}
	return r
}

func assertDeckMermaidAudit(t *testing.T, f contextualDeckTestFixture, r commandui.Reservation, status string) {
	t.Helper()
	if result := getUIResultEventually(t, f.a, r.Ticket); result.Status != status {
		t.Fatalf("Mermaid outcome: %+v; want %s", result, status)
	}
	f.controller.mu.Lock()
	instanceID := f.controller.instances["test-deck"].id
	f.controller.mu.Unlock()
	var row struct{ CommandID, SourceType, SourceInstanceID, SourceEventID, TriggerType, ObservedTriggerType, ObserverType, Status, ArgumentsSummary, TriggerSpecSnapshot string }
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", r.InvocationID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.CommandID != r.CommandID || row.Status != status || row.SourceType != "streamdeck.key" || row.TriggerType != "streamdeck.key" || row.ObservedTriggerType != "streamdeck.key" || row.ObserverType != "streamdeck.key" || instanceID == "" || row.SourceInstanceID != instanceID || row.SourceEventID != r.InvocationID {
		t.Fatalf("Mermaid audit lost physical provenance: %+v", row)
	}
	// The API never accepts diagram code. Both argument and trigger snapshots
	// stay redacted instead of creating a backend editor target/payload.
	if row.ArgumentsSummary != `{"version":1,"redacted":true}` || row.TriggerSpecSnapshot != `{"version":1,"redacted":true}` {
		t.Fatalf("Mermaid persisted input payload: %+v", row)
	}
	assertPageDeckCleanup(t, f.a, r.Ticket)
}

func TestContextualDeckMermaidAuditedUICompletion(t *testing.T) {
	for _, id := range []string{commandEditorMermaidApplyID, commandEditorMermaidRemoveID} {
		for _, status := range []string{"succeeded", "cancelled"} {
			t.Run(id+"/"+status, func(t *testing.T) {
				f := newContextualDeckTestFixture(t, workspace.TabTypeEditor, id)
				definition, exists := f.a.commandProduct.Load().registry.Lookup(id)
				if !exists || commandExecutionClassForDefinition(definition) != commandExecutionAuditedUI || !definition.AllowsSource(commandcatalog.StreamDeck) {
					t.Fatal("Mermaid is not an audited UI Deck command")
				}
				if _, err := definition.ValidateArguments([]byte(`{}`)); err != nil {
					t.Fatal(err)
				}
				if _, err := definition.ValidateArguments([]byte(`{"code":"graph TD; A-->B"}`)); err == nil {
					t.Fatal("Mermaid accepted diagram code as backend arguments")
				}
				event := f.press(t)
				if commandInvocationCount(t, id) != 0 {
					t.Fatal("offer wrote ledger")
				}
				r := beginDeckMermaidTest(t, f, event, id)
				h := takeUICommandFor(t, f.a, r.Ticket, id)
				if err := f.a.CompleteUICommand(r.Ticket, "forged-handoff", status); err == nil {
					t.Fatal("forged handoff completed")
				}
				if err := f.a.CompleteUICommand(r.Ticket, h.HandoffID, status); err != nil {
					t.Fatal(err)
				}
				assertDeckMermaidAudit(t, f, r, status)
				if err := f.a.CompleteUICommand(r.Ticket, h.HandoffID, status); err == nil {
					t.Fatal("completion replay succeeded")
				}
			})
		}
	}
}

func TestContextualDeckMermaidRejectsInvalidOffers(t *testing.T) {
	for _, scenario := range []string{"forged", "offer-expiry", "old-generation", "map-expiry", "wrong-chat", "wrong-page", "wrong-editor-id", "profile-ABA", "disconnect", "page-ingress"} {
		t.Run(scenario, func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeEditor, commandEditorMermaidRemoveID)
			event := f.press(t)
			p := f.a.commandProduct.Load()
			observed := f.observed
			switch scenario {
			case "forged":
				event.OfferID = "forged"
			case "offer-expiry":
				p.deckExecution.mu.Lock()
				offer := p.deckExecution.offers[event.OfferID]
				offer.expiresAt = time.Now().Add(-time.Second)
				p.deckExecution.offers[event.OfferID] = offer
				p.deckExecution.mu.Unlock()
			case "old-generation":
				f.a.ResetLocalCommandKeyboard(event.Generation)
				if _, err := f.a.GetLocalCommandKeyboardMap(); err != nil {
					t.Fatal(err)
				}
			case "map-expiry":
				p.keyboardMu.Lock()
				p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.keyboardMu.Unlock()
			case "wrong-chat":
				observed.SurfaceType = "chat"
			case "wrong-page":
				observed.SurfaceType, observed.SurfaceID = "profiles", ""
			case "wrong-editor-id":
				observed.SurfaceID = "foreign-editor"
			case "profile-ABA":
				if err := f.a.workspaceMgr.SetProfile("other"); err != nil {
					t.Fatal(err)
				}
				if err := f.a.workspaceMgr.SetProfile("dev"); err != nil {
					t.Fatal(err)
				}
			case "disconnect":
				f.controller.reset("test-deck")
			case "page-ingress":
				if r, err := f.a.BeginContextualDeckPageUICommand(event.OfferID, event.Generation, pageDeckObserved("profiles", "dev")); err == nil || r.Ticket != "" {
					t.Fatalf("Mermaid entered page ingress: %+v %v", r, err)
				}
			}
			if r, err := f.a.BeginContextualDeckUICommand(event.OfferID, event.Generation, observed); err == nil || r.Ticket != "" {
				t.Fatalf("invalid Mermaid offer admitted: %+v %v", r, err)
			}
			if commandInvocationCount(t, commandEditorMermaidRemoveID) != 0 {
				t.Fatal("invalid offer wrote invocation")
			}
			assertDeckLayerNoUIReservation(t, f.a)
		})
	}
}

func TestContextualDeckMermaidRequiresCanonicalEditor(t *testing.T) {
	for _, id := range []string{commandEditorMermaidApplyID, commandEditorMermaidRemoveID} {
		t.Run(id, func(t *testing.T) {
			// The binding matches the actual chat, but canonical projection must
			// exclude Mermaid before emitting any contextual offer.
			f := newContextualDeckTestFixture(t, workspace.TabTypeChat, id)
			ack, err := f.controller.Input(context.Background(), commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:1", Kind: commandinput.KeyDown})
			if err != nil || ack.InvocationID != "" {
				t.Fatalf("chat physical input: %+v %v", ack, err)
			}
			select {
			case event := <-f.events:
				t.Fatalf("chat emitted contextual Mermaid offer: %+v", event)
			default:
			}
			p := f.a.commandProduct.Load()
			p.deckExecution.mu.Lock()
			offers := len(p.deckExecution.offers)
			p.deckExecution.mu.Unlock()
			if offers != 0 {
				t.Fatalf("chat retained %d contextual offers", offers)
			}
			assertDeckLayerNoUIReservation(t, f.a)
			if commandInvocationCount(t, id) != 0 {
				t.Fatal("chat wrote Mermaid invocation")
			}
		})
	}
}

func TestContextualDeckMermaidRevalidatesBeforeTake(t *testing.T) {
	for _, id := range []string{commandEditorMermaidApplyID, commandEditorMermaidRemoveID} {
		for _, scenario := range []string{"reset", "expired-map", "disconnect", "profile-ABA", "changed-editor"} {
			t.Run(id+"/"+scenario, func(t *testing.T) {
				f := newContextualDeckTestFixture(t, workspace.TabTypeEditor, id)
				event := f.press(t)
				r := beginDeckMermaidTest(t, f, event, id)
				p := f.a.commandProduct.Load()
				switch scenario {
				case "reset":
					f.a.ResetLocalCommandKeyboard(event.Generation)
				case "expired-map":
					p.keyboardMu.Lock()
					p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
					p.keyboardMu.Unlock()
				case "disconnect":
					f.controller.reset("test-deck")
				case "profile-ABA":
					if err := f.a.workspaceMgr.SetProfile("other"); err != nil {
						t.Fatal(err)
					}
					if err := f.a.workspaceMgr.SetProfile("dev"); err != nil {
						t.Fatal(err)
					}
				case "changed-editor":
					if err := f.a.workspaceMgr.AddTab(workspace.Tab{ID: "another-mermaid-editor", Type: workspace.TabTypeEditor}); err != nil {
						t.Fatal(err)
					}
				}
				if h, err := f.a.TakeUICommand(r.Ticket); err == nil || h.HandoffID != "" {
					t.Fatalf("stale Mermaid source handed off: %+v %v", h, err)
				}
				_ = f.a.CancelUICommand(r.Ticket)
				assertPageDeckCleanup(t, f.a, r.Ticket)
				var succeeded int64
				if err := database.DB().Table("command_invocations").Where("invocation_id = ? AND status = ?", r.InvocationID, "succeeded").Count(&succeeded).Error; err != nil || succeeded != 0 {
					t.Fatalf("stale Mermaid succeeded: %d %v", succeeded, err)
				}
			})
		}
	}
}

func TestContextualDeckMermaidCancelledHandoffCannotComplete(t *testing.T) {
	for _, id := range []string{commandEditorMermaidApplyID, commandEditorMermaidRemoveID} {
		t.Run(id, func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeEditor, id)
			event := f.press(t)
			r := beginDeckMermaidTest(t, f, event, id)
			h := takeUICommandFor(t, f.a, r.Ticket, id)
			if err := f.a.CancelUICommand(r.Ticket); err != nil {
				t.Fatal(err)
			}
			if err := f.a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
				t.Fatal("cancelled handoff reported success")
			}
			// Cancellation after Take cannot assert that a UI effect never ran.
			// Preserve the broker's unknown outcome, distinct from UI-confirmed
			// Complete(..., "cancelled") covered above.
			assertDeckMermaidAudit(t, f, r, "outcome_unknown")
		})
	}
}

func TestContextualDeckMermaidRemoveReservationOutlivesOffer(t *testing.T) {
	f := newContextualDeckTestFixture(t, workspace.TabTypeEditor, commandEditorMermaidRemoveID)
	event := f.press(t)
	p := f.a.commandProduct.Load()
	p.deckExecution.mu.Lock()
	expiresAt := p.deckExecution.offers[event.OfferID].expiresAt
	p.deckExecution.mu.Unlock()
	r := beginDeckMermaidTest(t, f, event, commandEditorMermaidRemoveID)
	// Model UI preparation/confirmation before Take. The original ten-second
	// offer has already been consumed; its deadline must not cap the reservation.
	wait := time.Until(expiresAt) + 25*time.Millisecond
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		<-timer.C
	}
	if time.Now().Before(expiresAt) {
		t.Fatal("test did not cross offer deadline")
	}
	h := takeUICommandFor(t, f.a, r.Ticket, commandEditorMermaidRemoveID)
	if err := f.a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	assertDeckMermaidAudit(t, f, r, "succeeded")
}
