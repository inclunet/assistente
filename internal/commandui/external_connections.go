package commandui

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"assistente/internal/commandbridge"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"

	"github.com/google/uuid"
)

var (
	ErrExternalConnectionInvalid     = errors.New("conexão externa inválida")
	ErrExternalConnectionDenied      = errors.New("conexão externa recusada")
	ErrExternalConnectionStale       = errors.New("contexto da conexão externa obsoleto")
	ErrExternalConnectionFull        = errors.New("limite de conexões externas atingido")
	ErrExternalConnectionUnavailable = errors.New("transporte de conexão externa indisponível")
	ErrExternalConnectionClosed      = errors.New("registry de conexão externa encerrada")
)

const (
	externalInviteTTL     = 2 * time.Minute
	externalConnectionTTL = 45 * time.Second
	externalConnectionMax = 32
	externalOwnerStateMax = 64
)

type ExternalUIPrincipal struct {
	Issuer        string
	Subject       string
	UserID        string
	AuthContextID string
}

// ExternalUISurfaceContext espelha os campos de SurfaceContext do frontend.
// O payload completo fica apenas no estado local da conexão; HTTP recebe stamps.
type ExternalUISurfaceContext struct {
	SurfaceType     string          `json:"surfaceType"`
	SurfaceID       string          `json:"surfaceId"`
	Title           string          `json:"title,omitempty"`
	Mode            string          `json:"mode,omitempty"`
	Selection       json.RawMessage `json:"selection,omitempty"`
	Focus           json.RawMessage `json:"focus,omitempty"`
	Content         json.RawMessage `json:"content,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	SnapshotVersion string          `json:"snapshotVersion"`
	CapturedAt      time.Time       `json:"capturedAt,omitempty"`
	StaleAfterMs    int64           `json:"staleAfterMs,omitempty"`
}

type ExternalUIDestination struct {
	WorkspaceID string                   `json:"workspaceId"`
	TabID       string                   `json:"tabId,omitempty"`
	Surface     ExternalUISurfaceContext `json:"surface"`
}

type ExternalUIConnectionStatus struct {
	State            string                `json:"state"`
	ConnectionID     string                `json:"connectionId,omitempty"`
	Generation       string                `json:"generation,omitempty"`
	Owner            commandbridge.Owner   `json:"owner"`
	Target           ExternalUIDestination `json:"target"`
	TargetSnapshotID string                `json:"targetSnapshotId,omitempty"`
	ContextVersion   string                `json:"contextVersion,omitempty"`
	ExpiresAt        time.Time             `json:"expiresAt,omitempty"`
}

type ExternalUIInvitation struct {
	Invitation string    `json:"invitation"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type ExternalUIContextPublication struct {
	Owner                  commandbridge.Owner   `json:"owner"`
	ConnectionID           string                `json:"connectionId"`
	Generation             string                `json:"generation"`
	ExpectedTargetSnapshot string                `json:"expectedTargetSnapshotId"`
	ExpectedContextVersion string                `json:"expectedContextVersion"`
	Target                 ExternalUIDestination `json:"target"`
}

type ExternalUILease struct {
	Owner        commandbridge.Owner `json:"owner"`
	ConnectionID string              `json:"connectionId"`
	Generation   string              `json:"generation"`
}

type externalInvitation struct {
	hash      string
	owner     commandbridge.Owner
	target    ExternalUIDestination
	expiresAt time.Time
}

type externalConnection struct {
	status    ExternalUIConnectionStatus
	principal ExternalUIPrincipal
	issuer    string
	subject   string
}

// ExternalUIConnections é um store efêmero, limitado e por App. Não armazena
// JWTs; o grant roteia para um destino, mas não substitui a política do executor.
type ExternalUIConnections struct {
	mu               sync.Mutex
	now              func() time.Time
	invites          map[string]externalInvitation
	connections      map[string]externalConnection
	ownerInvites     map[string]string
	ownerConnections map[string]string
	ownerStates      map[string]ExternalUIConnectionStatus
	ownerStateAt     map[string]time.Time
	pending          map[string]*externalPending
	sequence         uint64
	notifyReady      func(ExternalUIReadyEvent)
	readyQueue       chan ExternalUIReadyEvent
	readyDone        chan struct{}
	readyWorker      bool
	closed           bool
}

type externalPending struct {
	connectionID, generation, invocationID, commandID string
	owner                                             commandbridge.Owner
	principal                                         ExternalUIPrincipal
	targetSnapshotID, contextVersion, receiptID       string
	arguments                                         json.RawMessage
	expiresAt                                         time.Time
	taken, terminal                                   bool
	done                                              chan commandexecution.Outcome
	timer                                             *time.Timer
	timerVersion                                      uint64
}

type ExternalUIReadyEvent struct {
	CommandID        string `json:"commandId"`
	ConnectionID     string `json:"connectionId"`
	Generation       string `json:"generation"`
	InvocationID     string `json:"invocationId"`
	TargetSnapshotID string `json:"targetSnapshotId"`
	ContextVersion   string `json:"contextVersion"`
}

type ExternalUIHandoff struct {
	InvocationID     string                `json:"invocationId"`
	CommandID        string                `json:"commandId"`
	Arguments        json.RawMessage       `json:"arguments"`
	ReceiptID        string                `json:"receiptId"`
	TargetSnapshotID string                `json:"targetSnapshotId"`
	ContextVersion   string                `json:"contextVersion"`
	Target           ExternalUIDestination `json:"target"`
}

func NewExternalUIConnections() *ExternalUIConnections {
	return &ExternalUIConnections{now: time.Now, invites: make(map[string]externalInvitation), connections: make(map[string]externalConnection), ownerInvites: make(map[string]string), ownerConnections: make(map[string]string), ownerStates: make(map[string]ExternalUIConnectionStatus), ownerStateAt: make(map[string]time.Time), pending: make(map[string]*externalPending), readyQueue: make(chan ExternalUIReadyEvent, externalConnectionMax), readyDone: make(chan struct{})}
}

// SetReadyNotifier instala o consumidor de uma fila limitada. O callback roda
// em worker fora do gate de execução e deve apenas publicar/enfileirar o evento;
// nunca aguarda a UI nem realiza o efeito do comando.
func (s *ExternalUIConnections) SetReadyNotifier(notify func(ExternalUIReadyEvent)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.notifyReady = notify
	if notify != nil && !s.readyWorker {
		s.readyWorker = true
		go s.runReadyNotifier()
	}
	s.mu.Unlock()
}

func (s *ExternalUIConnections) runReadyNotifier() {
	defer close(s.readyDone)
	for event := range s.readyQueue {
		s.mu.Lock()
		notify := s.notifyReady
		pending, active := s.pending[event.InvocationID]
		active = !s.closed && active && !pending.terminal && pending.commandID == event.CommandID && pending.connectionID == event.ConnectionID && pending.generation == event.Generation && pending.targetSnapshotID == event.TargetSnapshotID && pending.contextVersion == event.ContextVersion
		s.mu.Unlock()
		if notify != nil && active {
			func() {
				defer func() { _ = recover() }()
				notify(event)
			}()
		}
	}
}

func (s *ExternalUIConnections) Begin(owner commandbridge.Owner, target ExternalUIDestination) (ExternalUIInvitation, error) {
	if s == nil || !validExternalOwner(owner) || !validExternalDestination(target) {
		return ExternalUIInvitation{}, ErrExternalConnectionInvalid
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return ExternalUIInvitation{}, ErrExternalConnectionInvalid
	}
	invitation := base64.RawURLEncoding.EncodeToString(raw)
	clear(raw)
	hashBytes := sha256.Sum256([]byte(invitation))
	hash := hex.EncodeToString(hashBytes[:])
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ExternalUIInvitation{}, ErrExternalConnectionClosed
	}
	s.pruneLocked(now)
	if len(s.invites)+len(s.connections) >= externalConnectionMax {
		return ExternalUIInvitation{}, ErrExternalConnectionFull
	}
	ownerKey := externalOwnerKey(owner)
	if s.ownerInvites[ownerKey] != "" || s.ownerConnections[ownerKey] != "" {
		return ExternalUIInvitation{}, ErrExternalConnectionDenied
	}
	s.invites[hash] = externalInvitation{hash: hash, owner: owner, target: cloneExternalDestination(target), expiresAt: now.Add(externalInviteTTL)}
	s.ownerInvites[ownerKey] = hash
	s.setOwnerStateLocked(ownerKey, ExternalUIConnectionStatus{State: "waiting_claim", Owner: owner, Target: cloneExternalDestination(target), ExpiresAt: now.Add(externalInviteTTL)}, now)
	return ExternalUIInvitation{Invitation: invitation, ExpiresAt: now.Add(externalInviteTTL)}, nil
}

