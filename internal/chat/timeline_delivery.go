package chat

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
)

var ErrTimelineDeliveryBusy = errors.New("inscrição já possui leitor de atualizações")

// TimelineDelivery é a saída interna para um adaptador de eventos. Não é uma
// confirmação de recebimento: Replay continua disponível se o transporte falhar.
// SnapshotRevision identifica a consulta que originou a entrega, inclusive quando
// um Refresh/Navigate posterior devolve snapshot com a mesma sequência.
type TimelineDelivery struct {
	ConversationID   string
	WindowID         string
	Epoch            string
	SnapshotRevision string
	SyncRequired     bool
	Reason           string
	Batches          []CanonicalBatch
}

type timelineDeliveryCursor struct {
	epoch        string
	revision     string
	sequence     uint64
	syncRequired bool
}

// Next aguarda somente publicações confirmadas, sem SQL, polling, goroutine ou
// fila de payload adicional. Há um leitor por capability. O adaptador revalida
// a sessão ao emitir e descarta entregas de um snapshot já substituído.
// A autorização aqui é a do principal capturado; não detecta por si só escritas
// ou revogações persistidas externas ainda não observadas pelo coordenador.
func (s *TimelineSubscription) Next(ctx context.Context) (*TimelineDelivery, error) {
	if s == nil {
		return nil, ErrTimelineSubscription
	}
	if !s.reading.CompareAndSwap(false, true) {
		return nil, ErrTimelineDeliveryBusy
	}
	defer s.reading.Store(false)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, c, release, err := s.lockAuthorized(ctx)
		if err != nil {
			return nil, err
		}
		result, err := s.takeDeliveryLocked(ctx, c)
		release()
		if result != nil || err != nil {
			return result, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.changed:
		}
	}
}

func (s *TimelineSubscription) takeDeliveryLocked(ctx context.Context, c *projectionConversation) (*TimelineDelivery, error) {
	w := c.windows[s.windowID]
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.delivery.syncRequired {
		return nil, nil // somente um novo snapshot libera a entrega
	}
	result := &TimelineDelivery{ConversationID: s.key.conversationID, WindowID: s.windowID, Epoch: c.epoch, SnapshotRevision: strconv.FormatUint(w.snapshotRevision, 10)}
	switch {
	case w.delivery.epoch != c.epoch:
		result.Reason = "epoch_changed"
	case c.publicationErrorLocked() != nil:
		result.Reason = "publication_failed"
	case w.syncRequired:
		result.Reason = "window_invalidated"
	case w.delivery.sequence == c.publication.sequence:
		return nil, nil
	default:
		batches, ok := c.publication.replay(w.delivery.epoch, w.delivery.sequence)
		if ok {
			var err error
			batches, ok, err = replayWindowBatches(w, w.delivery.revision, batches)
			ok = ok && err == nil
		}
		if !ok {
			result.Reason = "journal_gap"
		} else {
			result.Batches = batches
		}
	}
	// O cancelamento durante a cópia não consome a posição de entrega.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result.Reason != "" {
		result.SyncRequired, w.delivery.syncRequired = true, true
	} else {
		w.delivery.sequence = c.publication.sequence
		w.delivery.revision = strconv.FormatUint(w.revision, 10)
	}
	return result, nil
}

// Sinais coalescidos não carregam conteúdo nem executam o transporte sob locks.
func (s *TimelineSubscription) signalDeliveryLocked() {
	if s == nil {
		return
	}
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (c *projectionConversation) signalDeliveriesLocked() {
	for _, w := range c.windows {
		w.handle.signalDeliveryLocked()
	}
}

func (c *projectionConversation) invalidateTimelineEpochLocked() {
	c.epoch = uuid.NewString()
	c.signalDeliveriesLocked()
}
