// Package subagent expõe a builtin tool `subagent` (AEP-0068), que delega
// tarefas a sub-agentes executando em sub-conversas próprias, persistidas e
// visíveis. A execução real é feita pelo internal/subagent.Manager, que reusa
// o pipeline oficial de envio (SendMessageUseCase) — esta tool é só a
// superfície exposta ao LLM.
package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/subagent"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// Runner é a porta de execução de sub-agentes. *subagent.Manager a satisfaz.
type Runner interface {
	Run(ctx context.Context, p subagent.RunParams) (subagent.RunResult, error)
	Status(ctx context.Context, conversationID, runID string) (subagent.StatusResult, error)
	Cancel(ctx context.Context, conversationID, runID string) (subagent.CancelResult, error)
}

// RunnerProvider resolve o Runner de forma tardia (lazy), pois a tool é
// registrada antes do Manager existir no wiring do app.
type RunnerProvider func() Runner

// Tool implementa tools.Tool para a builtin `subagent`.
type Tool struct {
	provider RunnerProvider
}

// NewWithProvider cria a tool com um provider lazy do Runner.
func NewWithProvider(provider RunnerProvider) *Tool {
	return &Tool{provider: provider}
}

func (t *Tool) Name() string { return "subagent" }

func (t *Tool) Description() string {
	return "Delegate work to a sub-agent running in its own persisted sub-conversation. Modes (driven by parameters): (1) send — provide 'prompt' to start a sub-agent. Without 'conversation_id' it creates a new sub-conversation; with 'conversation_id' it resumes an existing one, preserving its full context (like resuming by agent id). Add 'clear':true to reset that sub-conversation's history before sending. With 'background':false (default) it waits and returns the result; with 'background':true it returns a handle (conversation_id/run_id) immediately and the result is delivered back into this conversation when it completes. (2) status — omit 'prompt' and pass 'conversation_id' (optionally 'run_id') to query the current state of a run. (3) cancel — pass 'cancel':true with 'conversation_id' (optionally 'run_id') to cancel a running sub-agent. Optional 'profile' (defaults to the parent's profile), 'title' and 'model'. 'cancel' is mutually exclusive with 'prompt' and 'clear'."
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "Task/message for the sub-agent. Present = send/continue; omitted (with conversation_id) = status query."
			},
			"background": {
				"type": "boolean",
				"description": "If true, return a handle immediately and deliver the result back to this conversation when done. Default false (wait inline)."
			},
			"conversation_id": {
				"type": "string",
				"description": "Sub-conversation handle. Omit to create a new sub-conversation; provide to resume an existing one (preserving context) or to query status/cancel. Required for status/cancel/clear."
			},
			"clear": {
				"type": "boolean",
				"description": "Reset the sub-conversation history before sending. Requires conversation_id and prompt (clear is always reset + send). Mutually exclusive with cancel."
			},
			"run_id": {
				"type": "string",
				"description": "Specific run (turn) of a sub-conversation for status/cancel. If omitted, acts on the most recent run of conversation_id. Requires conversation_id."
			},
			"cancel": {
				"type": "boolean",
				"description": "Cancel a running sub-agent. Requires conversation_id. Mutually exclusive with prompt."
			},
			"profile": {
				"type": "string",
				"description": "Slug of the interaction profile for the sub-agent (model, behavior, enabled tools). Defaults to the parent's profile."
			},
			"title": {
				"type": "string",
				"description": "Optional title for the sub-conversation."
			},
			"model": {
				"type": "string",
				"description": "Optional model override for this run (overrides the model derived from the profile)."
			}
		}
	}`)
}

type subagentArgs struct {
	Prompt         string `json:"prompt"`
	Background     bool   `json:"background,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Cancel         bool   `json:"cancel,omitempty"`
	Clear          bool   `json:"clear,omitempty"`
	Profile        string `json:"profile,omitempty"`
	Title          string `json:"title,omitempty"`
	Model          string `json:"model,omitempty"`
}

