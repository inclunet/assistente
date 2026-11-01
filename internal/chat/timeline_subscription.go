package chat

import (
	"context"
	"errors"
	"strconv"

	"assistente/internal/database"
	"github.com/google/uuid"
)

// TimelineSession é implementada pelo transporte confiável, que captura e
// revalida a sessão. Nenhum owner/sessionId é aceito no pedido da superfície.
type TimelineSession interface {
	AuthorizeTimeline(context.Context) (context.Context, string, error)
}

var ErrTimelineSubscription = errors.New("inscrição de timeline inválida ou encerrada")
var ErrTimelineSubscriptionLimit = errors.New("limite de inscrições de timeline atingido")

type subscriptionSurface struct{ sessionID, surfaceID string }
type subscribedWindow struct {
	epoch   string
	version database.ChatTimelineVersion
	id      string
	surface subscriptionSurface
	itemIDs map[string]bool
}

// TimelineSubscription é um capability interno. Ainda não é binding Wails.
// Publicação automática e patches de participação seguem separados deste handle.
type TimelineSubscription struct {
	owner        *LiveProjection
	key          projectionKey
	conversation *projectionConversation
	session      TimelineSession
	windowID     string
	sessionID    string
	query        database.MessageWindowQuery
	reader       CanonicalWindowSnapshotReader
}

type SubscribedTimelineSnapshot struct {
	WindowID       string
	Epoch          string
	Sequence       string
	Items          []CanonicalItem
	OrderedItemIDs []string
	TotalCount     int
	StartIndex     int
	EndIndex       int
	HasBefore      bool
	HasAfter       bool
	BeforeCursor   string
	AfterCursor    string
}

func (p *LiveProjection) SubscribeTimeline(ctx context.Context, session TimelineSession, surfaceID string, query database.MessageWindowQuery, reader CanonicalWindowSnapshotReader) (*TimelineSubscription, *SubscribedTimelineSnapshot, error) {
	if session == nil || reader == nil || surfaceID == "" || len(surfaceID) > 256 || query.ConversationID == "" {
		return nil, nil, ErrTimelineSubscription
	}
	authorized, sessionID, err := session.AuthorizeTimeline(ctx)
	if err != nil {
		return nil, nil, err
	}
	userID, err := database.RequireUserID(authorized)
	if err != nil || sessionID == "" {
		return nil, nil, ErrTimelineSubscription
	}
	key := projectionKey{userID, query.ConversationID}
	c, release := p.lockConversation(key, true)
	defer release()
	surface := subscriptionSurface{sessionID, surfaceID}
	if len(c.windows) >= 64 {
		return nil, nil, ErrTimelineSubscriptionLimit
	}
	for _, window := range c.windows {
		if window.surface == surface {
			return nil, nil, ErrTimelineSubscription
		}
	}
	snapshot, err := p.readWindowLocked(authorized, query, canonicalWindowAdapter{reader}, true, c)
	if err != nil {
		return nil, nil, err
	}
	id := uuid.NewString()
	if c.windows == nil {
		c.windows = make(map[string]*subscribedWindow)
	}
	window := &subscribedWindow{id: id, surface: surface, itemIDs: make(map[string]bool)}
	c.windows[id] = window
	handle := &TimelineSubscription{owner: p, key: key, conversation: c, session: session, windowID: id, sessionID: sessionID, query: cloneWindowQuery(query), reader: reader}
	return handle, subscriptionSnapshot(c, window, snapshot), nil
}

func cloneWindowQuery(query database.MessageWindowQuery) database.MessageWindowQuery {
	if query.ParentID != nil {
		parent := *query.ParentID
		query.ParentID = &parent
	}
	return query
}

