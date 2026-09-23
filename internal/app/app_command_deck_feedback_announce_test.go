package app

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"assistente/internal/commanddeck"
	"assistente/internal/workspace"
)

func TestCommandDeckFeedbackTitleBoundsUnicode(t *testing.T) {
	if got := commandDeckFeedbackTitle("  Copiar\x00 mensagem\n  "); got != "Copiar mensagem" {
		t.Fatalf("unsafe title: %q", got)
	}
	got := commandDeckFeedbackTitle(strings.Repeat("á🎛", 300))
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 256 || !strings.HasSuffix(got, "…") {
		t.Fatalf("invalid truncation: %q", got)
	}
}

func TestCommandDeckFeedbackRuntimeRendersAndAnnouncesConfirmedOutcome(t *testing.T) {
	f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandMessageCopyID)
	p := f.a.commandProduct.Load()
	events := make(chan commandDeckFeedbackEvent, 8)
	previous := f.a.emitter
	f.a.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
		if name == "command:deck-feedback" {
			events <- payload.(commandDeckFeedbackEvent)
			return
		}
		previous.Emit(name, payload)
	})
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var handle *appDeckHandle
	select {
	case handle = <-driver.opened:
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not open")
	}
	// Open writes the safe frame, followed by the configured frame.
	for range 2 {
		select {
		case <-handle.writes:
		case <-time.After(5 * time.Second):
			t.Fatal("missing initial frame")
		}
	}
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 1, Down: true}
	var offer CommandDeckContextualUIEvent
	select {
	case offer = <-f.events:
	case <-time.After(5 * time.Second):
		t.Fatal("missing physical offer")
	}
	r, err := f.a.BeginContextualDeckUICommand(offer.OfferID, offer.Generation, f.observed)
	if err != nil {
		t.Fatal(err)
	}
	message := messageCommandSeed(t, f.conversationID)
	if err := f.a.PrepareChatMessageCommand(r.Ticket, message.ID); err != nil {
		t.Fatal(err)
	}
	h := takeUICommandFor(t, f.a, r.Ticket, commandMessageCopyID)
	if err := f.a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	assertContextualDeckLedger(t, f.a, r.Ticket, r.InvocationID, commandMessageCopyID)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	rendered, announced := false, false
	for !rendered || !announced {
		select {
		case plan := <-handle.writes:
			for _, update := range plan.Updates {
				if update.Index == 1 && update.View.State == "succeeded" {
					rendered = true
				}
			}
		case event := <-events:
			if event.InvocationID == r.InvocationID && event.State == "succeeded" {
				announced = true
			}
		case <-timeout.C:
			t.Fatalf("missing runtime feedback: rendered=%v announced=%v", rendered, announced)
		}
	}
	resetTimeout := time.NewTimer(5 * time.Second)
	defer resetTimeout.Stop()
	for {
		select {
		case plan := <-handle.writes:
			for _, update := range plan.Updates {
				if update.Index == 1 && update.View.State == "conditional" {
					return
				}
			}
		case event := <-events:
			if event.InvocationID == r.InvocationID && event.State == "succeeded" {
				t.Fatal("runtime repeated terminal announcement")
			}
		case <-resetTimeout.C:
			t.Fatal("terminal feedback did not return to normal frame")
		}
	}
}

