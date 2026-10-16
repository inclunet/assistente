package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"assistente/internal/logging"
	"assistente/internal/messaging"
)

var ErrConversationActive = errors.New("conversa possui resposta ativa")

const deletionTombstoneTTL = 5 * time.Minute

// StreamingManager tracks cancellable streaming contexts per conversation.
// It enables barge-in (SIP): a new user utterance cancels the LLM response
// that is currently streaming.
type StreamingManager struct {
	mu           sync.Mutex
	contexts     map[string]context.CancelFunc
	generations  map[string]uint64
	completions  map[string]chan struct{}
	executions   map[string]map[string]context.CancelFunc
	reservations map[string]int
	deleted      map[string]time.Time
	nextGen      uint64
	now          func() time.Time

	// Optional: notifier to cancel pending gateway callbacks on barge-in.
	responseNotifier *messaging.ResponseNotifier
}

// NewStreamingManager creates a StreamingManager.
// responseNotifier may be nil when messaging is not configured.
func NewStreamingManager(notifier *messaging.ResponseNotifier) *StreamingManager {
	return &StreamingManager{
		contexts:         make(map[string]context.CancelFunc),
		generations:      make(map[string]uint64),
		completions:      make(map[string]chan struct{}),
		executions:       make(map[string]map[string]context.CancelFunc),
		reservations:     make(map[string]int),
		deleted:          make(map[string]time.Time),
		now:              time.Now,
		responseNotifier: notifier,
	}
}

// Begin cobre preparação, streaming e persistência com o mesmo cancelamento.
// Um novo envio aguarda a saída do worker anterior, inclusive após Cancel.
func (m *StreamingManager) Begin(ctx context.Context, conversationID string, executionIDs ...string) (context.Context, uint64, func(), error) {
	conversationID = strings.TrimSpace(conversationID)
	ctx, cancel := context.WithCancel(ctx)
	executionID := ""
	if len(executionIDs) > 0 {
		executionID = strings.TrimSpace(executionIDs[0])
	}
	if executionID != "" {
		m.mu.Lock()
		if m.executions[conversationID] == nil {
			m.executions[conversationID] = make(map[string]context.CancelFunc)
		}
		if m.executions[conversationID][executionID] != nil {
			m.mu.Unlock()
			cancel()
			return nil, 0, nil, ErrConversationActive
		}
		m.executions[conversationID][executionID] = cancel
		m.mu.Unlock()
	}
	removeExecution := func() {
		cancel()
		if executionID != "" {
			m.mu.Lock()
			delete(m.executions[conversationID], executionID)
			if len(m.executions[conversationID]) == 0 {
				delete(m.executions, conversationID)
			}
			m.mu.Unlock()
		}
	}
	acquired := false
	defer func() {
		if !acquired {
			removeExecution()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, err
		}
		m.mu.Lock()
		if m.isDeletedLocked(conversationID) {
			m.mu.Unlock()
			return nil, 0, nil, ErrConversationDeleted
		}
		if previous := m.completions[conversationID]; previous != nil {
			m.mu.Unlock()
			select {
			case <-previous:
				continue
			case <-ctx.Done():
				return nil, 0, nil, ctx.Err()
			}
		}
		if m.contexts[conversationID] != nil {
			m.mu.Unlock()
			return nil, 0, nil, ErrConversationActive
		}
		if err := ctx.Err(); err != nil {
			m.mu.Unlock()
			return nil, 0, nil, err
		}
		m.nextGen++
		generation := m.nextGen
		done := make(chan struct{})
		m.contexts[conversationID] = cancel
		m.generations[conversationID] = generation
		m.completions[conversationID] = done
		m.mu.Unlock()
		var once sync.Once
		finish := func() {
			once.Do(func() {
				removeExecution()
				m.mu.Lock()
				if m.generations[conversationID] == generation {
					delete(m.contexts, conversationID)
					delete(m.generations, conversationID)
				}
				if m.completions[conversationID] == done {
					delete(m.completions, conversationID)
				}
				close(done)
				m.mu.Unlock()
			})
		}
		acquired = true
		return ctx, generation, finish, nil
	}
}

// CancelExecution cancela somente o envio escolhido, inclusive enquanto aguarda
// a finalização do worker anterior. Não descarta outros itens da fila.
func (m *StreamingManager) CancelExecution(conversationID, executionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel := m.executions[strings.TrimSpace(conversationID)][strings.TrimSpace(executionID)]; cancel != nil {
		cancel()
	}
}

// ReserveConversation protege a janela entre o início do pipeline de envio e
// o registro do contexto de stream.
func (m *StreamingManager) ReserveConversation(conversationID string) (func(), bool) {
	conversationID = strings.TrimSpace(conversationID)
	m.mu.Lock()
	if m.isDeletedLocked(conversationID) {
		m.mu.Unlock()
		return func() {}, false
	}
	m.reservations[conversationID]++
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			if m.reservations[conversationID] <= 1 {
				delete(m.reservations, conversationID)
			} else {
				m.reservations[conversationID]--
			}
			m.mu.Unlock()
		})
	}, true
}

