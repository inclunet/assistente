package wailsapi

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
)

// ChatTimelineSession captura o principal autenticado no transporte confiável.
// O caller não fornece owner nem identificador de sessão no payload.
type ChatTimelineSession interface {
	Session
	CaptureTimelineSession(context.Context) (chat.TimelineSession, error)
	CaptureTimelineEmitter(context.Context) (context.Context, TimelineEventEmitter, error)
}

type ChatTimelineReader interface {
	chat.CanonicalWindowSnapshotReader
	chat.TurnSnapshotReader
}

var ErrChatTimelineNotWired = errors.New("timeline do chat indisponível")
var ErrTimelineSequence = errors.New("sequência de timeline inválida")

const maxTimelineHandles = 256

// ChatTimeline conserva capabilities, não uma segunda projeção dos itens.
type ChatTimeline struct {
	mu         sync.Mutex
	session    ChatTimelineSession
	projection *chat.LiveProjection
	reader     ChatTimelineReader
	generation uint64
	pending    int
	closed     bool
	windows    map[string]timelineHandle
}

type timelineHandle struct {
	stream         *timelineEventStream
	subscription   *chat.TimelineSubscription
	principal      timelinePrincipal
	reader         ChatTimelineReader
	conversationID string
}

type timelinePrincipal struct {
	api        *ChatTimeline
	generation uint64
	captured   chat.TimelineSession
}

func (p timelinePrincipal) AuthorizeTimeline(ctx context.Context) (context.Context, string, error) {
	authorized, sessionID, err := p.captured.AuthorizeTimeline(ctx)
	if err != nil {
		return nil, "", err
	}
	p.api.mu.Lock()
	valid := !p.api.closed && p.api.generation == p.generation
	p.api.mu.Unlock()
	if !valid {
		return nil, "", chat.ErrTimelineSubscription
	}
	return authorized, sessionID, nil
}

func NewChatTimeline() *ChatTimeline { return &ChatTimeline{windows: make(map[string]timelineHandle)} }

// O attach pertence ao bootstrap; nunca reabre uma registry encerrada.
func AttachChatTimeline(api *ChatTimeline, session ChatTimelineSession, projection *chat.LiveProjection, reader ChatTimelineReader) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.closed || api.projection != nil {
		return
	}
	api.session, api.projection, api.reader = session, projection, reader
}

// Detach invalida inclusive aberturas pendentes. O callback devolvido fecha as
// capabilities fora dos locks de autenticação/registry do chamador.
func DetachChatTimeline(api *ChatTimeline) func() { return detachChatTimeline(api, false) }

func CloseChatTimeline(api *ChatTimeline) { detachChatTimeline(api, true)() }

func detachChatTimeline(api *ChatTimeline, terminal bool) func() {
	if api == nil {
		return func() {}
	}
	api.mu.Lock()
	api.closed = api.closed || terminal
	api.generation++
	windows := api.windows
	api.windows = make(map[string]timelineHandle)
	api.mu.Unlock()
	return func() {
		for _, window := range windows {
			window.stream.cancel()
		}
		for _, window := range windows {
			window.stream.close()
		}
	}
}

func (api *ChatTimeline) reserve() (ChatTimelineSession, *chat.LiveProjection, ChatTimelineReader, uint64, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.closed || api.session == nil || api.projection == nil || api.reader == nil {
		return nil, nil, nil, 0, ErrChatTimelineNotWired
	}
	if len(api.windows)+api.pending >= maxTimelineHandles {
		return nil, nil, nil, 0, chat.ErrTimelineSubscriptionLimit
	}
	api.pending++
	return api.session, api.projection, api.reader, api.generation, nil
}

