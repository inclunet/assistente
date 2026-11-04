package wailsapi

import (
	"context"
	"sync"

	"assistente/internal/apidto"
	"assistente/internal/chat"
)

// TimelineEventEmitter pertence ao transporte autenticado. Emit deve validar
// a autorização vigente, respeitar cancelamento e não reentrar neste handle.
type TimelineEventEmitter interface {
	Emit(context.Context, apidto.TimelineEvent) error
	Close()
}

type timelineAuthorizedEmitter struct {
	principal timelinePrincipal
	TimelineEventEmitter
}

func (e timelineAuthorizedEmitter) Emit(ctx context.Context, event apidto.TimelineEvent) error {
	if _, _, err := e.principal.AuthorizeTimeline(ctx); err != nil {
		return err
	}
	return e.TimelineEventEmitter.Emit(ctx, event)
}

type timelineDeliveryFeed interface {
	Next(context.Context) (*chat.TimelineDelivery, error)
	Close()
}

// A exclusão protege somente entrega e troca da base, nunca a espera em Next.
// Ela também permite drenar uma emissão já admitida ao fechar a inscrição.
type timelineEventStream struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	feed      timelineDeliveryFeed
	emitter   TimelineEventEmitter
	epoch     string
	baseline  string
	surfaceID string
	closed    bool
	released  func()
}

// O caller registra o handle sob a registry antes de liberar seu mutex; released
// usa a mesma registry. Assim, uma saída imediata não antecede o registro.
func newTimelineEventStream(ctx context.Context, feed timelineDeliveryFeed, emitter TimelineEventEmitter, surfaceID string, snapshot *apidto.SubscribedTimelineSnapshot, released func()) *timelineEventStream {
	ctx, cancel := context.WithCancel(ctx)
	s := &timelineEventStream{ctx: ctx, cancel: cancel, done: make(chan struct{}), feed: feed, emitter: emitter, surfaceID: surfaceID, epoch: snapshot.Epoch, baseline: snapshot.WindowRevision, released: released}
	go s.run()
	return s
}

func (s *timelineEventStream) run() {
	defer close(s.done)
	defer s.released()
	defer s.emitter.Close()
	defer s.feed.Close()
	defer s.cancel()
	for {
		delivery, err := s.feed.Next(s.ctx)
		if err != nil {
			return
		}
		if err := s.deliver(delivery); err != nil {
			return
		}
	}
}

func (s *timelineEventStream) deliver(delivery *chat.TimelineDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return chat.ErrTimelineSubscription
	}
	if delivery.SnapshotRevision != s.baseline || (!delivery.SyncRequired && delivery.Epoch != s.epoch) {
		return nil // extraído antes de Refresh/Navigate: não pertence à base atual
	}
	return s.emitter.Emit(s.ctx, apidto.TimelineEvent{
		ConversationID: delivery.ConversationID, WindowID: delivery.WindowID,
		SurfaceID: s.surfaceID,
		Epoch:     delivery.Epoch, SnapshotRevision: delivery.SnapshotRevision,
		SyncRequired: delivery.SyncRequired, Reason: delivery.Reason,
		Updates: timelineUpdates(delivery.Batches),
	})
}

// O caller executa a consulta do core sob esta mesma exclusão; só um snapshot
// aceito troca a base. O mutex não ordena Promise RPC e evento no frontend.
func (s *timelineEventStream) replaceSnapshot(ctx context.Context, query func(context.Context) (*apidto.SubscribedTimelineSnapshot, error)) (*apidto.SubscribedTimelineSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return nil, chat.ErrTimelineSubscription
	}
	// A consulta conserva o principal/deadline do RPC e também termina quando
	// a inscrição fecha, antes da drenagem que espera por este mutex.
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	snapshot, err := query(ctx)
	if err == nil {
		s.epoch, s.baseline = snapshot.Epoch, snapshot.WindowRevision
	}
	return snapshot, err
}

// Consultas preservam o contexto do RPC e terminam também com a inscrição.
// Não adquire o mutex de emissão: leitores sem troca de snapshot só precisam
// compartilhar cancelamento, não bloquear a entrega de eventos.
func (s *timelineEventStream) queryContext(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	if s.ctx.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

// Cancelar vem antes da drenagem. Nenhum lock de auth, registry ou core deve
// estar adquirido pelo caller. Uma emissão já enfileirada no runtime não pode
// ser desfeita; o frontend precisa descartar capabilities encerradas.
func (s *timelineEventStream) close() {
	s.cancel()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.feed.Close()
	<-s.done
}
