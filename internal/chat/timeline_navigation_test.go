package chat

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/database"
)

func navigationFrom(snapshot *SubscribedTimelineSnapshot) TimelineWindowNavigation {
	return TimelineWindowNavigation{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, Anchor: "end", Direction: "before", Limit: 1}
}

func TestTimelineNavigationPreservesWindowAndOtherSurface(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "navigation"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}
	sub, initial, err := p.SubscribeTimeline(f.ctx, session, "moving", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	other, fixed, err := p.SubscribeTimeline(f.ctx, session, "fixed", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	request := navigationFrom(initial)
	request.Anchor, request.Cursor, request.Direction = "", initial.AfterCursor, "after"
	next, err := sub.Navigate(f.ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if next.WindowID != initial.WindowID || next.WindowRevision != "2" || !reflect.DeepEqual(next.OrderedItemIDs, []string{"turn:" + f.question}) {
		t.Fatalf("paginação não preservou identidade/ordem: %+v", next)
	}
	window := sub.conversation.windows[other.windowID]
	if window.revision != 1 || !window.itemIDs[fixed.OrderedItemIDs[0]] || len(window.itemIDs) != 1 {
		t.Fatal("navegação alterou outra superfície", window)
	}
	fresh, err := sub.Refresh(f.ctx)
	if err != nil || !reflect.DeepEqual(fresh.OrderedItemIDs, next.OrderedItemIDs) || fresh.WindowRevision != "3" {
		t.Fatalf("refresh perdeu consulta paginada: %+v %v", fresh, err)
	}
	if _, err := sub.Navigate(f.ctx, navigationFrom(next)); !errors.Is(err, ErrTimelineWindowStale) {
		t.Fatalf("snapshot anterior ao refresh aceito: %v", err)
	}
}

func TestTimelineNavigationRejectsConcurrentStaleRequests(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	start, outcomes := make(chan struct{}), make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; _, err := sub.Navigate(f.ctx, navigationFrom(initial)); outcomes <- err }()
	}
	close(start)
	success, stale := 0, 0
	for i := 0; i < 2; i++ {
		err := <-outcomes
		if err == nil {
			success++
		} else if errors.Is(err, ErrTimelineWindowStale) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("pedidos concorrentes: success=%d stale=%d", success, stale)
	}
}

func TestTimelineNavigationFailurePreservesMembershipAndQuery(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "s"}
	sub, initial, err := p.SubscribeTimeline(f.ctx, session, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	query := sub.query
	request := navigationFrom(initial)
	request.Anchor, request.Cursor = "", "invalid-cursor"
	if _, err := sub.Navigate(f.ctx, request); !errors.Is(err, database.ErrTimelineCursorInvalid) {
		t.Fatalf("cursor inválido: %v", err)
	}
	window := sub.conversation.windows[sub.windowID]
	if window.revision != 1 || !window.itemIDs[initial.OrderedItemIDs[0]] || !reflect.DeepEqual(query, sub.query) {
		t.Fatal("falha alterou inscrição")
	}
	session.denied = true
	if _, err := sub.Navigate(f.ctx, navigationFrom(initial)); err == nil {
		t.Fatal("sessão revogada navegou")
	}
	session.denied = false
	sub.Close()
	if _, err := sub.Navigate(f.ctx, navigationFrom(initial)); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatalf("inscrição fechada: %v", err)
	}
}

type navigationReaderFunc func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error)

func (read navigationReaderFunc) ReadCanonicalWindow(ctx context.Context, q database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	return read(ctx, q)
}

func TestTimelineNavigationDetectsEpochChangedDuringRead(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	sub.reader = navigationReaderFunc(func(ctx context.Context, q database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
		snapshot, err := f.reader.ReadCanonicalWindow(ctx, q)
		if err == nil {
			snapshot.Version.Incarnation = "replacement"
		}
		return snapshot, err
	})
	if _, err := sub.Navigate(f.ctx, navigationFrom(initial)); !errors.Is(err, ErrTimelineWindowStale) {
		t.Fatalf("navegação atravessou época: %v", err)
	}
	window := sub.conversation.windows[sub.windowID]
	if window.revision != 1 || !window.itemIDs[initial.OrderedItemIDs[0]] || sub.query.Anchor != "start" {
		t.Fatal("época recusada alterou inscrição")
	}
}

func TestTimelineNavigationKeepsThreadScopeAndRejectsRemovedAnchor(t *testing.T) {
	f := newSnapshotFixture(t)
	child, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "thread"})
	if err != nil {
		t.Fatal(err)
	}
	p := NewLiveProjection(f.reader)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "s"}
	root, rootSnapshot, err := p.SubscribeTimeline(f.ctx, session, "root", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	sub, initial, err := p.SubscribeTimeline(f.ctx, session, "thread", database.MessageWindowQuery{ConversationID: f.conversation, ParentID: &f.question, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	request := navigationFrom(initial)
	request.Anchor, request.Cursor = "", rootSnapshot.AfterCursor
	if _, err := sub.Navigate(f.ctx, request); !errors.Is(err, database.ErrTimelineCursorInvalid) {
		t.Fatalf("cursor da raiz atravessou escopo: %v", err)
	}
	next, err := sub.Navigate(f.ctx, navigationFrom(initial))
	if err != nil || !reflect.DeepEqual(next.OrderedItemIDs, []string{"message:" + child.ID}) {
		t.Fatalf("navegação saiu da thread: %+v %v", next, err)
	}
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, database.DeleteMessageWithContext(f.ctx, child.ID) }); err != nil {
		t.Fatal(err)
	}
	request = navigationFrom(next)
	request.Anchor, request.Cursor = "", next.BeforeCursor
	if _, err := sub.Navigate(f.ctx, request); !errors.Is(err, database.ErrTimelineCursorStale) {
		t.Fatalf("âncora excluída não pediu recuperação: %v", err)
	}
	fresh, err := sub.Refresh(f.ctx)
	if err != nil || len(fresh.Items) != 0 {
		t.Fatalf("recuperação após exclusão: %+v %v", fresh, err)
	}
}
