package chat

import (
	"time"
)

const (
	MessageWindowScopeConversation = "conversation"
	MessageWindowScopeThread       = "thread"

	MessageWindowAnchorStart = "start"
	MessageWindowAnchorEnd   = "end"

	MessageWindowDirectionBefore = "before"
	MessageWindowDirectionAfter  = "after"
	MessageWindowDirectionAround = "around"
)

type MessageWindowRequest struct {
	Scope           string `json:"scope"`
	ConversationID  string `json:"conversationId"`
	ThreadParentID  string `json:"threadParentId,omitempty"`
	Anchor          string `json:"anchor,omitempty"`
	AnchorMessageID string `json:"anchorMessageId,omitempty"`
	Direction       string `json:"direction"`
	Limit           int    `json:"limit"`
}

// TurnSegmentToolCall é a projeção leve de uma invocação canônica.
type TurnSegmentToolCall struct {
	InvocationID       string `json:"invocationId,omitempty"`
	ID                 string `json:"callId"`
	Name               string `json:"name"`
	Origin             string `json:"origin,omitempty"`
	ServerLabel        string `json:"serverLabel,omitempty"`
	Status             string `json:"status"`
	Iteration          int    `json:"iteration,omitempty"`
	DurationMs         int64  `json:"durationMs,omitempty"`
	InputPreview       string `json:"inputPreview,omitempty"`
	OutputPreview      string `json:"outputPreview,omitempty"`
	InputBytes         int64  `json:"inputBytes,omitempty"`
	OutputBytes        int64  `json:"outputBytes,omitempty"`
	HasDetails         bool   `json:"hasDetails"`
	ResultAvailability string `json:"resultAvailability"`
	// AssistantMessageID é metadado interno de hidratação para associar a
	// invocação à mensagem assistant que representou a iteração.
	AssistantMessageID string `json:"-"`
}

// TurnSegment é uma fatia ordenada cronologicamente de um turno do assistente:
// um trecho de texto OU uma sequência de tool calls dentro de uma única iteração
// do agentic loop. Issue #150: o chat history precisa preservar a cadeia de
// raciocínio (texto → tool_calls → texto → tool_calls → resposta final) dentro
// de uma ÚNICA entrada do timeline para que leitores de tela como NVDA leiam o
// turno inteiro como uma única mensagem do assistente.
type TurnSegment struct {
	Type      string                `json:"type"` // "text" | "tool_calls"
	Content   string                `json:"content,omitempty"`
	ToolCalls []TurnSegmentToolCall `json:"toolInvocations,omitempty"`
}

type EnrichedMessage struct {
	ID               string    `json:"id"`
	ConversationID   string    `json:"conversationId"`
	ParentID         *string   `json:"parentId,omitempty"`
	TurnID           *string   `json:"turnId,omitempty"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`
	Media            string    `json:"media,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	CacheReadTokens  int       `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int       `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens  int       `json:"cacheMissTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	Source           string    `json:"source,omitempty"`
	Pinned           bool      `json:"pinned"`
	CreatedAt        time.Time `json:"createdAt"`
	Timestamp        int64     `json:"timestamp"`
	IsStreaming      bool      `json:"isStreaming"`
	Internal         bool      `json:"internal"`
	// TurnSegments é populado pelo backend quando uma representação de turno
	// consolidado é construída a partir de múltiplas mensagens persistidas.
	// Cada segmento mantém a ordem cronológica e carrega somente projeções
	// leves das invocações; payloads integrais são buscados sob demanda.
	TurnSegments []TurnSegment `json:"turnSegments,omitempty"`
}

type MessageNode struct {
	Message       EnrichedMessage `json:"message"`
	Children      []MessageNode   `json:"children,omitempty"`
	Level         int             `json:"level"`
	ChildCount    int             `json:"childCount"`
	OriginalIndex *int            `json:"originalIndex,omitempty"`
}

type MessageWindow struct {
	Scope          string        `json:"scope"`
	ConversationID string        `json:"conversationId"`
	ThreadParentID string        `json:"threadParentId,omitempty"`
	Nodes          []MessageNode `json:"nodes"`
	TotalCount     int           `json:"totalCount"`
	StartIndex     int           `json:"startIndex"`
	EndIndex       int           `json:"endIndex"`
	HasBefore      bool          `json:"hasBefore"`
	HasAfter       bool          `json:"hasAfter"`
}

type ConversationWithThreads struct {
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	Threads []MessageNode `json:"threads"`
}
