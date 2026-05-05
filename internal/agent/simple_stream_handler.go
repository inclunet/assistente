package agent

import (
	"log"

	"assistente/internal/events"
	"assistente/internal/llm"
)

// SimpleStreamHandler implements llm.StreamHandler for the non-agentic (no-tool) path.
// It embeds BaseStreamHandler for throttled chunk emission and delegates save / notify /
// event emission to the owning Service — eliminating any *App reference in the main package.
type SimpleStreamHandler struct {
	BaseStreamHandler
	svc           *Service
	userMessageID uint   // ID of the user message (root of this response thread)
	profileSlug   string // Profile slug for TTS resolution
}

// NewSimpleStreamHandler constructs a SimpleStreamHandler bound to a conversation.
// It is created by the owning Service so it can close over its dependencies.
func (s *Service) NewSimpleStreamHandler(conversationID, userMessageID uint, profileSlug string) *SimpleStreamHandler {
	return &SimpleStreamHandler{
		BaseStreamHandler: BaseStreamHandler{
			Emitter:        s.emitter,
			ConversationID: conversationID,
		},
		svc:           s,
		userMessageID: userMessageID,
		profileSlug:   profileSlug,
	}
}

func (h *SimpleStreamHandler) OnError(err string) {
	content, _ := h.Finalize()
	h.Emitter.Emit("chat:stream", events.StreamEvent{
		Content:        content,
		Done:           true,
		Error:          err,
		ConversationId: h.ConversationID,
	})
}

// OnToolCalls is the safety fallback for when simple streaming unexpectedly receives tool calls.
// Delegates to OnDone to preserve any textual response.
func (h *SimpleStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	log.Printf("[TOOL_CALLS] recebido %d tool calls no streaming simples — delegando para OnDone", len(calls))
	h.OnDone(fullResponse, usage, model)
}

func (h *SimpleStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {
	if event.IsCompleted {
		log.Printf("[MCP Native] ✅ %s (server=%s, id=%s): %d bytes output",
			event.Name, event.ServerLabel, event.ID, len(event.Output))
	} else {
		log.Printf("[MCP Native] 🔧 %s (server=%s, id=%s)",
			event.Name, event.ServerLabel, event.ID)
	}
}

func (h *SimpleStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	accumulatedContent, accumulatedReasoning := h.Finalize()

	finalContent := fullResponse
	if finalContent == "" {
		finalContent = accumulatedContent
	}

	// Delegate save, notify, and event emission to the Service (same as agentic path).
	// turnID=0 → the assistant message is a root-level message (no parent thread node).
	h.svc.SaveAndFinish(h.ConversationID, 0, AgenticResult{
		FullResponse: finalContent,
		Reasoning:    accumulatedReasoning,
		Usage:        usage,
		Model:        model,
		IsDone:       true,
	}, h.profileSlug, nil)
}
