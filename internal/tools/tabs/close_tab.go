package tabs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

type closeTabArgs struct {
	Name  *string `json:"name,omitempty"`
	Index *int    `json:"index,omitempty"`
}

// CloseTabTool permite ao LLM fechar uma aba de conversa.
// Suporta fechar a aba atual, por nome ou por índice (1-based).
type CloseTabTool struct {
	mgr TabManager
}

func NewCloseTab(mgr TabManager) *CloseTabTool {
	return &CloseTabTool{mgr: mgr}
}

func (t *CloseTabTool) Name() string {
	return "close_tab"
}

func (t *CloseTabTool) Description() string {
	return "Closes a conversation tab. With no arguments, closes the current tab. Use 'name' to close a tab by its title, or 'index' (1-based) to close by position. Only one of name/index should be provided."
}

func (t *CloseTabTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Título da aba a ser fechada. Busca parcial case-insensitive."
			},
			"index": {
				"type": "integer",
				"description": "Índice da aba a ser fechada (1-based: 1 = primeira aba, 2 = segunda, etc.)"
			}
		},
		"additionalProperties": false
	}`)
}

func (t *CloseTabTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params closeTabArgs
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

func (t *CloseTabTool) closeCurrent() (tools.ToolResult, error) {
	activeTab, err := t.mgr.GetActiveTab()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao obter aba ativa: %v", err), IsError: true}, nil
	}

	if err := t.mgr.CloseTab(activeTab.ID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao fechar aba: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Aba '%s' fechada com sucesso", activeTab.Title),
		Metadata: map[string]any{
			"tab_id":    activeTab.ID,
			"tab_title": activeTab.Title,
		},
	}, nil
}

func (t *CloseTabTool) closeByName(name string) (tools.ToolResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return tools.ToolResult{Content: "O nome da aba não pode ser vazio", IsError: true}, nil
	}

	allTabs, err := t.mgr.GetAllTabs()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao listar abas: %v", err), IsError: true}, nil
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
			Content: fmt.Sprintf("Nenhuma aba encontrada com o nome '%s'. Abas disponíveis:\n%s", name, strings.Join(available, "\n")),
			IsError: true,
		}, nil
	}

	if len(matches) > 1 {
		titles := make([]string, len(matches))
		for i, m := range matches {
			titles[i] = fmt.Sprintf("- %s (id: %d)", m.Title, m.ID)
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Múltiplas abas correspondem a '%s':\n%s\nUse um nome mais específico ou feche por índice.", name, strings.Join(titles, "\n")),
			IsError: true,
		}, nil
	}

	tab := matches[0]
	if err := t.mgr.CloseTab(tab.ID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao fechar aba: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Aba '%s' fechada com sucesso", tab.Title),
		Metadata: map[string]any{
			"tab_id":    tab.ID,
			"tab_title": tab.Title,
		},
	}, nil
}

func (t *CloseTabTool) closeByIndex(index int) (tools.ToolResult, error) {
	allTabs, err := t.mgr.GetAllTabs()
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao listar abas: %v", err), IsError: true}, nil
	}

	if index < 1 || index > len(allTabs) {
		return tools.ToolResult{
			Content: fmt.Sprintf("Índice %d inválido. Há %d aba(s) abertas (use 1 a %d).", index, len(allTabs), len(allTabs)),
			IsError: true,
		}, nil
	}

	tab := allTabs[index-1]
	if err := t.mgr.CloseTab(tab.ID); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao fechar aba: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Aba #%d '%s' fechada com sucesso", index, tab.Title),
		Metadata: map[string]any{
			"tab_id":    tab.ID,
			"tab_title": tab.Title,
			"tab_index": index,
		},
	}, nil
}
