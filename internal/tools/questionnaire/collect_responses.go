package questionnaire

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"assistente/internal/questionnaire"
	"assistente/internal/tools"
)

// collectResponsesArgs são os argumentos da tool collect_responses.
type collectResponsesArgs struct {
	Title       string                   `json:"title,omitempty"`
	Description string                   `json:"description,omitempty"`
	Questions   []questionnaire.Question `json:"questions"`
	AllowCancel *bool                    `json:"allow_cancel,omitempty"`
	SubmitLabel string                   `json:"submit_label,omitempty"`
	CancelLabel string                   `json:"cancel_label,omitempty"`
}

// QuestionnaireRequester define o contrato para solicitar um questionário.
type QuestionnaireRequester interface {
	RequestQuestionnaire(ctx context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error)
}

// CollectResponsesTool solicita respostas estruturadas via UI.
type CollectResponsesTool struct {
	mgr QuestionnaireRequester
}

// NewCollectResponses cria uma nova instância da tool.
func NewCollectResponses(mgr QuestionnaireRequester) *CollectResponsesTool {
	return &CollectResponsesTool{mgr: mgr}
}

func (t *CollectResponsesTool) Name() string {
	return "collect_responses"
}

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *CollectResponsesTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "questionnaire", Class: "app_tool", Package: "basic", Risk: "read"}
}

func (t *CollectResponsesTool) Description() string {
	return "Collects structured answers via an in-app questionnaire. Prefer this whenever you need to ask the user questions, clarify requirements, compare alternatives, run quizzes/tests/simulations, or validate a plan before acting. Supports multiple questions and formats (text, long_text, number, boolean, single_choice, multiple_choice, scale, date); returns machine-readable answers."
}

func (t *CollectResponsesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {"type": "string", "description": "Título do questionário (ex: 'Dúvidas para o planejamento')"},
			"description": {"type": "string", "description": "Texto de apoio/introdução para contextualizar as perguntas"},
			"allow_cancel": {"type": "boolean", "description": "Se o usuário pode cancelar o questionário (padrão: true)"},
			"submit_label": {"type": "string", "description": "Texto do botão de envio"},
			"cancel_label": {"type": "string", "description": "Texto do botão de cancelar"},
			"questions": {
				"type": "array",
				"minItems": 1,
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string", "description": "Identificador único da pergunta"},
						"type": {"type": "string", "enum": ["text", "long_text", "number", "boolean", "single_choice", "multiple_choice", "scale", "date", "readonly_code"], "description": "Tipo da pergunta (texto curto/long, número, sim/não, escolha, escala, data, readonly_code)"},
						"prompt": {"type": "string", "description": "Enunciado/pergunta principal"},
						"description": {"type": "string", "description": "Descrição adicional/explicação"},
						"content": {"type": "string", "description": "Conteúdo somente-leitura (para readonly_code)"},
						"required": {"type": "boolean", "description": "Se a resposta é obrigatória"},
						"options": {"type": "array", "items": {"type": "string"}, "description": "Opções para perguntas de escolha (single/multiple_choice)"},
						"min": {"type": "number", "description": "Valor mínimo (number)"},
						"max": {"type": "number", "description": "Valor máximo (number)"},
						"step": {"type": "number", "description": "Passo (number)"},
						"placeholder": {"type": "string", "description": "Placeholder para text"},
						"default": {"description": "Valor inicial da resposta"}
					},
					"required": ["id", "type", "prompt"]
				}
			}
		},
		"required": ["questions"]
	}`)
}

func (t *CollectResponsesTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params collectResponsesArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao parsear argumentos: %v", err),
			IsError: true,
		}, nil
	}

	if len(params.Questions) == 0 {
		return tools.ToolResult{
			Content: "É necessário informar ao menos uma pergunta em 'questions'",
			IsError: true,
		}, nil
	}

	if t.mgr == nil {
		return tools.ToolResult{
			Content: "Gerenciador de questionários não está disponível",
			IsError: true,
		}, nil
	}

	if err := validateQuestions(params.Questions); err != nil {
		return tools.ToolResult{
			Content: err.Error(),
			IsError: true,
		}, nil
	}

	allowCancel := true
	if params.AllowCancel != nil {
		allowCancel = *params.AllowCancel
	}

	resp, err := t.mgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       strings.TrimSpace(params.Title),
		Description: strings.TrimSpace(params.Description),
		Questions:   params.Questions,
		AllowCancel: allowCancel,
		SubmitLabel: strings.TrimSpace(params.SubmitLabel),
		CancelLabel: strings.TrimSpace(params.CancelLabel),
	})
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao coletar respostas: %v", err),
			IsError: true,
		}, nil
	}

	if resp.Cancelled {
		return tools.ToolResult{
			Content: "Questionário cancelado pelo usuário",
			IsError: true,
		}, nil
	}

	payload := map[string]any{
		"id":      resp.ID,
		"answers": resp.Answers,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao serializar respostas: %v", err),
			IsError: true,
		}, nil
	}

	return tools.ToolResult{Content: string(payloadJSON)}, nil
}

func validateQuestions(questions []questionnaire.Question) error {
	allowedTypes := map[string]bool{
		"text":            true,
		"long_text":       true,
		"number":          true,
		"boolean":         true,
		"single_choice":   true,
		"multiple_choice": true,
		"scale":           true,
		"date":            true,
		"readonly_code":   true,
	}

	seen := make(map[string]struct{})
	var duplicates []string

	for _, q := range questions {
		id := strings.TrimSpace(q.ID)
		if id == "" {
			return fmt.Errorf("pergunta sem id: todo item em 'questions' precisa de id")
		}
		if _, ok := seen[id]; ok {
			duplicates = append(duplicates, id)
		} else {
			seen[id] = struct{}{}
		}

		qType := strings.TrimSpace(q.Type)
		if !allowedTypes[qType] {
			return fmt.Errorf("tipo de pergunta inválido '%s' (id: %s)", q.Type, id)
		}
		if strings.TrimSpace(q.Prompt) == "" {
			return fmt.Errorf("pergunta sem prompt (id: %s)", id)
		}
		if qType == "single_choice" || qType == "multiple_choice" {
			if len(q.Options) == 0 {
				return fmt.Errorf("pergunta '%s' precisa de opções em 'options'", id)
			}
		}
		if qType == "readonly_code" {
			// Não requer resposta; apenas exibe conteúdo.
			continue
		}
		if qType == "number" && q.Min != nil && q.Max != nil && *q.Min > *q.Max {
			return fmt.Errorf("pergunta '%s' tem min maior que max", id)
		}
	}

	if len(duplicates) > 0 {
		sort.Strings(duplicates)
		return fmt.Errorf("ids de perguntas duplicados: %s", strings.Join(duplicates, ", "))
	}

	return nil
}
