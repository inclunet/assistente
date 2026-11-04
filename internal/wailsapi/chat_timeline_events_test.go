package wailsapi

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
)

func TestChatTimelineEventsMatchReplayAndReleaseFailedTransport(t *testing.T) {
	f := newTimelineAPIFixture(t)
	events := make(chan apidto.TimelineEvent, 1)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.session.emit = func(ctx context.Context, event apidto.TimelineEvent) error {
		select {
		case events <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-release:
			return errors.New("transporte interrompido")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	snapshot, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	f.api.mu.Lock()
	handle := f.api.windows[snapshot.WindowID]
	f.api.mu.Unlock()
	err = f.projection.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) {
		message, createErr := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "segunda"})
		if createErr != nil {
			return nil, createErr
		}
		return []string{"message:" + message.ID}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.SurfaceID != f.request().SurfaceID || event.ConversationID != f.conversation || event.WindowID != snapshot.WindowID || event.SnapshotRevision != snapshot.WindowRevision || event.Epoch != snapshot.Epoch || event.SyncRequired || len(event.Updates) == 0 {
			t.Fatal("evento fora do contrato", event)
		}
		replay, replayErr := f.api.Replay(snapshot.WindowID, apidto.ReplayTimelineRequest{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, AfterSequence: snapshot.Sequence})
		if replayErr != nil || replay.SyncRequired || !reflect.DeepEqual(event.Updates, replay.Updates) {
			t.Fatal("evento diverge do replay", event, replay, replayErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("publicação não chegou ao transporte")
	}
	once.Do(func() { close(release) })
	select {
	case <-handle.stream.done:
	case <-time.After(3 * time.Second):
		t.Fatal("falha de transporte deixou worker vivo")
	}
	f.api.mu.Lock()
	count := len(f.api.windows)
	f.api.mu.Unlock()
	if count != 0 {
		t.Fatal("falha de transporte não liberou cota")
	}
}

func TestChatTimelineEventCancellationReleasesHandle(t *testing.T) {
	f := newTimelineAPIFixture(t)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	f.session.eventContext = ctx
	snapshot, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	f.api.mu.Lock()
	handle := f.api.windows[snapshot.WindowID]
	f.api.mu.Unlock()
	cancel() // mesmo sinal propagado pela lease após revogação externa
	waitTimelineEventTest(t, handle.stream.done)
	f.api.mu.Lock()
	count := len(f.api.windows)
	f.api.mu.Unlock()
	if count != 0 {
		t.Fatal("autorização encerrada deixou capability/cota registrada")
	}
}

type timelineEventTestFeed struct {
	values chan *chat.TimelineDelivery
	closed chan struct{}
	once   sync.Once
}

func (f *timelineEventTestFeed) Next(ctx context.Context) (*chat.TimelineDelivery, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.closed:
		return nil, chat.ErrTimelineSubscription
	case value := <-f.values:
		return value, nil
	}
}

func (f *timelineEventTestFeed) Close() { f.once.Do(func() { close(f.closed) }) }

type timelineEventTestEmitter struct {
	emit   func(context.Context, apidto.TimelineEvent) error
	closed chan struct{}
}

func (e timelineEventTestEmitter) Emit(ctx context.Context, event apidto.TimelineEvent) error {
	return e.emit(ctx, event)
}
func (e timelineEventTestEmitter) Close() { close(e.closed) }

