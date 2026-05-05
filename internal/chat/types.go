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
	Model            string    `json:"model,omitempty"`
	Source           string    `json:"source,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	Timestamp        int64     `json:"timestamp"`
	IsStreaming      bool      `json:"isStreaming"`
	Internal         bool      `json:"internal"`
}

type MessageNode struct {
	Message    EnrichedMessage `json:"message"`
	Children   []MessageNode   `json:"children,omitempty"`
	Level      int             `json:"level"`
	ChildCount int             `json:"childCount"`
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
