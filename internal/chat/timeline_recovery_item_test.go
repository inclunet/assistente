package chat

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/apidto"
)

func recoveryItemFixture(t *testing.T) (*TimelineSubscription, *fixtureTimelineSession, *ExecutionProjection, apidto.TimelineRecoveryItemRequest, *TimelineSelectionSnapshot, CanonicalItem) {
	t.Helper()
	sub, session, execution, buffers, snapshot := reserveFixture(t)
	ref := buffers.Reference
	item := CanonicalItem{ItemID: "turn:" + ref.TurnID, ConversationID: sub.key.conversationID, TurnID: &ref.TurnID,
		RecoveryReference: &ref, Revision: "7", State: "interrupted", Durability: "volatile", Kind: "response",
		ReplyToItemID: "question", Position: "opaque-position", MessageIDs: []string{"message"},
		Segments: []CanonicalSegment{{SegmentID: "opaque-text", Kind: "text", Text: "conteúdo 🦜"},
			{SegmentID: "tool", Kind: "tool_calls", Invocations: []TurnSegmentToolCall{{ID: "call", Name: "tool", Status: "completed", HasDetails: true}}}},
		Metadata: CanonicalMetadata{Role: "assistant", Model: "model", Pinned: true}}
	c := sub.conversation
	c.publication = canonicalPublication{epoch: c.epoch, items: map[string]CanonicalItem{}, itemSizes: map[string]int{}}
	c.publication.cacheItem(item)
	request := apidto.TimelineRecoveryItemRequest{Reference: ref, ItemID: item.ItemID, Epoch: c.epoch, Revision: item.Revision}
	return sub, session, execution, request, snapshot, item
}

func TestRecoveryItemExactWireSurvivesEvictionAndEpoch(t *testing.T) {
	sub, session, _, request, snapshot, want := recoveryItemFixture(t)
	for range 2 {
		receipt, err := sub.ReserveRecoveryItem(session.ctx, request, pageReader(snapshot))
		if err != nil || receipt.Identity != request {
			t.Fatal(receipt, err)
		}
	}
	c := sub.conversation
	want.Segments = append([]CanonicalSegment(nil), want.Segments...)
	c.publication.items[request.ItemID].Segments[0].Text = "cache alterado"
	c.publication.removeItem(request.ItemID)
	delete(c.turns, request.Reference.TurnID)
	c.epoch = "nova época"
	snapshot.Version.Incarnation = "importada"
	status, err := sub.ReconcileRecovery(session.ctx, request.Reference, pageReader(snapshot))
	if err != nil || status.State != "retained" {
		t.Fatal(status, err)
	}
	page, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: request.Reference}, pageReader(snapshot))
	if err != nil || page.Found {
		t.Fatal("reserva de item não promete buffers", page, err)
	}
	for range 2 {
		item, err := sub.ReadRecoveryItem(session.ctx, request, pageReader(snapshot))
		if err != nil || !reflect.DeepEqual(item, &want) {
			t.Fatalf("item alterado: %+v %v", item, err)
		}
		item.Segments[0].Text = "caller"
		item.Segments[1].Invocations[0].Name = "caller"
		*item.RecoveryReference = TimelineRecoveryReference{}
	}
	if _, err := sub.ReserveRecoveryItem(session.ctx, request, pageReader(snapshot)); err != nil {
		t.Fatal("idempotência após coleta", err)
	}
	for range 2 {
		if err := sub.ReleaseRecoveryItem(session.ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sub.ReadRecoveryItem(session.ctx, request, pageReader(snapshot)); !errors.Is(err, ErrRecoveryUnavailable) {
		t.Fatal(err)
	}
}

func TestRecoveryItemRefusalsDoNotAcquire(t *testing.T) {
	for _, scenario := range []string{"active", "missing", "reference", "epoch", "revision", "publication", "publication-error", "bytes", "count", "budget", "storage", "revoked", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			sub, session, e, request, snapshot, item := recoveryItemFixture(t)
			reader := pageReader(snapshot)
			c, w := sub.conversation, sub.conversation.windows[sub.windowID]
			want := ErrRecoveryUnavailable
			switch scenario {
			case "active":
				e.currentLocked().finished = false
				want = ErrRecoveryMutable
			case "missing":
				c.publication.removeItem(request.ItemID)
			case "reference":
				request.Reference.ExecutionID = "other"
			case "epoch":
				request.Epoch = "other"
				want = ErrRecoveryCursorStale
			case "revision":
				request.Revision = "8"
				want = ErrRecoveryCursorStale
			case "publication":
				c.publicationStage = &canonicalPublicationStage{}
				want = ErrProjectionRecoveryRequired
			case "publication-error":
				c.storagePublicationErr = errors.New("publicação incompleta")
				want = ErrProjectionRecoveryRequired
			case "bytes":
				item.Segments[0].Text = strings.Repeat("x", recoveryItemBytes)
				c.publication.cacheItem(item)
				want = ErrRecoveryReservationLimit
			case "count":
				w.recoveryReservations = map[TimelineRecoveryReference]recoveryReservation{}
				for i := range recoveryReservationLimit {
					ref := request.Reference
					ref.ExecutionID = fmt.Sprint(i)
					w.recoveryReservations[ref] = recoveryReservation{}
				}
				want = ErrRecoveryReservationLimit
			case "budget":
				w.recoveryReservations = map[TimelineRecoveryReference]recoveryReservation{request.Reference: {bytes: recoveryReservationBytes}}
				want = ErrRecoveryReservationLimit
			case "storage":
				want = errors.New("storage")
				reader = func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) { return nil, want }
			case "revoked":
				reader = func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) {
					session.id = "revoked"
					return snapshot, nil
				}
				want = ErrTimelineSubscription
			case "cancel":
				ctx, cancel := context.WithCancel(session.ctx)
				session.ctx = ctx
				reader = func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) {
					cancel()
					return snapshot, nil
				}
				want = context.Canceled
			}
			got, err := sub.ReserveRecoveryItem(session.ctx, request, reader)
			if got != nil || !errors.Is(err, want) || len(w.recoveryItems) != 0 {
				t.Fatal(got, err)
			}
			c.publicationStage = nil
		})
	}
}