func (s *ExternalUIConnections) Claim(invitation string, principal ExternalUIPrincipal) (ExternalUIConnectionStatus, error) {
	if s == nil || len(invitation) < 32 || len(invitation) > 128 || !validExternalPrincipal(principal) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	hashBytes := sha256.Sum256([]byte(invitation))
	hash := hex.EncodeToString(hashBytes[:])
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionClosed
	}
	s.pruneLocked(now)
	entry, ok := s.invites[hash]
	if !ok || !entry.expiresAt.After(now) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionDenied
	}
	if entry.owner.UserID != principal.UserID {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionDenied
	}
	if s.sequence == ^uint64(0) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionFull
	}
	connectionID, err := uuid.NewV7()
	if err != nil {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	s.sequence++
	generation := strconv.FormatUint(s.sequence, 10)
	targetSnapshotID, err := uuid.NewV7()
	if err != nil {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	contextVersion, err := uuid.NewV7()
	if err != nil {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	delete(s.invites, hash) // uso único, inclusive em nova tentativa concorrente
	status := ExternalUIConnectionStatus{State: "connected", ConnectionID: connectionID.String(), Generation: generation, Owner: entry.owner, Target: cloneExternalDestination(entry.target), TargetSnapshotID: targetSnapshotID.String(), ContextVersion: contextVersion.String(), ExpiresAt: now.Add(externalConnectionTTL)}
	s.connections[status.ConnectionID] = externalConnection{status: status, principal: cloneExternalPrincipal(principal), issuer: principal.Issuer, subject: principal.Subject}
	ownerKey := externalOwnerKey(entry.owner)
	delete(s.ownerInvites, ownerKey)
	s.ownerConnections[ownerKey] = status.ConnectionID
	s.setOwnerStateLocked(ownerKey, cloneExternalStatus(status), now)
	return cloneExternalStatus(status), nil
}

func (s *ExternalUIConnections) ReadConnection(owner commandbridge.Owner, principal ExternalUIPrincipal, connectionID, generation string) (ExternalUIConnectionStatus, error) {
	if s == nil || !validExternalOwner(owner) || !validExternalPrincipal(principal) || !validUUIDv7(connectionID) || !validGeneration(generation) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	s.pruneLocked(now)
	connection, ok := s.connections[connectionID]
	if !ok || connection.status.Generation != generation || connection.status.Owner != owner || !sameExternalPrincipal(connection.principal, principal) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionDenied
	}
	return cloneExternalStatus(connection.status), nil
}

// ReadForPrincipal é a leitura explícita do cliente JWT: não há seleção da
// última conexão. O connectionID e a generation devem vir do claim/cliente.
func (s *ExternalUIConnections) ReadForPrincipal(principal ExternalUIPrincipal, connectionID, generation string) (ExternalUIConnectionStatus, error) {
	if s == nil || !validExternalPrincipal(principal) || !validUUIDv7(connectionID) || !validGeneration(generation) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	connection, ok := s.connections[connectionID]
	if !ok || connection.status.Generation != generation || !sameExternalPrincipal(connection.principal, principal) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionDenied
	}
	return cloneExternalStatus(connection.status), nil
}

