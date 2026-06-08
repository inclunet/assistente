package agent

import (
	"log"
	"strings"

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

	// Alguns providers (ex.: Anthropic) emitem Arguments só no start-event.
	// Guardamos por ID para enriquecer o completed-event antes de persistir.
	nativeMCPArgsByID map[string]string
}

// NewAgenticStreamHandler cria um handler para uma iteração do agentic loop.
func NewAgenticStreamHandler(emitter events.Emitter, conversationID string, iteration int, surfaceOrigin *ports.ChatSurfaceOrigin, turnID string) *AgenticStreamHandler {
	return &AgenticStreamHandler{
		BaseStreamHandler: BaseStreamHandler{
			Emitter:        emitter,
			ConversationID: conversationID,
			TurnID:         turnID,
			SurfaceOrigin:  surfaceOrigin,
		},
		iteration:         iteration,
		nativeMCPArgsByID: make(map[string]string),
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
		if strings.TrimSpace(event.Arguments) == "" {
			if args := strings.TrimSpace(h.nativeMCPArgsByID[event.ID]); args != "" {
				event.Arguments = args
			}
		}
		h.nativeMCPEvents = append(h.nativeMCPEvents, event)
		h.mu.Unlock()

		status := "ok"
		errSummary := ""
		if event.Error != "" {
			status = "error"
			errSummary = truncateString(event.Error, MaxResultDisplaySize)
		}
		outputSummary := truncateString(event.Output, MaxResultDisplaySize)

		EmitToolEnd(h.Emitter, ports.ToolEndEvent{
			ConversationID: h.ConversationID,
			TurnID:         h.TurnID,
			Name:           event.Name,
			CallID:         event.ID,
			Status:         status,
			Summary:        outputSummary,
			Error:          errSummary,
			ServerLabel:    event.ServerLabel,
			Origin:         OriginMCPNative,
			SurfaceOrigin:  h.SurfaceOrigin,
		})

		if event.Error != "" {
			EmitToolFailure(h.Emitter, ports.ToolFailureEvent{
				ConversationID: h.ConversationID,
				TurnID:         h.TurnID,
				Name:           event.Name,
				CallID:         event.ID,
				ErrorKind:      "unknown",
				Retryable:      false,
				Message:        errSummary,
				WillRetry:      false,
				Attempt:        0,
				Origin:         OriginMCPNative,
				SurfaceOrigin:  h.SurfaceOrigin,
			})
		}

		log.Printf("[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
			event.Name, event.ServerLabel, event.ID, len(event.Output))
	} else {
		// Start-event: salva argumentos para enriquecer o completed-event depois.
		if strings.TrimSpace(event.ID) != "" && strings.TrimSpace(event.Arguments) != "" {
			h.mu.Lock()
			if _, ok := h.nativeMCPArgsByID[event.ID]; !ok {
				h.nativeMCPArgsByID[event.ID] = event.Arguments
			}
			h.mu.Unlock()
		}
		EmitToolStart(h.Emitter, ports.ToolStartEvent{
			ConversationID: h.ConversationID,
			TurnID:         h.TurnID,
			Name:           event.Name,
			CallID:         event.ID,
			Args:           event.Arguments,
			ServerLabel:    event.ServerLabel,
			Origin:         OriginMCPNative,
			SurfaceOrigin:  h.SurfaceOrigin,
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
