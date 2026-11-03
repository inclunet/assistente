package chat

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
)

// Fixture sem banco: a entrega deve depender somente do journal já autorizado.
func deliveryFixture(t *testing.T) (*TimelineSubscription, context.Context) {
	t.Helper()
	ctx := database.WithUserID(context.Background(), "owner")
	p := NewLiveProjection(nil)
	key := projectionKey{"owner", "conversation"}
	c := &projectionConversation{epoch: "epoch", windows: map[string]*subscribedWindow{}}
	p.conversations[key] = c
	s := &TimelineSubscription{owner: p, key: key, conversation: c, sessionID: "session", session: &fixtureTimelineSession{ctx: ctx, id: "session"}, windowID: "window", changed: make(chan struct{}, 1)}
	c.windows[s.windowID] = &subscribedWindow{id: s.windowID, handle: s, epoch: c.epoch, revision: 1, snapshotRevision: 1, itemIDs: map[string]bool{"message:one": true}, delivery: timelineDeliveryCursor{epoch: c.epoch, revision: "1"}}
	t.Cleanup(s.Close)
	return s, ctx
}

func publishDelivery(s *TimelineSubscription, text string) {
	c := s.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishCanonical(s.key.conversationID, []*TimelineItem{{ItemID: "message:one", ConversationID: s.key.conversationID, Kind: "message", Segments: []ResponseSegment{{SegmentID: "text", Kind: "text", Text: text}}}})
}

func nextDelivery(t *testing.T, s *TimelineSubscription, ctx context.Context) *TimelineDelivery {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	delivery, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return delivery
}

func TestTimelineDeliveryMatchesReplayAndOwnsPayload(t *testing.T) {
	s, ctx := deliveryFixture(t)
	publishDelivery(s, "first")
	publishDelivery(s, "first second")
	d := nextDelivery(t, s, ctx)
	if d.SyncRequired || d.ConversationID != "conversation" || d.WindowID != "window" || d.Epoch != "epoch" || d.SnapshotRevision != "1" || len(d.Batches) != 2 {
		t.Fatalf("entrega: %+v", d)
	}
	c := s.conversation
	c.mu.Lock()
	batches, ok := c.publication.replay("epoch", 0)
	expected, routed, err := replayWindowBatches(c.windows[s.windowID], "1", batches)
	c.mu.Unlock()
	if !ok || !routed || err != nil || !reflect.DeepEqual(expected, d.Batches) {
		t.Fatalf("eventos e replay divergiram: %v", err)
	}
	d.Batches[0].Operations[0].Item.Segments[0].Text = "mutated"
	c.mu.Lock()
	again, _ := c.publication.replay("epoch", 0)
	c.mu.Unlock()
	if again[0].Operations[0].Item.Segments[0].Text != "first" {
		t.Fatal("consumidor alterou journal")
	}
	if d.Batches[1].Operations[0].Text != " second" {
		t.Fatal("delta incremental perdido")
	}
}

func TestTimelineDeliveryEmptyScopedBatchesAdvanceSequence(t *testing.T) {
	s, ctx := deliveryFixture(t)
	s.conversation.windows[s.windowID].itemIDs = map[string]bool{}
	publishDelivery(s, "outside")
	d := nextDelivery(t, s, ctx)
	if d.SyncRequired || len(d.Batches) != 1 || len(d.Batches[0].Operations) != 0 || d.Batches[0].Sequence != "1" {
		t.Fatalf("lote vazio perdeu sequência ou vazou outro escopo: %+v", d)
	}
}

