package ports

// Chat event structs — typed payloads for all chat:* events.
// Every chat event MUST carry ConversationID (AEP-0040).

// ThinkingEvent is the payload for chat:thinking.
type ThinkingEvent struct {
	ConversationID uint   `json:"conversationId"`
	Content        string `json:"content,omitempty"`
	Done           bool   `json:"done"`
	Started        bool   `json:"started,omitempty"`
}

// DoneEvent is the payload for chat:done.
type DoneEvent struct {
	ConversationID     uint `json:"conversationId"`
	AssistantMessageID uint `json:"assistantMessageId,omitempty"`
	HadToolCalls       bool `json:"hadToolCalls,omitempty"`
	// AEP-0039 Fase 2: enriched done event
	Reason           string   `json:"reason,omitempty"`           // "completed" | "limit_reached" | "error"
	IterationCount   int      `json:"iterationCount,omitempty"`
	ToolCallCount    int      `json:"toolCallCount,omitempty"`
	ToolsUsed        []string `json:"toolsUsed,omitempty"`
	PromptTokens     int      `json:"promptTokens,omitempty"`
	CompletionTokens int      `json:"completionTokens,omitempty"`
	ErrorMessage     string   `json:"errorMessage,omitempty"`
}

// ErrorEvent is the payload for chat:error.
type ErrorEvent struct {
	ConversationID uint   `json:"conversationId"`
	Error          string `json:"error"`
}

// MessagesReadyEvent is the payload for chat:messages_ready.
type MessagesReadyEvent struct {
	ConversationID uint   `json:"conversationId"`
	UserMessageID  uint   `json:"userMessageId"`
	UserContent    string `json:"userContent"`
}

// ToolStartEvent is the payload for chat:tool_start.
type ToolStartEvent struct {
	ConversationID uint   `json:"conversationId"`
	Name           string `json:"name"`
	CallID         string `json:"callId"`
	Args           string `json:"args,omitempty"`
	ServerLabel    string `json:"serverLabel,omitempty"`
	Native         bool   `json:"native,omitempty"`          // DEPRECADO (AEP-0039) — usar Origin
	Origin         string `json:"origin,omitempty"`           // "builtin" | "mcp_bridge" | "mcp_native"
}

// ToolEndEvent is the payload for chat:tool_end.
type ToolEndEvent struct {
	ConversationID uint   `json:"conversationId"`
	Name           string `json:"name,omitempty"`
	CallID         string `json:"callId"`
	Status         string `json:"status"`
	Summary        string `json:"summary,omitempty"`
	Error          string `json:"error,omitempty"`
	ServerLabel    string `json:"serverLabel,omitempty"`
	Native         bool   `json:"native,omitempty"`          // DEPRECADO (AEP-0039) — usar Origin
	Origin         string `json:"origin,omitempty"`           // "builtin" | "mcp_bridge" | "mcp_native"
	DurationMs     int64  `json:"durationMs,omitempty"`       // AEP-0039 Fase 3
}

// ToolFailureEvent is the payload for chat:tool_failure (AEP-0039 Fase 3).
// Emitted when a tool execution fails with structured error classification.
// Distinct from tool_end with status="error" — this carries retry context.
type ToolFailureEvent struct {
	ConversationID uint   `json:"conversationId"`
	Name           string `json:"name"`
	CallID         string `json:"callId"`
	ErrorKind      string `json:"errorKind"`                  // "timeout" | "invalid_args" | "not_found" | "panic" | "unknown"
	Retryable      bool   `json:"retryable"`
	Message        string `json:"message,omitempty"`
	DurationMs     int64  `json:"durationMs,omitempty"`
	Origin         string `json:"origin,omitempty"`           // "builtin" | "mcp_bridge" | "mcp_native"
	WillRetry      bool   `json:"willRetry,omitempty"`        // true se retry automático será tentado
}

