// Package commandui fornece o handoff efêmero entre a UI e o executor.
// Não autentica, autoriza, persiste ledger nem inicia handlers.
package commandui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

var (
	ErrInvalidConfiguration = errors.New("configuração do broker UI inválida")
	ErrInvalidRequest       = errors.New("solicitação UI inválida")
	ErrNotFound             = errors.New("reserva UI não encontrada")
	ErrOwnerMismatch        = errors.New("owner UI divergente")
	ErrNotStarted           = errors.New("invocação UI ainda não iniciada")
	ErrAlreadyTaken         = errors.New("reserva UI já consumida")
	ErrAlreadyCompleted     = errors.New("reserva UI já concluída")
	ErrCommitRequired       = errors.New("comando UI exige commit explícito")
	ErrAlreadyCommitting    = errors.New("reserva UI já está em commit")
	ErrOutcomeUnknown       = errors.New("resultado da reserva UI desconhecido após possível efeito parcial")
	ErrCancelled            = errors.New("reserva UI cancelada")
	ErrExpired              = errors.New("reserva UI expirada")
	ErrClosed               = errors.New("broker UI fechado")
)

const maxEntries = 64

const maxReservationTTL = 5 * time.Minute

type Owner struct {
	UserID      string `json:"userId"`
	SessionID   string `json:"sessionId"`
	WorkspaceID string `json:"workspaceId"`
}

type Reservation struct {
	Ticket       string `json:"ticket"`
	InvocationID string `json:"invocationId"`
	CommandID    string `json:"commandId"`
}

type Handoff struct {
	Ticket       string `json:"ticket"`
	InvocationID string `json:"invocationId"`
	CommandID    string `json:"commandId"`
	HandoffID    string `json:"handoffId"`
}

type entry struct {
	reservation Reservation
	owner       Owner
	handoffID   string
	ready       chan struct{}
	readyOnce   sync.Once
	done        chan commandexecution.Outcome
	expiresAt   time.Time
	startCtx    context.Context
	started     bool
	taken       bool
	committing  bool
	terminal    bool
	status      commandledger.Status
	terminalErr error
}

type Broker struct {
	mu           sync.Mutex
	entries      map[string]*entry
	maxPending   int
	ttl          time.Duration
	byInvocation map[string]*entry
	stop         chan struct{}
	done         chan struct{}
	stopOnce     sync.Once
	closed       bool
}

func New(maxPending int, ttl time.Duration) (*Broker, error) {
	if maxPending <= 0 || maxPending > maxEntries || ttl <= 0 {
		return nil, ErrInvalidConfiguration
	}
	b := &Broker{entries: make(map[string]*entry, maxPending), byInvocation: make(map[string]*entry, maxPending), maxPending: maxPending, ttl: ttl, stop: make(chan struct{}), done: make(chan struct{})}
	go b.expirer()
	return b, nil
}

func (b *Broker) Reserve(owner Owner, commandID string) (Reservation, error) {
	if b == nil {
		return Reservation{}, ErrInvalidRequest
	}
	return b.reserveWithTTL(owner, commandID, b.ttl)
}

// ReserveWithTTL cria uma reserva com prazo próprio para fluxos que precisam
// manter a UI aberta por mais tempo. O TTL padrão do broker não é alterado.
func (b *Broker) ReserveWithTTL(owner Owner, commandID string, ttl time.Duration) (Reservation, error) {
	if b == nil || ttl <= 0 || ttl > maxReservationTTL {
		return Reservation{}, ErrInvalidRequest
	}
	return b.reserveWithTTL(owner, commandID, ttl)
}

