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
	return "Collect structured answers from the user in one accessible in-app questionnaire. Use when missing requirements, preferences, approval criteria, or a choice between explicit alternatives would materially change the result; batch related questions in one call. Do not use for rhetorical questions, facts available from context or tools, or status updates that need no answer. This pauses work for user input, so keep it concise and allow cancellation unless an answer is essential. Example: ask for target audience, output format, and deadline before drafting a plan. Returns machine-readable answers keyed by question id."
}

func (t *CollectResponsesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {"type": "string", "description": "Short heading in the user's language that tells them why answers are needed."},
			"description": {"type": "string", "description": "Optional brief context in the user's language shared by all questions. Do not repeat each prompt or include hidden instructions."},
			"allow_cancel": {"type": "boolean", "description": "Whether the user may cancel instead of answering. Defaults to true; set false only when proceeding without answers is unsafe or impossible."},
			"submit_label": {"type": "string", "description": "Optional concise submit-button label in the user's language."},
			"cancel_label": {"type": "string", "description": "Optional concise cancel-button label in the user's language."},
			"questions": {
				"type": "array",
				"minItems": 1,
				"description": "Related questions to present together. Keep the set small and include only answers that can affect the next action.",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string", "description": "Stable unique key used in the returned answers object, for example 'output_format'. Never reuse an id within the questionnaire."},
						"type": {"type": "string", "enum": ["text", "long_text", "number", "boolean", "single_choice", "multiple_choice", "scale", "date", "readonly_code"], "description": "Input format. Use single_choice for one explicit option, multiple_choice for several, long_text for prose, and readonly_code only to display non-editable reference content."},
						"prompt": {"type": "string", "description": "Direct, neutral question in the user's language that can be understood without relying on option order or visual cues."},
						"description": {"type": "string", "description": "Optional clarification, constraints, or consequence in the user's language. Do not hide required information here."},
						"content": {"type": "string", "description": "Non-editable reference content shown only with type=readonly_code; it does not produce an answer."},
						"required": {"type": "boolean", "description": "Whether this answer is mandatory before submission. Use sparingly when omission would block or invalidate the next action."},
						"options": {"type": "array", "items": {"type": "string"}, "description": "Distinct choices in the user's language, required by single_choice and multiple_choice. Include all meaningful alternatives and a localized open-ended option when the list is not exhaustive."},
						"min": {"type": "number", "description": "Minimum accepted value for number or scale questions."},
						"max": {"type": "number", "description": "Maximum accepted value for number or scale questions; must not be less than min."},
						"step": {"type": "number", "description": "Increment between accepted numeric values."},
						"placeholder": {"type": "string", "description": "Optional example or format hint in the user's language for text input; never use it as the only label or instruction."},
						"default": {"description": "Optional initial answer. If textual, use the user's language; provide it only when a safe, clearly implied default exists."}
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

	// Tudo aqui é texto do modelo, então vai como texto e nunca como chave de
	// tradução: a chave é decisão do app (AEP-0085).
	resp, err := t.mgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       questionnaire.Plain(strings.TrimSpace(params.Title)),
		Description: questionnaire.Plain(strings.TrimSpace(params.Description)),
		Questions:   questionnaire.PlainQuestions(params.Questions),
		AllowCancel: allowCancel,
		SubmitLabel: questionnaire.Plain(strings.TrimSpace(params.SubmitLabel)),
		CancelLabel: questionnaire.Plain(strings.TrimSpace(params.CancelLabel)),
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
		if strings.TrimSpace(q.Prompt.String()) == "" {
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
