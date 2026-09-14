package history

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

const maxGetMessagesIDs = 20

type getMessagesArgs struct {
	IDs                []string `json:"ids"`
	IncludeToolResults bool     `json:"include_tool_results,omitempty"`
}

type messageReader interface {
	GetMessageWithContext(ctx context.Context, messageID string) (*database.ChatMessage, error)
	GetTurnMessagesWithContext(ctx context.Context, turnID string) ([]database.ChatMessage, error)
}

type databaseMessageReader struct{}

func (databaseMessageReader) GetMessageWithContext(ctx context.Context, messageID string) (*database.ChatMessage, error) {
	return database.GetMessageWithContext(ctx, messageID)
}

func (databaseMessageReader) GetTurnMessagesWithContext(ctx context.Context, turnID string) ([]database.ChatMessage, error) {
	return database.GetTurnMessagesWithContext(ctx, turnID)
}

// GetMessagesTool reidrata mensagens do histórico pelo ID sem expor campos
// binários grandes, como áudio e mídia em base64.
type GetMessagesTool struct {
	reader messageReader
}

func NewGetMessages() *GetMessagesTool {
	return &GetMessagesTool{reader: databaseMessageReader{}}
}

func (t *GetMessagesTool) Name() string { return "get_messages" }

func (t *GetMessagesTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "history", Class: "read_context", Package: "history", Risk: "read"}
}

func (t *GetMessagesTool) Description() string {
	return "Rehydrates the complete textual content and tool-call fields of known historical message IDs, while omitting large binary media. Use it after search_conversations (or another result that supplied IDs) when snippets are insufficient and exact message content is needed. IDs may reference any conversations accessible to the current user; the response identifies each conversation, so keep related IDs together when reconstructing context. Do not use it to discover messages, inspect conversation metadata or summaries, or load a whole conversation. Request at most 20 IDs per call and batch further IDs across calls; include_tool_results expands the selected turns and increases output cost, so enable it only when tool outputs are needed."
}

func (t *GetMessagesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"ids": {
				"type": "array",
				"description": "Known message IDs to rehydrate, in the desired order (maximum 20 per call). IDs may come from accessible conversations; batch additional IDs in separate calls.",
				"items": {"type": "string", "minLength": 1},
				"minItems": 1,
				"maxItems": 20
			},
			"include_tool_results": {
				"type": "boolean",
				"description": "When true, also includes role=tool result messages from the selected messages' turns. Default false; enable only when those outputs are needed because expansion can substantially increase response size."
			}
		},
		"required": ["ids"],
		"additionalProperties": false
	}`)
}

type getMessagePayload struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	Role           string          `json:"role"`
	Content        string          `json:"content"`
	ToolCalls      json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	CreatedAt      string          `json:"created_at"`
}

type getMessagesPayload struct {
	Messages []getMessagePayload `json:"messages"`
}

type requestedTurn struct {
	ConversationID string
	TurnID         string
}

func (t *GetMessagesTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params getMessagesArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return toolArgumentError("Error parsing arguments: %v", err), nil
	}
	if len(params.IDs) == 0 {
		return toolArgumentError("ids must contain at least one message ID"), nil
	}
	if len(params.IDs) > maxGetMessagesIDs {
		return toolArgumentError("ids accepts at most %d message IDs", maxGetMessagesIDs), nil
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return tools.ToolResult{Content: "Message access rejected: authenticated user scope is required.", IsError: true}, nil
	}
	if t.reader == nil {
		return tools.ToolResult{Content: "Message reader is not configured.", IsError: true}, nil
	}

	messages := make([]database.ChatMessage, 0, len(params.IDs))
	seen := make(map[string]struct{}, len(params.IDs))
	expandedTurns := make(map[string]struct{})
	requestedTurns := make([]requestedTurn, 0, len(params.IDs))
	for _, rawID := range params.IDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return toolArgumentError("ids must not contain empty message IDs"), nil
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		msg, err := t.reader.GetMessageWithContext(ctx, id)
		if err != nil {
			return tools.ToolResult{
				Content: fmt.Sprintf("Message %q was not found or is not accessible to the current user.", id),
				IsError: true,
			}, nil
		}
		seen[id] = struct{}{}
		messages = append(messages, *msg)

		turnID := ""
		if msg.TurnID != nil {
			turnID = strings.TrimSpace(*msg.TurnID)
		} else if msg.Role == "user" {
			// TurnID aponta para a mensagem user raiz; nela própria o campo pode
			// ser nulo, então o ID da mensagem identifica o turno.
			turnID = msg.ID
		}
		if params.IncludeToolResults && turnID != "" {
			turnKey := msg.ConversationID + "\x00" + turnID
			if _, expanded := expandedTurns[turnKey]; expanded {
				continue
			}
			expandedTurns[turnKey] = struct{}{}
			requestedTurns = append(requestedTurns, requestedTurn{ConversationID: msg.ConversationID, TurnID: turnID})
		}
	}

	payload := getMessagesPayload{Messages: make([]getMessagePayload, 0, len(messages))}
	for _, msg := range messages {
		item, _ := messagePayload(msg)
		payload.Messages = append(payload.Messages, item)
	}
	if params.IncludeToolResults {
		if err := t.appendRequestedToolResults(ctx, userID, requestedTurns, seen, &payload); err != nil {
			return tools.ToolResult{Content: "Tool results could not be read for the requested message.", IsError: true}, nil
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return tools.ToolResult{Content: "Messages could not be encoded.", IsError: true}, nil
	}

	return tools.ToolResult{
		Content:    string(encoded),
		Structured: true,
		Metadata: map[string]any{
			"requested": len(params.IDs),
			"returned":  len(payload.Messages),
		},
	}, nil
}

func (t *GetMessagesTool) appendRequestedToolResults(
	ctx context.Context,
	userID string,
	turns []requestedTurn,
	seen map[string]struct{},
	payload *getMessagesPayload,
) error {
	canonicalTurnIDs := make([]string, 0, len(turns))
	for _, turn := range turns {
		canonicalTurnIDs = append(canonicalTurnIDs, turn.TurnID)
	}
	displays, err := toolinvocations.LoadChatToolInvocationDisplaysForTurnIDsWithUser(ctx, userID, canonicalTurnIDs)
	if err != nil {
		return err
	}
	for _, turn := range turns {
		for _, display := range displays[turn.TurnID] {
			ledgerID := strings.TrimSpace(display.LedgerID)
			if ledgerID == "" {
				ledgerID = turn.TurnID + ":" + display.ID
			}
			if _, duplicate := seen[ledgerID]; duplicate {
				continue
			}
			seen[ledgerID] = struct{}{}
			payload.Messages = append(payload.Messages, getMessagePayload{
				ID:             ledgerID,
				ConversationID: turn.ConversationID,
				Role:           "tool",
				Content:        display.ModelResult,
				ToolCallID:     display.ID,
				CreatedAt:      display.CreatedAt.UTC().Format(time.RFC3339Nano),
			})
		}
	}
	return nil
}

func messagePayload(msg database.ChatMessage) (getMessagePayload, error) {
	return getMessagePayload{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		Role:           msg.Role,
		Content:        msg.Content,
		CreatedAt:      msg.CreatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func toolArgumentError(format string, args ...any) tools.ToolResult {
	return tools.ToolResult{Content: fmt.Sprintf(format, args...), IsError: true}
}
