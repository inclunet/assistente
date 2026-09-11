package agent

import (
	"assistente/internal/logging"
	"context"
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
	finish llm.FinishInfo
	usage  llm.Usage

	// MCP tool events acumulados durante o streaming (para persistência)
	nativeMCPEvents []llm.MCPToolEvent

	// Alguns providers (ex.: Anthropic) emitem Arguments só no start-event.
	// Guardamos por ID para enriquecer o completed-event antes de persistir.
	nativeMCPArgsByID map[string]string
}

var (
	_ llm.TurnNoticeSink = (*AgenticStreamHandler)(nil)
	_ llm.UsageSink      = (*AgenticStreamHandler)(nil)
)

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

func (h *AgenticStreamHandler) OnFinishReason(info llm.FinishInfo) {
	h.mu.Lock()
	h.finish = info
	h.mu.Unlock()
}

func (h *AgenticStreamHandler) OnUsage(usage llm.Usage) {
	h.mu.Lock()
	h.usage = usage
	h.mu.Unlock()
}

func (h *AgenticStreamHandler) ResetStreamAttempt() {
	h.BaseStreamHandler.ResetStreamAttempt()
	h.mu.Lock()
	h.result = AgenticResult{}
	h.finish = llm.FinishInfo{}
	h.usage = llm.Usage{}
	h.nativeMCPEvents = nil
	h.nativeMCPArgsByID = make(map[string]string)
	h.mu.Unlock()
}

func (h *AgenticStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	content, reasoning := h.Finalize()
	h.mu.Lock()
	mcpEvents := h.nativeMCPEvents
	finish := h.finish
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
		Finish:          finish,
	}
}

func (h *AgenticStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {
	h.FlushStream()
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
			ConversationID:     h.ConversationID,
			TurnID:             h.TurnID,
			AssistantMessageID: h.AssistantMessageID,
			Name:               event.Name,
			CallID:             event.ID,
			Status:             status,
			Summary:            outputSummary,
			Error:              errSummary,
			ServerLabel:        event.ServerLabel,
			Origin:             OriginMCPNative,
			SurfaceOrigin:      h.SurfaceOrigin,
		})

		if event.Error != "" {
			EmitToolFailure(h.Emitter, ports.ToolFailureEvent{
				ConversationID:     h.ConversationID,
				TurnID:             h.TurnID,
				AssistantMessageID: h.AssistantMessageID,
				Name:               event.Name,
				CallID:             event.ID,
				ErrorKind:          "unknown",
				Retryable:          false,
				Message:            errSummary,
				WillRetry:          false,
				Attempt:            0,
				Origin:             OriginMCPNative,
				SurfaceOrigin:      h.SurfaceOrigin,
			})
		}

		logging.Infof(context.Background(), "agent.agentic-stream-handler", "[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
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
			ConversationID:     h.ConversationID,
			TurnID:             h.TurnID,
			AssistantMessageID: h.AssistantMessageID,
			Name:               event.Name,
			CallID:             event.ID,
			Args:               event.Arguments,
			ServerLabel:        event.ServerLabel,
			Origin:             OriginMCPNative,
			SurfaceOrigin:      h.SurfaceOrigin,
		})

		logging.Infof(context.Background(), "agent.agentic-stream-handler", "[MCP Native] 🔧 %s (server=%s, id=%s)",
			event.Name, event.ServerLabel, event.ID)
	}
}

func (h *AgenticStreamHandler) OnError(err string) {
	h.FlushStream()
	h.FinishThinkingIfActive()
	content, reasoning := h.Finalize()
	h.mu.Lock()
	finish := h.finish
	usage := h.usage
	mcpEvents := h.nativeMCPEvents
	h.nativeMCPEvents = nil
	h.mu.Unlock()
	h.result = AgenticResult{
		FullResponse:    content,
		Reasoning:       reasoning,
		NativeMCPEvents: mcpEvents,
		Usage:           usage,
		Error:           err,
		Finish:          finish,
	}
}

func (h *AgenticStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	content, reasoning := h.Finalize()
	h.mu.Lock()
	mcpEvents := h.nativeMCPEvents
	finish := h.finish
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
		Finish:          finish,
	}
}
