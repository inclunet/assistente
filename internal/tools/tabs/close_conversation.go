package tabs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

type closeConversationArgs struct {
	Name  *string `json:"name,omitempty"`
	Index *int    `json:"index,omitempty"`
}

// CloseConversationTool permite ao LLM fechar uma conversa.
// Suporta fechar a conversa atual, por nome ou por índice (1-based).
type CloseConversationTool struct {
	mgr TabManager
}

func NewCloseConversation(mgr TabManager) *CloseConversationTool {
	return &CloseConversationTool{mgr: mgr}
}

func (t *CloseConversationTool) Name() string {
	return "close_conversation"
}

func (t *CloseConversationTool) Description() string {
	return "Closes a conversation. With no arguments, closes the current conversation. Use 'name' to close by title (partial, case-insensitive), or 'index' (1-based) to close by position. Only one of name/index should be provided."
}

func (t *CloseConversationTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Título da conversa a ser fechada. Busca parcial case-insensitive."
			},
			"index": {
				"type": "integer",
				"description": "Índice da conversa a ser fechada (1-based: 1 = primeira, 2 = segunda, etc.)"
			}
		},
		"additionalProperties": false
	}`)
}

func (t *CloseConversationTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params closeConversationArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if params.Name != nil && params.Index != nil {
		return tools.ToolResult{Content: "Forneça apenas 'name' ou 'index', não ambos", IsError: true}, nil
	}

	if params.Name != nil {
		return t.closeByName(*params.Name)
	}

	if params.Index != nil {
		return t.closeByIndex(*params.Index)
	}

	return t.closeCurrent()
}

func (t *CloseConversationTool) closeCurrent() (tools.ToolResult, error) {
	activeTab, err := t.mgr.GetActiveTab()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao obter conversa ativa: %v", err), IsError: true}, nil
	}

	if err := t.mgr.CloseTab(activeTab.ID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao fechar conversa: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Conversa '%s' fechada com sucesso", activeTab.Title),
		Metadata: map[string]any{
			"conversation_id": activeTab.ID,
			"title":           activeTab.Title,
		},
	}, nil
}

func (t *CloseConversationTool) closeByName(name string) (tools.ToolResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return tools.ToolResult{Content: "O nome da conversa não pode ser vazio", IsError: true}, nil
	}

	allTabs, err := t.mgr.GetAllTabs()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao listar conversas: %v", err), IsError: true}, nil
	}

	nameLower := strings.ToLower(name)
	var matches []TabInfo
	for _, tab := range allTabs {
		if strings.ToLower(tab.Title) == nameLower {
			matches = append(matches, tab)
		}
	}

	if len(matches) == 0 {
		for _, tab := range allTabs {
			if strings.Contains(strings.ToLower(tab.Title), nameLower) {
				matches = append(matches, tab)
			}
		}
	}

	if len(matches) == 0 {
		available := make([]string, len(allTabs))
		for i, tab := range allTabs {
			available[i] = fmt.Sprintf("%d. %s", i+1, tab.Title)
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Nenhuma conversa encontrada com o nome '%s'. Conversas disponíveis:\n%s", name, strings.Join(available, "\n")),
			IsError: true,
		}, nil
	}

	if len(matches) > 1 {
		titles := make([]string, len(matches))
		for i, m := range matches {
			titles[i] = fmt.Sprintf("- %s (id: %d)", m.Title, m.ID)
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Múltiplas conversas correspondem a '%s':\n%s\nUse um nome mais específico ou feche por índice.", name, strings.Join(titles, "\n")),
			IsError: true,
		}, nil
	}

	tab := matches[0]
	if err := t.mgr.CloseTab(tab.ID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao fechar conversa: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Conversa '%s' fechada com sucesso", tab.Title),
		Metadata: map[string]any{
			"conversation_id": tab.ID,
			"title":           tab.Title,
		},
	}, nil
}

func (t *CloseConversationTool) closeByIndex(index int) (tools.ToolResult, error) {
	allTabs, err := t.mgr.GetAllTabs()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao listar conversas: %v", err), IsError: true}, nil
	}

	if index < 1 || index > len(allTabs) {
		return tools.ToolResult{
			Content: fmt.Sprintf("Índice %d inválido. Há %d conversa(s) aberta(s) (use 1 a %d).", index, len(allTabs), len(allTabs)),
			IsError: true,
		}, nil
	}

	tab := allTabs[index-1]
	if err := t.mgr.CloseTab(tab.ID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao fechar conversa: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Conversa #%d '%s' fechada com sucesso", index, tab.Title),
		Metadata: map[string]any{
			"conversation_id": tab.ID,
			"title":           tab.Title,
			"index":           index,
		},
	}, nil
}