func TestRecoveryItemVersionIsolationAndCleanup(t *testing.T) {
	sub, session, _, request, snapshot, _ := recoveryItemFixture(t)
	other := &TimelineSubscription{owner: sub.owner, key: sub.key, conversation: sub.conversation, session: session, sessionID: session.id, windowID: "other", changed: make(chan struct{}, 1)}
	sub.conversation.windows["other"] = &subscribedWindow{id: "other", handle: other}
	t.Cleanup(other.Close)
	for _, s := range []*TimelineSubscription{sub, other} {
		if _, err := s.ReserveRecoveryItem(session.ctx, request, pageReader(snapshot)); err != nil {
			t.Fatal(err)
		}
	}
	bad := request
	bad.Revision = "8"
	if _, err := sub.ReserveRecoveryItem(session.ctx, bad, pageReader(snapshot)); !errors.Is(err, ErrRecoveryCursorStale) {
		t.Fatal(err)
	}
	if _, err := sub.ReadRecoveryItem(session.ctx, bad, pageReader(snapshot)); !errors.Is(err, ErrRecoveryCursorStale) {
		t.Fatal(err)
	}
	if err := sub.ReleaseRecoveryItem(session.ctx, bad); !errors.Is(err, ErrRecoveryCursorStale) {
		t.Fatal(err)
	}
	session.id = "revoked"
	if _, err := sub.ReadRecoveryItem(session.ctx, request, pageReader(snapshot)); err == nil {
		t.Fatal("leu revogado")
	}
	if err := sub.ReleaseRecoveryItem(session.ctx, request); err == nil {
		t.Fatal("liberou revogado")
	}
	session.id = sub.sessionID
	sub.Close()
	if _, err := sub.ReadRecoveryItem(session.ctx, request, pageReader(snapshot)); err == nil {
		t.Fatal("leu fechado")
	}
	if _, err := other.ReadRecoveryItem(session.ctx, request, pageReader(snapshot)); err != nil {
		t.Fatal(err)
	}
	sub.conversation.discardRecoveryReservations(request.Reference.TurnID, request.Reference.ExecutionID)
	if _, err := other.ReadRecoveryItem(session.ctx, request, pageReader(snapshot)); !errors.Is(err, ErrRecoveryUnavailable) {
		t.Fatal(err)
	}
}

func TestRecoveryItemQuotaAlsoConstrainsBufferReservations(t *testing.T) {
	sub, session, _, request, snapshot, _ := recoveryItemFixture(t)
	w := sub.conversation.windows[sub.windowID]
	w.recoveryItems = map[TimelineRecoveryReference]recoveryItemReservation{}
	for i := range recoveryReservationLimit {
		ref := request.Reference
		ref.ExecutionID = fmt.Sprint(i)
		w.recoveryItems[ref] = recoveryItemReservation{}
	}
	page, err := sub.ReadRecoveryPage(session.ctx, apidto.TimelineRecoveryPageRequest{Reference: request.Reference}, pageReader(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	_, err = sub.ReserveRecovery(session.ctx, apidto.TimelineRecoveryReservationRequest{Reference: request.Reference, ContentVersion: page.ContentVersion}, pageReader(snapshot))
	if !errors.Is(err, ErrRecoveryReservationLimit) {
		t.Fatal(err)
	}
}

func TestRecoveryItemDeletionDoesNotReviveCapability(t *testing.T) {
	sub, session, _, request, snapshot, _ := recoveryItemFixture(t)
	if _, err := sub.ReserveRecoveryItem(session.ctx, request, pageReader(snapshot)); err != nil {
		t.Fatal(err)
	}
	sub.conversation.clearDeletedConversationLocked()
	sub.conversation.turns[request.Reference.TurnID] = &liveTurn{}
	if _, err := sub.ReadRecoveryItem(session.ctx, request, pageReader(snapshot)); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatal("exclusão/recriação reativou reserva", err)
	}
}
