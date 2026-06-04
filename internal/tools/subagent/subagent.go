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
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/toolinvocations"
)

// Runner é a porta de execução de sub-agentes. *subagent.Manager a satisfaz.
type Runner interface {
	Run(ctx context.Context, p subagent.RunParams) (subagent.RunResult, error)
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
	return "Delegate a task to a sub-agent running in its own persisted sub-conversation, and wait for the result (synchronous). Provide 'prompt' with the task. Optionally set 'profile' (slug of the interaction profile that defines the sub-agent's model, behavior and enabled tools — defaults to the parent's profile), 'title' (sub-conversation title) and 'model' (override the model for this run). Returns conversation_id and run_id (durable handles), status and the sub-agent's result_summary."
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "Task/message for the sub-agent. Required."
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
		"required": ["prompt"],
		"additionalProperties": false
	}`)
}

type subagentArgs struct {
	Prompt  string `json:"prompt"`
	Profile string `json:"profile,omitempty"`
	Title   string `json:"title,omitempty"`
	Model   string `json:"model,omitempty"`
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
			return tools.ToolResult{Content: fmt.Sprintf("argumentos inválidos: %v", err), IsError: true}, nil
		}
	}
	if strings.TrimSpace(a.Prompt) == "" {
		return tools.ToolResult{Content: "o parâmetro 'prompt' é obrigatório", IsError: true}, nil
	}
	if t.provider == nil {
		return tools.ToolResult{Content: "sub-agentes indisponíveis: runner não configurado", IsError: true}, nil
	}
	runner := t.provider()
	if runner == nil {
		return tools.ToolResult{Content: "sub-agentes indisponíveis no momento", IsError: true}, nil
	}

	// Contexto de invocação: conversa-pai, turno e profile herdado.
	inv, _ := invocationctx.Get(ctx)

	// Vínculo com o turno-pai (AEP-0068). Uma chamada vinda do chat/workspace
	// SEMPRE tem conversa/turno pai (o agentic loop carimba o InvocationContext
	// — ver internal/agent/service.go). Se faltarem, é bug de wiring e NÃO
	// devemos criar sub-conversa órfã (kind=subagent some da listagem principal,
	// virando conversa difícil de descobrir/debugar). Falha fechado.
	//
	// A única exceção legítima sem-pai é a origem job (eventctx.Provenance.Source
	// == "job", único valor de automação; ver internal/eventctx/eventctx.go),
	// ponto de entrada formalizado na F4: nesse caso o sub-agente pode rodar
	// sem um turno-pai. A distinção é EXPLÍCITA por origem (eventctx.Provenance),
	// não pela mera ausência de InvocationContext.
	hasParent := strings.TrimSpace(inv.ConversationID) != "" && strings.TrimSpace(inv.TurnID) != ""
	if !hasParent && !originAllowsParentless(ctx) {
		return tools.ToolResult{
			Content: "sub-agente requer um turno-pai: invocado sem conversation_id/turn_id de invocação (possível erro de wiring do agentic loop)",
			IsError: true,
		}, nil
	}

	profile := strings.TrimSpace(a.Profile)
	if profile == "" {
		profile = inv.ProfileSlug
	}

	params := subagent.RunParams{
		ParentConversationID: inv.ConversationID,
		ParentTurnID:         inv.TurnID,
		ParentInvocationID:   toolinvocations.CurrentInvocationID(ctx),
		Prompt:               a.Prompt,
		ProfileSlug:          profile,
		Model:                strings.TrimSpace(a.Model),
		Title:                strings.TrimSpace(a.Title),
	}

	res, err := runner.Run(ctx, params)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao iniciar sub-agente: %v", err), IsError: true}, nil
	}

	payload, mErr := json.Marshal(res)
	if mErr != nil {
		return tools.ToolResult{Content: fmt.Sprintf("erro ao serializar resultado do sub-agente: %v", mErr), IsError: true}, nil
	}

	// IMPORTANTE (AEP-0068, "Retorno da tool"): o desfecho do sub-agente
	// (succeeded/failed/timed_out/cancelled) é DADO de negócio, exposto no campo
	// `status` do payload (e na metadata) — NÃO é falha da tool. A tool executou
	// corretamente a operação solicitada. Marcar IsError com base no status faria
	// o pipeline de toolinvocations (ver statusForExecution) persistir o JSON
	// inteiro do RunResult como error_message e emitir tool_failure/retries
	// indevidos. IsError fica reservado a falhas da PRÓPRIA tool (args inválidos,
	// wiring ausente, erro ao criar conversa/enviar), tratadas acima.
	return tools.ToolResult{
		Content: string(payload),
		IsError: false,
		Metadata: map[string]any{
			"conversation_id": res.ConversationID,
			"run_id":          res.RunID,
			"status":          res.Status,
		},
	}, nil
}
