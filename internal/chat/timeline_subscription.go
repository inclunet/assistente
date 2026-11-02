package chat

import (
	"context"
	"errors"
	"strconv"

	"assistente/internal/database"
	"github.com/google/uuid"
)

// TimelineSession é implementada pelo transporte confiável, que captura e
// revalida a sessão. A autorização não pode reentrar no projetor: ocorre sob a
// barreira da conversa. Nenhum owner/sessionId é aceito no pedido da superfície.
type TimelineSession interface {
	AuthorizeTimeline(context.Context) (context.Context, string, error)
}

var ErrTimelineSubscription = errors.New("inscrição de timeline inválida ou encerrada")
var ErrTimelineSubscriptionLimit = errors.New("limite de inscrições de timeline atingido")
var ErrTimelineWindowStale = errors.New("revisão da janela mudou; sincronização necessária")

type subscriptionSurface struct{ sessionID, surfaceID string }
type subscribedWindow struct {
	revision uint64
	epoch    string
	version  database.ChatTimelineVersion
	id       string
	surface  subscriptionSurface
	itemIDs  map[string]bool
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
	WindowRevision string
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
	// A primeira autorização identifica a chave; esta confirma a sessão depois
	// de eventual espera por um writer, antes de consultar dados ou criar a janela.
	authorized, currentSessionID, err := session.AuthorizeTimeline(ctx)
	if err != nil {
		return nil, nil, err
	}
	currentUserID, err := database.RequireUserID(authorized)
	if err != nil || currentSessionID != sessionID || currentUserID != userID {
		return nil, nil, ErrTimelineSubscription
	}
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
	window.revision++
	result := &SubscribedTimelineSnapshot{WindowID: window.id, Epoch: c.epoch, Sequence: strconv.FormatUint(c.publication.sequence, 10), Items: make([]CanonicalItem, 0, len(snapshot.Items)), OrderedItemIDs: make([]string, 0, len(snapshot.Items)), TotalCount: snapshot.Window.TotalCount, StartIndex: snapshot.Window.StartIndex, EndIndex: snapshot.Window.EndIndex, HasBefore: snapshot.Window.HasBefore, HasAfter: snapshot.Window.HasAfter, BeforeCursor: snapshot.BeforeCursor, AfterCursor: snapshot.AfterCursor}
	result.WindowRevision = strconv.FormatUint(window.revision, 10)
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

// A sessão é validada depois da espera por writers. O capability já fixa a
// chave usada para obter a barreira, sem confiar em owner enviado pelo caller.
func (s *TimelineSubscription) lockAuthorized(ctx context.Context) (context.Context, *projectionConversation, func(), error) {
	if s == nil || s.owner == nil || s.session == nil {
		return nil, nil, nil, ErrTimelineSubscription
	}
	c, release := s.owner.lockConversation(s.key, false)
	if c == nil || c != s.conversation || c.windows[s.windowID] == nil {
		release()
		return nil, nil, nil, ErrTimelineSubscription
	}
	authorized, err := s.authorize(ctx)
	if err != nil {
		release()
		return nil, nil, nil, err
	}
	return authorized, c, release, nil
}

// Refresh recupera explicitamente a inscrição. Position nil repete a consulta;
// uma posição explícita permite abandonar um cursor obsoleto, preservando escopo.
// Não é chamada por token nem faz fallback silencioso para outra página.
func (s *TimelineSubscription) Refresh(ctx context.Context, windowRevision string, position *TimelineWindowPosition) (*SubscribedTimelineSnapshot, error) {
	authorized, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if windowRevision != strconv.FormatUint(c.windows[s.windowID].revision, 10) {
		return nil, ErrTimelineWindowStale
	}
	query := s.query
	if position != nil {
		query = s.windowQuery(*position)
	}
	snapshot, err := s.owner.readWindowLocked(authorized, query, canonicalWindowAdapter{s.reader}, true, c)
	if err != nil {
		return nil, err
	}
	s.query = cloneWindowQuery(query)
	return subscriptionSnapshot(c, c.windows[s.windowID], snapshot), nil
}

// TimelineWindowNavigation altera somente a fatia da inscrição existente.
// Owner, sessão, conversa e escopo vêm do capability, nunca deste pedido.
type TimelineWindowNavigation struct {
	Epoch          string
	WindowRevision string
	TimelineWindowPosition
}

type TimelineWindowPosition struct {
	Cursor       string
	Anchor       string
	AnchorItemID string
	Direction    string
	Limit        int
}

func (s *TimelineSubscription) windowQuery(position TimelineWindowPosition) database.MessageWindowQuery {
	return database.MessageWindowQuery{ConversationID: s.key.conversationID, ParentID: s.query.ParentID, Cursor: position.Cursor, Anchor: position.Anchor, AnchorItemID: position.AnchorItemID, Direction: position.Direction, Limit: position.Limit}
}

// Navigate serializa consulta e troca da participação com os writers da conversa.
// Dois pedidos da mesma revisão não podem sobrescrever a navegação um do outro.
// Uma leitura recusada preserva a consulta e os IDs da janela anterior.
func (s *TimelineSubscription) Navigate(ctx context.Context, request TimelineWindowNavigation) (*SubscribedTimelineSnapshot, error) {
	authorized, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	window := c.windows[s.windowID]
	if request.Epoch != c.epoch || window.epoch != c.epoch || request.WindowRevision != strconv.FormatUint(window.revision, 10) {
		return nil, ErrTimelineWindowStale
	}
	query := s.windowQuery(request.TimelineWindowPosition)
	snapshot, err := s.owner.readWindowLocked(authorized, query, canonicalWindowAdapter{s.reader}, true, c)
	if err != nil {
		return nil, err
	}
	// Import/recriação pode ser detectado pela leitura, inclusive quando o writer
	// veio de fora do coordenador. Nunca aceita navegação baseada na época antiga.
	if request.Epoch != c.epoch {
		return nil, ErrTimelineWindowStale
	}
	s.query = cloneWindowQuery(query)
	return subscriptionSnapshot(c, window, snapshot), nil
}

// Replay lê somente operações já publicadas para os IDs materializados desta
// inscrição. Lotes vazios preservam a sequência compartilhada. Não é sync público.
func (s *TimelineSubscription) Replay(ctx context.Context, epoch, windowRevision string, after uint64, reader TurnSnapshotReader) ([]CanonicalBatch, bool, error) {
	authorized, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return nil, false, err
	}
	defer release()
	if reader == nil {
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
	if windowRevision != strconv.FormatUint(c.windows[s.windowID].revision, 10) {
		return nil, false, ErrTimelineWindowStale
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
