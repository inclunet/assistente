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
	Emitter            events.Emitter
	ConversationID     string
	TurnID             string
	AssistantMessageID string
	SurfaceOrigin      *ports.ChatSurfaceOrigin

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

// SetAssistantMessageID injeta o ID estável da mensagem assistant (placeholder) para este turno.
// Usado pelo RunAgenticLoop para garantir messageId consistente em chat:stream.
func (h *BaseStreamHandler) SetAssistantMessageID(messageID string) {
	h.AssistantMessageID = messageID
}

// SetInitialContent define o conteúdo inicial do stream (prefill).
// Útil para continuação explícita: o handler passa a emitir conteúdo cumulativo
// (prefill + novos chunks) sem sobrescrever o parcial já existente.
func (h *BaseStreamHandler) SetInitialContent(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.accumulatedContent = content
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
		MessageID:      h.AssistantMessageID,
		Content:        h.accumulatedContent,
		Done:           false,
		ConversationId: h.ConversationID,
		TurnID:         h.TurnID,
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
			TurnID:         h.TurnID,
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
		TurnID:         h.TurnID,
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
		TurnID:         h.TurnID,
		Content:        fullReasoning,
		Done:           true,
		SurfaceOrigin:  h.SurfaceOrigin,
	})
}

// FinishThinkingIfActive encerra/cancela thinking pendente antes de uma tentativa
// abortada, evitando timers e estado "pensando" vazarem entre retries.
func (h *BaseStreamHandler) FinishThinkingIfActive() {
	h.mu.Lock()
	if !h.isThinking && !h.pendingThinkingEmit && h.thinkingTimer == nil {
		h.mu.Unlock()
		return
	}
	if h.thinkingTimer != nil {
		h.thinkingTimer.Stop()
		h.thinkingTimer = nil
	}
	h.pendingThinkingEmit = false
	h.isThinking = false
	reasoning := h.accumulatedReasoning
	h.mu.Unlock()

	h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
		ConversationID: h.ConversationID,
		TurnID:         h.TurnID,
		Content:        reasoning,
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
