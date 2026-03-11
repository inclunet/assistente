package tabs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

type setTabTitleArgs struct {
	Title string `json:"title"`
}

// SetTabTitleTool permite ao LLM definir o título da aba/conversa ativa.
type SetTabTitleTool struct {
	mgr TabManager
}

func NewSetTabTitle(mgr TabManager) *SetTabTitleTool {
	return &SetTabTitleTool{mgr: mgr}
}

func (t *SetTabTitleTool) Name() string {
	return "set_tab_title"
}

func (t *SetTabTitleTool) Description() string {
	return "Sets or updates the title of the current conversation tab. Useful for organizing conversations with meaningful names. Can be invoked by skills or workflows to label sessions."
}

func (t *SetTabTitleTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {
				"type": "string",
				"description": "Novo título para a aba da conversa atual"
			}
		},
		"required": ["title"],
		"additionalProperties": false
	}`)
}

func (t *SetTabTitleTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params setTabTitleArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return tools.ToolResult{Content: "O título não pode ser vazio", IsError: true}, nil
	}

	activeTab, err := t.mgr.GetActiveTab()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao obter aba ativa: %v", err), IsError: true}, nil
	}

	if err := t.mgr.UpdateTabTitle(activeTab.ID, title); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao atualizar título: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Título da aba atualizado para: %s", title),
		Metadata: map[string]any{
			"tab_id":    activeTab.ID,
			"new_title": title,
		},
	}, nil
}
