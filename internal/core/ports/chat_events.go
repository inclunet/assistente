package ports

// Chat event structs — typed payloads for all chat:* events.
// Every chat event MUST carry ConversationID (AEP-0040).

const (
	// ChatErrorInternal is a stable UI-facing error code for unexpected backend failures.
	ChatErrorInternal = "internal_error"
	// ChatErrorAssistantPlaceholder is emitted when the assistant placeholder cannot be created.
	ChatErrorAssistantPlaceholder = "assistant_placeholder_error"
)

// ChatSurfaceOrigin identifies the frontend surface that initiated a chat turn.
type ChatSurfaceOrigin struct {
	SessionKey     string `json:"sessionKey"`
	ConversationID string `json:"conversationId"`
	TabID          string `json:"tabId,omitempty"`
	SurfaceID      string `json:"surfaceId"`
	SurfaceType    string `json:"surfaceType"`
}

func NewChatSurfaceOrigin(conversationID, sessionKey, surfaceID, surfaceType, tabID string) *ChatSurfaceOrigin {
	if sessionKey == "" || surfaceID == "" || surfaceType == "" {
		return nil
	}
	return &ChatSurfaceOrigin{
		SessionKey:     sessionKey,
		ConversationID: conversationID,
		TabID:          tabID,
		SurfaceID:      surfaceID,
		SurfaceType:    surfaceType,
	}
}

