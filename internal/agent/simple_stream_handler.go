package agent

import (
	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/logging"
	"context"
	"errors"
)

// SimpleStreamHandler implements llm.StreamHandler for the non-agentic (no-tool) path.
// It embeds BaseStreamHandler for throttled chunk emission and delegates save / notify /
// event emission to the owning Service — eliminating any *App reference in the main package.
type SimpleStreamHandler struct {
	BaseStreamHandler
	svc                   *Service
	ctx                   context.Context
	userMessageID         string // ID of the user message (root of this response thread)
	assistantMessageID    string // ID estável do assistant (placeholder) para este turno
	profileSlug           string // Profile slug for TTS resolution
	lastError             string
	suppressTerminalError bool
	// activity acompanha um turno conduzido por agente externo (AEP-0084):
	// ferramentas que o agente rodou e os segmentos já fechados.
	activity agentActivity
}

// SimpleStreamHandler também recebe a atividade de agentes externos e a marca
// de erro que a auto-recuperação não pode repetir.
var (
	_ llm.AgentActivitySink     = (*SimpleStreamHandler)(nil)
	_ llm.NonRetryableErrorSink = (*SimpleStreamHandler)(nil)
)

// NewSimpleStreamHandler constructs a SimpleStreamHandler bound to a conversation.
// It is created by the owning Service so it can close over its dependencies.
func (s *Service) NewSimpleStreamHandler(ctx context.Context, conversationID, userMessageID string, profileSlug string, surfaceOrigin *ports.ChatSurfaceOrigin) (*SimpleStreamHandler, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	assistantMsgID, err := chat.EnsureAssistantPlaceholder(ctx, s.msgRepo, conversationID, userMessageID)
	if errors.Is(err, chat.ErrConversationGone) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return &SimpleStreamHandler{
		BaseStreamHandler: BaseStreamHandler{
			Emitter:            s.emitter,
			ConversationID:     conversationID,
			TurnID:             userMessageID,
			AssistantMessageID: assistantMsgID,
			SurfaceOrigin:      surfaceOrigin,
		},
		svc:                s,
		ctx:                ctx,
		userMessageID:      userMessageID,
		assistantMessageID: assistantMsgID,
		profileSlug:        profileSlug,
	}, nil
}

func (h *SimpleStreamHandler) OnError(err string) {
	h.lastError = err
	h.closePendingAgentTools()
	h.FinishThinkingIfActive()
	content, _ := h.Finalize()
	// A supressão existe para não finalizar o streaming enquanto ainda há
	// tentativa pela frente. Um erro que não pode ser repetido encerra o turno
	// agora, e calá-lo deixaria a tela esperando por uma tentativa que não vem.
	if h.suppressTerminalError && !h.ErrorNotRetryable() {
		return
	}
	h.Emitter.Emit("chat:stream", events.StreamEvent{
		MessageID:      h.AssistantMessageID,
		Content:        content,
		Done:           true,
		Error:          err,
		ConversationId: h.ConversationID,
		TurnID:         h.TurnID,
		SurfaceOrigin:  h.SurfaceOrigin,
	})
}

func (h *SimpleStreamHandler) LastError() string {
	return h.lastError
}

// SuppressTerminalError evita emitir chat:stream terminal com Error.
// Usado pela auto-recuperação para não finalizar o streaming no frontend
// antes de esgotar as tentativas.
func (h *SimpleStreamHandler) SuppressTerminalError(v bool) {
	h.suppressTerminalError = v
}

// OnToolCalls is the safety fallback for when simple streaming unexpectedly receives tool calls.
// Delegates to OnDone to preserve any textual response.
func (h *SimpleStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	logging.Infof(context.Background(), "agent.simple-stream-handler", "[TOOL_CALLS] recebido %d tool calls no streaming simples — delegando para OnDone", len(calls))
	h.OnDone(fullResponse, usage, model)
}

func (h *SimpleStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {
	if event.IsCompleted {
		logging.Infof(context.Background(), "agent.simple-stream-handler", "[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
			event.Name, event.ServerLabel, event.ID, len(event.Output))
	} else {
		logging.Infof(context.Background(), "agent.simple-stream-handler", "[MCP Native] 🔧 %s (server=%s, id=%s)",
			event.Name, event.ServerLabel, event.ID)
	}
}

func (h *SimpleStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.closePendingAgentTools()
	accumulatedContent, accumulatedReasoning := h.Finalize()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = accumulatedContent
	}

	// Delegate save, notify, and event emission to the Service (same as agentic path).
	// The user message remains a standalone item; the assistant response carries the turn id.
	h.svc.SaveAndFinish(h.ctx, h.ConversationID, h.userMessageID, h.assistantMessageID, AgenticResult{
		FullResponse: finalContent,
		Reasoning:    accumulatedReasoning,
		Usage:        usage,
		Model:        model,
		IsDone:       true,
	}, h.profileSlug, nil, h.SurfaceOrigin)
}