func (b *Broker) reserveWithTTL(owner Owner, commandID string, ttl time.Duration) (Reservation, error) {
	if !validOwner(owner) || strings.TrimSpace(commandID) == "" || ttl <= 0 {
		return Reservation{}, ErrInvalidRequest
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Reservation{}, ErrClosed
	}
	b.expireLocked(time.Now())
	if len(b.entries) >= b.maxPending {
		return Reservation{}, ErrInvalidConfiguration
	}
	ticket, err := newID()
	if err != nil {
		return Reservation{}, err
	}
	invocationID, err := newID()
	if err != nil {
		return Reservation{}, err
	}
	handoffID, err := newID()
	if err != nil {
		return Reservation{}, err
	}
	reservation := Reservation{Ticket: ticket, InvocationID: invocationID, CommandID: commandID}
	b.entries[ticket] = &entry{reservation: reservation, owner: owner, handoffID: handoffID, ready: make(chan struct{}), done: make(chan commandexecution.Outcome, 1), expiresAt: time.Now().Add(ttl)}
	b.byInvocation[invocationID] = b.entries[ticket]
	return reservation, nil
}

func (b *Broker) Start(ctx context.Context, owner Owner, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
	if b == nil || ctx == nil || !validOwner(owner) || strings.TrimSpace(invocation.ID) == "" || strings.TrimSpace(invocation.CommandID) == "" {
		return commandexecution.ExecutionHandle{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.lookupInvocationLocked(owner, invocation.ID)
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	if e.reservation.CommandID != invocation.CommandID || invocation.Principal.UserID != owner.UserID || invocation.Principal.SessionID != owner.SessionID {
		return commandexecution.ExecutionHandle{}, ErrOwnerMismatch
	}
	if e.terminal {
		return commandexecution.ExecutionHandle{}, terminalError(e)
	}
	if e.started {
		return commandexecution.ExecutionHandle{}, ErrAlreadyCompleted
	}
	e.started = true
	e.startCtx = ctx
	e.readyOnce.Do(func() { close(e.ready) })
	return commandexecution.ExecutionHandle{ID: e.reservation.Ticket, Done: e.done, Cancel: func() { _ = b.Cancel(owner, e.reservation.Ticket) }}, nil
}

func (b *Broker) Take(ctx context.Context, owner Owner, ticket string) (Handoff, error) {
	return b.TakeValidated(ctx, owner, ticket, nil)
}

// TakeValidated revalida a admissão após a espera, antes de consumir o handoff.
// validate roda sem o mutex do broker e não pode produzir o efeito UI.
func (b *Broker) TakeValidated(ctx context.Context, owner Owner, ticket string, validate func() error) (Handoff, error) {
	if b == nil || ctx == nil || !validOwner(owner) || strings.TrimSpace(ticket) == "" {
		return Handoff{}, ErrInvalidRequest
	}
	b.mu.Lock()
	e, err := b.lookupLocked(owner, ticket)
	if err != nil {
		b.mu.Unlock()
		return Handoff{}, err
	}
	ready := e.ready
	b.mu.Unlock()
	select {
	case <-ready:
	case <-ctx.Done():
		return Handoff{}, ctx.Err()
	}
	if validate != nil {
		if err := validate(); err != nil {
			_ = b.Cancel(owner, ticket)
			return Handoff{}, err
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Handoff{}, err
	}
	b.expireLocked(time.Now())
	if e.terminal {
		return Handoff{}, terminalError(e)
	}
	if e.taken {
		return Handoff{}, ErrAlreadyTaken
	}
	if e.startCtx != nil {
		if err := e.startCtx.Err(); err != nil {
			_ = b.finishLocked(ticket, e, commandledger.Cancelled, ErrCancelled)
			return Handoff{}, ErrCancelled
		}
	}
	if !e.started {
		return Handoff{}, ErrNotStarted
	}
	e.taken = true
	return Handoff{Ticket: e.reservation.Ticket, InvocationID: e.reservation.InvocationID, CommandID: e.reservation.CommandID, HandoffID: e.handoffID}, nil
}

// ValidateHandoff verifica um handoff sem consumi-lo nem iniciar commit. A
// expiração é processada pelo lookup; nenhuma outra transição é produzida.
func (b *Broker) ValidateHandoff(owner Owner, ticket, handoffID string) error {
	if b == nil || !validOwner(owner) || strings.TrimSpace(ticket) == "" || strings.TrimSpace(handoffID) == "" {
		return ErrInvalidRequest
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.lookupLocked(owner, ticket)
	if err != nil {
		return err
	}
	if !e.taken {
		return ErrNotStarted
	}
	if e.handoffID != handoffID {
		return ErrOwnerMismatch
	}
	if e.committing {
		return ErrAlreadyCommitting
	}
	if e.startCtx != nil {
		if err := e.startCtx.Err(); err != nil {
			return ErrCancelled
		}
	}
	return nil
}

func (b *Broker) Complete(owner Owner, ticket, handoffID, status string) error {
	if b == nil || !validOwner(owner) || strings.TrimSpace(ticket) == "" || strings.TrimSpace(handoffID) == "" {
		return ErrInvalidRequest
	}
	statusValue := commandledger.Status(status)
	if statusValue != commandledger.Succeeded && statusValue != commandledger.Failed && statusValue != commandledger.Cancelled {
		return ErrInvalidRequest
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.lookupLocked(owner, ticket)
	if err != nil {
		return err
	}
	if !e.taken {
		return ErrNotStarted
	}
	if e.committing {
		return ErrAlreadyCommitting
	}
	if e.handoffID != handoffID {
		return ErrOwnerMismatch
	}
	if e.startCtx != nil {
		if err := e.startCtx.Err(); err != nil {
			_ = b.finishLocked(ticket, e, commandledger.OutcomeUnknown, commandledger.ErrInconsistent)
			return commandledger.ErrInconsistent
		}
	}
	return b.finishLocked(ticket, e, statusValue, nil)
}

// Commit consome o handoff para uma mutação contextual. O callback roda fora
// do mutex do broker; o estado committing impede replay/cancelamento concorrente
// enquanto a persistência e a revalidação de domínio acontecem.
func (b *Broker) Commit(ctx context.Context, owner Owner, ticket, handoffID string, apply func(context.Context) error) error {
	if b == nil || ctx == nil || !validOwner(owner) || strings.TrimSpace(ticket) == "" || strings.TrimSpace(handoffID) == "" || apply == nil {
		return ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	e, err := b.lookupLocked(owner, ticket)
	if err != nil {
		b.mu.Unlock()
		return err
	}
	if !e.taken {
		b.mu.Unlock()
		return ErrNotStarted
	}
	if e.handoffID != handoffID {
		b.mu.Unlock()
		return ErrOwnerMismatch
	}
	if e.committing {
		b.mu.Unlock()
		return ErrAlreadyCommitting
	}
	if e.startCtx != nil {
		if err := e.startCtx.Err(); err != nil {
			_ = b.finishLocked(ticket, e, commandledger.OutcomeUnknown, ErrCancelled)
			b.mu.Unlock()
			return ErrCancelled
		}
	}
	e.committing = true
	startCtx := e.startCtx
	b.mu.Unlock()

	applyCtx, applyCancel := context.WithCancel(ctx)
	stopStart := func() {}
	if startCtx != nil {
		stop := context.AfterFunc(startCtx, applyCancel)
		stopStart = func() { stop() }
	}
	applyErr := applySafely(applyCtx, apply)
	stopStart()
	applyCancel()
	b.mu.Lock()
	defer b.mu.Unlock()
	if e.terminal {
		if applyErr != nil {
			return applyErr
		}
		return ErrAlreadyCompleted
	}
	if applyErr != nil {
		status := commandledger.Failed
		switch {
		case errors.Is(applyErr, ErrCancelled):
			status = commandledger.Cancelled
		case errors.Is(applyErr, ErrOutcomeUnknown):
			status = commandledger.OutcomeUnknown
		}
		_ = b.finishLocked(ticket, e, status, applyErr)
		return applyErr
	}
	return b.finishLocked(ticket, e, commandledger.Succeeded, nil)
}

func applySafely(ctx context.Context, apply func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrOutcomeUnknown
		}
	}()
	return apply(ctx)
}

func (b *Broker) Cancel(owner Owner, ticket string) error {
	if b == nil || !validOwner(owner) || strings.TrimSpace(ticket) == "" {
		return ErrInvalidRequest
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.lookupLocked(owner, ticket)
	if err != nil {
		return err
	}
	if e.committing {
		return ErrAlreadyCommitting
	}
	status := commandledger.Cancelled
	if e.taken {
		status = commandledger.OutcomeUnknown
	}
	return b.finishLocked(ticket, e, status, ErrCancelled)
}

// Close sinaliza o encerramento sem aguardar a goroutine de expiração.
func (b *Broker) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	for ticket, e := range b.entries {
		if e.committing {
			continue
		}
		status := commandledger.Cancelled
		reason := ErrClosed
		if e.taken {
			status = commandledger.OutcomeUnknown
		}
		_ = b.finishLocked(ticket, e, status, reason)
	}
	b.mu.Unlock()
	b.stopOnce.Do(func() { close(b.stop) })
}

// Shutdown fecha o broker e aguarda a saída do expirer. Deve ser chamado fora
// de locks de App/gate; não inicia handlers nem faz I/O externo.
func (b *Broker) Shutdown(ctx context.Context) error {
	if b == nil || ctx == nil {
		return ErrInvalidRequest
	}
	b.Close()
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Broker) lookupLocked(owner Owner, ticket string) (*entry, error) {
	if b.closed {
		return nil, ErrClosed
	}
	b.expireLocked(time.Now())
	e, ok := b.entries[ticket]
	if !ok {
		return nil, ErrNotFound
	}
	if e.owner != owner {
		return nil, ErrOwnerMismatch
	}
	return e, nil
}

func (b *Broker) lookupInvocationLocked(owner Owner, invocationID string) (*entry, error) {
	if b.closed {
		return nil, ErrClosed
	}
	b.expireLocked(time.Now())
	e, ok := b.byInvocation[invocationID]
	if !ok {
		return nil, ErrNotFound
	}
	if e.owner != owner {
		return nil, ErrOwnerMismatch
	}
	return e, nil
}

func (b *Broker) finishLocked(ticket string, e *entry, status commandledger.Status, reason error) error {
	if e.terminal {
		return ErrAlreadyCompleted
	}
	e.terminal, e.status, e.terminalErr = true, status, reason
	e.readyOnce.Do(func() { close(e.ready) })
	e.done <- commandexecution.Outcome{Status: status, Result: json.RawMessage(`{}`)}
	delete(b.entries, ticket)
	delete(b.byInvocation, e.reservation.InvocationID)
	return nil
}

func (b *Broker) expireLocked(now time.Time) {
	for ticket, e := range b.entries {
		if e.committing {
			continue
		}
		if !now.Before(e.expiresAt) {
			status := commandledger.Cancelled
			if e.taken {
				status = commandledger.OutcomeUnknown
			}
			_ = b.finishLocked(ticket, e, status, ErrExpired)
		}
	}
}

func (b *Broker) expirer() {
	interval := b.ttl / 2
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer close(b.done)
	for {
		select {
		case now := <-ticker.C:
			b.mu.Lock()
			if !b.closed {
				b.expireLocked(now)
			}
			b.mu.Unlock()
		case <-b.stop:
			return
		}
	}
}

func validOwner(owner Owner) bool {
	if _, err := uuid.Parse(owner.UserID); err != nil {
		return false
	}
	if _, err := uuid.Parse(owner.SessionID); err != nil {
		return false
	}
	return strings.TrimSpace(owner.WorkspaceID) != ""
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func terminalError(e *entry) error {
	if e.terminalErr != nil {
		return e.terminalErr
	}
	if e.status == commandledger.OutcomeUnknown {
		return commandledger.ErrInconsistent
	}
	return ErrCancelled
}
