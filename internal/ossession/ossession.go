package ossession

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// State é a observação de segurança da sessão interativa do sistema
// operacional. Quando Known é falso, Locked permanece true como default
// seguro e não deve ser interpretado como uma observação autoritativa.
type State struct {
	Known  bool
	Locked bool
}

func unknownState() State { return State{Locked: true} }

var (
	errObserverNil = errors.New("ossession: observe must not be nil")
	errNilContext  = errors.New("ossession: context must not be nil")
	errUnsupported = errors.New("ossession: unsupported platform")
	errPumpStopped = errors.New("ossession: native message pump stopped")
)

// Watch observa, de forma contínua, lock/unlock e mudanças de sessão do
// sistema operacional. A implementação sempre começa fechada/desconhecida;
// somente uma observação autoritativa pode publicar Known=true.
func Watch(ctx context.Context, observe func(State) error) (watchErr error) {
	if ctx == nil {
		return errNilContext
	}
	if observe == nil {
		return errObserverNil
	}

	// O primeiro e o último callback são sempre fail-closed. Isso também
	// cobre cancelamento antes da criação de qualquer recurso nativo.
	defer func() {
		if recovered := recover(); recovered != nil {
			watchErr = fmt.Errorf("ossession: panic in Watch: %v", recovered)
		}
		if err := safeObserve(observe, unknownState()); err != nil && watchErr == nil {
			watchErr = err
		}
	}()
	if err := safeObserve(observe, unknownState()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return watchPlatform(ctx, observe)
}

func safeObserve(observe func(State) error, state State) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("ossession: panic in observe: %v", recovered)
		}
	}()
	return observe(state)
}

type pumpEvent struct {
	state State
}

// eventQueue é uma fila FIFO sem capacidade fixa. A WindowProc nunca espera
// por observe e nunca perde um evento por canal cheio.
type eventQueue struct {
	mu       sync.Mutex
	events   []pumpEvent
	head     int
	closed   bool
	closeErr error
	notify   chan struct{}
}

func newEventQueue() *eventQueue {
	return &eventQueue{notify: make(chan struct{}, 1)}
}

func (q *eventQueue) push(event pumpEvent) {
	q.mu.Lock()
	q.events = append(q.events, event)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *eventQueue) close(err error) {
	q.mu.Lock()
	q.closed = true
	q.closeErr = err
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *eventQueue) pop(ctx context.Context) (pumpEvent, bool, error) {
	for {
		if err := ctx.Err(); err != nil {
			return pumpEvent{}, false, err
		}

		q.mu.Lock()
		// Após falha terminal, eventos antigos (inclusive unlock) já não
		// representam uma observação viva e não podem reabrir o host.
		if q.closed && q.closeErr != nil {
			err := q.closeErr
			q.mu.Unlock()
			return pumpEvent{}, false, err
		}
		if q.head < len(q.events) {
			event := q.events[q.head]
			q.head++
			if q.head > 256 && q.head*2 >= len(q.events) {
				q.events = append([]pumpEvent(nil), q.events[q.head:]...)
				q.head = 0
			}
			q.mu.Unlock()
			return event, true, nil
		}
		if q.closed {
			err := q.closeErr
			q.mu.Unlock()
			return pumpEvent{}, false, err
		}
		notify := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return pumpEvent{}, false, ctx.Err()
		case <-notify:
		}
	}
}

func pump(
	ctx context.Context,
	next func(context.Context) (pumpEvent, bool, error),
	observe func(State) error,
	stop func(),
) (pumpErr error) {
	defer func() {
		stop()
		if recovered := recover(); recovered != nil {
			pumpErr = fmt.Errorf("ossession: panic in pump: %v", recovered)
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		event, ok, sourceErr := next(ctx)
		if sourceErr != nil {
			return sourceErr
		}
		if !ok {
			return errPumpStopped
		}
		if err := observe(event.state); err != nil {
			return fmt.Errorf("ossession: observe: %w", err)
		}
	}
}
