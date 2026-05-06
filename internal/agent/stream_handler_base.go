package agent

import (
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/events"
)

// BaseStreamHandler contém os campos e métodos compartilhados entre
// os stream handlers do pacote agent (SimpleStreamHandler e AgenticStreamHandler).
// Lida com throttling de 50 ms para os eventos chat:stream e chat:thinking.
// Emitter e ConversationID são exportados para permitir construção fora do pacote.
type BaseStreamHandler struct {
	Emitter        events.Emitter
	ConversationID string
	SurfaceOrigin  *ports.ChatSurfaceOrigin

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

func (h *BaseStreamHandler) OnChunk(content string) {
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

func (h *BaseStreamHandler) emitStreamEvent() {
	h.Emitter.Emit("chat:stream", events.StreamEvent{
		Content:        h.accumulatedContent,
		Done:           false,
		ConversationId: h.ConversationID,
		SurfaceOrigin:  h.SurfaceOrigin,
	})
}

func (h *BaseStreamHandler) OnThinking(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isThinking {
		h.isThinking = true
		h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
			ConversationID: h.ConversationID,
			Content:        content,
			Done:           false,
			Started:        true,
			SurfaceOrigin:  h.SurfaceOrigin,
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

func (h *BaseStreamHandler) emitThinkingEvent() {
	h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
		ConversationID: h.ConversationID,
		Content:        h.accumulatedReasoning,
		Done:           false,
		SurfaceOrigin:  h.SurfaceOrigin,
	})
}

func (h *BaseStreamHandler) OnThinkingDone(fullReasoning string) {
	h.mu.Lock()
	if h.thinkingTimer != nil {
		h.thinkingTimer.Stop()
		h.thinkingTimer = nil
	}
	h.pendingThinkingEmit = false
	h.isThinking = false
	h.mu.Unlock()

	h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
		ConversationID: h.ConversationID,
		Content:        fullReasoning,
		Done:           true,
		SurfaceOrigin:  h.SurfaceOrigin,
	})
}

// cancelPendingChunkTimer cancela o timer de throttle de chunk, se houver.
// Deve ser chamado com h.mu locked.
func (h *BaseStreamHandler) cancelPendingChunkTimer() {
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
}

// Finalize cancela timers pendentes e retorna o conteúdo acumulado (texto + reasoning).
// Thread-safe. Chamado por OnDone, OnError e OnToolCalls dos handlers concretos.
func (h *BaseStreamHandler) Finalize() (content, reasoning string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancelPendingChunkTimer()
	return h.accumulatedContent, h.accumulatedReasoning
}