// ThinkingEvent is the payload for chat:thinking.
type ThinkingEvent struct {
	ConversationID     string             `json:"conversationId"`
	TurnID             string             `json:"turnId,omitempty"`
	AssistantMessageID string             `json:"assistantMessageId,omitempty"`
	Content            string             `json:"content,omitempty"`
	Done               bool               `json:"done"`
	Started            bool               `json:"started,omitempty"`
	SurfaceOrigin      *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// DoneEvent is the payload for chat:done.
type DoneEvent struct {
	ConversationID     string `json:"conversationId"`
	TurnID             string `json:"turnId,omitempty"`
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
	HadToolCalls       bool   `json:"hadToolCalls,omitempty"`
	// AEP-0039 Fase 2: enriched done event
	Reason           string             `json:"reason,omitempty"` // "completed" | "limit_reached" | "error"
	IterationCount   int                `json:"iterationCount,omitempty"`
	ToolCallCount    int                `json:"toolCallCount,omitempty"`
	ToolsUsed        []string           `json:"toolsUsed,omitempty"`
	PromptTokens     int                `json:"promptTokens,omitempty"`
	CompletionTokens int                `json:"completionTokens,omitempty"`
	CacheReadTokens  int                `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int                `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens  int                `json:"cacheMissTokens,omitempty"`
	ErrorMessage     string             `json:"errorMessage,omitempty"`
	SurfaceOrigin    *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ErrorEvent is the payload for chat:error.
type ErrorEvent struct {
	ConversationID string             `json:"conversationId"`
	Error          string             `json:"error"`
	SurfaceOrigin  *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// MessagesReadyEvent is the payload for chat:messages_ready.
type MessagesReadyEvent struct {
	ConversationID string             `json:"conversationId"`
	UserMessageID  string             `json:"userMessageId"`
	TurnID         string             `json:"turnId,omitempty"`
	UserContent    string             `json:"userContent"`
	SurfaceOrigin  *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// SkillLoadedEvent is the payload for chat:skill_loaded.
type SkillLoadedEvent struct {
	ConversationID string             `json:"conversationId"`
	TurnID         string             `json:"turnId,omitempty"`
	Slug           string             `json:"slug"`
	DisplayName    string             `json:"displayName,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	SurfaceOrigin  *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolStartEvent is the payload for chat:tool_start.
type ToolStartEvent struct {
	ConversationID     string `json:"conversationId"`
	TurnID             string `json:"turnId,omitempty"`
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
	Name               string `json:"name"`
	CallID             string `json:"callId"`
	Args               string `json:"args,omitempty"`
	// Summary descreve a chamada em texto legível quando a ferramenta não é do
	// app e não há argumentos estruturados para mostrar (AEP-0084 D7).
	Summary       string             `json:"summary,omitempty"`
	ServerLabel   string             `json:"serverLabel,omitempty"`
	Origin        string             `json:"origin,omitempty"` // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	Attempt       int                `json:"attempt"`          // Tentativa (0=primeira, 1=retry)
	SurfaceOrigin *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolEndEvent is the payload for chat:tool_end.
type ToolEndEvent struct {
	ConversationID     string             `json:"conversationId"`
	TurnID             string             `json:"turnId,omitempty"`
	AssistantMessageID string             `json:"assistantMessageId,omitempty"`
	Name               string             `json:"name,omitempty"`
	CallID             string             `json:"callId"`
	Status             string             `json:"status"`
	Summary            string             `json:"summary,omitempty"`
	Error              string             `json:"error,omitempty"`
	ServerLabel        string             `json:"serverLabel,omitempty"`
	Origin             string             `json:"origin,omitempty"`     // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	DurationMs         int64              `json:"durationMs,omitempty"` // AEP-0039 Fase 3
	Attempt            int                `json:"attempt"`              // Tentativa (0=primeira, 1=retry)
	SurfaceOrigin      *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolFailureEvent is the payload for chat:tool_failure (AEP-0039 Fase 3).
// Emitted when a tool execution fails with structured error classification.
// Distinct from tool_end with status="error" — this carries retry context.
type ToolFailureEvent struct {
	ConversationID     string             `json:"conversationId"`
	TurnID             string             `json:"turnId,omitempty"`
	AssistantMessageID string             `json:"assistantMessageId,omitempty"`
	Name               string             `json:"name"`
	CallID             string             `json:"callId"`
	ErrorKind          string             `json:"errorKind"` // "timeout" | "invalid_args" | "not_found" | "panic" | "cancelled" | "unknown"
	Retryable          bool               `json:"retryable"`
	Message            string             `json:"message,omitempty"`
	DurationMs         int64              `json:"durationMs,omitempty"`
	Origin             string             `json:"origin,omitempty"`    // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	WillRetry          bool               `json:"willRetry,omitempty"` // true se retry automático será tentado
	Attempt            int                `json:"attempt"`             // Tentativa (0=primeira, 1=retry)
	SurfaceOrigin      *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// ToolSummary describes a tool invocation within an iteration (AEP-0039 Fase 2+3).
type ToolSummary struct {
	Name        string `json:"name"`
	Status      string `json:"status"`               // "ok" | "error"
	ErrorKind   string `json:"errorKind,omitempty"`  // AEP-0039 Fase 3
	DurationMs  int64  `json:"durationMs,omitempty"` // AEP-0039 Fase 3
	Origin      string `json:"origin,omitempty"`     // "builtin" | "mcp_bridge" | "mcp_native" | "acp_agent"
	ServerLabel string `json:"serverLabel,omitempty"`
}

// SegmentDoneEvent is the payload for chat:segment_done.
type SegmentDoneEvent struct {
	ConversationID     string `json:"conversationId"`
	TurnID             string `json:"turnId,omitempty"`
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
	Content            string `json:"content,omitempty"`
	Iteration          int    `json:"iteration"`
	HasMore            bool   `json:"hasMore"`
	// AEP-0039 Fase 2: tools executed in this iteration
	ToolsInIteration []ToolSummary      `json:"toolsInIteration,omitempty"`
	SurfaceOrigin    *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}

// TokenStatsEvent is the payload for chat:token_stats.
type TokenStatsEvent struct {
	ConversationID      string  `json:"conversationId"`
	TotalTokens         int     `json:"totalTokens"`
	ContextTokens       int     `json:"contextTokens"`
	ContextLimit        int     `json:"contextLimit"`
	ContextUsage        float64 `json:"contextUsage"`
	IsNearLimit         bool    `json:"isNearLimit"`
	IsCritical          bool    `json:"isCritical"`
	PromptTokens        int     `json:"promptTokens"`
	CompletionTokens    int     `json:"completionTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens    int     `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens     int     `json:"cacheMissTokens,omitempty"`
	CacheHitRate        float64 `json:"cacheHitRate"`
	CacheTokensReported bool    `json:"cacheTokensReported"`
	PromptCacheEnabled  *bool   `json:"promptCacheEnabled,omitempty"`
	MessageCount        int     `json:"messageCount"`
	ModelCallCount      int     `json:"modelCallCount"`
}

// TokenStatsUpdateEvent is the payload for chat:token_stats_update (realtime during agentic loop).
type TokenStatsUpdateEvent struct {
	ConversationID              string  `json:"conversationId"`
	PromptTokens                int     `json:"promptTokens"`
	CompletionTokens            int     `json:"completionTokens"`
	TotalTokens                 int     `json:"totalTokens"`
	CacheReadTokens             int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens            int     `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens             int     `json:"cacheMissTokens,omitempty"`
	CacheHitRate                float64 `json:"cacheHitRate"`
	CacheTokensReported         bool    `json:"cacheTokensReported"`
	PromptCacheEnabled          *bool   `json:"promptCacheEnabled,omitempty"`
	ContextTokens               int     `json:"contextTokens"`
	ContextUsage                float64 `json:"contextUsage"`
	ContextLimit                int     `json:"contextLimit"`
	IsNearLimit                 bool    `json:"isNearLimit"`
	IsCritical                  bool    `json:"isCritical"`
	MessageCount                int     `json:"messageCount"`
	ModelCallCount              int     `json:"modelCallCount"`
	SystemPromptEstimatedTokens int     `json:"systemPromptEstimatedTokens"`
	SummaryTokens               int     `json:"summaryTokens"`
	MessagesInContextCount      int     `json:"messagesInContextCount"`
	MessagesInContextTokens     int     `json:"messagesInContextTokens"`
	MessagesOutOfContextCount   int     `json:"messagesOutOfContextCount"`
	MessagesOutOfContextTokens  int     `json:"messagesOutOfContextTokens"`
	ToolsUsedCount              int     `json:"toolsUsedCount"`
	ToolBreakdown               any     `json:"toolBreakdown,omitempty"`
}

// ContextWarningEvent is the payload for chat:context_warning.
// ContextTokens reflete a ocupação ATUAL da janela de contexto (usage do
// último turno), não o acumulado de billing (issue #197 / AEP-0012).
type ContextWarningEvent struct {
	ConversationID string  `json:"conversationId"`
	Level          string  `json:"level"` // "warning" | "critical"
	Message        string  `json:"message"`
	Percentage     float64 `json:"percentage"`
	ContextTokens  int     `json:"contextTokens"`
	ContextLimit   int     `json:"contextLimit"`
}

// ChatNoticeKindAttachmentsNotSent identifica o turno que seguiu sem parte dos
// anexos, porque o provedor não os recebe (AEP-0084). Não é falha: o turno
// segue com o texto, e a pessoa é avisada do que ficou de fora em vez de
// esperar uma resposta sobre uma imagem que o modelo nunca viu.
const ChatNoticeKindAttachmentsNotSent = "attachments_not_sent"

// ChatNoticeEvent is the payload for chat:notice — um aviso sobre o turno que
// não é a resposta, não é falha e não encerra nada.
//
// O motivo vai como código, e não como frase: quem exibe traduz para o idioma
// de quem lê.
type ChatNoticeEvent struct {
	ConversationID string `json:"conversationId"`
	Kind           string `json:"kind"`
	Count          int    `json:"count,omitempty"`
}

// SummaryStartedEvent is the payload for chat:summary_started.
type SummaryStartedEvent struct {
	ConversationID string `json:"conversationId"`
	MessageCount   int    `json:"messageCount"`
}

// SummaryErrorCodeAgentProvider identifica a recusa de resumir uma conversa
// cujo provider é um agente externo (AEP-0084 D14). Não é uma falha: é uma
// condição prevista, e a interface a traduz no idioma de quem lê.
const SummaryErrorCodeAgentProvider = "agent_provider"

// SummaryErrorEvent is the payload for chat:summary_error.
//
// Code nomeia os motivos que a interface sabe traduzir; quando vazio, resta
// Error, que é a mensagem crua (mensagem de erro do provedor, por exemplo).
type SummaryErrorEvent struct {
	ConversationID string `json:"conversationId"`
	Error          string `json:"error"`
	Code           string `json:"code,omitempty"`
}

// SummaryCompletedEvent is the payload for chat:summary_completed.
type SummaryCompletedEvent struct {
	ConversationID       string `json:"conversationId"`
	SummaryUpToMessageID string `json:"summaryUpToMessageId"`
	SummaryLength        int    `json:"summaryLength"`
	MessageCount         int    `json:"messageCount"`
}

// MessageDeletedEvent is the payload for message:deleted.
type MessageDeletedEvent struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
}

// MessageUpdatedEvent is the payload for message:updated.
type MessageUpdatedEvent struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	Content        string `json:"content"`
}

// ConversationRenamedEvent is the payload for conversation:renamed.
type ConversationRenamedEvent struct {
	ConversationID string `json:"conversationId"`
	NewTitle       string `json:"newTitle"`
}