func (api *ChatTimeline) Open(request apidto.OpenTimelineRequest) (*apidto.SubscribedTimelineSnapshot, error) {
	session, projection, reader, generation, err := api.reserve()
	if err != nil {
		return nil, err
	}
	defer func() {
		api.mu.Lock()
		api.pending--
		api.mu.Unlock()
	}()
	return WithUser(session, func(ctx context.Context) (*apidto.SubscribedTimelineSnapshot, error) {
		captured, err := session.CaptureTimelineSession(ctx)
		if err != nil {
			return nil, err
		}
		if captured == nil {
			return nil, chat.ErrTimelineSubscription
		}
		principal := timelinePrincipal{api: api, generation: generation, captured: captured}
		eventCtx, emitter, err := session.CaptureTimelineEmitter(ctx)
		if err != nil {
			return nil, err
		}
		owned := false
		defer func() {
			if !owned {
				emitter.Close()
			}
		}()
		query := database.MessageWindowQuery{ConversationID: request.ConversationID, ParentID: request.ScopeParentID, Cursor: request.Cursor, Anchor: request.Anchor, AnchorItemID: request.AnchorItemID, Direction: request.Direction, Limit: request.Limit}
		subscription, snapshot, err := projection.SubscribeTimeline(ctx, principal, request.SurfaceID, query, reader)
		if err != nil {
			return nil, err
		}
		if _, _, err = principal.AuthorizeTimeline(ctx); err != nil {
			subscription.Close()
			return nil, err
		}
		api.mu.Lock()
		valid := !api.closed && api.generation == generation && eventCtx.Err() == nil
		if valid {
			var stream *timelineEventStream
			stream = newTimelineEventStream(eventCtx, subscription, timelineAuthorizedEmitter{principal, emitter}, snapshot, func() {
				api.mu.Lock()
				if current, ok := api.windows[snapshot.WindowID]; ok && current.stream == stream {
					delete(api.windows, snapshot.WindowID)
				}
				api.mu.Unlock()
			})
			api.windows[snapshot.WindowID] = timelineHandle{stream: stream, subscription: subscription, principal: principal, reader: reader, conversationID: request.ConversationID}
			owned = true
		}
		api.mu.Unlock()
		if !valid {
			subscription.Close()
			return nil, chat.ErrTimelineSubscription
		}
		return snapshot, nil
	})
}

func withTimelineHandle[T any](api *ChatTimeline, windowID string, closedOK bool, fn func(context.Context, timelineHandle) (T, error)) (T, error) {
	var zero T
	api.mu.Lock()
	session, closed := api.session, api.closed
	window, found := api.windows[windowID]
	api.mu.Unlock()
	if closed || session == nil {
		return zero, ErrChatTimelineNotWired
	}
	return WithUser(session, func(ctx context.Context) (T, error) {
		if !found {
			if closedOK {
				return zero, nil
			}
			return zero, chat.ErrTimelineSubscription
		}
		if _, _, err := window.principal.AuthorizeTimeline(ctx); err != nil {
			return zero, err
		}
		result, err := fn(ctx, window)
		if err != nil {
			return zero, err
		}
		if _, _, err := window.principal.AuthorizeTimeline(ctx); err != nil {
			return zero, err
		}
		return result, nil
	})
}

func (api *ChatTimeline) Refresh(windowID string, request apidto.RefreshTimelineRequest) (*apidto.SubscribedTimelineSnapshot, error) {
	return withTimelineHandle(api, windowID, false, func(ctx context.Context, w timelineHandle) (*apidto.SubscribedTimelineSnapshot, error) {
		return w.stream.replaceSnapshot(ctx, func(queryCtx context.Context) (*apidto.SubscribedTimelineSnapshot, error) {
			return w.subscription.Refresh(queryCtx, request.WindowRevision, request.Position)
		})
	})
}

func (api *ChatTimeline) Navigate(windowID string, request apidto.TimelineWindowNavigation) (*apidto.SubscribedTimelineSnapshot, error) {
	return withTimelineHandle(api, windowID, false, func(ctx context.Context, w timelineHandle) (*apidto.SubscribedTimelineSnapshot, error) {
		return w.stream.replaceSnapshot(ctx, func(queryCtx context.Context) (*apidto.SubscribedTimelineSnapshot, error) {
			return w.subscription.Navigate(queryCtx, request)
		})
	})
}

func (api *ChatTimeline) SetRetention(windowID string, request apidto.TimelineRetentionRequest) error {
	_, err := withTimelineHandle(api, windowID, false, func(ctx context.Context, w timelineHandle) (struct{}, error) {
		return struct{}{}, w.subscription.SetRetention(ctx, request)
	})
	return err
}

func (api *ChatTimeline) Replay(windowID string, request apidto.ReplayTimelineRequest) (*apidto.TimelineReplay, error) {
	return withTimelineHandle(api, windowID, false, func(ctx context.Context, w timelineHandle) (*apidto.TimelineReplay, error) {
		after, err := strconv.ParseUint(request.AfterSequence, 10, 64)
		if err != nil || strconv.FormatUint(after, 10) != request.AfterSequence {
			return nil, ErrTimelineSequence
		}
		batches, ok, err := w.subscription.Replay(ctx, request.Epoch, request.WindowRevision, after, w.reader)
		if err != nil {
			return nil, err
		}
		result := &apidto.TimelineReplay{ConversationID: w.conversationID, WindowID: windowID, SyncRequired: !ok, Updates: timelineUpdates(batches)}
		return result, nil
	})
}

func (api *ChatTimeline) Close(windowID string) error {
	_, err := withTimelineHandle(api, windowID, true, func(_ context.Context, w timelineHandle) (struct{}, error) {
		w.stream.close()
		return struct{}{}, nil
	})
	return err
}
