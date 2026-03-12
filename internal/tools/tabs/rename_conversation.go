package tabs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

type renameConversationArgs struct {
	Title string `json:"title"`
}

// RenameConversationTool permite ao LLM renomear a conversa ativa.
type RenameConversationTool struct {
	mgr TabManager
}

func NewRenameConversation(mgr TabManager) *RenameConversationTool {
	return &RenameConversationTool{mgr: mgr}
}

func (t *RenameConversationTool) Name() string {
	return "rename_conversation"
}

func (t *RenameConversationTool) Description() string {
	return "Renames the current conversation. Useful for organizing conversations with meaningful names. Can be invoked by skills or workflows to label sessions."
}

func (t *RenameConversationTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {
				"type": "string",
				"description": "Novo título para a conversa atual"
			}
		},
		"required": ["title"],
		"additionalProperties": false
	}`)
}

func (t *RenameConversationTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params renameConversationArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "O título não pode ser vazio", IsError: true}, nil
	}

	activeTab, err := t.mgr.GetActiveTab()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao obter conversa ativa: %v", err), IsError: true}, nil
	}

	if err := t.mgr.UpdateTabTitle(activeTab.ID, title); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao renomear conversa: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Conversa renomeada para: %s", title),
		Metadata: map[string]any{
			"tab_id":    activeTab.ID,
			"new_title": title,
		},
	}, nil
}
