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
	"time"

	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"

	"github.com/google/uuid"
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
	Sequences  []Sequence
	Now        func() time.Time
}

type Controller struct {
	mu          sync.Mutex
	bridge      Bridge
	sessionID   string
	owner       commandbridge.Owner
	generation  uint64
	resolve     InvocationFactory
	sequences   map[string]Sequence
	pending     *pendingSequence
	releases    map[sequenceRelease]string
	now         func() time.Time
	suspended   bool
	closed      bool
	revision    uint64
	transitions uint64
}

type Sequence struct {
	PrefixKey string
	Keys      []string
	Timeout   time.Duration
}

type pendingSequence struct {
	source   string
	prefix   string
	deadline time.Time
	keys     map[string]struct{}
}

type sequenceRelease struct {
	source string
	key    string
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
	sequences, err := buildSequences(config.Sequences)
	if err != nil {
		return nil, err
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Controller{
		bridge:     config.Bridge,
		sessionID:  config.SessionID,
		owner:      config.Owner,
		generation: config.Generation,
		resolve:    config.Resolve,
		sequences:  sequences,
		now:        now,
	}, nil
}

func (c *Controller) Input(ctx context.Context, event Event) (commandbridge.InvocationAck, error) {
	if ctx == nil {
		return commandbridge.InvocationAck{}, ErrInvalidEvent
	}
	if !validEvent(event) {
		return commandbridge.InvocationAck{}, ErrInvalidEvent
	}
	if err := ctx.Err(); err != nil {
		return commandbridge.InvocationAck{}, err
	}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return commandbridge.InvocationAck{}, err
	}
	if c.closed {
		c.mu.Unlock()
		return commandbridge.InvocationAck{}, ErrAdapterClosed
	}
	if c.suspended || c.transitions != 0 {
		c.mu.Unlock()
		return commandbridge.InvocationAck{}, ErrAdapterSuspended
	}
	event, pending := c.applySequenceLocked(event)
	if pending {
		c.mu.Unlock()
		return commandbridge.InvocationAck{Accepted: false, Reason: "sequence-pending"}, nil
	}
	generation := c.generation
	revision := c.revision
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
	// Resolve pode atravessar blur, bloqueio ou troca de geração. A entrega
	// curta à bridge é serializada com a invalidação, não com a resolução.
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return commandbridge.InvocationAck{}, err
	}
	if c.closed {
		return commandbridge.InvocationAck{}, ErrAdapterClosed
	}
	if c.suspended || c.transitions != 0 {
		return commandbridge.InvocationAck{}, ErrAdapterSuspended
	}
	if c.revision != revision || c.generation != generation {
		return commandbridge.InvocationAck{}, ErrStaleGeneration
	}
	resolved.SessionID = sessionID
	resolved.Generation = generation
	// Identidade de ocorrência pertence à borda confiável, não ao resolvedor.
	resolved.SourceEventID = ""
	if event.Kind == commandinput.KeyDown && !event.Repeat {
		sourceEvent, err := uuid.NewV7()
		if err != nil {
			return commandbridge.InvocationAck{}, err
		}
		resolved.SourceEventID = sourceEvent.String()
	}
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

func (c *Controller) applySequenceLocked(event Event) (Event, bool) {
	release := sequenceRelease{source: event.SourceInstance, key: event.Key}
	if key, exists := c.releases[release]; exists && (event.Kind == commandinput.KeyUp || event.Repeat) {
		event.Key = key
		if event.Kind == commandinput.KeyUp {
			delete(c.releases, release)
		}
		return event, false
	}
	if len(c.sequences) == 0 || event.Kind != commandinput.KeyDown || event.Repeat {
		if event.Kind == commandinput.KeyUp || event.Kind == commandinput.KeyDown {
			c.clearExpiredSequenceLocked()
		}
		return event, false
	}
	now := c.now()
	if c.pending != nil {
		if !now.Before(c.pending.deadline) {
			c.pending = nil
		} else if c.pending.source != event.SourceInstance {
			return event, false
		} else if _, ok := c.pending.keys[event.Key]; ok {
			prefix := c.pending.prefix
			c.pending = nil
			event.Key = prefix + " " + event.Key
			if c.releases == nil {
				c.releases = make(map[sequenceRelease]string)
			}
			c.releases[release] = event.Key
			return event, false
		} else {
			c.pending = nil
		}
	}
	sequence, ok := c.sequences[event.Key]
	if !ok {
		return event, false
	}
	c.pending = &pendingSequence{source: event.SourceInstance, prefix: sequence.PrefixKey, deadline: now.Add(sequence.Timeout), keys: make(map[string]struct{}, len(sequence.Keys))}
	for _, key := range sequence.Keys {
		c.pending.keys[key] = struct{}{}
	}
	return event, true
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
	c.revision++
	c.transitions++
	c.pending = nil
	c.releases = nil
	defer c.finishTransition()
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
		c.pending = nil
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
	c.revision++
	c.pending = nil
	c.releases = nil
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
	c.revision++
	c.transitions++
	c.pending = nil
	c.releases = nil
	defer c.finishTransition()
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
			c.pending = nil
		}
		c.mu.Unlock()
	} else if kind == commandbridge.LifecycleBlur {
		c.mu.Lock()
		if !c.closed && c.generation == generation {
			c.pending = nil
		}
		c.mu.Unlock()
	}
	return nil
}

func (c *Controller) finishTransition() {
	c.mu.Lock()
	c.transitions--
	c.mu.Unlock()
}

func (c *Controller) clearExpiredSequenceLocked() {
	if c.pending != nil && !c.now().Before(c.pending.deadline) {
		c.pending = nil
	}
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
		(event.Kind != commandinput.KeyUp || !event.Repeat)
}

func buildSequences(values []Sequence) (map[string]Sequence, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]Sequence, len(values))
	for _, sequence := range values {
		if strings.TrimSpace(sequence.PrefixKey) != sequence.PrefixKey || sequence.PrefixKey == "" ||
			sequence.Timeout <= 0 || len(sequence.Keys) == 0 {
			return nil, ErrInvalidConfiguration
		}
		keys := make([]string, 0, len(sequence.Keys))
		seen := make(map[string]struct{}, len(sequence.Keys))
		for _, key := range sequence.Keys {
			if strings.TrimSpace(key) != key || key == "" {
				return nil, ErrInvalidConfiguration
			}
			if _, ok := seen[key]; ok {
				return nil, ErrInvalidConfiguration
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		if _, exists := result[sequence.PrefixKey]; exists {
			return nil, ErrInvalidConfiguration
		}
		result[sequence.PrefixKey] = Sequence{PrefixKey: sequence.PrefixKey, Keys: keys, Timeout: sequence.Timeout}
	}
	return result, nil
}
