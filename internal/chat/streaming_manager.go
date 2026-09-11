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
		reservations:     make(map[string]int),
		deleted:          make(map[string]time.Time),
		now:              time.Now,
		responseNotifier: notifier,
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
	conversationID = strings.TrimSpace(conversationID)
	m.mu.Lock()
	cancel, ok := m.contexts[conversationID]
	if ok {
		cancel()
		delete(m.contexts, conversationID)
		delete(m.generations, conversationID)
	}
	m.mu.Unlock()

	if ok {
		if m.responseNotifier != nil {
			m.responseNotifier.Cancel(conversationID)
		}
		logging.Infof(context.Background(), "chat.streaming-manager", "[LLM] Streaming cancelado para conversa %s (barge-in)", conversationID)
	}
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