// ToolSummary describes a tool invocation within an iteration (AEP-0039 Fase 2+3).
type ToolSummary struct {
	Name       string `json:"name"`
	Status     string `json:"status"`                       // "ok" | "error"
	ErrorKind  string `json:"errorKind,omitempty"`           // AEP-0039 Fase 3
	DurationMs int64  `json:"durationMs,omitempty"`          // AEP-0039 Fase 3
}

// SegmentDoneEvent is the payload for chat:segment_done.
type SegmentDoneEvent struct {
	ConversationID uint   `json:"conversationId"`
	Content        string `json:"content,omitempty"`
	Iteration      int    `json:"iteration"`
	HasMore        bool   `json:"hasMore"`
	// AEP-0039 Fase 2: tools executed in this iteration
	ToolsInIteration []ToolSummary `json:"toolsInIteration,omitempty"`
}

// TokenStatsEvent is the payload for chat:token_stats.
type TokenStatsEvent struct {
	ConversationID   uint    `json:"conversationId"`
	TotalTokens      int     `json:"totalTokens"`
	ContextLimit     int     `json:"contextLimit"`
	ContextUsage     float64 `json:"contextUsage"`
	IsNearLimit      bool    `json:"isNearLimit"`
	IsCritical       bool    `json:"isCritical"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	MessageCount     int     `json:"messageCount"`
}

// TokenStatsUpdateEvent is the payload for chat:token_stats_update (realtime during agentic loop).
type TokenStatsUpdateEvent struct {
	ConversationID              uint     `json:"conversationId"`
	PromptTokens                int      `json:"promptTokens"`
	CompletionTokens            int      `json:"completionTokens"`
	TotalTokens                 int      `json:"totalTokens"`
	ContextUsage                float64  `json:"contextUsage"`
	ContextLimit                int      `json:"contextLimit"`
	IsNearLimit                 bool     `json:"isNearLimit"`
	IsCritical                  bool     `json:"isCritical"`
	MessageCount                int      `json:"messageCount"`
	SystemPromptEstimatedTokens int      `json:"systemPromptEstimatedTokens"`
	SummaryTokens               int      `json:"summaryTokens"`
	MessagesInContextCount      int      `json:"messagesInContextCount"`
	MessagesInContextTokens     int      `json:"messagesInContextTokens"`
	ToolsUsedCount              int      `json:"toolsUsedCount"`
	ToolBreakdown               any      `json:"toolBreakdown,omitempty"`
}

// ContextWarningEvent is the payload for chat:context_warning.
type ContextWarningEvent struct {
	ConversationID uint    `json:"conversationId"`
	Level          string  `json:"level"` // "warning" | "critical"
	Message        string  `json:"message"`
	Percentage     float64 `json:"percentage"`
	TotalTokens    int     `json:"totalTokens"`
	ContextLimit   int     `json:"contextLimit"`
}

// SummaryStartedEvent is the payload for chat:summary_started.
type SummaryStartedEvent struct {
	ConversationID uint `json:"conversationId"`
	MessageCount   int  `json:"messageCount"`
}

// SummaryErrorEvent is the payload for chat:summary_error.
type SummaryErrorEvent struct {
	ConversationID uint   `json:"conversationId"`
	Error          string `json:"error"`
}

// SummaryCompletedEvent is the payload for chat:summary_completed.
type SummaryCompletedEvent struct {
	ConversationID      uint `json:"conversationId"`
	SummaryUpToMessageID uint `json:"summaryUpToMessageId"`
	SummaryLength       int  `json:"summaryLength"`
	MessageCount        int  `json:"messageCount"`
}

// MessageDeletedEvent is the payload for message:deleted.
type MessageDeletedEvent struct {
	ConversationID uint `json:"conversationId"`
	MessageID      uint `json:"messageId"`
}

// MessageUpdatedEvent is the payload for message:updated.
type MessageUpdatedEvent struct {
	ConversationID uint   `json:"conversationId"`
	MessageID      uint   `json:"messageId"`
	Content        string `json:"content"`
}

// ConversationRenamedEvent is the payload for conversation:renamed.
type ConversationRenamedEvent struct {
	ConversationID uint   `json:"conversationId"`
	NewTitle       string `json:"newTitle"`
}
