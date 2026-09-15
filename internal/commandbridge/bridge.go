// Package commandbridge contém a ponte tipada entre entradas de comando e um
// host confiável. Ela não conhece Wails, o sistema operacional ou handlers de
// produto: o host monta Port e decide como transportar a mensagem.
package commandbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"assistente/internal/commandinput"

	"github.com/google/uuid"
)

var (
	ErrInvalidConfiguration = errors.New("configuração da ponte de comandos inválida")
	ErrInvalidRequest       = errors.New("pedido da ponte de comandos inválido")
	ErrUnknownSession       = errors.New("sessão da ponte de comandos desconhecida")
	ErrStaleGeneration      = errors.New("geração da ponte de comandos obsoleta")
	ErrSessionUnavailable   = errors.New("sessão da ponte de comandos indisponível")
	ErrCapabilityDenied     = errors.New("capability da ponte de comandos negada")
	ErrOwnershipConflict    = errors.New("ownership da ponte de comandos em conflito")
	ErrInvocationReplay     = errors.New("invocação da ponte de comandos repetida")
	ErrUnknownInvocation    = errors.New("invocação da ponte de comandos desconhecida")
	ErrBridgeClosed         = errors.New("ponte de comandos encerrada")
)

type Ownership string

const (
	OwnershipLocal  Ownership = "local"
	OwnershipGlobal Ownership = "global"
)

type Source string

const (
	SourceKeyboardLocal  Source = "keyboard.local"
	SourceKeyboardGlobal Source = "keyboard.global"
	SourceStreamDeck     Source = "streamdeck.key"
	SourcePalette        Source = "palette"
	SourceUIAction       Source = "ui.action"
	SourceChat           Source = "chat"
	SourceCLI            Source = "cli"
	SourceEvent          Source = "event"
	SourceSystem         Source = "system"
)

// Owner é copiado no ingresso e comparado por valor em todos os retornos.
type Owner struct {
	UserID      string `json:"userId"`
	SessionID   string `json:"sessionId"`
	WorkspaceID string `json:"workspaceId"`
}

// Capability é uma autorização opaca, vinculada a comando, owner e geração.
type Capability struct {
	ID         string `json:"id"`
	CommandID  string `json:"commandId"`
	Generation uint64 `json:"generation,string"`
	Owner      Owner  `json:"owner"`
}

// Session é o vínculo autenticado de uma ponte.
type Session struct {
	ID         string `json:"id"`
	Generation uint64 `json:"generation,string"`
	Owner      Owner  `json:"owner"`
}

// Invocation é o envelope mínimo transportado pela porta confiável.
type Invocation struct {
	SessionID    string    `json:"sessionId"`
	InvocationID string    `json:"invocationId"`
	CommandID    string    `json:"commandId"`
	Generation   uint64    `json:"generation,string"`
	CapabilityID string    `json:"capabilityId"`
	Ownership    Ownership `json:"ownership"`
	Source       Source    `json:"source"`
	OccurrenceID string    `json:"occurrenceId,omitempty"`
	EventID      string    `json:"eventId,omitempty"`
}

// InvocationAck confirma somente o encaminhamento; não significa execução.
type InvocationAck struct {
	InvocationID string `json:"invocationId"`
	Accepted     bool   `json:"accepted"`
	Reason       string `json:"reason,omitempty"`
}

