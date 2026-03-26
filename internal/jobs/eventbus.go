package jobs

import (
	"context"
	"log"
	"sync"
)

// EventHandler e chamado quando um evento e publicado.
// Recebe o nome do evento e o payload.
type EventHandler func(ctx context.Context, eventName string, payload map[string]any)

// EventBus implementa pub/sub de eventos para encadeamento de jobs.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]namedHandler
	closed   bool
}

type namedHandler struct {
	id      string // identificador (normalmente jobID) para unsubscribe
	handler EventHandler
}

// NewEventBus cria um event bus vazio.
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[string][]namedHandler),
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

// Publish dispara um evento, executando todos os handlers registrados.
// Cada handler roda em goroutine separada para nao bloquear o publisher.
func (eb *EventBus) Publish(ctx context.Context, eventName string, payload map[string]any) {
	eb.mu.RLock()
	handlers := make([]namedHandler, len(eb.handlers[eventName]))
	copy(handlers, eb.handlers[eventName])
	closed := eb.closed
	eb.mu.RUnlock()

	if closed {
		return
	}

	if len(handlers) == 0 {
		log.Printf("[EventBus] Event %q published with no listeners", eventName)
		return
	}

	log.Printf("[EventBus] Event %q published to %d listener(s)", eventName, len(handlers))

	for _, h := range handlers {
		go func(nh namedHandler) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[EventBus] Panic in handler %q for event %q: %v", nh.id, eventName, r)
				}
			}()
			nh.handler(ctx, eventName, payload)
		}(h)
	}
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

// Close impede novos publishes e subscriptions.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.closed = true
	eb.handlers = make(map[string][]namedHandler)
}
