package jobs

import (
	"assistente/internal/logging"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const droppedEventWarningInterval = time.Hour

// EventHandler e chamado quando um evento e publicado.
// Recebe o nome do evento e o payload.
type EventHandler func(ctx context.Context, eventName string, payload map[string]any)

// EventBusStats expõe métricas operacionais do barramento de jobs.
type EventBusStats struct {
	EventsDropped uint64 `json:"events_dropped"`
}

// EventBus implementa pub/sub de eventos para encadeamento de jobs.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]namedHandler
	closed   bool
	// wg rastreia as goroutines de fan-out em voo para que Close possa drená-las
	// (shutdown gracioso). Sem isso, um handler (ex.: execução de job) pode
	// sobreviver ao Close/Stop e continuar escrevendo no estado global — em
	// testes, escrevendo no DB de OUTRO teste após o swap do singleton.
	wg sync.WaitGroup

	eventsDropped atomic.Uint64
	dropMu        sync.Mutex
	lastDropWarn  map[string]time.Time
	now           func() time.Time
}

type namedHandler struct {
	id      string // identificador (normalmente jobID) para unsubscribe
	handler EventHandler
}

// NewEventBus cria um event bus vazio.
func NewEventBus() *EventBus {
	return &EventBus{
		handlers:     make(map[string][]namedHandler),
		lastDropWarn: make(map[string]time.Time),
		now:          time.Now,
	}
}

// Subscribe registra um handler para um evento.
// subscriberID permite desregistrar depois (normalmente o jobID).
func (eb *EventBus) Subscribe(eventName string, subscriberID string, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.closed {
		return
	}

	eb.handlers[eventName] = append(eb.handlers[eventName], namedHandler{
		id:      subscriberID,
		handler: handler,
	})
}

// Unsubscribe remove todos os handlers de um subscriber para um evento.
func (eb *EventBus) Unsubscribe(eventName string, subscriberID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	handlers := eb.handlers[eventName]
	filtered := make([]namedHandler, 0, len(handlers))
	for _, h := range handlers {
		if h.id != subscriberID {
			filtered = append(filtered, h)
		}
	}

	if len(filtered) == 0 {
		delete(eb.handlers, eventName)
	} else {
		eb.handlers[eventName] = filtered
	}
}

// UnsubscribeAll remove todos os handlers de um subscriber em todos os eventos.
func (eb *EventBus) UnsubscribeAll(subscriberID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventName, handlers := range eb.handlers {
		filtered := make([]namedHandler, 0, len(handlers))
		for _, h := range handlers {
			if h.id != subscriberID {
				filtered = append(filtered, h)
			}
		}
		if len(filtered) == 0 {
			delete(eb.handlers, eventName)
		} else {
			eb.handlers[eventName] = filtered
		}
	}
}

// Publish dispara um evento, executando todos os handlers registrados, e
// informa se ao menos um consumidor habilitado recebeu o evento.
// Cada handler roda em goroutine separada para nao bloquear o publisher.
func (eb *EventBus) Publish(ctx context.Context, eventName string, payload map[string]any) bool {
	eb.mu.RLock()
	if eb.closed {
		eb.mu.RUnlock()
		return false
	}
	handlers := make([]namedHandler, len(eb.handlers[eventName]))
	copy(handlers, eb.handlers[eventName])
	if len(handlers) == 0 {
		eb.mu.RUnlock()
		dropped, warn := eb.recordDropped(eventName)
		if warn {
			logging.Logger(ctx, "jobs.eventbus").Warn(
				"event dropped because it has no enabled listeners",
				slog.String("event_name", eventName),
				slog.String("reason", "no_enabled_listeners"),
				slog.Uint64("events_dropped", dropped),
				slog.Duration("warning_throttle", droppedEventWarningInterval),
			)
		}
		return false
	}
	// Registra as goroutines no WaitGroup AINDA sob o RLock: Close adquire o
	// write-lock exclusivo, então ou o Add acontece antes de Close (e o Wait as
	// aguarda) ou Close já marcou closed e este Publish teria retornado acima.
	// Isso evita a corrida clássica de Add-após-Wait.
	eb.wg.Add(len(handlers))
	eb.mu.RUnlock()

	// Guard central anti-data-race: o mesmo payload (e o mesmo slice _chain_history)
	// é entregue a todos os handlers concorrentes, que downstream fazem
	// append(history, jobID). Clipando para cap == len antes do fan-out, cada append
	// aloca um novo backing array em vez de escrever in-place no compartilhado. Vale
	// para QUALQUER publisher (PublishDomainEvent, executor, futuros), não só um.
	// payload pode ser nil (publishers legados): ler de map nil é seguro em Go, mas
	// o nil-check explícito evita a escrita em map nil e deixa a intenção clara.
	if payload != nil {
		if h, ok := payload["_chain_history"].([]string); ok {
			payload["_chain_history"] = clipHistory(h)
		}
	}

	logging.Debugf(ctx, "jobs.eventbus", "[EventBus] Event %q published to %d listener(s)", eventName, len(handlers))

	for _, h := range handlers {
		go func(nh namedHandler) {
			defer eb.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logging.Errorf(ctx, "jobs.eventbus", "[EventBus] Panic in handler %q for event %q: %v", nh.id, eventName, r)
				}
			}()
			nh.handler(ctx, eventName, payload)
		}(h)
	}
	return true
}

func (eb *EventBus) recordDropped(eventName string) (uint64, bool) {
	dropped := eb.eventsDropped.Add(1)
	now := eb.now()

	eb.dropMu.Lock()
	defer eb.dropMu.Unlock()
	last := eb.lastDropWarn[eventName]
	if !last.IsZero() && now.Sub(last) < droppedEventWarningInterval {
		return dropped, false
	}
	eb.lastDropWarn[eventName] = now
	return dropped, true
}

// Stats retorna um snapshot consistente das métricas do barramento.
func (eb *EventBus) Stats() EventBusStats {
	if eb == nil {
		return EventBusStats{}
	}
	return EventBusStats{EventsDropped: eb.eventsDropped.Load()}
}

// SubscriberCount retorna o numero de subscribers para um evento.
func (eb *EventBus) SubscriberCount(eventName string) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.handlers[eventName])
}

// Events retorna a lista de eventos que tem subscribers.
func (eb *EventBus) Events() []string {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	events := make([]string, 0, len(eb.handlers))
	for e := range eb.handlers {
		events = append(events, e)
	}
	return events
}

// Close impede novos publishes e subscriptions e DRENA as goroutines de fan-out
// em voo (shutdown gracioso): após Close, nenhum handler disparado por este bus
// continua executando. O wg.Wait roda FORA do lock para não travar handlers que
// chamem Publish (encadeamento) durante o dreno — um Publish após closed retorna
// cedo sem registrar novas goroutines, então o Wait sempre converge.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	eb.closed = true
	eb.handlers = make(map[string][]namedHandler)
	eb.mu.Unlock()

	eb.wg.Wait()
}
