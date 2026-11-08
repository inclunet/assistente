package wailsapi

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/testutil/dbfixture"
)

type timelineTestSession struct {
	mu            sync.Mutex
	user, session string
	emit          func(context.Context, apidto.TimelineEvent) error
	eventContext  context.Context
}

func (s *timelineTestSession) CaptureTimelineEmitter(ctx context.Context) (context.Context, TimelineEventEmitter, error) {
	if s.eventContext != nil {
		ctx = s.eventContext
	}
	emit := s.emit
	if emit == nil {
		emit = func(context.Context, apidto.TimelineEvent) error { return nil }
	}
	return ctx, timelineEventTestEmitter{emit: emit, closed: make(chan struct{})}, nil
}

func (s *timelineTestSession) AuthenticatedContext() (context.Context, error) {
	return s.AuthenticatedTimelineContext(context.Background())
}
func (s *timelineTestSession) AuthenticatedTimelineContext(parent context.Context) (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.user == "" {
		return nil, database.ErrUserScopeRequired
	}
	return database.WithUserID(parent, s.user), parent.Err()
}
func (s *timelineTestSession) AdminContext() (context.Context, error) {
	return s.AuthenticatedContext()
}

type timelineTestPrincipal struct {
	source        *timelineTestSession
	user, session string
}

func (s *timelineTestSession) CaptureTimelineSession(ctx context.Context) (chat.TimelineSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := database.RequireUserID(ctx)
	if err != nil || u != s.user {
		return nil, chat.ErrTimelineSubscription
	}
	return timelineTestPrincipal{s, s.user, s.session}, nil
}
func (p timelineTestPrincipal) AuthorizeTimeline(ctx context.Context) (context.Context, string, error) {
	p.source.mu.Lock()
	defer p.source.mu.Unlock()
	u, err := database.RequireUserID(ctx)
	if err != nil || u != p.user || p.source.user != p.user || p.source.session != p.session {
		return nil, "", chat.ErrTimelineSubscription
	}
	return ctx, p.session, ctx.Err()
}

type timelineAPIFixture struct {
	api                   *ChatTimeline
	session               *timelineTestSession
	projection            *chat.LiveProjection
	reader                *chat.DBTimelineReader
	ctx                   context.Context
	conversation, message string
}

func newTimelineAPIFixture(t *testing.T) timelineAPIFixture {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	dbfixture.Open(t)
	if err := database.DB().Create(&database.User{UUIDModel: database.UUIDModel{ID: "timeline-user"}, Username: "timeline-user"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "timeline-user")
	conversation, err := database.CreateConversationWithContext(ctx, "timeline", "")
	if err != nil {
		t.Fatal(err)
	}
	message, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conversation.ID, Role: "user", Content: "primeira"})
	if err != nil {
		t.Fatal(err)
	}
	reader := chat.NewDBTimelineReader(database.DB())
	projection := chat.NewLiveProjection(reader)
	session := &timelineTestSession{user: "timeline-user", session: "session"}
	api := NewChatTimeline()
	AttachChatTimeline(api, session, projection, reader)
	t.Cleanup(func() { CloseChatTimeline(api) })
	return timelineAPIFixture{api, session, projection, reader, ctx, conversation.ID, message.ID}
}
func (f timelineAPIFixture) request() apidto.OpenTimelineRequest {
	return apidto.OpenTimelineRequest{ConversationID: f.conversation, SurfaceID: "surface", TimelineWindowPosition: apidto.TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 2}}
}

