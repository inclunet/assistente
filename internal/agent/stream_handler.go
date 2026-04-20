package agent

import (
	"log"

	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
)

// agenticStreamHandler implementa IterationHandler para uso no agentic loop.
// Emite eventos de streaming para o frontend em tempo real (chat:stream, chat:thinking),
// mas NÃO salva no banco e NÃO emite chat:done — o loop controla isso.
type agenticStreamHandler struct {
	events.BaseStreamHandler
	iteration int

	// Resultado da iteração (preenchido por OnDone/OnToolCalls/OnError)
	result AgenticResult

	// MCP tool events acumulados durante o streaming (para persistência)
	nativeMCPEvents []llm.MCPToolEvent
}

// NewIterationHandler cria um IterationHandler para uma iteração do agentic loop.
func NewIterationHandler(emitter events.Emitter, conversationID uint, iteration int) IterationHandler {
	return &agenticStreamHandler{
		BaseStreamHandler: events.BaseStreamHandler{
			Emitter:        emitter,
			ConversationID: conversationID,
		},
		iteration: iteration,
	}
}

// Result implementa IterationHandler.
func (h *agenticStreamHandler) Result() AgenticResult {
	return h.result
}

func (h *agenticStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	h.Mu.Lock()
	h.CancelPendingChunkTimer()
	content := h.AccumulatedContent
	reasoning := h.AccumulatedReasoning
	mcpEvents := h.nativeMCPEvents
	h.nativeMCPEvents = nil
	h.Mu.Unlock()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = content
	}

	h.result = AgenticResult{
		FullResponse:    finalContent,
		Reasoning:       reasoning,
		ToolCalls:       calls,
		NativeMCPEvents: mcpEvents,
		Usage:           usage,
		Model:           model,
		IsDone:          false,
	}
}

func (h *agenticStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {
	if event.IsCompleted {
		h.Mu.Lock()
		h.nativeMCPEvents = append(h.nativeMCPEvents, event)
		h.Mu.Unlock()

		status := "ok"
		errSummary := ""
		if event.Error != "" {
			status = "error"
			errSummary = truncateString(event.Error, 200)
		}
		outputSummary := truncateString(event.Output, 200)

		EmitToolEnd(h.Emitter, ports.ToolEndEvent{
			ConversationID: h.ConversationID,
			Name:           event.Name,
			CallID:         event.ID,
			Status:         status,
			Summary:        outputSummary,
			Error:          errSummary,
			ServerLabel:    event.ServerLabel,
			Origin:         OriginMCPNative,
		})

		log.Printf("[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
			event.Name, event.ServerLabel, event.ID, len(event.Output))
	} else {
		EmitToolStart(h.Emitter, ports.ToolStartEvent{
			ConversationID: h.ConversationID,
			Name:           event.Name,
			CallID:         event.ID,
			Args:           event.Arguments,
			ServerLabel:    event.ServerLabel,
			Origin:         OriginMCPNative,
		})

		log.Printf("[MCP Native] 🔧 %s (server=%s, id=%s)",
			event.Name, event.ServerLabel, event.ID)
	}
}

func (h *agenticStreamHandler) OnError(err string) {
	h.Mu.Lock()
	h.CancelPendingChunkTimer()
	h.Mu.Unlock()

	h.result = AgenticResult{
		Error: err,
	}
}

func (h *agenticStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.Mu.Lock()
	h.CancelPendingChunkTimer()
	content := h.AccumulatedContent
	reasoning := h.AccumulatedReasoning
	mcpEvents := h.nativeMCPEvents
	h.nativeMCPEvents = nil
	h.Mu.Unlock()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = content
	}

	h.result = AgenticResult{
		FullResponse:    finalContent,
		Reasoning:       reasoning,
		NativeMCPEvents: mcpEvents,
		Usage:           usage,
		Model:           model,
		IsDone:          true,
	}
}
