package history

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
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
	if _, err := database.RequireUserID(ctx); err != nil {
		return tools.ToolResult{Content: "Message access rejected: authenticated user scope is required.", IsError: true}, nil
	}
	if t.reader == nil {
		return tools.ToolResult{Content: "Message reader is not configured.", IsError: true}, nil
	}

	messages := make([]database.ChatMessage, 0, len(params.IDs))
	seen := make(map[string]struct{}, len(params.IDs))
	expandedTurns := make(map[string]struct{})
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
			turnMessages, err := t.reader.GetTurnMessagesWithContext(ctx, turnID)
			if err != nil {
				return tools.ToolResult{Content: "Tool results could not be read for the requested message.", IsError: true}, nil
			}
			expandedTurns[turnKey] = struct{}{}
			for _, turnMsg := range turnMessages {
				if turnMsg.Role != "tool" || turnMsg.ConversationID != msg.ConversationID {
					continue
				}
				if _, duplicate := seen[turnMsg.ID]; duplicate {
					continue
				}
				seen[turnMsg.ID] = struct{}{}
				messages = append(messages, turnMsg)
			}
		}
	}

	payload := getMessagesPayload{Messages: make([]getMessagePayload, 0, len(messages))}
	for _, msg := range messages {
		item, err := messagePayload(msg)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Message %q contains invalid tool_calls JSON.", msg.ID), IsError: true}, nil
		}
		payload.Messages = append(payload.Messages, item)
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

func messagePayload(msg database.ChatMessage) (getMessagePayload, error) {
	item := getMessagePayload{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		Role:           msg.Role,
		Content:        msg.Content,
		ToolCallID:     msg.ToolCallID,
		CreatedAt:      msg.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if toolCalls := strings.TrimSpace(msg.ToolCalls); toolCalls != "" {
		var calls []json.RawMessage
		if err := json.Unmarshal([]byte(toolCalls), &calls); err == nil {
			if len(calls) > 0 {
				item.ToolCalls = json.RawMessage(toolCalls)
			}
			return item, nil
		}

		// Compatibilidade com histórico legado: a timeline também aceita uma
		// única tool call persistida como objeto e a trata como array unitário.
		var single map[string]json.RawMessage
		if err := json.Unmarshal([]byte(toolCalls), &single); err != nil {
			return getMessagePayload{}, fmt.Errorf("invalid tool_calls JSON")
		}
		normalized, err := json.Marshal([]map[string]json.RawMessage{single})
		if err != nil {
			return getMessagePayload{}, fmt.Errorf("normalize legacy tool_calls: %w", err)
		}
		item.ToolCalls = normalized
	}
	return item, nil
}

func toolArgumentError(format string, args ...any) tools.ToolResult {
	return tools.ToolResult{Content: fmt.Sprintf(format, args...), IsError: true}
}