func TestChatTimelineRPCsUseCanonicalWindowAndReplay(t *testing.T) {
	f := newTimelineAPIFixture(t)
	snapshot, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = f.api.Navigate(snapshot.WindowID, apidto.TimelineWindowNavigation{Mode: apidto.TimelineWindowFollowTail, Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, TimelineWindowPosition: apidto.TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.api.SetRetention(snapshot.WindowID, apidto.TimelineRetentionRequest{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, ItemIDs: []string{"message:" + f.message}}); err != nil {
		t.Fatal(err)
	}
	var nextID string
	err = f.projection.ApplyStorageMutation(f.ctx, f.conversation, []string{}, func() ([]string, error) {
		message, createErr := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "segunda"})
		if createErr != nil {
			return nil, createErr
		}
		nextID = message.ID
		return []string{"message:" + message.ID}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.api.Replay(snapshot.WindowID, apidto.ReplayTimelineRequest{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, AfterSequence: snapshot.Sequence})
	if err != nil || replay.SyncRequired || len(replay.Updates) == 0 {
		t.Fatal("replay não entregou transição", replay, err)
	}
	ids := append([]string{}, snapshot.OrderedItemIDs...)
	revision := snapshot.WindowRevision
	for _, update := range replay.Updates {
		if update.ConversationID != f.conversation {
			t.Fatal("escopo ausente")
		}
		for _, patch := range update.WindowPatches {
			if patch.WindowID != snapshot.WindowID || patch.PreviousRevision != revision || patch.SyncRequired {
				t.Fatal("patch incoerente", patch)
			}
			revision = patch.Revision
			for _, splice := range patch.Splices {
				tail := append([]string{}, ids[splice.Index+splice.DeleteCount:]...)
				ids = append(append(ids[:splice.Index:splice.Index], splice.InsertIDs...), tail...)
			}
		}
	}
	if !reflect.DeepEqual(ids, []string{"message:" + f.message, "message:" + nextID}) {
		t.Fatal("ordem não veio do backend", ids)
	}
	encoded, err := json.Marshal(replay)
	if err != nil || strings.Contains(string(encoded), "operationIndexes") {
		t.Fatal("roteamento interno exposto", err)
	}
	fresh, err := f.api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: revision})
	if err != nil || !reflect.DeepEqual(fresh.OrderedItemIDs, ids) || !reflect.DeepEqual(fresh.RetainedItemIDs, []string{"message:" + f.message}) {
		t.Fatal("refresh divergiu", fresh, err)
	}
	for range 2 {
		if err := f.api.Close(snapshot.WindowID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: fresh.WindowRevision}); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("handle fechado aceito", err)
	}
}

func TestChatTimelineRejectsForeignPrincipalAndInvalidSequence(t *testing.T) {
	f := newTimelineAPIFixture(t)
	snapshot, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "01", "+1", "-1", "18446744073709551616", "1.0"} {
		_, err := f.api.Replay(snapshot.WindowID, apidto.ReplayTimelineRequest{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, AfterSequence: value})
		if !errors.Is(err, ErrTimelineSequence) {
			t.Fatal("sequência inválida aceita", value, err)
		}
	}
	if _, err := f.api.Replay(snapshot.WindowID, apidto.ReplayTimelineRequest{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, AfterSequence: "9007199254740993"}); err != nil {
		t.Fatal("sequência decimal perdeu precisão", err)
	}
	f.session.mu.Lock()
	f.session.session = "other-session"
	f.session.mu.Unlock()
	if err := f.api.Close(snapshot.WindowID); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("sessão alheia fechou capability", err)
	}
	if _, err := f.api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: snapshot.WindowRevision}); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("sessão alheia leu capability", err)
	}
}

type blockedTimelineReader struct {
	*chat.DBTimelineReader
	entered, release chan struct{}
	skip             bool
}

func (r *blockedTimelineReader) ReadCanonicalWindow(ctx context.Context, query database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error) {
	snapshot, err := r.DBTimelineReader.ReadCanonicalWindow(ctx, query)
	if r.skip {
		return snapshot, err
	}
	close(r.entered)
	<-r.release
	return snapshot, err
}

