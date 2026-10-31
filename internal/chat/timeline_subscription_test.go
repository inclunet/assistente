package chat

import (
	"assistente/internal/database"
	"context"
	"errors"
	"fmt"
	"testing"
)

type fixtureTimelineSession struct {
	ctx    context.Context
	id     string
	denied bool
}

func (s *fixtureTimelineSession) AuthorizeTimeline(context.Context) (context.Context, string, error) {
	if s.denied {
		return nil, "", errors.New("revoked")
	}
	return s.ctx, s.id, nil
}

func TestTimelineSubscriptionsIsolateSurfacesAndSession(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection()
	session := &fixtureTimelineSession{ctx: f.ctx, id: "session"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}
	first, a, err := p.SubscribeTimeline(f.ctx, session, "history", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	query.Anchor = "end"
	query.Direction = "before"
	second, b, err := p.SubscribeTimeline(f.ctx, session, "tail", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	if a.WindowID == b.WindowID || a.OrderedItemIDs[0] == b.OrderedItemIDs[0] {
		t.Fatal("janelas misturadas")
	}
	batches, ok, err := first.Replay(f.ctx, a.Epoch, 0, f.reader)
	if err != nil || !ok {
		t.Fatalf("replay: %v %v", ok, err)
	}
	found := 0
	for _, batch := range batches {
		for _, op := range batch.Operations {
			if op.ItemID != a.OrderedItemIDs[0] {
				t.Fatal("item de outra janela")
			}
			found++
		}
	}
	if found == 0 {
		t.Fatal("replay sem operações da janela assinada")
	}
	first.Close()
	first.Close()
	if _, err := first.Refresh(f.ctx); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatalf("fechada: %v", err)
	}
	if _, err := second.Refresh(f.ctx); err != nil {
		t.Fatal("fechar uma fechou outra", err)
	}
	session.id = "new-session"
	if _, err := second.Refresh(f.ctx); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatalf("sessão trocada: %v", err)
	}
	session.id = "session"
	session.denied = true
	if _, _, err := second.Replay(f.ctx, b.Epoch, 0, f.reader); err == nil {
		t.Fatal("revogação ignorada")
	}
	second.Close()
}

func TestTimelineSubscriptionInvalidatesReplayAfterStorageChange(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection()
	session := &fixtureTimelineSession{ctx: f.ctx, id: "s"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 2}
	sub, snapshot, err := p.SubscribeTimeline(f.ctx, session, "surface", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if err := database.DeleteMessageWithContext(f.ctx, f.question); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := sub.Replay(f.ctx, snapshot.Epoch, 0, f.reader); err != nil || ok {
		t.Fatalf("replay ressuscitou exclusão: %v %v", ok, err)
	}
	fresh, err := sub.Refresh(f.ctx)
	if err != nil || len(fresh.Items) != 0 {
		t.Fatalf("recuperação: %+v %v", fresh, err)
	}
	// A janela nova vazia não devolve operações antigas de itens removidos.
	batches, ok, err := sub.Replay(f.ctx, fresh.Epoch, 0, f.reader)
	if err != nil || !ok {
		t.Fatalf("replay vazio: %v %v", ok, err)
	}
	for _, batch := range batches {
		if len(batch.Operations) != 0 {
			t.Fatal("item removido no replay")
		}
	}
}

func TestTimelineSubscriptionSnapshotDoesNotMutatePublication(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection()
	session := &fixtureTimelineSession{ctx: f.ctx, id: "s"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}
	sub, snapshot, err := p.SubscribeTimeline(f.ctx, session, "surface", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	snapshot.Items[0].Segments[0].Text = "corrompido"
	fresh, err := sub.Refresh(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Items[0].Segments[0].Text == "corrompido" {
		t.Fatal("snapshot corrompeu cache")
	}
	if _, _, err := p.SubscribeTimeline(f.ctx, session, "surface", query, f.reader); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatalf("superfície duplicada: %v", err)
	}
}

func TestTimelineSubscriptionLimitReleasedByClose(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection()
	session := &fixtureTimelineSession{ctx: f.ctx, id: "s"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}
	handles := make([]*TimelineSubscription, 0, 64)
	defer func() {
		for _, handle := range handles {
			handle.Close()
		}
	}()
	for i := 0; i < 64; i++ {
		handle, _, err := p.SubscribeTimeline(f.ctx, session, fmt.Sprint(i), query, f.reader)
		if err != nil {
			t.Fatal(i, err)
		}
		handles = append(handles, handle)
	}
	if _, _, err := p.SubscribeTimeline(f.ctx, session, "overflow", query, f.reader); !errors.Is(err, ErrTimelineSubscriptionLimit) {
		t.Fatalf("limite: %v", err)
	}
	handles[0].Close()
	handle, _, err := p.SubscribeTimeline(f.ctx, session, "replacement", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	session.ctx = database.WithUserID(context.Background(), "other-user")
	if _, err := handle.Refresh(f.ctx); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatalf("owner trocado: %v", err)
	}
}