func TestTimelineDeliveryCommitAndRollback(t *testing.T) {
	s, ctx := deliveryFixture(t)
	c := s.conversation
	c.mu.Lock()
	finish := c.beginCanonicalPublication()
	c.publishCanonicalChanges("conversation", nil, []string{"message:one"})
	finish("conversation", false)
	c.mu.Unlock()
	select {
	case <-s.changed:
		t.Fatal("rollback sinalizou publicação")
	default:
	}
	c.mu.Lock()
	finish = c.beginCanonicalPublication()
	c.publishCanonicalChanges("conversation", nil, []string{"message:one"})
	finish("conversation", true)
	c.mu.Unlock()
	d := nextDelivery(t, s, ctx)
	if d.SyncRequired || len(d.Batches) != 1 || d.Batches[0].Sequence != "1" || d.Batches[0].Operations[0].Kind != "item_remove" {
		t.Fatalf("commit inválido: %+v", d)
	}
}

func TestTimelineDeliveryRecoveryConditionsSuspendUntilSnapshot(t *testing.T) {
	for _, reason := range []string{"epoch_changed", "publication_failed", "window_invalidated", "journal_gap", "stage_overflow", "large_batch"} {
		t.Run(reason, func(t *testing.T) {
			s, ctx := deliveryFixture(t)
			c := s.conversation
			c.mu.Lock()
			switch reason {
			case "epoch_changed":
				c.invalidateTimelineEpochLocked()
			case "publication_failed":
				c.storagePublicationErr = errors.New("failed")
				c.signalDeliveriesLocked()
			case "window_invalidated":
				c.windows[s.windowID].syncRequired = true
				c.signalDeliveriesLocked()
			case "journal_gap":
				for i := 0; i <= canonicalReplayBatches; i++ {
					c.publishCanonicalChanges("conversation", nil, []string{"message:one"})
				}
			case "stage_overflow":
				finish := c.beginCanonicalPublication()
				c.publishCanonicalChanges("conversation", nil, []string{strings.Repeat("x", canonicalReplayBytes)})
				finish("conversation", true)
			case "large_batch":
				c.publishCanonicalChanges("conversation", nil, []string{strings.Repeat("x", canonicalReplayBytes)})
			}
			c.mu.Unlock()
			select {
			case <-s.changed:
			default:
				t.Fatal("invalidação não acordou leitor")
			}
			d := nextDelivery(t, s, ctx)
			expected := reason
			if reason == "stage_overflow" || reason == "large_batch" {
				expected = "journal_gap"
			}
			if !d.SyncRequired || d.Reason != expected || len(d.Batches) != 0 {
				t.Fatalf("recuperação: %+v", d)
			}
			c.mu.Lock()
			blocked, err := s.takeDeliveryLocked(ctx, c)
			c.mu.Unlock()
			if blocked != nil || err != nil {
				t.Fatalf("repetiu SyncRequired: %+v %v", blocked, err)
			}
		})
	}
}

func TestTimelineDeliveryCancellationDoesNotConsume(t *testing.T) {
	s, ctx := deliveryFixture(t)
	publishDelivery(s, "pending")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Next(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if d := nextDelivery(t, s, ctx); len(d.Batches) != 1 {
		t.Fatal("cancelamento consumiu lote")
	}
}

// O hook confirma que Next já entrou no percurso de espera, sem sleeps.
type deliverySession struct {
	ctx     context.Context
	entered chan struct{}
	denied  *atomic.Bool
}

func (s deliverySession) AuthorizeTimeline(context.Context) (context.Context, string, error) {
	if s.denied.Load() {
		return nil, "", ErrTimelineSubscription
	}
	select {
	case s.entered <- struct{}{}:
	default:
	}
	return s.ctx, "session", nil
}

func TestTimelineDeliveryWaitingLifecycle(t *testing.T) {
	for _, action := range []string{"publish", "close", "delete", "cancel", "revoke"} {
		t.Run(action, func(t *testing.T) {
			s, ctx := deliveryFixture(t)
			entered := make(chan struct{}, 1)
			denied := &atomic.Bool{}
			s.session = deliverySession{ctx, entered, denied}
			waiting, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := s.Next(waiting); result <- err }()
			select {
			case <-entered:
			case <-waiting.Done():
				t.Fatal("leitor não iniciou")
			}
			if _, err := s.Next(ctx); !errors.Is(err, ErrTimelineDeliveryBusy) {
				t.Fatalf("dois leitores aceitos: %v", err)
			}
			switch action {
			case "publish":
				publishDelivery(s, "wake")
			case "close":
				s.Close()
			case "delete":
				s.conversation.mu.Lock()
				s.conversation.clearDeletedConversationLocked()
				s.conversation.mu.Unlock()
			case "cancel":
				cancel()
			case "revoke":
				s.conversation.mu.Lock()
				denied.Store(true)
				s.signalDeliveryLocked()
				s.conversation.mu.Unlock()
			}
			err := <-result
			if action == "publish" && err != nil || action != "publish" && err == nil || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("espera não encerrou corretamente: %v", err)
			}
		})
	}
}