type Result struct {
	SessionID    string          `json:"sessionId"`
	InvocationID string          `json:"invocationId"`
	CommandID    string          `json:"commandId"`
	Generation   uint64          `json:"generation,string"`
	CapabilityID string          `json:"capabilityId"`
	Ownership    Ownership       `json:"ownership"`
	OccurrenceID string          `json:"occurrenceId,omitempty"`
	EventID      string          `json:"eventId,omitempty"`
	Owner        Owner           `json:"owner"`
	Status       ResultStatus    `json:"status"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type ResultStatus string

const (
	ResultSucceeded ResultStatus = "succeeded"
	ResultFailed    ResultStatus = "failed"
	ResultCancelled ResultStatus = "cancelled"
)

type ResultAck struct {
	InvocationID string `json:"invocationId"`
	Accepted     bool   `json:"accepted"`
	Reason       string `json:"reason,omitempty"`
}

type CancelRequest struct {
	SessionID    string `json:"sessionId"`
	InvocationID string `json:"invocationId"`
	Generation   uint64 `json:"generation,string"`
	CapabilityID string `json:"capabilityId"`
	Owner        Owner  `json:"owner"`
}

type CancelAck struct {
	InvocationID string `json:"invocationId"`
	Accepted     bool   `json:"accepted"`
	Reason       string `json:"reason,omitempty"`
}

// Port é a única saída da ponte. Dispatch deve ser um handoff não bloqueante:
// ele confirma o aceite e retorna; espera de execução pertence ao host. A
// ponte segura o gate apenas durante esse handoff curto.
type Port interface {
	Dispatch(context.Context, Invocation) (InvocationAck, error)
	Cancel(context.Context, CancelRequest) error
}

// ShutdownPort é uma capacidade opcional da porta para liberar o adapter.
// A ponte sempre invalida e cancela seu estado antes de chamá-la; portas que
// não possuem recursos próprios podem implementar somente Port. Shutdown e
// Cancel devem honrar o contexto recebido: a ponte aguarda cancelamentos já
// admitidos e não promete um prazo máximo de encerramento.
type ShutdownPort interface {
	Shutdown(context.Context) error
}

type Config struct {
	Port         Port
	Capabilities []Capability
}

type Bridge struct {
	mu             sync.Mutex
	capabilityGate sync.RWMutex
	port           Port
	capabilities   map[string]Capability
	sessions       map[string]*sessionState
	claims         map[string]struct{}
	pending        map[string]pendingInvocation
	closed         bool
	shutdownDone   chan struct{}
	shutdownErr    error
	cancelWG       sync.WaitGroup
}

type sessionState struct {
	session      Session
	tracker      *commandinput.Tracker
	locked       bool
	dispatchGate sync.RWMutex
}

type pendingInvocation struct {
	invocation Invocation
	owner      Owner
	claimKey   string
}

// New copia capabilities para que alterações posteriores do montador não
// mudem a autorização publicada.
func New(config Config) (*Bridge, error) {
	if config.Port == nil || len(config.Capabilities) == 0 {
		return nil, ErrInvalidConfiguration
	}
	caps := make(map[string]Capability, len(config.Capabilities))
	for _, capability := range config.Capabilities {
		if !validCapability(capability) {
			return nil, fmt.Errorf("%w: capability incompleta", ErrInvalidConfiguration)
		}
		if _, exists := caps[capability.ID]; exists {
			return nil, fmt.Errorf("%w: capability duplicada", ErrInvalidConfiguration)
		}
		caps[capability.ID] = capability
	}
	return &Bridge{port: config.Port, capabilities: caps, sessions: make(map[string]*sessionState), claims: make(map[string]struct{}), pending: make(map[string]pendingInvocation)}, nil
}

// OpenSession abre uma sessão nova. Um SessionID já usado não pode ser
// reaberto, inclusive após logout, impedindo replay de callbacks antigos.
func (b *Bridge) OpenSession(session Session) error {
	if b == nil || !validSession(session) {
		return ErrInvalidRequest
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrBridgeClosed
	}
	if _, exists := b.sessions[session.ID]; exists {
		return ErrInvalidRequest
	}
	tracker := commandinput.New()
	if err := tracker.Reset(session.Generation); err != nil {
		return err
	}
	b.sessions[session.ID] = &sessionState{session: session, tracker: tracker}
	return nil
}

// AdvanceGeneration troca a geração e cancela, fora do mutex, as invocações
// da geração anterior. A mudança é atômica para novas entradas.
func (b *Bridge) AdvanceGeneration(ctx context.Context, sessionID string, generation uint64) error {
	if b == nil || ctx == nil || strings.TrimSpace(sessionID) == "" || generation == 0 {
		return ErrInvalidRequest
	}
	b.mu.Lock()
	state, ok := b.sessions[sessionID]
	if b.closed {
		b.mu.Unlock()
		return ErrBridgeClosed
	}
	b.mu.Unlock()
	if !ok {
		return ErrUnknownSession
	}
	state.dispatchGate.Lock()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		state.dispatchGate.Unlock()
		return ErrBridgeClosed
	}
	if state.locked {
		b.mu.Unlock()
		state.dispatchGate.Unlock()
		return ErrSessionUnavailable
	}
	if generation <= state.session.Generation {
		b.mu.Unlock()
		state.dispatchGate.Unlock()
		return ErrStaleGeneration
	}
	state.session.Generation = generation
	if err := state.tracker.Reset(generation); err != nil {
		b.mu.Unlock()
		state.dispatchGate.Unlock()
		return err
	}
	cancel := b.invalidateSessionLocked(sessionID)
	b.cancelWG.Add(1)
	b.mu.Unlock()
	state.dispatchGate.Unlock()
	return b.cancelTracked(ctx, cancel)
}

// ReplaceCapabilities publica uma nova fotografia de capabilities. O host
// deve chamá-la depois de AdvanceGeneration para emitir autorizações da nova
// geração; a fotografia anterior deixa de ser válida de forma atômica.
func (b *Bridge) ReplaceCapabilities(capabilities []Capability) error {
	if b == nil || len(capabilities) == 0 {
		return ErrInvalidConfiguration
	}
	caps := make(map[string]Capability, len(capabilities))
	for _, capability := range capabilities {
		if !validCapability(capability) {
			return fmt.Errorf("%w: capability incompleta", ErrInvalidConfiguration)
		}
		if _, exists := caps[capability.ID]; exists {
			return fmt.Errorf("%w: capability duplicada", ErrInvalidConfiguration)
		}
		caps[capability.ID] = capability
	}
	b.capabilityGate.Lock()
	defer b.capabilityGate.Unlock()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBridgeClosed
	}
	b.capabilities = caps
	b.mu.Unlock()
	return nil
}

// Invoke registra a pendência antes do callback. Isso fecha a janela em que
// uma porta poderia devolver o resultado imediatamente.
func (b *Bridge) Invoke(ctx context.Context, invocation Invocation, owner Owner) (InvocationAck, error) {
	if b == nil || ctx == nil || !validInvocation(invocation) || !validOwner(owner) {
		return InvocationAck{}, ErrInvalidRequest
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return InvocationAck{}, ErrBridgeClosed
	}
	state, ok := b.sessions[invocation.SessionID]
	if !ok {
		b.mu.Unlock()
		return InvocationAck{}, ErrUnknownSession
	}
	if state.locked {
		b.mu.Unlock()
		return InvocationAck{}, ErrSessionUnavailable
	}
	if invocation.Generation != state.session.Generation || !sameOwner(owner, state.session.Owner) {
		b.mu.Unlock()
		return InvocationAck{}, ErrStaleGeneration
	}
	capability, ok := b.capabilities[invocation.CapabilityID]
	if !ok || capability.CommandID != invocation.CommandID || capability.Generation != invocation.Generation || !sameOwner(capability.Owner, owner) {
		b.mu.Unlock()
		return InvocationAck{}, ErrCapabilityDenied
	}
	if _, exists := b.pending[invocation.InvocationID]; exists {
		b.mu.Unlock()
		return InvocationAck{}, ErrInvocationReplay
	}
	claimKey := ""
	if invocation.OccurrenceID != "" {
		claimKey = ownershipClaimKey(invocation, owner)
		if _, exists := b.claims[claimKey]; exists {
			b.mu.Unlock()
			return InvocationAck{}, ErrOwnershipConflict
		}
		b.claims[claimKey] = struct{}{}
	}
	b.pending[invocation.InvocationID] = pendingInvocation{invocation: invocation, owner: owner, claimKey: claimKey}
	b.mu.Unlock()

	state.dispatchGate.RLock()
	b.capabilityGate.RLock()
	b.mu.Lock()
	current, stillPending := b.pending[invocation.InvocationID]
	currentCapability, capabilityStillValid := b.capabilities[invocation.CapabilityID]
	if b.closed || !stillPending || current.invocation != invocation || state.locked || state.session.Generation != invocation.Generation || !capabilityStillValid || currentCapability.CommandID != invocation.CommandID || currentCapability.Generation != invocation.Generation || !sameOwner(currentCapability.Owner, owner) {
		if stillPending {
			b.removePendingLocked(invocation.InvocationID)
		}
		b.mu.Unlock()
		b.capabilityGate.RUnlock()
		state.dispatchGate.RUnlock()
		return InvocationAck{}, ErrStaleGeneration
	}
	b.mu.Unlock()

	ack, err := b.port.Dispatch(ctx, invocation)
	b.capabilityGate.RUnlock()
	state.dispatchGate.RUnlock()
	if err != nil || !ack.Accepted {
		b.mu.Lock()
		b.removePendingLocked(invocation.InvocationID)
		b.mu.Unlock()
		if err != nil {
			return InvocationAck{}, err
		}
		if ack.InvocationID != "" && ack.InvocationID != invocation.InvocationID {
			return InvocationAck{}, ErrInvalidRequest
		}
		return InvocationAck{InvocationID: invocation.InvocationID, Reason: ack.Reason}, ErrInvalidRequest
	}
	if ack.InvocationID != "" && ack.InvocationID != invocation.InvocationID {
		b.mu.Lock()
		b.removePendingLocked(invocation.InvocationID)
		b.mu.Unlock()
		return InvocationAck{}, ErrInvalidRequest
	}
	return InvocationAck{InvocationID: invocation.InvocationID, Accepted: true, Reason: ack.Reason}, nil
}

// AcceptResult fecha uma pendência somente com todos os vínculos da
// invocação. Resultados atrasados ou duplicados nunca liberam um claim atual.
func (b *Bridge) AcceptResult(result Result) (ResultAck, error) {
	if b == nil || !validResult(result) {
		return ResultAck{}, ErrInvalidRequest
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ResultAck{}, ErrBridgeClosed
	}
	defer b.mu.Unlock()
	pending, ok := b.pending[result.InvocationID]
	if !ok {
		return ResultAck{}, ErrUnknownInvocation
	}
	want := pending.invocation
	if result.SessionID != want.SessionID || result.CommandID != want.CommandID || result.Generation != want.Generation || result.CapabilityID != want.CapabilityID || result.Ownership != want.Ownership || result.OccurrenceID != want.OccurrenceID || result.EventID != want.EventID || !sameOwner(result.Owner, pending.owner) {
		return ResultAck{}, ErrInvalidRequest
	}
	b.removePendingLocked(result.InvocationID)
	return ResultAck{InvocationID: result.InvocationID, Accepted: true}, nil
}

// Cancel libera ownership somente depois da confirmação da porta.
func (b *Bridge) Cancel(ctx context.Context, request CancelRequest) (CancelAck, error) {
	if b == nil || ctx == nil || !validCancelRequest(request) {
		return CancelAck{}, ErrInvalidRequest
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return CancelAck{}, ErrBridgeClosed
	}
	pending, ok := b.pending[request.InvocationID]
	if !ok {
		b.mu.Unlock()
		return CancelAck{}, ErrUnknownInvocation
	}
	if !matchesCancel(pending, request) {
		b.mu.Unlock()
		return CancelAck{}, ErrInvalidRequest
	}
	b.cancelWG.Add(1)
	b.mu.Unlock()
	defer b.cancelWG.Done()
	if err := b.port.Cancel(ctx, request); err != nil {
		return CancelAck{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	current, exists := b.pending[request.InvocationID]
	if !exists || current != pending {
		return CancelAck{}, ErrUnknownInvocation
	}
	b.removePendingLocked(request.InvocationID)
	return CancelAck{InvocationID: request.InvocationID, Accepted: true}, nil
}

// Input aplica a normalização de down/repeat/up. Só a primeira borda de down
// chega a Invoke; repeat, release e callbacks de geração antiga não executam.
type Input struct {
	SessionID  string
	Source     string
	Key        string
	Generation uint64
	Kind       commandinput.EventKind
	Repeat     bool
	Invocation Invocation
	Owner      Owner
}

func (b *Bridge) Input(ctx context.Context, input Input) (InvocationAck, error) {
	if b == nil || ctx == nil || strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.Key) == "" {
		return InvocationAck{}, ErrInvalidRequest
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return InvocationAck{}, ErrBridgeClosed
	}
	state, ok := b.sessions[input.SessionID]
	if !ok {
		b.mu.Unlock()
		return InvocationAck{}, ErrUnknownSession
	}
	if state.locked {
		b.mu.Unlock()
		return InvocationAck{}, ErrSessionUnavailable
	}
	edge, err := state.tracker.Transition(commandinput.Event{SourceInstance: input.Source, Key: input.Key, Generation: input.Generation, Kind: input.Kind, Repeat: input.Repeat})
	b.mu.Unlock()
	if err != nil {
		return InvocationAck{}, err
	}
	if input.Kind == commandinput.KeyUp || !edge {
		return InvocationAck{Accepted: false, Reason: "not-a-dispatch-edge"}, nil
	}
	if input.Invocation.SessionID != input.SessionID || input.Invocation.Generation != input.Generation {
		return InvocationAck{}, ErrInvalidRequest
	}
	normalized := normalizeOccurrence(input.Source, input.Key)
	invocation := input.Invocation
	if invocation.OccurrenceID == "" {
		invocation.OccurrenceID = normalized
	} else if invocation.OccurrenceID != normalized {
		return InvocationAck{}, ErrInvalidRequest
	}
	return b.Invoke(ctx, invocation, input.Owner)
}

type LifecycleKind uint8

const (
	LifecycleGeneration LifecycleKind = iota + 1
	LifecycleRepeat
	LifecycleRelease
	LifecycleBlur
	LifecycleLock
	LifecycleLogout
)

type LifecycleEvent struct {
	Kind       LifecycleKind
	SessionID  string
	Generation uint64
	Input      *Input
}

// Lifecycle aplica geração, repeat, release, blur, lock e logout. Lock/logout
// invalidam pendências e claims antes de qualquer callback de cancelamento.
func (b *Bridge) Lifecycle(ctx context.Context, event LifecycleEvent) error {
	if b == nil || ctx == nil || strings.TrimSpace(event.SessionID) == "" {
		return ErrInvalidRequest
	}
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return ErrBridgeClosed
	}
	switch event.Kind {
	case LifecycleGeneration:
		return b.AdvanceGeneration(ctx, event.SessionID, event.Generation)
	case LifecycleRepeat, LifecycleRelease:
		if event.Input == nil {
			return ErrInvalidRequest
		}
		input := *event.Input
		input.SessionID = event.SessionID
		if event.Kind == LifecycleRepeat {
			input.Kind, input.Repeat = commandinput.KeyDown, true
		} else {
			input.Kind, input.Repeat = commandinput.KeyUp, false
		}
		_, err := b.Input(ctx, input)
		return err
	case LifecycleBlur:
		b.mu.Lock()
		state, ok := b.sessions[event.SessionID]
		if !ok {
			b.mu.Unlock()
			return ErrUnknownSession
		}
		err := state.tracker.Blur(event.Generation)
		b.mu.Unlock()
		return err
	case LifecycleLock, LifecycleLogout:
		b.mu.Lock()
		state, ok := b.sessions[event.SessionID]
		if !ok {
			b.mu.Unlock()
			return ErrUnknownSession
		}
		b.mu.Unlock()
		state.dispatchGate.Lock()
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			state.dispatchGate.Unlock()
			return ErrBridgeClosed
		}
		if event.Generation != 0 && event.Generation != state.session.Generation {
			b.mu.Unlock()
			state.dispatchGate.Unlock()
			return ErrStaleGeneration
		}
		state.locked = true
		cancel := b.invalidateSessionLocked(event.SessionID)
		b.cancelWG.Add(1)
		b.mu.Unlock()
		state.dispatchGate.Unlock()
		return b.cancelTracked(ctx, cancel)
	default:
		return ErrInvalidRequest
	}
}

// Shutdown encerra a ponte uma única vez. Ele marca todas as sessões como
// indisponíveis, limpa pressão e claims, aguarda somente o handoff curto de
// cada Dispatch, cancela as pendências fora dos gates e por fim libera a
// porta, quando ela implementa ShutdownPort. O método é idempotente depois
// da primeira chamada, inclusive quando a porta reporta erro. Uma chamada
// concorrente pode retornar ctx.Err enquanto a primeira ainda encerra; isso
// não antecipa a liberação da porta.
func (b *Bridge) Shutdown(ctx context.Context) error {
	if b == nil || ctx == nil {
		return ErrInvalidRequest
	}
	b.mu.Lock()
	if b.closed {
		done := b.shutdownDone
		b.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			}
			b.mu.Lock()
			err := b.shutdownErr
			b.mu.Unlock()
			return err
		}
		return nil
	}
	b.closed = true
	b.shutdownDone = make(chan struct{})
	done := b.shutdownDone
	states := make([]*sessionState, 0, len(b.sessions))
	for _, state := range b.sessions {
		states = append(states, state)
	}
	b.mu.Unlock()

	cancel := make([]CancelRequest, 0)
	for _, state := range states {
		state.dispatchGate.Lock()
		b.mu.Lock()
		state.locked = true
		_ = state.tracker.Blur(state.session.Generation)
		cancel = append(cancel, b.invalidateSessionLocked(state.session.ID)...)
		b.mu.Unlock()
		state.dispatchGate.Unlock()
	}

	b.cancelWG.Wait()
	var first error
	if err := b.cancelAll(ctx, cancel); err != nil {
		first = err
	}
	if shutdownPort, ok := b.port.(ShutdownPort); ok {
		if err := shutdownPort.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	b.mu.Lock()
	clear(b.sessions)
	clear(b.capabilities)
	clear(b.pending)
	clear(b.claims)
	b.shutdownErr = first
	close(done)
	b.mu.Unlock()
	return first
}

func (b *Bridge) invalidateSessionLocked(sessionID string) []CancelRequest {
	cancel := make([]CancelRequest, 0)
	for id, pending := range b.pending {
		if pending.invocation.SessionID != sessionID {
			continue
		}
		cancel = append(cancel, CancelRequest{SessionID: sessionID, InvocationID: id, Generation: pending.invocation.Generation, CapabilityID: pending.invocation.CapabilityID, Owner: pending.owner})
		b.removePendingLocked(id)
	}
	return cancel
}

func (b *Bridge) cancelAll(ctx context.Context, requests []CancelRequest) error {
	var first error
	for _, request := range requests {
		if err := b.port.Cancel(ctx, request); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (b *Bridge) cancelTracked(ctx context.Context, requests []CancelRequest) error {
	defer b.cancelWG.Done()
	return b.cancelAll(ctx, requests)
}

func (b *Bridge) removePendingLocked(invocationID string) {
	pending, ok := b.pending[invocationID]
	if !ok {
		return
	}
	delete(b.pending, invocationID)
	if pending.claimKey != "" {
		delete(b.claims, pending.claimKey)
	}
}

func validOwner(owner Owner) bool {
	return validText(owner.UserID) && validText(owner.SessionID) && validText(owner.WorkspaceID)
}

func sameOwner(left, right Owner) bool { return left == right }

func validSession(session Session) bool {
	return strings.TrimSpace(session.ID) == session.ID && session.ID != "" && session.Generation > 0 && validOwner(session.Owner) && session.Owner.SessionID == session.ID
}

func validCapability(capability Capability) bool {
	return strings.TrimSpace(capability.ID) == capability.ID && capability.ID != "" && strings.TrimSpace(capability.CommandID) == capability.CommandID && capability.CommandID != "" && capability.Generation > 0 && validOwner(capability.Owner)
}

func validInvocation(invocation Invocation) bool {
	if !validText(invocation.SessionID) || !validUUIDv7(invocation.InvocationID) || !validText(invocation.CommandID) || invocation.Generation == 0 || !validText(invocation.CapabilityID) || !validOwnership(invocation.Ownership) || !validSource(invocation.Source) || (invocation.OccurrenceID != "" && !validText(invocation.OccurrenceID)) {
		return false
	}
	if invocation.Source == SourceEvent {
		return validUUIDv7(invocation.EventID)
	}
	return invocation.EventID == ""
}

func validOwnership(ownership Ownership) bool {
	return ownership == OwnershipLocal || ownership == OwnershipGlobal
}

func validResult(result Result) bool {
	return validInvocation(Invocation{SessionID: result.SessionID, InvocationID: result.InvocationID, CommandID: result.CommandID, Generation: result.Generation, CapabilityID: result.CapabilityID, Ownership: result.Ownership, Source: SourceUIAction, OccurrenceID: result.OccurrenceID}) && (result.EventID == "" || validUUIDv7(result.EventID)) && validOwner(result.Owner) && validResultStatus(result.Status)
}

func validResultStatus(status ResultStatus) bool {
	return status == ResultSucceeded || status == ResultFailed || status == ResultCancelled
}

func validSource(source Source) bool {
	switch source {
	case SourceKeyboardLocal, SourceKeyboardGlobal, SourceStreamDeck, SourcePalette, SourceUIAction, SourceChat, SourceCLI, SourceEvent, SourceSystem:
		return true
	default:
		return false
	}
}

// validUUIDv7 aplica-se a IDs de invocação e eventos de origem. OccurrenceID
// é uma chave opaca do adapter físico e deliberadamente não passa por aqui.
func validUUIDv7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func validCancelRequest(request CancelRequest) bool {
	return strings.TrimSpace(request.SessionID) == request.SessionID && request.SessionID != "" && strings.TrimSpace(request.InvocationID) == request.InvocationID && request.InvocationID != "" && request.Generation > 0 && strings.TrimSpace(request.CapabilityID) == request.CapabilityID && request.CapabilityID != "" && validOwner(request.Owner)
}

func matchesCancel(pending pendingInvocation, request CancelRequest) bool {
	invocation := pending.invocation
	return invocation.SessionID == request.SessionID && invocation.Generation == request.Generation && invocation.CapabilityID == request.CapabilityID && sameOwner(pending.owner, request.Owner)
}

func validText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}

func normalizeOccurrence(source, key string) string {
	return fmt.Sprintf("%d:%s%d:%s", len(source), source, len(key), key)
}

func ownershipClaimKey(invocation Invocation, owner Owner) string {
	// O commandID fica deliberadamente fora: ownership pertence ao adapter/
	// trigger físico, não ao comando. Invocações diretas sem ocorrência não
	// adquirem claim e podem coexistir legitimamente.
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%s", invocation.SessionID, owner.UserID, owner.SessionID, owner.WorkspaceID, invocation.Generation, invocation.OccurrenceID)
}
