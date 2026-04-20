package agent

import (
	"log"

	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
)

// AgenticStreamHandler implementa IterationHandler para o agentic loop.
// Emite eventos de streaming para o frontend em tempo real (chat:stream, chat:thinking,
// chat:tool_start, chat:tool_end), mas NÃO salva no banco e NÃO emite chat:done —
// o loop em service.go controla isso.
type AgenticStreamHandler struct {
	BaseStreamHandler
	iteration int

	// Resultado da iteração (preenchido por OnDone/OnToolCalls/OnError)
	result AgenticResult

	// MCP tool events acumulados durante o streaming (para persistência)
	nativeMCPEvents []llm.MCPToolEvent
}

// NewAgenticStreamHandler cria um handler para uma iteração do agentic loop.
func NewAgenticStreamHandler(emitter events.Emitter, conversationID uint, iteration int) *AgenticStreamHandler {
	return &AgenticStreamHandler{
		BaseStreamHandler: BaseStreamHandler{
			Emitter:        emitter,
			ConversationID: conversationID,
		},
		iteration: iteration,
	}
}

// Result implementa IterationHandler.
func (h *AgenticStreamHandler) Result() AgenticResult {
	return h.result
}

func (h *AgenticStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	h.mu.Lock()
	h.cancelPendingChunkTimer()
	content := h.accumulatedContent
	reasoning := h.accumulatedReasoning
	mcpEvents := h.nativeMCPEvents
	h.nativeMCPEvents = nil
	h.mu.Unlock()

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

func (h *AgenticStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {
	if event.IsCompleted {
		h.mu.Lock()
		h.nativeMCPEvents = append(h.nativeMCPEvents, event)
		h.mu.Unlock()

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

func (h *AgenticStreamHandler) OnError(err string) {
	h.mu.Lock()
	h.cancelPendingChunkTimer()
	h.mu.Unlock()

	h.result = AgenticResult{
		Error: err,
	}
}

func (h *AgenticStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.mu.Lock()
	h.cancelPendingChunkTimer()
	content := h.accumulatedContent
	reasoning := h.accumulatedReasoning
	mcpEvents := h.nativeMCPEvents
	h.nativeMCPEvents = nil
	h.mu.Unlock()

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
