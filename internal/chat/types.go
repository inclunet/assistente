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

// TurnSegmentToolCall descreve uma chamada de ferramenta dentro de um segmento
// de turno consolidado. Espelha a estrutura emitida pelo evento chat:segment_done
// (AEP-0039) para que o frontend renderize segmentos canônicos vindos do histórico
// com o mesmo componente usado durante o streaming.
type TurnSegmentToolCall struct {
	ID          string                  `json:"id"`
	Type        string                  `json:"type"`
	Function    TurnSegmentToolFunction `json:"function"`
	Result      string                  `json:"result,omitempty"`
	Origin      string                  `json:"origin,omitempty"`
	ServerLabel string                  `json:"server_label,omitempty"`
	Iteration   int                     `json:"iteration,omitempty"`
	DurationMs  int64                   `json:"duration_ms,omitempty"`
	// AssistantMessageID é metadado interno de hidratação para associar a
	// invocação L3-free à mensagem assistant que representou a iteração.
	AssistantMessageID string `json:"-"`
}

// TurnSegmentToolFunction encapsula o nome e os argumentos de uma tool call.
type TurnSegmentToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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
	ToolCalls []TurnSegmentToolCall `json:"toolCalls,omitempty"`
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
	ToolCalls        string    `json:"toolCalls,omitempty"`
	ToolCallID       string    `json:"toolCallId,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	CacheReadTokens  int       `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int       `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens  int       `json:"cacheMissTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	Source           string    `json:"source,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	Timestamp        int64     `json:"timestamp"`
	IsStreaming      bool      `json:"isStreaming"`
	Internal         bool      `json:"internal"`
	// TurnSegments é populado pelo backend quando uma representação de turno
	// consolidado é construída a partir de múltiplas mensagens persistidas
	// (Issue #150). Cada segmento mantém a ordem cronológica do turno; ToolCalls
	// continua presente para retrocompatibilidade com renderizadores que ainda
	// dependem do JSON concatenado.
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
