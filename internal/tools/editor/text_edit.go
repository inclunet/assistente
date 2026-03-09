package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/questionnaire"
	"assistente/internal/tools"
)

type textEditArgs struct {
	Format      string `json:"format,omitempty"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Notes       string `json:"notes,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type QuestionnaireRequester interface {
	RequestQuestionnaire(ctx context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error)
}

// TextEditTool é uma tool pensada para uso dentro do editor: ela NÃO mexe em filesystem.
// Ela apenas solicita confirmação do usuário (via questionário) e retorna um patch estruturado
// para substituir o texto selecionado.
type TextEditTool struct {
	mgr QuestionnaireRequester
}

func NewTextEdit(mgr QuestionnaireRequester) *TextEditTool {
	return &TextEditTool{mgr: mgr}
}

func (t *TextEditTool) Name() string {
	return "text_edit"
}

func (t *TextEditTool) Description() string {
	return "Proposes a text replacement for the currently selected excerpt in the editor and asks the user to confirm (Apply/Reject) via an in-app questionnaire. Returns a structured patch when approved. Use this in editor contexts instead of filesystem tools."
}

func (t *TextEditTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"format": {
				"type": "string",
				"enum": ["markdown", "plain"],
				"description": "Output format of replacement text"
			},
			"original": {
				"type": "string",
				"description": "Original selected text (for review/confirmation). Can be empty to indicate insertion at cursor (no selection)."
			},
			"replacement": {
				"type": "string",
				"description": "Replacement text to apply"
			},
			"notes": {
				"type": "string",
				"description": "Optional notes to show to the user"
			},
			"title": {
				"type": "string",
				"description": "Optional questionnaire title"
			},
			"description": {
				"type": "string",
				"description": "Optional questionnaire description"
			}
		},
		"required": ["original", "replacement"]
	}`)
}

func (t *TextEditTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params textEditArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	format := strings.TrimSpace(params.Format)
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "plain" {
		return tools.ToolResult{Content: "Parâmetro 'format' inválido (use markdown ou plain)", IsError: true}, nil
	}

	original := params.Original
	replacement := params.Replacement
	// original pode ser vazio para inserção no cursor (sem seleção).
	if replacement == "" {
		return tools.ToolResult{Content: "Parâmetro 'replacement' é obrigatório", IsError: true}, nil
	}

	if t.mgr == nil {
		return tools.ToolResult{Content: "Gerenciador de questionários não está disponível", IsError: true}, nil
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = "Aplicar alteração no editor"
	}
	desc := strings.TrimSpace(params.Description)
	if desc == "" {
		if strings.TrimSpace(original) == "" {
			desc = "Revise o conteúdo e clique em Aplicar para inserir no cursor."
		} else {
			desc = "Revise o antes/depois e clique em Aplicar para confirmar."
		}
	}

	beforeContent := original
	if strings.TrimSpace(beforeContent) == "" {
		beforeContent = "∅ (sem seleção — inserção no cursor)"
	}

	questions := []questionnaire.Question{
		{ID: "before", Type: "readonly_code", Prompt: "Antes", Content: beforeContent},
		{ID: "after", Type: "readonly_code", Prompt: "Depois", Content: replacement},
	}
	if strings.TrimSpace(params.Notes) != "" {
		questions = append(questions, questionnaire.Question{ID: "notes", Type: "readonly_code", Prompt: "Notas", Content: strings.TrimSpace(params.Notes)})
	}

	resp, err := t.mgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       title,
		Description: desc,
		Questions:   questions,
		AllowCancel: true,
		SubmitLabel: "Aplicar",
		CancelLabel: "Rejeitar",
	})
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err), IsError: true}, nil
	}
	if resp.Cancelled {
		return tools.ToolResult{Content: "Alteração rejeitada pelo usuário", IsError: true}, nil
	}

	patch := map[string]any{
		"v":           1,
		"op":          "replace_selection",
		"format":      format,
		"replacement": replacement,
	}

	payload, err := json.Marshal(patch)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao serializar patch: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{Content: string(payload)}, nil
}