func TestChatTimelinePendingOpenCannotSurviveInvalidation(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "logout", true: "shutdown"}[terminal], func(t *testing.T) {
			f := newTimelineAPIFixture(t)
			blocked := &blockedTimelineReader{DBTimelineReader: f.reader, entered: make(chan struct{}), release: make(chan struct{})}
			release := sync.OnceFunc(func() { close(blocked.release) })
			t.Cleanup(release)
			f.api.mu.Lock()
			f.api.reader = blocked
			f.api.mu.Unlock()
			result := make(chan error, 1)
			go func() {
				snapshot, err := f.api.Open(f.request())
				if snapshot != nil {
					result <- errors.New("snapshot invalidado retornado")
					return
				}
				result <- err
			}()
			select {
			case <-blocked.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("leitura não iniciou")
			}
			if terminal {
				CloseChatTimeline(f.api)
			} else {
				DetachChatTimeline(f.api)()
			}
			f.api.mu.Lock()
			pending := f.api.pending
			f.api.mu.Unlock()
			if pending != 1 {
				t.Fatal("invalidação liberou recurso ainda em uso", pending)
			}
			release()
			select {
			case err := <-result:
				if !errors.Is(err, chat.ErrTimelineSubscription) {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("abertura não terminou")
			}
			f.api.mu.Lock()
			leaked := len(f.api.windows) != 0 || f.api.pending != 0
			f.api.reader = f.reader
			f.api.mu.Unlock()
			if leaked {
				t.Fatal("abertura invalidada reteve capacidade")
			}
			AttachChatTimeline(f.api, f.session, f.projection, f.reader)
			_, err := f.api.Open(f.request())
			if terminal && !errors.Is(err, ErrChatTimelineNotWired) {
				t.Fatal("attach reabriu registry encerrada", err)
			}
			if !terminal && err != nil {
				t.Fatal("logout impediu nova inscrição", err)
			}
		})
	}
}

func TestChatTimelinePendingOpenRejectsExpiredEventAuthorization(t *testing.T) {
	f := newTimelineAPIFixture(t)
	eventCtx, cancelEvents := context.WithCancel(f.ctx)
	defer cancelEvents()
	f.session.eventContext = eventCtx
	blocked := &blockedTimelineReader{DBTimelineReader: f.reader, entered: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(blocked.release) })
	t.Cleanup(release)
	f.api.reader = blocked
	result := make(chan error, 1)
	go func() {
		snapshot, err := f.api.Open(f.request())
		if snapshot != nil {
			result <- errors.New("Open devolveu capability com autorização encerrada")
			return
		}
		result <- err
	}()
	waitTimelineEventTest(t, blocked.entered)
	cancelEvents() // revogação externa não altera a geração local da registry
	release()
	select {
	case err := <-result:
		if !errors.Is(err, chat.ErrTimelineSubscription) {
			t.Fatal("abertura com autorização encerrada não foi recusada", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("abertura não terminou")
	}
	f.api.mu.Lock()
	count, pending := len(f.api.windows), f.api.pending
	f.api.mu.Unlock()
	if count != 0 || pending != 0 {
		t.Fatalf("abertura recusada reteve cota: handles=%d pending=%d", count, pending)
	}
}

type gatedTimelineSession struct {
	*timelineTestSession
	entered, release chan struct{}
}

func (s *gatedTimelineSession) AuthenticatedContext() (context.Context, error) {
	s.entered <- struct{}{}
	<-s.release
	return s.timelineTestSession.AuthenticatedContext()
}
func TestChatTimelineCapacityIncludesPendingOpens(t *testing.T) {
	f := newTimelineAPIFixture(t)
	gate := &gatedTimelineSession{f.session, make(chan struct{}, maxTimelineHandles), make(chan struct{})}
	release := sync.OnceFunc(func() { close(gate.release) })
	t.Cleanup(release)
	f.api.session = gate
	results := make(chan error, maxTimelineHandles)
	for range maxTimelineHandles {
		go func() { _, err := f.api.Open(f.request()); results <- err }()
	}
	for range maxTimelineHandles {
		select {
		case <-gate.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("abertura não iniciou")
		}
	}
	if _, err := f.api.Open(f.request()); !errors.Is(err, chat.ErrTimelineSubscriptionLimit) {
		t.Fatal("aberturas pendentes escaparam do limite", err)
	}
	DetachChatTimeline(f.api)()
	if _, err := f.api.Open(f.request()); !errors.Is(err, chat.ErrTimelineSubscriptionLimit) {
		t.Fatal("invalidação liberou reservas ainda em uso", err)
	}
	release()
	for range maxTimelineHandles {
		select {
		case err := <-results:
			if !errors.Is(err, chat.ErrTimelineSubscription) {
				t.Fatal("abertura invalidada aceita", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("abertura não terminou")
		}
	}
	if _, err := f.api.Open(f.request()); err != nil {
		t.Fatal("reservas concluídas não liberaram capacidade", err)
	}
}

func TestChatTimelineRefreshDiscardsResultAfterInvalidation(t *testing.T) {
	f := newTimelineAPIFixture(t)
	blocked := &blockedTimelineReader{DBTimelineReader: f.reader, entered: make(chan struct{}), release: make(chan struct{}), skip: true}
	release := sync.OnceFunc(func() { close(blocked.release) })
	t.Cleanup(release)
	f.api.reader = blocked
	snapshot, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	blocked.skip = false
	result := make(chan error, 1)
	go func() {
		fresh, err := f.api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: snapshot.WindowRevision})
		if fresh != nil {
			result <- errors.New("refresh devolveu conteúdo após invalidação")
			return
		}
		result <- err
	}()
	select {
	case <-blocked.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh não iniciou")
	}
	cleanup := DetachChatTimeline(f.api)
	release()
	select {
	case err := <-result:
		if !errors.Is(err, chat.ErrTimelineSubscription) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh não terminou")
	}
	cleanup()
}
func TestChatTimelineEmptyScopedBatchStillAdvancesSequence(t *testing.T) {
	f := newTimelineAPIFixture(t)
	snapshot, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	childID := ""
	err = f.projection.ApplyStorageMutation(f.ctx, f.conversation, []string{}, func() ([]string, error) {
		child, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.message, Role: "user", Content: "thread separada"})
		if err != nil {
			return nil, err
		}
		childID = child.ID
		return []string{"message:" + child.ID}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.api.Replay(snapshot.WindowID, apidto.ReplayTimelineRequest{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, AfterSequence: snapshot.Sequence})
	if err != nil || replay.SyncRequired || len(replay.Updates) != 1 {
		t.Fatal("lote vazio descartado", replay, err)
	}
	update := replay.Updates[0]
	if len(update.Operations) != 1 || update.Operations[0].Item == nil || update.Operations[0].ItemID != "message:"+f.message || update.Operations[0].Item.Metadata.ChildCount != 1 {
		t.Fatal("criação de filho não atualizou somente a contagem do pai", update)
	}
	if err := f.projection.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:" + childID}, func() ([]string, error) {
		return nil, database.UpdateMessageTextWithContext(f.ctx, childID, "conteúdo privado da thread alterado")
	}); err != nil {
		t.Fatal(err)
	}
	replay, err = f.api.Replay(snapshot.WindowID, apidto.ReplayTimelineRequest{Epoch: snapshot.Epoch, WindowRevision: snapshot.WindowRevision, AfterSequence: update.Sequence})
	if err != nil || replay.SyncRequired || len(replay.Updates) != 1 {
		t.Fatal("lote vazio descartado", replay, err)
	}
	previous := update.Sequence
	update = replay.Updates[0]
	if update.Sequence == previous || update.Operations == nil || len(update.Operations) != 0 || len(update.WindowPatches) != 0 {
		t.Fatal("lote fora do escopo vazou conteúdo ou perdeu sequência", update)
	}
}

