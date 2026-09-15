// Package commandadapter contém o ciclo de vida comum de adapters físicos.
//
// O pacote não registra hotkeys, não conhece HID e não executa handlers de
// produto. Um listener concreto entrega eventos aqui; o controller só faz o
// handoff tipado para commandbridge com sessão, owner e geração vigentes.
package commandadapter

import (
	"context"
	"errors"
	"strings"
	"sync"

	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"
)

var (
	ErrInvalidConfiguration = errors.New("configuração do adapter de comandos inválida")
	ErrInvalidEvent         = errors.New("evento do adapter de comandos inválido")
	ErrAdapterClosed        = errors.New("adapter de comandos encerrado")
	ErrAdapterSuspended     = errors.New("adapter de comandos suspenso")
	ErrStaleGeneration      = errors.New("geração do adapter de comandos obsoleta")
)

type Bridge interface {
	Input(context.Context, commandbridge.Input) (commandbridge.InvocationAck, error)
	Lifecycle(context.Context, commandbridge.LifecycleEvent) error
}

// InvocationFactory resolve a invocação candidata sem chamar handler final.
// Retornar ok=false significa que a entrada não possui binding publicado.
type InvocationFactory func(Event) (commandbridge.Invocation, bool, error)

type Config struct {
	Bridge     Bridge
	SessionID  string
	Owner      commandbridge.Owner
	Generation uint64
	Resolve    InvocationFactory
}

type Controller struct {
	mu         sync.Mutex
	bridge     Bridge
	sessionID  string
	owner      commandbridge.Owner
	generation uint64
	resolve    InvocationFactory
	suspended  bool
	closed     bool
}

type Event struct {
	SourceInstance string
	Key            string
	Kind           commandinput.EventKind
	Repeat         bool
}

func New(config Config) (*Controller, error) {
	if config.Bridge == nil || strings.TrimSpace(config.SessionID) == "" || !validOwner(config.Owner) ||
		config.Owner.SessionID != config.SessionID || config.Generation == 0 || config.Resolve == nil {
		return nil, ErrInvalidConfiguration
	}
	return &Controller{
		bridge:     config.Bridge,
		sessionID:  config.SessionID,
		owner:      config.Owner,
		generation: config.Generation,
		resolve:    config.Resolve,
	}, nil
}

func (c *Controller) Input(ctx context.Context, event Event) (commandbridge.InvocationAck, error) {
	if ctx == nil {
		return commandbridge.InvocationAck{}, ErrInvalidEvent
	}
	if !validEvent(event) {
		return commandbridge.InvocationAck{}, ErrInvalidEvent
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return commandbridge.InvocationAck{}, ErrAdapterClosed
	}
	if c.suspended {
		c.mu.Unlock()
		return commandbridge.InvocationAck{}, ErrAdapterSuspended
	}
	generation := c.generation
	sessionID := c.sessionID
	owner := c.owner
	resolve := c.resolve
	bridge := c.bridge
	c.mu.Unlock()

	resolved, ok, err := resolve(Event{
		SourceInstance: event.SourceInstance,
		Key:            event.Key,
		Kind:           event.Kind,
		Repeat:         event.Repeat,
	})
	if err != nil {
		return commandbridge.InvocationAck{}, err
	}
	if !ok {
		return commandbridge.InvocationAck{Accepted: false, Reason: "no-binding"}, nil
	}
	resolved.SessionID = sessionID
	resolved.Generation = generation
	return bridge.Input(ctx, commandbridge.Input{
		SessionID:  sessionID,
		Source:     event.SourceInstance,
		Key:        event.Key,
		Generation: generation,
		Kind:       event.Kind,
		Repeat:     event.Repeat,
		Invocation: resolved,
		Owner:      owner,
	})
}

func (c *Controller) AdvanceGeneration(ctx context.Context, generation uint64) error {
	if ctx == nil || generation == 0 {
		return ErrInvalidEvent
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrAdapterClosed
	}
	if generation <= c.generation {
		c.mu.Unlock()
		return ErrStaleGeneration
	}
	sessionID := c.sessionID
	bridge := c.bridge
	c.mu.Unlock()

	if err := bridge.Lifecycle(ctx, commandbridge.LifecycleEvent{
		Kind:       commandbridge.LifecycleGeneration,
		SessionID:  sessionID,
		Generation: generation,
	}); err != nil {
		return err
	}
	c.mu.Lock()
	if !c.closed && generation > c.generation {
		c.generation = generation
		c.suspended = false
	}
	c.mu.Unlock()
	return nil
}

func (c *Controller) Blur(ctx context.Context) error {
	return c.lifecycle(ctx, commandbridge.LifecycleBlur, false)
}

func (c *Controller) Lock(ctx context.Context) error {
	return c.lifecycle(ctx, commandbridge.LifecycleLock, true)
}

func (c *Controller) Logout(ctx context.Context) error {
	return c.lifecycle(ctx, commandbridge.LifecycleLogout, true)
}

func (c *Controller) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidEvent
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.suspended = true
	sessionID := c.sessionID
	generation := c.generation
	bridge := c.bridge
	c.mu.Unlock()
	return bridge.Lifecycle(ctx, commandbridge.LifecycleEvent{
		Kind:       commandbridge.LifecycleLock,
		SessionID:  sessionID,
		Generation: generation,
	})
}

func (c *Controller) lifecycle(ctx context.Context, kind commandbridge.LifecycleKind, suspend bool) error {
	if ctx == nil {
		return ErrInvalidEvent
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrAdapterClosed
	}
	sessionID := c.sessionID
	generation := c.generation
	bridge := c.bridge
	c.mu.Unlock()

	if err := bridge.Lifecycle(ctx, commandbridge.LifecycleEvent{
		Kind:       kind,
		SessionID:  sessionID,
		Generation: generation,
	}); err != nil {
		return err
	}
	if suspend {
		c.mu.Lock()
		if !c.closed && c.generation == generation {
			c.suspended = true
		}
		c.mu.Unlock()
	}
	return nil
}

func validOwner(owner commandbridge.Owner) bool {
	return strings.TrimSpace(owner.UserID) == owner.UserID && owner.UserID != "" &&
		strings.TrimSpace(owner.SessionID) == owner.SessionID && owner.SessionID != "" &&
		strings.TrimSpace(owner.WorkspaceID) == owner.WorkspaceID && owner.WorkspaceID != ""
}

func validEvent(event Event) bool {
	return strings.TrimSpace(event.SourceInstance) == event.SourceInstance && event.SourceInstance != "" &&
		strings.TrimSpace(event.Key) == event.Key && event.Key != "" &&
		(event.Kind == commandinput.KeyDown || event.Kind == commandinput.KeyUp) &&
		!(event.Kind == commandinput.KeyUp && event.Repeat)
}