func (s *ExternalUIConnections) PublishContext(publication ExternalUIContextPublication) (ExternalUIConnectionStatus, error) {
	if s == nil || !validExternalOwner(publication.Owner) || !validUUIDv7(publication.ConnectionID) || !validGeneration(publication.Generation) ||
		!validUUIDv7(publication.ExpectedTargetSnapshot) || !validUUIDv7(publication.ExpectedContextVersion) || !validExternalDestination(publication.Target) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	s.pruneLocked(now)
	connection, ok := s.connections[publication.ConnectionID]
	if !ok || connection.status.Generation != publication.Generation || connection.status.Owner != publication.Owner ||
		connection.status.TargetSnapshotID != publication.ExpectedTargetSnapshot || connection.status.ContextVersion != publication.ExpectedContextVersion {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionStale
	}
	if publication.Target.WorkspaceID != connection.status.Target.WorkspaceID || publication.Target.WorkspaceID != publication.Owner.WorkspaceID {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionDenied
	}
	snapshotID, err := uuid.NewV7()
	if err != nil {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	contextVersion, err := uuid.NewV7()
	if err != nil {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	connection.status.Target = cloneExternalDestination(publication.Target)
	connection.status.TargetSnapshotID = snapshotID.String()
	connection.status.ContextVersion = contextVersion.String()
	connection.status.ExpiresAt = now.Add(externalConnectionTTL)
	s.connections[publication.ConnectionID] = connection
	key := externalOwnerKey(publication.Owner)
	s.setOwnerStateLocked(key, cloneExternalStatus(connection.status), now)
	s.invalidateConnectionPendingLocked(publication.ConnectionID, publication.ExpectedTargetSnapshot, publication.ExpectedContextVersion)
	return cloneExternalStatus(connection.status), nil
}

func (s *ExternalUIConnections) Heartbeat(lease ExternalUILease) (ExternalUIConnectionStatus, error) {
	if s == nil || !validExternalOwner(lease.Owner) || !validUUIDv7(lease.ConnectionID) || !validGeneration(lease.Generation) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	s.pruneLocked(now)
	connection, ok := s.connections[lease.ConnectionID]
	if !ok || connection.status.Generation != lease.Generation || connection.status.Owner != lease.Owner {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionDenied
	}
	connection.status.ExpiresAt = now.Add(externalConnectionTTL)
	s.connections[lease.ConnectionID] = connection
	s.setOwnerStateLocked(externalOwnerKey(lease.Owner), cloneExternalStatus(connection.status), now)
	return cloneExternalStatus(connection.status), nil
}

func (s *ExternalUIConnections) Disconnect(lease ExternalUILease) error {
	if s == nil || !validExternalOwner(lease.Owner) || !validUUIDv7(lease.ConnectionID) || !validGeneration(lease.Generation) {
		return ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	connection, ok := s.connections[lease.ConnectionID]
	if !ok || connection.status.Generation != lease.Generation || connection.status.Owner != lease.Owner {
		return ErrExternalConnectionDenied
	}
	delete(s.connections, lease.ConnectionID)
	key := externalOwnerKey(lease.Owner)
	delete(s.ownerConnections, key)
	s.setOwnerStateLocked(key, ExternalUIConnectionStatus{State: "disconnected", Owner: lease.Owner}, s.now().UTC())
	s.invalidateConnectionPendingLocked(lease.ConnectionID, "", "")
	return nil
}

func (s *ExternalUIConnections) RevokePrincipal(issuer, subject string) {
	if s == nil || issuer == "" || subject == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	for id, connection := range s.connections {
		if connection.issuer == issuer && connection.subject == subject {
			key := externalOwnerKey(connection.status.Owner)
			delete(s.ownerConnections, key)
			s.setOwnerStateLocked(key, ExternalUIConnectionStatus{State: "disconnected", Owner: connection.status.Owner}, s.now().UTC())
			s.invalidateConnectionPendingLocked(id, "", "")
			delete(s.connections, id)
		}
	}
}

func (s *ExternalUIConnections) ReadForOwner(owner commandbridge.Owner) (ExternalUIConnectionStatus, error) {
	if s == nil || !validExternalOwner(owner) {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	status, ok := s.ownerStates[externalOwnerKey(owner)]
	if !ok {
		status = ExternalUIConnectionStatus{State: "disconnected", Owner: owner}
	}
	if status.Owner != owner {
		return ExternalUIConnectionStatus{}, ErrExternalConnectionDenied
	}
	return cloneExternalStatus(status), nil
}

func (s *ExternalUIConnections) RevokeOwner(owner commandbridge.Owner) {
	if s == nil || !validExternalOwner(owner) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	key := externalOwnerKey(owner)
	if hash := s.ownerInvites[key]; hash != "" {
		delete(s.invites, hash)
	}
	if id := s.ownerConnections[key]; id != "" {
		delete(s.connections, id)
		s.invalidateConnectionPendingLocked(id, "", "")
	}
	delete(s.ownerInvites, key)
	delete(s.ownerConnections, key)
	s.setOwnerStateLocked(key, ExternalUIConnectionStatus{State: "disconnected", Owner: owner}, s.now().UTC())
}

// Clear invalida links, convites e handoffs sem reiniciar a sequência de
// gerações; reset/logout não pode ressuscitar grants antigos (ABA).
func (s *ExternalUIConnections) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.clearLocked()
}

func (s *ExternalUIConnections) clearLocked() {
	for id := range s.connections {
		s.invalidateConnectionPendingLocked(id, "", "")
	}
	for id, pending := range s.pending {
		s.finishPendingLocked(id, pending, pendingTerminalOutcome(pending))
	}
	for {
		select {
		case <-s.readyQueue:
		default:
			goto readyQueueDrained
		}
	}
readyQueueDrained:
	clear(s.invites)
	clear(s.connections)
	clear(s.ownerInvites)
	clear(s.ownerConnections)
	clear(s.ownerStates)
	clear(s.ownerStateAt)
}

// Close encerra terminalmente a registry. O worker termina depois que um
// callback já em andamento retorna; este método não espera pelo callback.
func (s *ExternalUIConnections) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.clearLocked()
	close(s.readyQueue) // todos os sends são serializados pelo mesmo mutex.
	if !s.readyWorker {
		close(s.readyDone)
	}
	s.mu.Unlock()
}

func (s *ExternalUIConnections) StartExternal(owner commandbridge.Owner, principal ExternalUIPrincipal, connectionID, generation, targetSnapshotID, contextVersion, invocationID, commandID string, arguments json.RawMessage) (commandexecution.ExecutionHandle, error) {
	if s == nil || !validExternalOwner(owner) || !validExternalPrincipal(principal) || !validUUIDv7(connectionID) || !validGeneration(generation) || !validUUIDv7(targetSnapshotID) || !validUUIDv7(contextVersion) || !validUUIDv7(invocationID) || !validExternalText(commandID, 256) || len(arguments) == 0 || len(arguments) > 64*1024 || !json.Valid(arguments) || arguments[0] != '{' {
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionInvalid
	}
	args := append(json.RawMessage(nil), arguments...)
	receipt, err := uuid.NewV7()
	if err != nil {
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionInvalid
	}
	entry := &externalPending{connectionID: connectionID, generation: generation, invocationID: invocationID, commandID: commandID, owner: owner, principal: cloneExternalPrincipal(principal), targetSnapshotID: targetSnapshotID, contextVersion: contextVersion, receiptID: receipt.String(), arguments: args, expiresAt: s.now().UTC().Add(externalConnectionTTL), done: make(chan commandexecution.Outcome, 1)}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionClosed
	}
	s.pruneLocked(s.now().UTC())
	connection, ok := s.connections[connectionID]
	if !ok || connection.status.Generation != generation || connection.status.Owner != owner || !sameExternalPrincipal(connection.principal, principal) || connection.status.TargetSnapshotID != targetSnapshotID || connection.status.ContextVersion != contextVersion {
		s.mu.Unlock()
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionStale
	}
	if _, exists := s.pending[invocationID]; exists {
		s.mu.Unlock()
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionDenied
	}
	if len(s.pending) >= externalConnectionMax {
		s.mu.Unlock()
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionFull
	}
	s.pending[invocationID] = entry
	s.schedulePendingExpiryLocked(invocationID, entry)
	event := ExternalUIReadyEvent{CommandID: commandID, ConnectionID: connectionID, Generation: generation, InvocationID: invocationID, TargetSnapshotID: targetSnapshotID, ContextVersion: contextVersion}
	if s.notifyReady == nil {
		s.finishPendingLocked(invocationID, entry, commandledger.Cancelled)
		s.mu.Unlock()
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionUnavailable
	}
	select {
	case s.readyQueue <- event:
	default:
		s.finishPendingLocked(invocationID, entry, commandledger.Cancelled)
		s.mu.Unlock()
		return commandexecution.ExecutionHandle{}, ErrExternalConnectionFull
	}
	s.mu.Unlock()
	return commandexecution.ExecutionHandle{ID: receipt.String(), Done: entry.done, Cancel: func() { s.cancelExternal(invocationID) }}, nil
}

func (s *ExternalUIConnections) TakeExternal(owner commandbridge.Owner, connectionID, generation, invocationID, targetSnapshotID, contextVersion string) (ExternalUIHandoff, error) {
	if s == nil || !validExternalOwner(owner) || !validUUIDv7(connectionID) || !validGeneration(generation) || !validUUIDv7(invocationID) || !validUUIDv7(targetSnapshotID) || !validUUIDv7(contextVersion) {
		return ExternalUIHandoff{}, ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	pending, ok := s.pending[invocationID]
	connection, connected := s.connections[connectionID]
	if !ok || !connected || pending.terminal || pending.taken || pending.owner != owner || pending.connectionID != connectionID || pending.generation != generation || pending.targetSnapshotID != targetSnapshotID || pending.contextVersion != contextVersion || connection.status.Generation != generation || connection.status.TargetSnapshotID != targetSnapshotID || connection.status.ContextVersion != contextVersion || connection.status.Owner != owner {
		return ExternalUIHandoff{}, ErrExternalConnectionStale
	}
	pending.taken = true
	pending.expiresAt = s.now().UTC().Add(externalConnectionTTL)
	s.schedulePendingExpiryLocked(invocationID, pending)
	return ExternalUIHandoff{InvocationID: invocationID, CommandID: pending.commandID, Arguments: append(json.RawMessage(nil), pending.arguments...), ReceiptID: pending.receiptID, TargetSnapshotID: targetSnapshotID, ContextVersion: contextVersion, Target: cloneExternalDestination(connection.status.Target)}, nil
}

func (s *ExternalUIConnections) CompleteExternal(owner commandbridge.Owner, connectionID, generation, invocationID, receiptID, targetSnapshotID, contextVersion, outcome string) error {
	if s == nil || !validExternalOwner(owner) || !validUUIDv7(connectionID) || !validGeneration(generation) || !validUUIDv7(invocationID) || !validUUIDv7(receiptID) || !validUUIDv7(targetSnapshotID) || !validUUIDv7(contextVersion) {
		return ErrExternalConnectionInvalid
	}
	status := commandledger.Status(outcome)
	if outcome == "unknown" {
		status = commandledger.OutcomeUnknown
	}
	if status != commandledger.Succeeded && status != commandledger.Failed && status != commandledger.Cancelled && status != commandledger.OutcomeUnknown {
		return ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	pending, ok := s.pending[invocationID]
	if !ok || pending.terminal || !pending.taken || pending.owner != owner || pending.connectionID != connectionID || pending.generation != generation || pending.receiptID != receiptID || pending.targetSnapshotID != targetSnapshotID || pending.contextVersion != contextVersion {
		return ErrExternalConnectionStale
	}
	connection, connected := s.connections[connectionID]
	if !connected || connection.status.Owner != owner || connection.status.Generation != generation || connection.status.TargetSnapshotID != targetSnapshotID || connection.status.ContextVersion != contextVersion {
		status = commandledger.OutcomeUnknown
	}
	s.finishPendingLocked(invocationID, pending, status)
	return nil
}

func (s *ExternalUIConnections) cancelExternal(invocationID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending, ok := s.pending[invocationID]; ok {
		outcome := commandledger.Cancelled
		if pending.taken {
			outcome = commandledger.OutcomeUnknown
		}
		s.finishPendingLocked(invocationID, pending, outcome)
	}
}

func (s *ExternalUIConnections) schedulePendingExpiryLocked(invocationID string, pending *externalPending) {
	if pending.timer != nil {
		pending.timer.Stop()
	}
	pending.timerVersion++
	version := pending.timerVersion
	pending.timer = time.AfterFunc(externalConnectionTTL, func() { s.cancelExpiredExternal(invocationID, version) })
}

func (s *ExternalUIConnections) cancelExpiredExternal(invocationID string, timerVersion uint64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending, ok := s.pending[invocationID]; ok && pending.timerVersion == timerVersion {
		s.finishPendingLocked(invocationID, pending, pendingTerminalOutcome(pending))
	}
}

func (s *ExternalUIConnections) invalidateConnectionPendingLocked(connectionID, snapshotID, contextVersion string) {
	for id, pending := range s.pending {
		if pending.connectionID != connectionID {
			continue
		}
		if snapshotID != "" && (pending.targetSnapshotID != snapshotID || pending.contextVersion != contextVersion) {
			continue
		}
		outcome := commandledger.Cancelled
		if pending.taken {
			outcome = commandledger.OutcomeUnknown
		}
		s.finishPendingLocked(id, pending, outcome)
	}
}

func (s *ExternalUIConnections) finishPendingLocked(id string, pending *externalPending, outcome commandledger.Status) {
	if pending.terminal {
		return
	}
	pending.terminal = true
	if pending.timer != nil {
		pending.timer.Stop()
	}
	result := commandexecution.Outcome{Status: outcome}
	if outcome == commandledger.Succeeded {
		// O handoff visual confirma somente a ação; não aceita payload de
		// resultado da interface. O executor comum ainda valida seu schema.
		result.Result = json.RawMessage(`{}`)
	}
	pending.done <- result
	close(pending.done)
	delete(s.pending, id)
}

func (s *ExternalUIConnections) Validate(connectionID, generation string, principal ExternalUIPrincipal, targetSnapshotID, contextVersion string) error {
	if s == nil || !validUUIDv7(connectionID) || !validGeneration(generation) || !validExternalPrincipal(principal) || !validUUIDv7(targetSnapshotID) || !validUUIDv7(contextVersion) {
		return ErrExternalConnectionInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now().UTC())
	connection, ok := s.connections[connectionID]
	if !ok || connection.status.Generation != generation || !sameExternalPrincipal(connection.principal, principal) {
		return ErrExternalConnectionDenied
	}
	if connection.status.TargetSnapshotID != targetSnapshotID || connection.status.ContextVersion != contextVersion {
		return ErrExternalConnectionStale
	}
	return nil
}

func (s *ExternalUIConnections) pruneLocked(now time.Time) {
	for hash, invite := range s.invites {
		if !invite.expiresAt.After(now) {
			delete(s.invites, hash)
			key := externalOwnerKey(invite.owner)
			delete(s.ownerInvites, key)
			s.setOwnerStateLocked(key, ExternalUIConnectionStatus{State: "disconnected", Owner: invite.owner}, now)
		}
	}
	for id, connection := range s.connections {
		if !connection.status.ExpiresAt.After(now) {
			key := externalOwnerKey(connection.status.Owner)
			delete(s.ownerConnections, key)
			s.setOwnerStateLocked(key, ExternalUIConnectionStatus{State: "disconnected", Owner: connection.status.Owner}, now)
			s.invalidateConnectionPendingLocked(id, "", "")
			delete(s.connections, id)
		}
	}
	for id, pending := range s.pending {
		if !pending.expiresAt.After(now) {
			s.finishPendingLocked(id, pending, pendingTerminalOutcome(pending))
		}
	}
}

func pendingTerminalOutcome(pending *externalPending) commandledger.Status {
	if pending != nil && pending.taken {
		return commandledger.OutcomeUnknown
	}
	return commandledger.Cancelled
}

func (s *ExternalUIConnections) setOwnerStateLocked(key string, status ExternalUIConnectionStatus, now time.Time) {
	s.ownerStates[key] = status
	s.ownerStateAt[key] = now
	for len(s.ownerStates) > externalOwnerStateMax {
		oldestKey := ""
		var oldest time.Time
		for candidate, candidateStatus := range s.ownerStates {
			if candidateStatus.State != "disconnected" {
				continue
			}
			if oldestKey == "" || s.ownerStateAt[candidate].Before(oldest) {
				oldestKey, oldest = candidate, s.ownerStateAt[candidate]
			}
		}
		if oldestKey == "" {
			return
		} // ativos são limitados por externalConnectionMax.
		delete(s.ownerStates, oldestKey)
		delete(s.ownerStateAt, oldestKey)
	}
}

func validExternalOwner(owner commandbridge.Owner) bool {
	return validExternalText(owner.UserID, 128) && validExternalText(owner.SessionID, 128) && validExternalText(owner.WorkspaceID, 128)
}

// externalOwnerKey usa separadores que não podem aparecer em nenhum campo do
// owner, mantendo a chave sem colisões entre combinações de IDs.
func externalOwnerKey(owner commandbridge.Owner) string {
	return owner.UserID + "\x00" + owner.SessionID + "\x00" + owner.WorkspaceID
}

func validExternalPrincipal(principal ExternalUIPrincipal) bool {
	return validExternalText(principal.Issuer, 2048) && validExternalText(principal.Subject, 512) && validExternalText(principal.UserID, 128) && validExternalText(principal.AuthContextID, 4096)
}

func validExternalDestination(target ExternalUIDestination) bool {
	return validExternalText(target.WorkspaceID, 128) && (target.TabID == "" || validExternalText(target.TabID, 128)) &&
		validExternalText(target.Surface.SurfaceType, 64) && validExternalText(target.Surface.SurfaceID, 128) &&
		validExternalText(target.Surface.SnapshotVersion, 256) && validRawObject(target.Surface.Selection) && validRawObject(target.Surface.Focus) &&
		validRawObject(target.Surface.Content) && validRawObject(target.Surface.Metadata) && target.Surface.StaleAfterMs >= 0
}

func validRawObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func validExternalText(value string, max int) bool {
	return value != "" && len(value) <= max && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}

func validUUIDv7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func validGeneration(value string) bool {
	n, err := strconv.ParseUint(value, 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == value
}

func sameExternalPrincipal(left, right ExternalUIPrincipal) bool {
	return left.Issuer == right.Issuer && left.Subject == right.Subject && left.UserID == right.UserID && left.AuthContextID == right.AuthContextID
}

func cloneExternalPrincipal(p ExternalUIPrincipal) ExternalUIPrincipal { return p }

func cloneExternalDestination(target ExternalUIDestination) ExternalUIDestination {
	cloneRaw := func(raw json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), raw...) }
	target.Surface.Selection = cloneRaw(target.Surface.Selection)
	target.Surface.Focus = cloneRaw(target.Surface.Focus)
	target.Surface.Content = cloneRaw(target.Surface.Content)
	target.Surface.Metadata = cloneRaw(target.Surface.Metadata)
	return target
}

func cloneExternalStatus(status ExternalUIConnectionStatus) ExternalUIConnectionStatus {
	status.Target = cloneExternalDestination(status.Target)
	return status
}
