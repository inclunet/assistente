package main

import (
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// baseStreamHandler contém os campos e métodos compartilhados entre
// agenticStreamHandler (agent.go) e appStreamHandler (llm.go).
// Lida com throttling de 50ms para os eventos chat:stream e chat:thinking.
type baseStreamHandler struct {
	app            *App
	conversationID uint

	accumulatedContent   string
	accumulatedReasoning string
	isThinking           bool

	mu            sync.Mutex
	lastEmitTime  time.Time
	throttleTimer *time.Timer
	pendingEmit   bool

	lastThinkingEmitTime time.Time
	thinkingTimer        *time.Timer
	pendingThinkingEmit  bool
}

func (h *baseStreamHandler) OnChunk(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.accumulatedContent += content

	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	if now.Sub(h.lastEmitTime) >= throttleInterval {
		h.emitStreamEvent()
		h.lastEmitTime = now
		h.pendingEmit = false
		if h.throttleTimer != nil {
			h.throttleTimer.Stop()
			h.throttleTimer = nil
		}
		return
	}

	if !h.pendingEmit {
		h.pendingEmit = true
		remainingTime := throttleInterval - now.Sub(h.lastEmitTime)
		h.throttleTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.pendingEmit {
				h.emitStreamEvent()
				h.lastEmitTime = time.Now()
				h.pendingEmit = false
			}
		})
	}
}

func (h *baseStreamHandler) emitStreamEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        h.accumulatedContent,
		Done:           false,
		ConversationId: h.conversationID,
	})
}

func (h *baseStreamHandler) OnThinking(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isThinking {
		h.isThinking = true
		runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
			"content":        content,
			"done":           false,
			"conversationId": h.conversationID,
			"started":        true,
		})
	}

	h.accumulatedReasoning += content

	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	if now.Sub(h.lastThinkingEmitTime) >= throttleInterval {
		h.emitThinkingEvent()
		h.lastThinkingEmitTime = now
		h.pendingThinkingEmit = false
		if h.thinkingTimer != nil {
			h.thinkingTimer.Stop()
			h.thinkingTimer = nil
		}
		return
	}

	if !h.pendingThinkingEmit {
		h.pendingThinkingEmit = true
		remainingTime := throttleInterval - now.Sub(h.lastThinkingEmitTime)
		h.thinkingTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.pendingThinkingEmit {
				h.emitThinkingEvent()
				h.lastThinkingEmitTime = time.Now()
				h.pendingThinkingEmit = false
			}
		})
	}
}

func (h *baseStreamHandler) emitThinkingEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        h.accumulatedReasoning,
		"done":           false,
		"conversationId": h.conversationID,
	})
}

func (h *baseStreamHandler) OnThinkingDone(fullReasoning string) {
	h.mu.Lock()
	if h.thinkingTimer != nil {
		h.thinkingTimer.Stop()
		h.thinkingTimer = nil
	}
	h.pendingThinkingEmit = false
	h.isThinking = false
	h.mu.Unlock()

	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        fullReasoning,
		"done":           true,
		"conversationId": h.conversationID,
	})
}

// cancelPendingChunkTimer cancela o timer de throttle de chunk, se houver.
// Deve ser chamado com h.mu locked.
func (h *baseStreamHandler) cancelPendingChunkTimer() {
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
}
