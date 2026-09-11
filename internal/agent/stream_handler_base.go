package agent

import (
	"strings"
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
)

// BaseStreamHandler contém os campos e métodos compartilhados entre
// os stream handlers do pacote agent (SimpleStreamHandler e AgenticStreamHandler).
// Agrupa deltas de chat:stream em janelas curtas e mantém chat:thinking
// throttled separadamente.
// Emitter e ConversationID são exportados para permitir construção fora do pacote.
const StreamCoalesceInterval = 24 * time.Millisecond

type BaseStreamHandler struct {
	Emitter            events.Emitter
	ConversationID     string
	TurnID             string
	AssistantMessageID string
	SurfaceOrigin      *ports.ChatSurfaceOrigin

	accumulatedContent   strings.Builder
	promotedContent      strings.Builder
	pendingDelta         strings.Builder
	initialContent       string
	streamSequence       uint64
	streamStarted        bool
	accumulatedReasoning string
	isThinking           bool
	errorNotRetryable    bool

	mu            sync.Mutex
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
// Útil para continuação explícita: o primeiro lote envia o prefill uma única
// vez como BaseContent e os chunks novos continuam viajando como delta.
func (h *BaseStreamHandler) SetInitialContent(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.initialContent = content
	h.accumulatedContent.Reset()
	h.accumulatedContent.WriteString(content)
}

// MarkErrorNotRetryable registra que o erro deste turno não pode ser repetido
// pela auto-recuperação. Quem marca é o provider, antes de OnError, porque só
// ele sabe se o pedido chegou a sair — repetir um turno que um agente de código
// já aceitou é refazer edição de arquivo e comando (AEP-0084 D4).
func (h *BaseStreamHandler) MarkErrorNotRetryable() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errorNotRetryable = true
}

// ErrorNotRetryable é consultado pelos laços de recuperação antes de tentar de novo.
func (h *BaseStreamHandler) ErrorNotRetryable() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.errorNotRetryable
}

// OnTurnNotice emite avisos de tentativa para qualquer handler de chat,
// inclusive o agêntico. O aviso é evento próprio e nunca vira conteúdo salvo
// como se tivesse sido escrito pelo modelo.
func (h *BaseStreamHandler) OnTurnNotice(notice llm.TurnNotice) {
	if h.Emitter == nil || strings.TrimSpace(string(notice.Kind)) == "" {
		return
	}
	h.Emitter.Emit("chat:notice", ports.ChatNoticeEvent{
		ConversationID: h.ConversationID,
		Kind:           string(notice.Kind),
		Count:          notice.Count,
		Model:          notice.Model,
	})
}

func (h *BaseStreamHandler) OnChunk(content string) {
	if content == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	h.accumulatedContent.WriteString(content)
	h.pendingDelta.WriteString(content)

	if !h.pendingEmit {
		h.pendingEmit = true
		h.throttleTimer = time.AfterFunc(StreamCoalesceInterval, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.pendingEmit {
				h.flushPendingDeltaLocked()
			}
		})
	}
}

func (h *BaseStreamHandler) flushPendingDeltaLocked() {
	// O emitter pode reter o payload depois deste método. Clone evita que o
	// evento dependa do armazenamento interno do Builder que será resetado.
	delta := strings.Clone(h.pendingDelta.String())
	if delta == "" {
		h.cancelPendingChunkTimer()
		return
	}
	h.pendingDelta.Reset()
	h.pendingEmit = false
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	reset := !h.streamStarted
	baseContent := ""
	if reset {
		baseContent = h.initialContent
	}
	h.Emitter.Emit("chat:stream", events.StreamEvent{
		MessageID:      h.AssistantMessageID,
		Delta:          delta,
		Reset:          reset,
		BaseContent:    baseContent,
		Sequence:       h.streamSequence,
		Done:           false,
		ConversationId: h.ConversationID,
		TurnID:         h.TurnID,
		SurfaceOrigin:  h.SurfaceOrigin,
	})
	h.streamStarted = true
	h.streamSequence++
}

// FlushStream emite imediatamente o delta pendente. Deve ser chamado antes de
// eventos terminais, de tool ou de segmento para preservar a ordem no IPC.
func (h *BaseStreamHandler) FlushStream() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.flushPendingDeltaLocked()
}

func (h *BaseStreamHandler) OnThinking(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isThinking {
		h.isThinking = true
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
		ConversationID:     h.ConversationID,
		TurnID:             h.TurnID,
		AssistantMessageID: h.AssistantMessageID,
		Content:            h.accumulatedReasoning,
		Done:               false,
		SurfaceOrigin:      h.SurfaceOrigin,
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
		ConversationID:     h.ConversationID,
		TurnID:             h.TurnID,
		AssistantMessageID: h.AssistantMessageID,
		Content:            fullReasoning,
		Done:               true,
		SurfaceOrigin:      h.SurfaceOrigin,
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
		ConversationID:     h.ConversationID,
		TurnID:             h.TurnID,
		AssistantMessageID: h.AssistantMessageID,
		Content:            reasoning,
		Done:               true,
		SurfaceOrigin:      h.SurfaceOrigin,
	})
}

// ResetStreamAttempt encerra o thinking ainda ativo e descarta apenas o
// raciocínio da tentativa que será repetida. O conteúdo visível não é apagado:
// providers só podem chamar esta capability quando repetir não duplicará texto
// nem efeitos já entregues.
func (h *BaseStreamHandler) ResetStreamAttempt() {
	h.mu.Lock()
	active := h.isThinking || h.pendingThinkingEmit || h.thinkingTimer != nil
	if h.thinkingTimer != nil {
		h.thinkingTimer.Stop()
		h.thinkingTimer = nil
	}
	h.pendingThinkingEmit = false
	h.isThinking = false
	h.accumulatedReasoning = ""
	h.lastThinkingEmitTime = time.Time{}
	h.mu.Unlock()

	if active {
		h.Emitter.Emit("chat:thinking", ports.ThinkingEvent{
			ConversationID:     h.ConversationID,
			TurnID:             h.TurnID,
			AssistantMessageID: h.AssistantMessageID,
			// Vazio fecha a live region sem promover como definitivo o
			// raciocínio da tentativa descartada.
			Content:       "",
			Done:          true,
			SurfaceOrigin: h.SurfaceOrigin,
		})
	}
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
	h.flushPendingDeltaLocked()
	return h.promotedContent.String() + h.accumulatedContent.String(), h.accumulatedReasoning
}

// UnreadTail devolve o texto que ainda não virou segmento e diz se o turno já
// foi lido em blocos. É o que a leitura final precisa saber: com a resposta já
// falada pedaço a pedaço, ler o turno inteiro no fim faria a pessoa ouvir tudo
// de novo (AEP-0084 D13).
func (h *BaseStreamHandler) UnreadTail() (tail string, readInSegments bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.accumulatedContent.String(), h.promotedContent.Len() != 0
}

// CutSegment fecha o bloco de texto corrente: devolve o que foi acumulado desde
// o corte anterior e zera o acumulador visível, para que o próximo chat:stream
// não repita o texto que a UI já promoveu a segmento. O conteúdo cortado
// continua contando para Finalize, que devolve o turno inteiro.
func (h *BaseStreamHandler) CutSegment() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.flushPendingDeltaLocked()
	// O segmento sobrevive ao reset/reuso do acumulador.
	segment := strings.Clone(h.accumulatedContent.String())
	h.promotedContent.WriteString(segment)
	h.accumulatedContent.Reset()
	h.initialContent = ""
	h.streamStarted = false
	h.streamSequence = 0
	return segment
}
