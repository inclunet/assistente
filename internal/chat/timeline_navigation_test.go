package chat

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
)

func navigationFrom(snapshot *SubscribedTimelineSnapshot) TimelineWindowNavigation {
	return TimelineWindowNavigation{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, TimelineWindowPosition: TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 1}}
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
	fresh, err := sub.Refresh(f.ctx, next.WindowRevision, nil)
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
	fresh, err := sub.Refresh(f.ctx, next.WindowRevision, nil)
	if err != nil || len(fresh.Items) != 0 {
		t.Fatalf("recuperação após exclusão: %+v %v", fresh, err)
	}
}

func TestTimelineNavigationRecoversAcceptedCursorAfterAnchorRemoval(t *testing.T) {
	for _, changeEpoch := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-epoch", true: "new-epoch"}[changeEpoch], func(t *testing.T) {
			f := newSnapshotFixture(t)
			p := NewLiveProjection(f.reader)
			sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			request := navigationFrom(initial)
			request.Anchor, request.Cursor, request.Direction = "", initial.AfterCursor, "after"
			page, err := sub.Navigate(f.ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) {
				if err := database.DeleteMessageWithContext(f.ctx, f.question); err != nil {
					return nil, err
				}
				if changeEpoch {
					return nil, f.db.Exec("UPDATE chat_timeline_versions SET incarnation=lower(hex(randomblob(16))) WHERE conversation_id=?", f.conversation).Error
				}
				return nil, nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := sub.Refresh(f.ctx, page.WindowRevision, nil); !errors.Is(err, database.ErrTimelineCursorStale) {
				t.Fatalf("cursor aceito deveria estar obsoleto: %v", err)
			}
			if changeEpoch {
				if _, err := sub.Navigate(f.ctx, navigationFrom(page)); !errors.Is(err, ErrTimelineWindowStale) {
					t.Fatalf("época antiga navegou: %v", err)
				}
			}
			fresh, err := sub.Refresh(f.ctx, page.WindowRevision, &TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 1})
			if err != nil || fresh.WindowID != page.WindowID || len(fresh.Items) != 0 || fresh.WindowRevision == page.WindowRevision {
				t.Fatalf("recuperação explícita: %+v %v", fresh, err)
			}
			if changeEpoch && fresh.Epoch == page.Epoch {
				t.Fatal("recuperação conservou época antiga")
			}
		})
	}
}

func TestTimelineNavigationRejectsReplayWaitingForPreviousPage(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	reading, release := make(chan struct{}), make(chan struct{})
	sub.reader = navigationReaderFunc(func(ctx context.Context, q database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
		close(reading)
		<-release
		return f.reader.ReadCanonicalWindow(ctx, q)
	})
	type navigationResult struct {
		snapshot *SubscribedTimelineSnapshot
		err      error
	}
	navigated := make(chan navigationResult, 1)
	go func() {
		snapshot, err := sub.Navigate(f.ctx, navigationFrom(initial))
		navigated <- navigationResult{snapshot, err}
	}()
	<-reading
	replayStarted, replayed := make(chan struct{}), make(chan error, 1)
	go func() {
		close(replayStarted)
		batches, ok, err := sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader)
		if ok || len(batches) != 0 {
			replayed <- errors.New("replay antigo devolveu dados da nova página")
			return
		}
		replayed <- err
	}()
	<-replayStarted
	close(release)
	result := <-navigated
	if result.err != nil {
		t.Fatal(result.err)
	}
	if err := <-replayed; !errors.Is(err, ErrTimelineWindowStale) {
		t.Fatalf("replay antigo: %v", err)
	}
	current := result.snapshot
	batches, ok, err := sub.Replay(f.ctx, current.Epoch, current.WindowRevision, 0, f.reader)
	if err != nil || !ok {
		t.Fatalf("replay atual recusado: %v", err)
	}
	for _, batch := range batches {
		for _, op := range batch.Operations {
			if op.ItemID != "turn:"+f.question {
				t.Fatal("replay atual contém página antiga", op.ItemID)
			}
		}
	}
}