func subscriptionSnapshot(c *projectionConversation, window *subscribedWindow, snapshot *TimelineWindowSnapshot) *SubscribedTimelineSnapshot {
	result := &SubscribedTimelineSnapshot{WindowID: window.id, Epoch: c.epoch, Sequence: strconv.FormatUint(c.publication.sequence, 10), Items: make([]CanonicalItem, 0, len(snapshot.Items)), OrderedItemIDs: make([]string, 0, len(snapshot.Items)), TotalCount: snapshot.Window.TotalCount, StartIndex: snapshot.Window.StartIndex, EndIndex: snapshot.Window.EndIndex, HasBefore: snapshot.Window.HasBefore, HasAfter: snapshot.Window.HasAfter, BeforeCursor: snapshot.BeforeCursor, AfterCursor: snapshot.AfterCursor}
	window.version = snapshot.Version
	window.epoch = c.epoch
	window.itemIDs = make(map[string]bool, len(snapshot.Items))
	for _, item := range snapshot.Items {
		view := canonicalItem(item)
		if cached, ok := c.publication.items[item.ItemID]; ok {
			view.Revision = cached.Revision
		} else {
			view.Revision = result.Sequence
		}
		result.Items = append(result.Items, view)
		result.OrderedItemIDs = append(result.OrderedItemIDs, item.ItemID)
		window.itemIDs[item.ItemID] = true
	}
	return result
}

func (s *TimelineSubscription) authorize(ctx context.Context) (context.Context, error) {
	if s == nil || s.owner == nil || s.session == nil {
		return nil, ErrTimelineSubscription
	}
	authorized, sessionID, err := s.session.AuthorizeTimeline(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := database.RequireUserID(authorized)
	if err != nil || sessionID != s.sessionID || userID != s.key.userID {
		return nil, ErrTimelineSubscription
	}
	return authorized, nil
}

// Refresh é uma recuperação explícita. Não é chamada por token e não afirma
// substituir a futura entrega automática de patches da janela.
func (s *TimelineSubscription) Refresh(ctx context.Context) (*SubscribedTimelineSnapshot, error) {
	authorized, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	c, release := s.owner.lockConversation(s.key, false)
	defer release()
	if c != s.conversation || c == nil || c.windows[s.windowID] == nil {
		return nil, ErrTimelineSubscription
	}
	snapshot, err := s.owner.readWindowLocked(authorized, s.query, canonicalWindowAdapter{s.reader}, true, c)
	if err != nil {
		return nil, err
	}
	return subscriptionSnapshot(c, c.windows[s.windowID], snapshot), nil
}

// Replay lê somente operações já publicadas para os IDs materializados desta
// inscrição. Lotes vazios preservam a sequência compartilhada. Não é sync público.
func (s *TimelineSubscription) Replay(ctx context.Context, epoch string, after uint64, reader TurnSnapshotReader) ([]CanonicalBatch, bool, error) {
	authorized, err := s.authorize(ctx)
	if err != nil {
		return nil, false, err
	}
	if reader == nil {
		return nil, false, ErrTimelineSubscription
	}
	c, release := s.owner.lockConversation(s.key, false)
	defer release()
	if c != s.conversation || c == nil || c.windows[s.windowID] == nil {
		return nil, false, ErrTimelineSubscription
	}
	// Reautoriza posse e invalida época em import/recriação antes de devolver memória.
	version, err := reader.ReadVersion(authorized, s.key.conversationID)
	if err != nil {
		return nil, false, err
	}
	reconcileProjectionVersion(c, version)
	if err := c.publicationErrorLocked(); err != nil {
		return nil, false, err
	}
	if c.publication.epoch != c.epoch || c.windows[s.windowID].epoch != c.epoch || c.windows[s.windowID].version != version {
		return nil, false, nil
	}
	batches, ok := c.publication.replay(epoch, after)
	if !ok {
		return nil, false, nil
	}
	ids := c.windows[s.windowID].itemIDs
	for i := range batches {
		operations := make([]CanonicalOperation, 0)
		for _, op := range batches[i].Operations {
			if ids[op.ItemID] {
				operations = append(operations, op)
			}
		}
		batches[i].Operations = operations
	}
	return batches, true, nil
}

// Fechar libera somente este capability, inclusive após revogação. Não retorna
// conteúdo nem exige reautorizar uma sessão que já perdeu o direito de leitura.
func (s *TimelineSubscription) Close() {
	if s == nil || s.owner == nil {
		return
	}
	c, release := s.owner.lockConversation(s.key, false)
	defer release()
	if c == s.conversation && c != nil {
		delete(c.windows, s.windowID)
	}
}
