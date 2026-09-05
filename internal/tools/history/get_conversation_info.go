package history

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

type getConversationInfoArgs struct {
	ConversationID  string `json:"conversation_id,omitempty"`
	IncludeMessages bool   `json:"include_messages,omitempty"`
	MessageLimit    int    `json:"message_limit,omitempty"`
}

// GetConversationInfoTool expõe metadados da conversa atual (ou de uma conversa
// específica do usuário) para o agente. O caso de uso principal é descobrir o
// conversation_id da conversa em andamento — retornado de forma proeminente —
// para então vincular tasks/tasklists a ela (tools `task`/`task_list`), além de
// listar tasks/tasklists já vinculadas. Por padrão, sem conversation_id, usa a
// conversa do turno corrente (carimbada no InvocationContext).
type GetConversationInfoTool struct{}

// NewGetConversationInfo cria a tool. Não requer dependências: usa as funções
// database.*WithContext, que já são fail-closed por user_id no contexto.
func NewGetConversationInfo() *GetConversationInfoTool {
	return &GetConversationInfoTool{}
}

func (t *GetConversationInfoTool) Name() string { return "get_conversation_info" }

func (t *GetConversationInfoTool) Description() string {
	return "Returns metadata and the rolling summary for the current conversation, or for one specified by ID, including its conversation_id, title, channel, message count, and linked tasks or task lists. Use it to understand a conversation at low cost or obtain its ID for links; request a small recent-message window only when the summary is insufficient. Do not use it to search for matching discussions (use search_conversations) or rehydrate complete messages by ID (use get_messages). It inspects exactly one conversation per call; including messages increases output cost and is capped at the 50 most recent root messages, not paginated history."
}

func (t *GetConversationInfoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"conversation_id": {
				"type": "string",
				"description": "Exact conversation ID to inspect. Omit to use the current invocation's conversation; only one conversation is returned per call."
			},
			"include_messages": {
				"type": "boolean",
				"description": "When true, also returns a recent root-message window. Default false; keep false when metadata or the rolling summary is enough because message content increases output cost."
			},
			"message_limit": {
				"type": "integer",
				"description": "Maximum recent root messages when include_messages is true (default 20, maximum 50). This is a cap on the newest messages, not a pagination cursor."
			}
		},
		"additionalProperties": false
	}`)
}

func (t *GetConversationInfoTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params getConversationInfoArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
		}
	}

	convID := strings.TrimSpace(params.ConversationID)
	if convID == "" {
		if inv, ok := invocationctx.Get(ctx); ok {
			convID = strings.TrimSpace(inv.ConversationID)
		}
	}
	if convID == "" {
		return tools.ToolResult{
			Content: "No conversation id available: this tool must run inside a conversation, or you must pass conversation_id explicitly.",
			IsError: true,
		}, nil
	}

	conv, err := database.GetConversationInfoWithContext(ctx, convID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Conversation not found (id=%s): %v", convID, err), IsError: true}, nil
	}

	response := map[string]any{
		"conversation_id": conv.ID,
		"title":           conv.Title,
		"message_count":   conv.MessageCount,
	}
	if conv.Channel != "" {
		response["channel"] = conv.Channel
	}
	if conv.ContactID != "" {
		response["contact_id"] = conv.ContactID
	}
	if conv.Kind != "" {
		response["kind"] = conv.Kind
	}
	if conv.ParentConversationID != "" {
		response["parent_conversation_id"] = conv.ParentConversationID
	}

	if summary, _, sErr := database.GetConversationSummaryWithContext(ctx, convID); sErr == nil && strings.TrimSpace(summary) != "" {
		response["summary"] = summary
	}

	// Tasks/tasklists já vinculadas a esta conversa (best-effort).
	if lists, lErr := database.GetTaskListsByConversationIDWithContext(ctx, convID); lErr == nil && len(lists) > 0 {
		linked := make([]map[string]any, 0, len(lists))
		for _, l := range lists {
			item := map[string]any{"id": l.ID, "title": l.Title}
			if l.Slug != "" {
				item["slug"] = l.Slug
			}
			linked = append(linked, item)
		}
		response["linked_task_lists"] = linked
	}
	if tasksLinked, tErr := database.GetTasksByConversationIDWithContext(ctx, convID); tErr == nil && len(tasksLinked) > 0 {
		linked := make([]map[string]any, 0, len(tasksLinked))
		for _, tk := range tasksLinked {
			item := map[string]any{
				"id":           tk.ID,
				"title":        tk.Title,
				"task_list_id": tk.TaskListID,
				"status_id":    tk.StatusID,
			}
			if tk.Code != "" {
				item["code"] = tk.Code
			}
			linked = append(linked, item)
		}
		response["linked_tasks"] = linked
	}

	if params.IncludeMessages {
		limit := params.MessageLimit
		if limit <= 0 {
			limit = 20
		}
		if limit > 50 {
			limit = 50
		}
		msgs, mErr := database.GetRecentRootMessagesWithContext(ctx, convID, limit)
		if mErr == nil && len(msgs) > 0 {
			items := make([]map[string]any, 0, len(msgs))
			for _, m := range msgs {
				items = append(items, map[string]any{
					"id":      m.ID,
					"role":    m.Role,
					"content": m.Content,
					"date":    m.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			response["recent_messages"] = items
		}
	}

	resultJSON, _ := json.Marshal(response)
	return tools.ToolResult{
		Content:  string(resultJSON),
		Metadata: map[string]any{"conversation_id": convID},
	}, nil
}
