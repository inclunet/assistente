package events

import (
	"assistente/internal/core/ports"
	"strings"
	"sync"
	"time"
)

// BaseStreamHandler contém os campos e métodos compartilhados entre
// stream handlers (agentic e chat direto).
// Lida com coalescing de 24 ms para os eventos chat:stream e throttling de
// chat:thinking.
//
// Campos são exportados para permitir embeddings em outros pacotes internos.
type BaseStreamHandler struct {
	Emitter            Emitter
	ConversationID     string
	TurnID             string
	AssistantMessageID string
	SurfaceOrigin      *ports.ChatSurfaceOrigin

	PendingDelta         strings.Builder
	AccumulatedReasoning strings.Builder
	IsThinking           bool

	Mu            sync.Mutex
	LastEmitTime  time.Time
	ThrottleTimer *time.Timer
	PendingEmit   bool
	StreamStarted bool
	Sequence      uint64

	LastThinkingEmitTime time.Time
	ThinkingTimer        *time.Timer
	PendingThinkingEmit  bool
}

func (h *BaseStreamHandler) OnChunk(content string) {
	h.Mu.Lock()
	defer h.Mu.Unlock()

	_, _ = h.PendingDelta.WriteString(content)

	const throttleInterval = 24 * time.Millisecond
	now := time.Now()

	if now.Sub(h.LastEmitTime) >= throttleInterval {
		h.emitStreamEvent()
		h.LastEmitTime = now
		h.PendingEmit = false
		if h.ThrottleTimer != nil {
			h.ThrottleTimer.Stop()
			h.ThrottleTimer = nil
		}
		return
	}

	if !h.PendingEmit {
		h.PendingEmit = true
		remainingTime := throttleInterval - now.Sub(h.LastEmitTime)
		h.ThrottleTimer = time.AfterFunc(remainingTime, func() {
			h.Mu.Lock()
			defer h.Mu.Unlock()
			if h.PendingEmit {
				h.emitStreamEvent()
				h.LastEmitTime = time.Now()
				h.PendingEmit = false
			}
		})
	}
}

func (h *BaseStreamHandler) emitStreamEvent() {
	delta := strings.Clone(h.PendingDelta.String())
	h.PendingDelta.Reset()
	if delta == "" {
		return
	}
	h.Emitter.Emit("chat:stream", StreamEvent{
		MessageID:      h.AssistantMessageID,
		Delta:          delta,
		Reset:          !h.StreamStarted,
		Sequence:       h.Sequence,
		Done:           false,
		ConversationId: h.ConversationID,
		TurnID:         h.TurnID,
		SurfaceOrigin:  h.SurfaceOrigin,
	})
	h.StreamStarted = true
	h.Sequence++
}

func (h *BaseStreamHandler) OnThinking(content string) {
	h.Mu.Lock()
	defer h.Mu.Unlock()

	if !h.IsThinking {
		h.IsThinking = true
		h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
			ConversationID:     h.ConversationID,
			TurnID:             h.TurnID,
			AssistantMessageID: h.AssistantMessageID,
			Content:            content,
			Done:               false,
			Started:            true,
			SurfaceOrigin:      h.SurfaceOrigin,
		})
	}

	_, _ = h.AccumulatedReasoning.WriteString(content)

	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	if now.Sub(h.LastThinkingEmitTime) >= throttleInterval {
		h.emitThinkingEvent()
		h.LastThinkingEmitTime = now
		h.PendingThinkingEmit = false
		if h.ThinkingTimer != nil {
			h.ThinkingTimer.Stop()
			h.ThinkingTimer = nil
		}
		return
	}

	if !h.PendingThinkingEmit {
		h.PendingThinkingEmit = true
		remainingTime := throttleInterval - now.Sub(h.LastThinkingEmitTime)
		h.ThinkingTimer = time.AfterFunc(remainingTime, func() {
			h.Mu.Lock()
			defer h.Mu.Unlock()
			if h.PendingThinkingEmit {
				h.emitThinkingEvent()
				h.LastThinkingEmitTime = time.Now()
				h.PendingThinkingEmit = false
			}
		})
	}
}

func (h *BaseStreamHandler) emitThinkingEvent() {
	h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
		ConversationID:     h.ConversationID,
		TurnID:             h.TurnID,
		AssistantMessageID: h.AssistantMessageID,
		Content:            strings.Clone(h.AccumulatedReasoning.String()),
		Done:               false,
		SurfaceOrigin:      h.SurfaceOrigin,
	})
}

func (h *BaseStreamHandler) OnThinkingDone(fullReasoning string) {
	h.Mu.Lock()
	if h.ThinkingTimer != nil {
		h.ThinkingTimer.Stop()
		h.ThinkingTimer = nil
	}
	h.PendingThinkingEmit = false

	if fullReasoning != "" {
		h.AccumulatedReasoning.Reset()
		_, _ = h.AccumulatedReasoning.WriteString(fullReasoning)
	}
	reasoning := strings.Clone(h.AccumulatedReasoning.String())
	h.Mu.Unlock()

	h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
		ConversationID:     h.ConversationID,
		TurnID:             h.TurnID,
		AssistantMessageID: h.AssistantMessageID,
		Content:            reasoning,
		Done:               true,
		SurfaceOrigin:      h.SurfaceOrigin,
	})
}

// CancelPendingChunkTimer cancela timers de throttle pendentes.
// Deve ser chamado com Mu held pelo chamador.
func (h *BaseStreamHandler) CancelPendingChunkTimer() {
	if h.ThrottleTimer != nil {
		h.ThrottleTimer.Stop()
		h.ThrottleTimer = nil
	}
	h.PendingEmit = false
}