func TestCommandDeckFeedbackRealCompletionAndAnnouncement(t *testing.T) {
	f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandMessageCopyID)
	p := f.a.commandProduct.Load()
	events := make(chan commandDeckFeedbackEvent, 8)
	previous := f.a.emitter
	f.a.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
		if name == "command:deck-feedback" {
			events <- payload.(commandDeckFeedbackEvent)
			return
		}
		previous.Emit(name, payload)
	})
	offer := f.press(t)
	reservation, err := f.a.BeginContextualDeckUICommand(offer.OfferID, offer.Generation, f.observed)
	if err != nil {
		t.Fatal(err)
	}
	message := messageCommandSeed(t, f.conversationID)
	if err := f.a.PrepareChatMessageCommand(reservation.Ticket, message.ID); err != nil {
		t.Fatal(err)
	}
	handoff := takeUICommandFor(t, f.a, reservation.Ticket, commandMessageCopyID)
	f.controller.mu.Lock()
	instance := f.controller.instances["test-deck"]
	f.controller.mu.Unlock()
	const identity = "streamdeck.key:test-deck:key:1"
	state, invocation := p.deckFeedbackSnapshot(identity, instance.id, f.controller.versions, f.controller.generation)
	if state != "running" || invocation != reservation.InvocationID {
		t.Fatalf("handoff must not imply success: %q %q", state, invocation)
	}
	if err := f.a.CompleteUICommand(reservation.Ticket, handoff.HandoffID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	assertContextualDeckLedger(t, f.a, reservation.Ticket, reservation.InvocationID, commandMessageCopyID)
	state, invocation = p.deckFeedbackSnapshot(identity, instance.id, f.controller.versions, f.controller.generation)
	if state != "succeeded" || invocation != reservation.InvocationID {
		t.Fatalf("confirmed completion missing: %q %q", state, invocation)
	}
	bindings := map[int]commandDeckBinding{1: {identity: identity, title: "Copiar mensagem", feedbackState: state, feedbackInvocationID: invocation}}
	f.controller.announceDeckFeedback(context.Background(), "test-deck", bindings)
	select {
	case event := <-events:
		if event.InvocationID != invocation || event.State != state || event.Title != "Copiar mensagem" || event.UserID != p.principal.UserID || event.SessionID != p.principal.SessionID || event.WorkspaceID != p.workspaceID || event.Generation != offer.Generation || event.ExpiresAt <= time.Now().UnixMilli() {
			t.Fatalf("invalid announcement: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing accessible feedback")
	}
	f.controller.announceDeckFeedback(context.Background(), "test-deck", bindings)
	select {
	case event := <-events:
		t.Fatalf("duplicate announcement: %+v", event)
	default:
	}
	// Capture generation invalidates the frame even before a physical reopen.
	p.mu.Lock()
	p.deckCaptureGeneration++
	p.mu.Unlock()
	f.controller.mu.Lock()
	clear(f.controller.feedbackAnnounced)
	f.controller.mu.Unlock()
	f.controller.announceDeckFeedback(context.Background(), "test-deck", bindings)
	select {
	case event := <-events:
		t.Fatalf("stale capture generation announced: %+v", event)
	default:
	}
	p.mu.Lock()
	p.deckCaptureGeneration--
	p.mu.Unlock()
	// A stale successful frame must not speak again after reconnection.
	f.controller.opened("test-deck")
	f.controller.mu.Lock()
	clear(f.controller.feedbackAnnounced)
	f.controller.mu.Unlock()
	f.controller.announceDeckFeedback(context.Background(), "test-deck", bindings)
	select {
	case event := <-events:
		t.Fatalf("stale connection announced: %+v", event)
	default:
	}
}

func TestCommandDeckFeedbackRealNonSuccessCompletion(t *testing.T) {
	for _, status := range []string{"failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			f := newContextualDeckTestFixture(t, workspace.TabTypeChat, commandMessageCopyID)
			p := f.a.commandProduct.Load()
			offer := f.press(t)
			r, err := f.a.BeginContextualDeckUICommand(offer.OfferID, offer.Generation, f.observed)
			if err != nil {
				t.Fatal(err)
			}
			message := messageCommandSeed(t, f.conversationID)
			if err := f.a.PrepareChatMessageCommand(r.Ticket, message.ID); err != nil {
				t.Fatal(err)
			}
			h := takeUICommandFor(t, f.a, r.Ticket, commandMessageCopyID)
			if err := f.a.CompleteUICommand(r.Ticket, h.HandoffID, status); err != nil {
				t.Fatal(err)
			}
			if result := getUIResultEventually(t, f.a, r.Ticket); result.Status != status {
				t.Fatalf("unexpected executor result: %+v", result)
			}
			f.controller.mu.Lock()
			instance := f.controller.instances["test-deck"]
			f.controller.mu.Unlock()
			got, invocation := p.deckFeedbackSnapshot("streamdeck.key:test-deck:key:1", instance.id, f.controller.versions, f.controller.generation)
			if got != status || invocation != r.InvocationID {
				t.Fatalf("feedback disagrees with actual result: %q %q", got, invocation)
			}
		})
	}
}
