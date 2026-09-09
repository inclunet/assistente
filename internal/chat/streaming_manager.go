package chat

import (
	"assistente/internal/logging"
	"context"
	"sync"

	"assistente/internal/messaging"
)

// StreamingManager tracks cancellable streaming contexts per conversation.
// It enables barge-in (SIP): a new user utterance cancels the LLM response
// that is currently streaming.
type StreamingManager struct {
	mu          sync.Mutex
	contexts    map[string]context.CancelFunc
	generations map[string]uint64
	nextGen     uint64

	// Optional: notifier to cancel pending gateway callbacks on barge-in.
	responseNotifier *messaging.ResponseNotifier
}

// NewStreamingManager creates a StreamingManager.
// responseNotifier may be nil when messaging is not configured.
func NewStreamingManager(notifier *messaging.ResponseNotifier) *StreamingManager {
	return &StreamingManager{
		contexts:         make(map[string]context.CancelFunc),
		generations:      make(map[string]uint64),
		responseNotifier: notifier,
	}
}

// Register stores a cancellable context for the given conversation.
// If a previous context exists it is cancelled first (new message overrides in-flight response).
// The returned generation can be passed to UnregisterIfCurrent so completion of
// an older turn cannot remove the cancellation handle of a newer turn.
func (m *StreamingManager) Register(conversationID string, cancel context.CancelFunc) uint64 {
	m.mu.Lock()
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

// Unregister removes the streaming context once a response completes normally.
func (m *StreamingManager) Unregister(conversationID string) {
	m.mu.Lock()
	delete(m.contexts, conversationID)
	delete(m.generations, conversationID)
	m.mu.Unlock()
}

// UnregisterIfCurrent removes a context only when generation still identifies
// the active turn. It prevents late cleanup from an old goroutine from
// unregistering a newer turn in the same conversation.
func (m *StreamingManager) UnregisterIfCurrent(conversationID string, generation uint64) bool {
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

// Mu acquires the internal mutex, calls fn with the raw contexts map, then releases it.
// Intended for tests that need white-box inspection; avoid in production code.
func (m *StreamingManager) Mu(fn func(map[string]context.CancelFunc)) {
	m.mu.Lock()
	fn(m.contexts)
	m.mu.Unlock()
}