func TestTimelineEventStreamDiscardsReplacedSnapshot(t *testing.T) {
	emitted := 0
	emitter := timelineEventTestEmitter{emit: func(context.Context, apidto.TimelineEvent) error { emitted++; return nil }}
	ctx, cancel := context.WithCancel(context.Background())
	s := &timelineEventStream{ctx: ctx, cancel: cancel, emitter: emitter, epoch: "epoch", baseline: "1"}
	defer s.cancel()
	old := &chat.TimelineDelivery{Epoch: "epoch", SnapshotRevision: "1"}
	if _, err := s.replaceSnapshot(ctx, func(context.Context) (*apidto.SubscribedTimelineSnapshot, error) {
		return &apidto.SubscribedTimelineSnapshot{Epoch: "epoch", WindowRevision: "2"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.deliver(old); err != nil || emitted != 0 {
		t.Fatal("entrega antiga atravessou snapshot", err)
	}
	if _, err := s.replaceSnapshot(ctx, func(context.Context) (*apidto.SubscribedTimelineSnapshot, error) { return nil, errors.New("failed") }); err == nil {
		t.Fatal("falha perdida")
	}
	if err := s.deliver(&chat.TimelineDelivery{Epoch: "epoch", SnapshotRevision: "2"}); err != nil || emitted != 1 {
		t.Fatal("consulta recusada substituiu base", err)
	}
	if err := s.deliver(&chat.TimelineDelivery{Epoch: "new-epoch", SnapshotRevision: "2", SyncRequired: true, Reason: "epoch_changed"}); err != nil || emitted != 2 {
		t.Fatal("invalidação de época não chegou", err)
	}
}

func TestTimelineEventStreamCloseDrainsEmitterAndReleasesFeed(t *testing.T) {
	feed := &timelineEventTestFeed{values: make(chan *chat.TimelineDelivery, 1), closed: make(chan struct{})}
	entered, cancelled, release, emitterClosed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	emitter := timelineEventTestEmitter{closed: emitterClosed, emit: func(ctx context.Context, event apidto.TimelineEvent) error {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		return ctx.Err()
	}}
	released := make(chan struct{})
	s := newTimelineEventStream(context.Background(), feed, emitter, "surface", &apidto.SubscribedTimelineSnapshot{Epoch: "epoch", WindowRevision: "1"}, func() { close(released) })
	feed.values <- &chat.TimelineDelivery{ConversationID: "c", WindowID: "w", Epoch: "epoch", SnapshotRevision: "1"}
	waitTimelineEventTest(t, entered)
	done := make(chan struct{})
	go func() { s.close(); close(done) }()
	waitTimelineEventTest(t, cancelled)
	select {
	case <-done:
		t.Fatal("fechamento retornou com emissão pendente")
	default:
	}
	close(release)
	waitTimelineEventTest(t, done)
	waitTimelineEventTest(t, emitterClosed)
	waitTimelineEventTest(t, feed.closed)
	waitTimelineEventTest(t, released)
	s.close() // idempotente
}

func TestTimelineEventStreamCancellationWakesIdleWorker(t *testing.T) {
	feed := &timelineEventTestFeed{values: make(chan *chat.TimelineDelivery), closed: make(chan struct{})}
	emitter := timelineEventTestEmitter{closed: make(chan struct{}), emit: func(context.Context, apidto.TimelineEvent) error { return errors.New("unexpected") }}
	ctx, cancel := context.WithCancel(context.Background())
	s := newTimelineEventStream(ctx, feed, emitter, "surface", &apidto.SubscribedTimelineSnapshot{Epoch: "epoch", WindowRevision: "1"}, func() {})
	cancel()
	waitTimelineEventTest(t, s.done)
	waitTimelineEventTest(t, emitter.closed)
	waitTimelineEventTest(t, feed.closed)
}

func waitTimelineEventTest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("entrega não encerrou no prazo")
	}
}

func TestTimelineEventStreamCloseCancelsSnapshotQuery(t *testing.T) {
	feed := &timelineEventTestFeed{values: make(chan *chat.TimelineDelivery), closed: make(chan struct{})}
	emitter := timelineEventTestEmitter{closed: make(chan struct{}), emit: func(context.Context, apidto.TimelineEvent) error { return errors.New("unexpected") }}
	s := newTimelineEventStream(context.Background(), feed, emitter, "surface", &apidto.SubscribedTimelineSnapshot{Epoch: "epoch", WindowRevision: "1"}, func() {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // libera a consulta mesmo se a regressão voltar a bloquear Close
	entered, queryDone, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var queryErr error
	go func() {
		_, queryErr = s.replaceSnapshot(ctx, func(queryCtx context.Context) (*apidto.SubscribedTimelineSnapshot, error) {
			close(entered)
			<-queryCtx.Done()
			return nil, queryCtx.Err()
		})
		close(queryDone)
	}()
	waitTimelineEventTest(t, entered)
	go func() { s.close(); close(closed) }()
	waitTimelineEventTest(t, queryDone)
	waitTimelineEventTest(t, closed)
	if !errors.Is(queryErr, context.Canceled) || ctx.Err() != nil {
		t.Fatal("Close não cancelou somente a consulta da inscrição", queryErr, ctx.Err())
	}
}
