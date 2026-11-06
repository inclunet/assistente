package chat

const (
	MessageWindowScopeConversation = "conversation"
	MessageWindowScopeThread       = "thread"

	MessageWindowAnchorStart = "start"
	MessageWindowAnchorEnd   = "end"

	MessageWindowDirectionBefore = "before"
	MessageWindowDirectionAfter  = "after"
	MessageWindowDirectionAround = "around"
)

type MessageWindow struct {
	Scope          string `json:"scope"`
	ConversationID string `json:"conversationId"`
	ThreadParentID string `json:"threadParentId,omitempty"`
	TotalCount     int    `json:"totalCount"`
	StartIndex     int    `json:"startIndex"`
	EndIndex       int    `json:"endIndex"`
	HasBefore      bool   `json:"hasBefore"`
	HasAfter       bool   `json:"hasAfter"`
}
