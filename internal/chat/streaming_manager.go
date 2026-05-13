package chat

import (
	"context"
	"log"
	"sync"

	"assistente/internal/messaging"
)

// StreamingManager tracks cancellable streaming contexts per conversation.
// It enables barge-in (SIP): a new user utterance cancels the LLM response
// that is currently streaming.
type StreamingManager struct {
	mu       sync.Mutex
	contexts map[string]context.CancelFunc

	// Optional: notifier to cancel pending gateway callbacks on barge-in.
	responseNotifier *messaging.ResponseNotifier
}

// NewStreamingManager creates a StreamingManager.
// responseNotifier may be nil when messaging is not configured.
func NewStreamingManager(notifier *messaging.ResponseNotifier) *StreamingManager {
	return &StreamingManager{
		contexts:         make(map[string]context.CancelFunc),
		responseNotifier: notifier,
	}
}

// Register stores a cancellable context for the given conversation.
// If a previous context exists it is cancelled first (new message overrides in-flight response).
func (m *StreamingManager) Register(conversationID string, cancel context.CancelFunc) {
	m.mu.Lock()
	if prev, ok := m.contexts[conversationID]; ok {
		prev()
	}
	m.contexts[conversationID] = cancel
	m.mu.Unlock()
}

// Unregister removes the streaming context once a response completes normally.
func (m *StreamingManager) Unregister(conversationID string) {
	m.mu.Lock()
	delete(m.contexts, conversationID)
	m.mu.Unlock()
}

// Cancel cancels the in-flight LLM response for the given conversation (barge-in).
// It is a no-op when there is no streaming in progress for that conversation.
func (m *StreamingManager) Cancel(conversationID string) {
	m.mu.Lock()
	cancel, ok := m.contexts[conversationID]
	if ok {
		cancel()
		delete(m.contexts, conversationID)
	}
	m.mu.Unlock()

	if ok {
		if m.responseNotifier != nil {
			m.responseNotifier.Cancel(conversationID)
		}
		log.Printf("[LLM] Streaming cancelado para conversa %s (barge-in)", conversationID)
	}
}

// Mu acquires the internal mutex, calls fn with the raw contexts map, then releases it.
// Intended for tests that need white-box inspection; avoid in production code.
func (m *StreamingManager) Mu(fn func(map[string]context.CancelFunc)) {
	m.mu.Lock()
	fn(m.contexts)
	m.mu.Unlock()
}
