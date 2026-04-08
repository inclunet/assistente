package main

import (
	"context"
	"log"

	"assistente/internal/agent"
	"assistente/internal/config"
	"assistente/internal/llm"
)

// ==================== Agentic Stream Handler ====================

// agenticStreamHandler implementa agent.IterationHandler para uso no agentic loop.
// Emite eventos de streaming para o frontend em tempo real (chat:stream, chat:thinking),
// mas NÃO salva no banco e NÃO emite chat:done — o loop controla isso.
type agenticStreamHandler struct {
	baseStreamHandler
	iteration int

	// Resultado da iteração (preenchido por OnDone/OnToolCalls/OnError)
	result agent.AgenticResult

	// MCP tool events acumulados durante o streaming (para persistência)
	nativeMCPEvents []llm.MCPToolEvent
}

// Result implementa agent.IterationHandler.
func (h *agenticStreamHandler) Result() agent.AgenticResult {
	return h.result
}

func (h *agenticStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
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

	h.result = agent.AgenticResult{
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

		h.emitter.Emit("chat:tool_end", map[string]interface{}{
			"name":           event.Name,
			"callId":         event.ID,
			"status":         status,
			"summary":        outputSummary,
			"error":          errSummary,
			"serverLabel":    event.ServerLabel,
			"native":         true,
			"conversationId": h.conversationID,
		})

		log.Printf("[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
			event.Name, event.ServerLabel, event.ID, len(event.Output))
	} else {
		h.emitter.Emit("chat:tool_start", map[string]interface{}{
			"name":           event.Name,
			"callId":         event.ID,
			"args":           event.Arguments,
			"serverLabel":    event.ServerLabel,
			"native":         true,
			"conversationId": h.conversationID,
		})

		log.Printf("[MCP Native] 🔧 %s (server=%s, id=%s)",
			event.Name, event.ServerLabel, event.ID)
	}
}

func (h *agenticStreamHandler) OnError(err string) {
	h.mu.Lock()
	h.cancelPendingChunkTimer()
	h.mu.Unlock()

	h.result = agent.AgenticResult{
		Error: err,
	}
}

func (h *agenticStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
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

	h.result = agent.AgenticResult{
		FullResponse:    finalContent,
		Reasoning:       reasoning,
		NativeMCPEvents: mcpEvents,
		Usage:           usage,
		Model:           model,
		IsDone:          true,
	}
}

// ==================== Agentic Loop (thin adapter) ====================

// runAgenticLoop delega para a.agentSvc.RunAgenticLoop.
func (a *App) runAgenticLoop(
	ctx context.Context,
	_ *config.Config,
	messages []llm.Message,
	params llm.ChatParams,
	conversationID uint,
	turnID uint,
	toolDefs []llm.ToolDefinition,
	streamer llm.Streamer,
) {
	a.agentSvc.RunAgenticLoop(ctx, messages, params, conversationID, turnID, toolDefs, streamer,
		func(convID uint, iter int) agent.IterationHandler {
			return &agenticStreamHandler{
				baseStreamHandler: baseStreamHandler{
					emitter:        a.emitter,
					conversationID: convID,
				},
				iteration: iter,
			}
		},
	)
}

// truncateString trunca uma string ao tamanho máximo.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