func TestChatTimelineFailsClosedWithoutWiringOrAuthentication(t *testing.T) {
	api := NewChatTimeline()
	checks := []func() error{
		func() error { _, err := api.Open(apidto.OpenTimelineRequest{}); return err },
		func() error { _, err := api.Refresh("missing", apidto.RefreshTimelineRequest{}); return err },
		func() error { _, err := api.Navigate("missing", apidto.TimelineWindowNavigation{}); return err },
		func() error { _, err := api.SetRetention("missing", apidto.TimelineRetentionRequest{}); return err },
		func() error { _, err := api.Replay("missing", apidto.ReplayTimelineRequest{}); return err },
		func() error { return api.Close("missing") },
	}
	for _, check := range checks {
		if err := check(); !errors.Is(err, ErrChatTimelineNotWired) {
			t.Fatal(err)
		}
	}
	session := &timelineTestSession{}
	reader := new(chat.DBTimelineReader)
	AttachChatTimeline(api, session, chat.NewLiveProjection(reader), reader)
	for _, check := range checks {
		if err := check(); !errors.Is(err, database.ErrUserScopeRequired) {
			t.Fatal("operação sem autenticação aceita", err)
		}
	}
	if api.pending != 0 || len(api.windows) != 0 {
		t.Fatal("recusa de autenticação consumiu capacidade")
	}
}