// PrepareConversationDeletion falha sem cancelar trabalho em andamento. Em
// sucesso, mantém o gate fechado até release para impedir novos pipelines
// durante a transação de exclusão.
func (m *StreamingManager) PrepareConversationDeletion(conversationIDs []string) (func(committed bool), error) {
	normalized := make([]string, 0, len(conversationIDs))
	seen := make(map[string]struct{}, len(conversationIDs))
	for _, rawID := range conversationIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	m.mu.Lock()
	for _, conversationID := range normalized {
		if m.isDeletedLocked(conversationID) {
			m.mu.Unlock()
			return func(bool) {}, ErrConversationActive
		}
		if m.reservations[conversationID] > 0 || m.contexts[conversationID] != nil {
			m.mu.Unlock()
			return func(bool) {}, ErrConversationActive
		}
	}

	var once sync.Once
	return func(committed bool) {
		once.Do(func() {
			if committed {
				expiresAt := m.now().Add(deletionTombstoneTTL)
				for _, conversationID := range normalized {
					m.deleted[conversationID] = expiresAt
				}
			}
			m.mu.Unlock()
		})
	}, nil
}

// PrepareConversationRestoration fecha o gate até o caller informar os IDs
// cujo commit de importação foi concluído.
func (m *StreamingManager) PrepareConversationRestoration() func([]string) {
	m.mu.Lock()
	var once sync.Once
	return func(conversationIDs []string) {
		once.Do(func() {
			for _, rawID := range conversationIDs {
				delete(m.deleted, strings.TrimSpace(rawID))
			}
			m.mu.Unlock()
		})
	}
}

// Register stores a cancellable context for the given conversation.
// If a previous context exists it is cancelled first (new message overrides in-flight response).
// The returned generation can be passed to UnregisterIfCurrent so completion of
// an older turn cannot remove the cancellation handle of a newer turn.
func (m *StreamingManager) Register(conversationID string, cancel context.CancelFunc) uint64 {
	conversationID = strings.TrimSpace(conversationID)
	m.mu.Lock()
	if m.isDeletedLocked(conversationID) {
		m.mu.Unlock()
		cancel()
		return 0
	}
	if prev, ok := m.contexts[conversationID]; ok {
		prev()
	}
	m.nextGen++
	generation := m.nextGen
	m.contexts[conversationID] = cancel
	m.generations[conversationID] = generation
	m.mu.Unlock()
	return generation
}

// Unregister removes the active context unconditionally. Use it only when no
// newer turn can have registered the same conversation; concurrent goroutine
// cleanup must prefer UnregisterIfCurrent with the generation from Register.
func (m *StreamingManager) Unregister(conversationID string) {
	conversationID = strings.TrimSpace(conversationID)
	m.mu.Lock()
	delete(m.contexts, conversationID)
	delete(m.generations, conversationID)
	m.mu.Unlock()
}

// UnregisterIfCurrent removes a context only when generation still identifies
// the active turn. It prevents late cleanup from an old goroutine from
// unregistering a newer turn in the same conversation.
func (m *StreamingManager) UnregisterIfCurrent(conversationID string, generation uint64) bool {
	conversationID = strings.TrimSpace(conversationID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.generations[conversationID]; !ok || current != generation {
		return false
	}
	delete(m.contexts, conversationID)
	delete(m.generations, conversationID)
	return true
}

// Cancel cancels the in-flight LLM response for the given conversation (barge-in).
// It is a no-op when there is no streaming in progress for that conversation.
func (m *StreamingManager) Cancel(conversationID string) {
	m.cancelGeneration(conversationID, 0, false)
}

// CurrentGeneration captures the active response identity, not just its conversation.
func (m *StreamingManager) CurrentGeneration(conversationID string) (uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	generation, ok := m.generations[strings.TrimSpace(conversationID)]
	return generation, ok && m.contexts[strings.TrimSpace(conversationID)] != nil
}

// CancelIfCurrent cannot cancel a newer response registered after admission.
func (m *StreamingManager) CancelIfCurrent(conversationID string, generation uint64) bool {
	return m.cancelGeneration(conversationID, generation, true)
}

func (m *StreamingManager) cancelGeneration(conversationID string, generation uint64, exact bool) bool {
	conversationID = strings.TrimSpace(conversationID)
	m.mu.Lock()
	cancel, ok := m.contexts[conversationID]
	if exact && m.generations[conversationID] != generation {
		m.mu.Unlock()
		return false
	}
	if exact && generation == 0 && !ok {
		m.mu.Unlock()
		return true // Captured absence: frontend may cancel its serialization/queue.
	}
	if ok {
		cancel()
		// Execuções gerenciadas só liberam a conversa após o worker sair.
		if m.completions[conversationID] == nil {
			delete(m.contexts, conversationID)
			delete(m.generations, conversationID)
		}
	}
	m.mu.Unlock()

	if ok {
		if m.responseNotifier != nil {
			m.responseNotifier.Cancel(conversationID)
		}
		logging.Infof(context.Background(), "chat.streaming-manager", "[LLM] Streaming cancelado para conversa %s (barge-in)", conversationID)
	}
	return ok
}

func (m *StreamingManager) isDeletedLocked(conversationID string) bool {
	now := m.now()
	for id, expiresAt := range m.deleted {
		if !expiresAt.After(now) {
			delete(m.deleted, id)
		}
	}
	expiresAt, deleted := m.deleted[conversationID]
	return deleted && expiresAt.After(now)
}

// Mu acquires the internal mutex, calls fn with the raw contexts map, then releases it.
// Intended for tests that need white-box inspection; avoid in production code.
func (m *StreamingManager) Mu(fn func(map[string]context.CancelFunc)) {
	m.mu.Lock()
	fn(m.contexts)
	m.mu.Unlock()
}