type revocableNavigationSession struct {
	ctx     context.Context
	revoked atomic.Bool
}

func (s *revocableNavigationSession) AuthorizeTimeline(context.Context) (context.Context, string, error) {
	if s.revoked.Load() {
		return nil, "", errors.New("revoked")
	}
	return s.ctx, "session", nil
}

func TestTimelineNavigationReauthorizesAfterWaitingForWriter(t *testing.T) {
	for _, operation := range []string{"navigate", "refresh", "replay", "subscribe"} {
		t.Run(operation, func(t *testing.T) {
			f := newSnapshotFixture(t)
			p := NewLiveProjection(f.reader)
			session := &revocableNavigationSession{ctx: f.ctx}
			query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}
			sub, initial, err := p.SubscribeTimeline(f.ctx, session, "surface", query, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			c, unlock := p.lockConversation(sub.key, false)
			var released atomic.Bool
			release := func() {
				if released.CompareAndSwap(false, true) {
					unlock()
				}
			}
			defer release()
			outcome := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "navigate":
					_, err = sub.Navigate(f.ctx, navigationFrom(initial))
				case "refresh":
					_, err = sub.Refresh(f.ctx, initial.WindowRevision, &TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 1})
				case "replay":
					_, _, err = sub.Replay(f.ctx, initial.Epoch, initial.WindowRevision, 0, f.reader)
				case "subscribe":
					_, _, err = p.SubscribeTimeline(f.ctx, session, "new-surface", query, f.reader)
				}
				outcome <- err
			}()
			// Espera a reserva real da chamada: com a barreira ainda fechada, ela
			// não pode concluir. Nenhuma duração arbitrária decide a intercalação.
			deadline := time.Now().Add(3 * time.Second)
			for {
				p.mu.Lock()
				waiting := c.readers == 2
				p.mu.Unlock()
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("chamada não aguardou writer")
				}
				runtime.Gosched()
			}
			session.revoked.Store(true)
			release()
			if err := <-outcome; err == nil {
				t.Fatal("sessão revogada leu ou alterou inscrição")
			}
			if len(c.windows) != 1 || c.windows[sub.windowID].revision != 1 {
				t.Fatal("revogação alterou janela")
			}
		})
	}
}

func TestTimelineNavigationRecoveryCannotOverwriteNewerPage(t *testing.T) {
	for _, firstOperation := range []string{"navigate", "refresh"} {
		t.Run(firstOperation, func(t *testing.T) {
			f := newSnapshotFixture(t)
			p := NewLiveProjection(f.reader)
			sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			reading, release := make(chan struct{}), make(chan struct{})
			var reads atomic.Int32
			sub.reader = navigationReaderFunc(func(ctx context.Context, q database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
				if reads.Add(1) == 1 {
					close(reading)
					<-release
				}
				return f.reader.ReadCanonicalWindow(ctx, q)
			})
			first, second := make(chan error, 1), make(chan error, 1)
			go func() {
				var err error
				if firstOperation == "navigate" {
					_, err = sub.Navigate(f.ctx, navigationFrom(initial))
				} else {
					_, err = sub.Refresh(f.ctx, initial.WindowRevision, &TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 1})
				}
				first <- err
			}()
			<-reading
			go func() {
				_, err := sub.Refresh(f.ctx, initial.WindowRevision, &TimelineWindowPosition{Anchor: "start", Direction: "after", Limit: 1})
				second <- err
			}()
			close(release)
			if err := <-first; err != nil {
				t.Fatal(err)
			}
			if err := <-second; !errors.Is(err, ErrTimelineWindowStale) {
				t.Fatalf("recuperação antiga sobrescreveu página: %v", err)
			}
			if sub.query.Anchor != "end" || sub.conversation.windows[sub.windowID].revision != 2 || reads.Load() != 1 {
				t.Fatal("recuperação antiga leu ou alterou página")
			}
		})
	}
}
