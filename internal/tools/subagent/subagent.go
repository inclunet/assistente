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

	"assistente/internal/eventctx"
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
	return "Delegate work to a sub-agent running in its own persisted sub-conversation. Modes (driven by parameters): (1) send — provide 'prompt' to start a sub-agent; with 'background':false (default) it waits and returns the result; with 'background':true it returns a handle (conversation_id/run_id) immediately and the result is delivered back into this conversation when it completes. (2) status — omit 'prompt' and pass 'conversation_id' (optionally 'run_id') to query the current state of a run. (3) cancel — pass 'cancel':true with 'conversation_id' (optionally 'run_id') to cancel a running sub-agent. Optional 'profile' (defaults to the parent's profile), 'title' and 'model'. 'cancel' is mutually exclusive with 'prompt'."
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
				"description": "Sub-conversation handle. Required for status/cancel. (Reusing an existing sub-conversation to send a new prompt is added in a later phase.)"
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
		},
		"additionalProperties": false
	}`)
}

type subagentArgs struct {
	Prompt         string `json:"prompt"`
	Background     bool   `json:"background,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Cancel         bool   `json:"cancel,omitempty"`
	Profile        string `json:"profile,omitempty"`
	Title          string `json:"title,omitempty"`
	Model          string `json:"model,omitempty"`
}

// originAllowsParentless informa se a origem da chamada permite rodar o
// sub-agente SEM vínculo com um turno-pai. Apenas a origem job
// (eventctx.Provenance.Source == "job", único valor de automação — ver
// internal/eventctx/eventctx.go; carimbada pelo executor de jobs antes de
// resolver a tool, ver internal/jobs/executor.go) é aceitável sem pai
// (AEP-0068, ponto de entrada formalizado na F4). Chamadas de chat NUNCA caem
// aqui: o agentic loop sempre fornece conversation_id/turn_id.
func originAllowsParentless(ctx context.Context) bool {
	prov, ok := eventctx.From(ctx)
	return ok && prov.Source == "job"
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
	if a.Cancel && conversationID == "" {
		return errResult("'cancel' requer 'conversation_id'"), nil
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
		// Reuso de sub-conversa existente (resume) é Fase 3.
		if conversationID != "" {
			return errResult("reuso de sub-conversa existente (resume) ainda não é suportado nesta fase"), nil
		}
		inv, _ := invocationctx.Get(ctx)

		// Vínculo com o turno-pai (AEP-0068). Uma chamada vinda do chat/workspace
		// SEMPRE tem conversa/turno pai (o agentic loop carimba o InvocationContext
		// — ver internal/agent/service.go). Se faltarem ao CRIAR uma nova
		// sub-conversa, é bug de wiring e NÃO devemos criar sub-conversa órfã
		// (kind=subagent some da listagem principal). Falha fechado. A única
		// exceção legítima sem-pai é a origem job (eventctx.Provenance.Source ==
		// "job", único valor de automação; ver internal/eventctx/eventctx.go),
		// ponto de entrada formalizado na F4: distinção EXPLÍCITA por origem
		// (eventctx.Provenance), não pela mera ausência de InvocationContext.
		hasParent := strings.TrimSpace(inv.ConversationID) != "" && strings.TrimSpace(inv.TurnID) != ""
		if !hasParent && !originAllowsParentless(ctx) {
			return errResult("sub-agente requer um turno-pai: invocado sem conversation_id/turn_id de invocação (possível erro de wiring do agentic loop)"), nil
		}

		profile := strings.TrimSpace(a.Profile)
		if profile == "" {
			profile = inv.ProfileSlug
		}
		res, err := runner.Run(ctx, subagent.RunParams{
			ParentConversationID: inv.ConversationID,
			ParentTurnID:         inv.TurnID,
			ParentInvocationID:   toolinvocations.CurrentInvocationID(ctx),
			Prompt:               prompt,
			ProfileSlug:          profile,
			Model:                strings.TrimSpace(a.Model),
			Title:                strings.TrimSpace(a.Title),
			Background:           a.Background,
		})
		if err != nil {
			return errResult(fmt.Sprintf("erro ao iniciar sub-agente: %v", err)), nil
		}
		// IMPORTANTE (AEP-0068, "Retorno da tool"): o desfecho do sub-agente
		// (succeeded/failed/timed_out/cancelled) é DADO de negócio, exposto no
		// campo `status` do payload (e na metadata) — NÃO é falha da tool.
		// Marcar IsError com base no status faria o pipeline de toolinvocations
		// (ver statusForExecution) persistir o JSON do RunResult como
		// error_message e emitir tool_failure/retries indevidos. IsError fica
		// reservado a falhas da PRÓPRIA tool (args inválidos, wiring ausente,
		// erro do runner/manager), tratadas acima.
		return jsonResult(res, false, map[string]any{"conversation_id": res.ConversationID, "run_id": res.RunID, "status": res.Status}), nil

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