func TestTimelineDeliveryRefreshDiscardsOldBaseline(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(nil)
	s, before, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 2}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.conversation.mu.Lock()
	s.conversation.invalidateTimelineEpochLocked()
	s.conversation.mu.Unlock()
	stale := nextDelivery(t, s, f.ctx)
	fresh, err := s.Refresh(f.ctx, before.WindowRevision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.WindowRevision == stale.SnapshotRevision || fresh.Epoch != stale.Epoch {
		t.Fatal("snapshot não distingue entrega antiga")
	}
	s.conversation.mu.Lock()
	s.conversation.publishCanonicalChanges(f.conversation, nil, []string{fresh.OrderedItemIDs[0]})
	s.conversation.mu.Unlock()
	d := nextDelivery(t, s, f.ctx)
	sequence, _ := strconv.ParseUint(fresh.Sequence, 10, 64)
	if d.SyncRequired || d.SnapshotRevision != fresh.WindowRevision || len(d.Batches) != 1 || d.Batches[0].Sequence != fmt.Sprint(sequence+1) {
		t.Fatalf("refresh não retomou da nova base: %+v", d)
	}
}

func TestTimelineDeliveryTracksSuccessiveSplicesAcrossSurfaces(t *testing.T) {
	f, p := membershipFixture(t)
	history, fixed := membershipSubscribe(t, f, p, "history", 2)
	tail, initial := membershipSubscribe(t, f, p, "tail", 2)
	initial = followTail(t, f, tail, initial, 2)
	ids, revision := append([]string{}, initial.OrderedItemIDs...), initial.WindowRevision
	var delivered []CanonicalBatch
	for i, id := range []string{"new", "newest"} {
		membershipWrite(t, f, p, id, 30+i)
		d := nextDelivery(t, tail, f.ctx)
		if d.SyncRequired || len(d.Batches) == 0 || d.SnapshotRevision != initial.WindowRevision {
			t.Fatalf("cauda perdeu atualização: %+v", d)
		}
		delivered = append(delivered, d.Batches...)
		for _, batch := range d.Batches {
			for _, patch := range batch.WindowPatches {
				if patch.PreviousRevision != revision || patch.WindowID != tail.windowID {
					t.Fatal("revisão/roteamento divergente")
				}
				revision = patch.Revision
				ids = applyModeTestSplices(ids, patch.Splices)
			}
		}
		h := nextDelivery(t, history, f.ctx)
		if h.SyncRequired || h.SnapshotRevision != fixed.WindowRevision {
			t.Fatal("histórico perdeu base")
		}
		for _, batch := range h.Batches {
			for _, patch := range batch.WindowPatches {
				if len(patch.Splices) != 0 || patch.WindowID != history.windowID {
					t.Fatal("histórico moveu ou recebeu patch da cauda")
				}
			}
		}
	}
	if !reflect.DeepEqual(ids, []string{"message:new", "message:newest"}) || !reflect.DeepEqual(delivered, membershipReplay(t, f, tail, initial)) {
		t.Fatal("entrega e replay não reconstruíram a mesma cauda")
	}
}