func (t *Tool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a subagentArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Sprintf("argumentos inválidos: %v", err)), nil
		}
	}
	prompt := strings.TrimSpace(a.Prompt)
	conversationID := strings.TrimSpace(a.ConversationID)
	runID := strings.TrimSpace(a.RunID)

	// Validações de combinação (AEP-0068).
	if a.Cancel && prompt != "" {
		return errResult("'cancel' e 'prompt' são mutuamente exclusivos"), nil
	}
	if a.Cancel && a.Clear {
		return errResult("'cancel' e 'clear' são mutuamente exclusivos"), nil
	}
	if a.Cancel && conversationID == "" {
		return errResult("'cancel' requer 'conversation_id'"), nil
	}
	if a.Clear && conversationID == "" {
		return errResult("'clear' requer 'conversation_id' (nada a resetar)"), nil
	}
	if a.Clear && prompt == "" {
		return errResult("'clear' requer 'prompt': clear é sempre reset + envio na mesma chamada"), nil
	}
	if runID != "" && conversationID == "" {
		return errResult("'run_id' requer 'conversation_id'"), nil
	}
	if !a.Cancel && prompt == "" && conversationID == "" && runID == "" {
		return errResult("nada a fazer: informe 'prompt' (enviar), 'conversation_id' (status) ou 'cancel'"), nil
	}

	if t.provider == nil {
		return errResult("sub-agentes indisponíveis: runner não configurado"), nil
	}
	runner := t.provider()
	if runner == nil {
		return errResult("sub-agentes indisponíveis no momento"), nil
	}

	switch {
	case a.Cancel:
		res, err := runner.Cancel(ctx, conversationID, runID)
		if err != nil {
			return errResult(fmt.Sprintf("erro ao cancelar sub-agente: %v", err)), nil
		}
		return jsonResult(res, false, map[string]any{"conversation_id": res.ConversationID, "run_id": res.RunID, "status": res.Status, "cancelled": res.Cancelled}), nil

	case prompt != "":
		// Enviar: cria sub-conversa nova (sem conversation_id) ou continua uma
		// existente (resume, com conversation_id), opcionalmente resetando antes
		// (clear). A continuidade de contexto é garantida pelo pipeline oficial,
		// que carrega o histórico da conversa pelo conversation_id.
		inv, _ := invocationctx.Get(ctx)
		profile := strings.TrimSpace(a.Profile)
		if profile == "" {
			profile = inv.ProfileSlug
		}
		res, err := runner.Run(ctx, subagent.RunParams{
			ParentConversationID: inv.ConversationID,
			ParentTurnID:         inv.TurnID,
			ParentInvocationID:   toolinvocations.CurrentInvocationID(ctx),
			ConversationID:       conversationID,
			Clear:                a.Clear,
			Prompt:               prompt,
			ProfileSlug:          profile,
			Model:                strings.TrimSpace(a.Model),
			Title:                strings.TrimSpace(a.Title),
			Background:           a.Background,
		})
		if err != nil {
			return errResult(fmt.Sprintf("erro ao iniciar sub-agente: %v", err)), nil
		}
		isError := res.Status == subagent.StatusFailed ||
			res.Status == subagent.StatusTimedOut ||
			res.Status == subagent.StatusCancelled
		return jsonResult(res, isError, map[string]any{"conversation_id": res.ConversationID, "run_id": res.RunID, "status": res.Status}), nil

	default:
		// Status (prompt omitido).
		res, err := runner.Status(ctx, conversationID, runID)
		if err != nil {
			return errResult(fmt.Sprintf("erro ao consultar status do sub-agente: %v", err)), nil
		}
		return jsonResult(res, false, map[string]any{"conversation_id": res.ConversationID, "run_id": res.RunID, "status": res.Status}), nil
	}
}

func errResult(msg string) tools.ToolResult {
	return tools.ToolResult{Content: msg, IsError: true}
}

func jsonResult(v any, isError bool, metadata map[string]any) tools.ToolResult {
	payload, err := json.Marshal(v)
	if err != nil {
		return errResult(fmt.Sprintf("erro ao serializar resultado do sub-agente: %v", err))
	}
	return tools.ToolResult{Content: string(payload), IsError: isError, Metadata: metadata}
}
