package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"assistente/internal/eventctx"
	"assistente/internal/profileaccess"
	"assistente/internal/subagent"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// parentCtx devolve um ctx de chamada de chat com vínculo pai válido
// (conversation_id/turn_id de invocação), como o agentic loop carimba.
func parentCtx() context.Context {
	return invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "parent-conv",
		TurnID:         "parent-turn",
	})
}

type fakeRunner struct {
	lastParams   subagent.RunParams
	result       subagent.RunResult
	err          error
	statusResult subagent.StatusResult
	statusErr    error
	cancelResult subagent.CancelResult
	cancelErr    error
	lastStatusID [2]string // conversationID, runID
	lastCancelID [2]string
}

type fakeProfileAuthorizer struct {
	allowed bool
	err     error
	calls   int
	request profileaccess.AuthorizationRequest
}

func (f *fakeProfileAuthorizer) Authorize(_ context.Context, request profileaccess.AuthorizationRequest) (bool, error) {
	f.calls++
	f.request = request
	return f.allowed, f.err
}

func (f *fakeRunner) Run(_ context.Context, p subagent.RunParams) (subagent.RunResult, error) {
	f.lastParams = p
	return f.result, f.err
}

func (f *fakeRunner) Status(_ context.Context, conversationID, runID string) (subagent.StatusResult, error) {
	f.lastStatusID = [2]string{conversationID, runID}
	return f.statusResult, f.statusErr
}

func (f *fakeRunner) Cancel(_ context.Context, conversationID, runID string) (subagent.CancelResult, error) {
	f.lastCancelID = [2]string{conversationID, runID}
	return f.cancelResult, f.cancelErr
}

// TestToolStatusByRunIDAlone garante que, no modo status (sem prompt e sem
// cancel), run_id pode ser usado SEM conversation_id: o Manager resolve o run
// pelo run_id (AEP-0068). A tool deve chamar Status(ctx, "", runID).
func TestToolStatusByRunIDAlone(t *testing.T) {
	runner := &fakeRunner{statusResult: subagent.StatusResult{ConversationID: "child-conv", RunID: "run-1", Status: subagent.StatusRunning}}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"run_id":"run-1"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("status por run_id sozinho não deveria falhar: %s", res.Content)
	}
	if runner.lastStatusID[0] != "" || runner.lastStatusID[1] != "run-1" {
		t.Fatalf("esperava Status(\"\", \"run-1\"), veio %#v", runner.lastStatusID)
	}
}

// TestToolSendWithRunIDWithoutConversationFails garante que run_id sem
// conversation_id continua proibido fora do status (send mantém a restrição).
func TestToolSendWithRunIDWithoutConversationFails(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(parentCtx(), json.RawMessage(`{"prompt":"faça X","run_id":"run-1"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if !res.IsError {
		t.Fatal("send com run_id sem conversation_id deveria falhar")
	}
	if runner.lastParams.Prompt != "" {
		t.Fatalf("runner não deveria ter sido chamado: %#v", runner.lastParams)
	}
}

// TestToolCancelWithRunIDWithoutConversationFails garante que cancel exige
// conversation_id mesmo com run_id.
func TestToolCancelWithRunIDWithoutConversationFails(t *testing.T) {
	runner := &fakeRunner{cancelResult: subagent.CancelResult{Status: subagent.StatusCancelled, Cancelled: true}}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"cancel":true,"run_id":"run-1"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if !res.IsError {
		t.Fatal("cancel com run_id sem conversation_id deveria falhar")
	}
	if runner.lastCancelID[1] != "" {
		t.Fatalf("runner.Cancel não deveria ter sido chamado: %#v", runner.lastCancelID)
	}
}

func TestToolMetadata(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	if tool.Name() != "subagent" {
		t.Fatalf("nome esperado subagent, veio %q", tool.Name())
	}
	if !json.Valid(tool.Parameters()) {
		t.Fatal("schema de parâmetros inválido")
	}
	description := strings.ToLower(tool.Description())
	for _, guidance := range []string{"when to use", "don't use", "synchronous", "background", "tokens", "concurrency"} {
		if !strings.Contains(description, guidance) {
			t.Fatalf("descrição sem orientação %q: %s", guidance, description)
		}
	}
}

func TestToolRequiresPrompt(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute não deve retornar erro Go: %v", err)
	}
	if !res.IsError {
		t.Fatal("esperava IsError para prompt ausente")
	}
}

func TestToolHappyPathPropagatesContext(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{
		ConversationID: "child-conv",
		RunID:          "run-1",
		Status:         subagent.StatusSucceeded,
		ResultSummary:  "ok",
	}}
	tool := NewWithProvider(func() Runner { return runner })

	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "parent-conv",
		TurnID:         "parent-turn",
		ProfileSlug:    "parent-profile",
	})
	ctx = toolinvocations.WithCurrentInvocationID(ctx, "inv-123")

	res, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"faça X"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("não esperava erro: %s", res.Content)
	}
	// Herança de profile do pai + encadeamento de invocação.
	if runner.lastParams.ProfileSlug != "parent-profile" {
		t.Fatalf("profile herdado esperado parent-profile, veio %q", runner.lastParams.ProfileSlug)
	}
	if runner.lastParams.ParentConversationID != "parent-conv" || runner.lastParams.ParentTurnID != "parent-turn" {
		t.Fatalf("contexto-pai não propagado: %#v", runner.lastParams)
	}
	if runner.lastParams.ParentInvocationID != "inv-123" {
		t.Fatalf("parent invocation esperado inv-123, veio %q", runner.lastParams.ParentInvocationID)
	}
	// Metadata exposta ao LLM/UI.
	if res.Metadata["conversation_id"] != "child-conv" || res.Metadata["run_id"] != "run-1" {
		t.Fatalf("metadata inesperada: %#v", res.Metadata)
	}
}

func TestToolCancelRouting(t *testing.T) {
	runner := &fakeRunner{cancelResult: subagent.CancelResult{ConversationID: "c1", RunID: "r1", Status: subagent.StatusCancelled, Cancelled: true}}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"cancel":true,"conversation_id":"c1"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("cancel não deveria ser erro: %s", res.Content)
	}
	if runner.lastCancelID[0] != "c1" {
		t.Fatalf("cancel não roteado com conversation_id: %#v", runner.lastCancelID)
	}
	if res.Metadata["cancelled"] != true {
		t.Fatalf("metadata cancelled esperada true: %#v", res.Metadata)
	}
}

func TestToolStatusRouting(t *testing.T) {
	runner := &fakeRunner{statusResult: subagent.StatusResult{
		ConversationID:     "c1",
		RunID:              "r1",
		Status:             subagent.StatusSucceeded,
		AssistantMessageID: "msg-1",
	}}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"conversation_id":"c1"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("status não deveria ser erro: %s", res.Content)
	}
	if runner.lastStatusID[0] != "c1" {
		t.Fatalf("status não roteado com conversation_id: %#v", runner.lastStatusID)
	}
	if res.Metadata["assistant_message_id"] != "msg-1" {
		t.Fatalf("assistant_message_id ausente da metadata de status: %#v", res.Metadata)
	}
}

func TestToolValidationCancelWithPrompt(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"cancel":true,"prompt":"x","conversation_id":"c1"}`))
	if !res.IsError {
		t.Fatal("esperava erro: cancel + prompt são mutuamente exclusivos")
	}
}

func TestToolValidationCancelWithoutConversation(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"cancel":true}`))
	if !res.IsError {
		t.Fatal("esperava erro: cancel sem conversation_id")
	}
}

func TestToolValidationRunIDWithPrompt(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"x","conversation_id":"c1","run_id":"r1"}`))
	if !res.IsError {
		t.Fatal("esperava erro: run_id é para status/cancel e não pode acompanhar prompt")
	}
}

func TestToolValidationNothingToDo(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !res.IsError {
		t.Fatal("esperava erro: nada a fazer")
	}
}

func TestToolResumePropagatesConversationID(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{ConversationID: "c1", RunID: "r2", Status: subagent.StatusSucceeded}}
	tool := NewWithProvider(func() Runner { return runner })
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"continue","conversation_id":"c1"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("resume não deveria ser erro: %s", res.Content)
	}
	if runner.lastParams.ConversationID != "c1" {
		t.Fatalf("conversation_id não propagado ao Runner: %#v", runner.lastParams)
	}
	if runner.lastParams.Clear {
		t.Fatal("clear não deveria estar setado")
	}
}

func TestToolClearPropagated(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{ConversationID: "c1", RunID: "r3", Status: subagent.StatusSucceeded}}
	tool := NewWithProvider(func() Runner { return runner })
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"novo","conversation_id":"c1","clear":true}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("clear+prompt não deveria ser erro: %s", res.Content)
	}
	if !runner.lastParams.Clear || runner.lastParams.ConversationID != "c1" {
		t.Fatalf("clear/conversation_id não propagados: %#v", runner.lastParams)
	}
}

func TestToolValidationClearWithoutConversation(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"clear":true,"prompt":"x"}`))
	if !res.IsError {
		t.Fatal("esperava erro: clear sem conversation_id")
	}
}

func TestToolValidationClearWithoutPrompt(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"clear":true,"conversation_id":"c1"}`))
	if !res.IsError {
		t.Fatal("esperava erro: clear sem prompt")
	}
}

func TestToolValidationCancelWithClear(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"cancel":true,"clear":true,"conversation_id":"c1"}`))
	if !res.IsError {
		t.Fatal("esperava erro: cancel + clear são mutuamente exclusivos")
	}
}

func TestToolBackgroundFlagPropagated(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusRunning}}
	tool := NewWithProvider(func() Runner { return runner })
	if _, err := tool.Execute(parentCtx(), json.RawMessage(`{"prompt":"x","background":true}`)); err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if !runner.lastParams.Background {
		t.Fatal("background=true não propagado ao Runner")
	}
}

func TestToolRawReturnsIntegralContentAndMetadata(t *testing.T) {
	content := `{"data":"` + strings.Repeat("x", 20_000) + `"}`
	runner := &fakeRunner{result: subagent.RunResult{
		ConversationID:     "child-conv",
		RunID:              "run-raw",
		Status:             subagent.StatusSucceeded,
		ResultSummary:      content[:100],
		AssistantMessageID: "msg-raw",
		Response:           content,
	}}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(parentCtx(), json.RawMessage(`{"prompt":"gere dados","raw":true}`))
	if err != nil || res.IsError {
		t.Fatalf("raw síncrono falhou: result=%#v err=%v", res, err)
	}
	if res.Content != content {
		t.Fatalf("raw não devolveu completion.response integral: got=%d want=%d", len(res.Content), len(content))
	}
	if !json.Valid([]byte(res.Content)) {
		t.Fatal("raw deveria preservar o JSON puro produzido pelo sub-agente")
	}
	if res.Metadata["conversation_id"] != "child-conv" ||
		res.Metadata["run_id"] != "run-raw" ||
		res.Metadata["assistant_message_id"] != "msg-raw" {
		t.Fatalf("IDs ausentes da metadata raw: %#v", res.Metadata)
	}
}

func TestToolRawDefaultPreservesEnvelope(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{
		ConversationID: "child-conv",
		RunID:          "run-default",
		Status:         subagent.StatusSucceeded,
		ResultSummary:  "compatível",
		Response:       "conteúdo direto",
	}}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(parentCtx(), json.RawMessage(`{"prompt":"faça X"}`))
	if err != nil || res.IsError {
		t.Fatalf("modo default falhou: result=%#v err=%v", res, err)
	}
	var envelope subagent.RunResult
	if err := json.Unmarshal([]byte(res.Content), &envelope); err != nil {
		t.Fatalf("default deve manter envelope JSON: %v", err)
	}
	if envelope.ResultSummary != "compatível" || envelope.Response != "" {
		t.Fatalf("envelope compatível inesperado: %#v", envelope)
	}
}

func TestToolRawBackgroundFailsBeforeRun(t *testing.T) {
	runner := &fakeRunner{}
	tool := NewWithProvider(func() Runner { return runner })

	res, err := tool.Execute(parentCtx(), json.RawMessage(`{"prompt":"x","raw":true,"background":true}`))
	if err != nil || !res.IsError {
		t.Fatalf("raw+background deveria falhar por validação: result=%#v err=%v", res, err)
	}
	if runner.lastParams.Prompt != "" {
		t.Fatalf("runner não deveria ser chamado: %#v", runner.lastParams)
	}
}

func TestToolRawRejectedOutsideSend(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	for _, args := range []string{
		`{"conversation_id":"c1","raw":true}`,
		`{"conversation_id":"c1","cancel":true,"raw":true}`,
	} {
		res, err := tool.Execute(context.Background(), json.RawMessage(args))
		if err != nil || !res.IsError {
			t.Fatalf("raw fora de send deveria falhar para %s: result=%#v err=%v", args, res, err)
		}
	}
}

func TestToolRawStillRespectsExecutorLimit(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{
		ConversationID: "child-conv",
		RunID:          "run-large",
		Status:         subagent.StatusSucceeded,
		Response:       strings.Repeat("x", 2048),
	}}
	tool := NewWithProvider(func() Runner { return runner })
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	cfg := tools.DefaultExecutorConfig()
	cfg.MaxResultSize = 256
	executor := tools.NewExecutor(registry, cfg)

	exec := executor.ExecuteOne(parentCtx(), tools.ToolCall{
		ID: "call-raw",
		Function: tools.FunctionCall{
			Name:      "subagent",
			Arguments: `{"prompt":"gere muito","raw":true}`,
		},
	})
	if exec.Result.IsError {
		t.Fatalf("limite textual deve truncar, não falhar: %#v", exec)
	}
	if len(exec.Result.Content) > cfg.MaxResultSize || exec.Result.Metadata["truncated"] != true {
		t.Fatalf("raw deve respeitar limite comum do executor: len=%d metadata=%#v", len(exec.Result.Content), exec.Result.Metadata)
	}
}

// TestToolBusinessOutcomeIsNotToolError garante que o desfecho de NEGÓCIO do
// sub-agente (failed/timed_out/cancelled) NÃO marca IsError: a tool executou
// corretamente e o status é dado no payload. Marcar IsError faria o pipeline de
// toolinvocations persistir o JSON como error_message e emitir tool_failure/retry
// indevidos (ver statusForExecution).
func TestToolBusinessOutcomeIsNotToolError(t *testing.T) {
	for _, status := range []string{
		subagent.StatusFailed,
		subagent.StatusTimedOut,
		subagent.StatusCancelled,
		subagent.StatusSucceeded,
		subagent.StatusRunning,
		subagent.StatusQueued,
	} {
		runner := &fakeRunner{result: subagent.RunResult{ConversationID: "c", RunID: "r", Status: status}}
		tool := NewWithProvider(func() Runner { return runner })
		res, err := tool.Execute(parentCtx(), json.RawMessage(`{"prompt":"faça X"}`))
		if err != nil {
			t.Fatalf("status %s: Execute erro: %v", status, err)
		}
		if res.IsError {
			t.Fatalf("status %s: desfecho de negócio não deveria marcar IsError; conteúdo=%s", status, res.Content)
		}
		// O status continua disponível como dado (payload + metadata).
		if res.Metadata["status"] != status {
			t.Fatalf("status %s: metadata.status inesperada: %#v", status, res.Metadata)
		}
		var decoded subagent.RunResult
		if err := json.Unmarshal([]byte(res.Content), &decoded); err != nil {
			t.Fatalf("status %s: payload não é RunResult JSON: %v", status, err)
		}
		if decoded.Status != status {
			t.Fatalf("status %s: payload.status inesperado: %q", status, decoded.Status)
		}
	}
}

// TestToolRealFailureMarksError garante que falhas da PRÓPRIA tool marcam
// IsError=true: erro do runner/manager (envio/criação) e wiring ausente.
func TestToolRealFailureMarksError(t *testing.T) {
	// Erro real ao executar a operação (runner/manager retorna erro).
	runner := &fakeRunner{err: errors.New("falha ao enviar")}
	tool := NewWithProvider(func() Runner { return runner })
	res, err := tool.Execute(parentCtx(), json.RawMessage(`{"prompt":"x"}`))
	if err != nil {
		t.Fatalf("Execute não deve retornar erro Go: %v", err)
	}
	if !res.IsError {
		t.Fatal("erro real do runner deveria marcar IsError=true")
	}

	// Wiring ausente (provider nil) → erro da tool.
	noWiring := NewWithProvider(nil)
	res2, _ := noWiring.Execute(context.Background(), json.RawMessage(`{"prompt":"x"}`))
	if !res2.IsError {
		t.Fatal("wiring ausente deveria marcar IsError=true")
	}
}

// TestToolFailsClosedWithoutParent garante que uma chamada de chat sem vínculo
// pai (sem conversation_id/turn_id de invocação) falha fechado (IsError) e NÃO
// chama o runner — evitando criar sub-conversa órfã (AEP-0068).
func TestToolFailsClosedWithoutParent(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"sem-invocationctx", context.Background()},
		{"sem-conversation", invocationctx.With(context.Background(), invocationctx.InvocationContext{TurnID: "t"})},
		{"sem-turn", invocationctx.With(context.Background(), invocationctx.InvocationContext{ConversationID: "c"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}}
			tool := NewWithProvider(func() Runner { return runner })
			res, err := tool.Execute(tc.ctx, json.RawMessage(`{"prompt":"faça X"}`))
			if err != nil {
				t.Fatalf("Execute não deve retornar erro Go: %v", err)
			}
			if !res.IsError {
				t.Fatal("chamada de chat sem turno-pai deveria falhar fechado (IsError)")
			}
			if runner.lastParams.Prompt != "" {
				t.Fatalf("runner não deveria ter sido chamado (sub-conversa órfã): %#v", runner.lastParams)
			}
		})
	}
}

// TestToolAllowsParentlessForJobOrigin garante que a origem job/system (modo
// sem-pai EXPLÍCITO do AEP-0068, formalizado na F4) pode rodar o sub-agente sem
// vínculo pai: a tool NÃO falha fechado e chama o runner.
func TestToolAllowsParentlessForJobOrigin(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{ConversationID: "c", RunID: "r", Status: subagent.StatusSucceeded}}
	tool := NewWithProvider(func() Runner { return runner })

	ctx := eventctx.With(context.Background(), eventctx.Provenance{Source: "job", SourceJobID: "job-1"})
	res, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"faça X"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("origem job não deveria falhar fechado: %s", res.Content)
	}
	if runner.lastParams.Prompt != "faça X" {
		t.Fatalf("runner deveria ter sido chamado no modo job sem-pai: %#v", runner.lastParams)
	}
}

// TestToolResumeBackgroundRequiresParent garante (thread :201) que um resume
// (conversation_id presente) com background:true e origem CHAT SEM vínculo pai
// falha fechado: a entrega da conclusão (auto-wake) injeta o aviso na conversa do
// pai e, sem parent, deliver() retorna cedo e a notificação nunca chega (AEP-0068).
func TestToolResumeBackgroundRequiresParent(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"sem-invocationctx", context.Background()},
		{"sem-conversation", invocationctx.With(context.Background(), invocationctx.InvocationContext{TurnID: "t"})},
		{"sem-turn", invocationctx.With(context.Background(), invocationctx.InvocationContext{ConversationID: "c"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{result: subagent.RunResult{ConversationID: "c1", RunID: "r", Status: subagent.StatusRunning}}
			tool := NewWithProvider(func() Runner { return runner })
			res, err := tool.Execute(tc.ctx, json.RawMessage(`{"prompt":"continue","conversation_id":"c1","background":true}`))
			if err != nil {
				t.Fatalf("Execute não deve retornar erro Go: %v", err)
			}
			if !res.IsError {
				t.Fatal("resume+background sem turno-pai (origem chat) deveria falhar fechado (IsError)")
			}
			if runner.lastParams.Prompt != "" {
				t.Fatalf("runner não deveria ter sido chamado sem parent em background: %#v", runner.lastParams)
			}
		})
	}
}

// TestToolResumeSyncWithoutParentAllowed garante que o resume SÍNCRONO sem pai
// segue permitido (não regride): no síncrono o resultado volta pelo retorno da
// tool, sem auto-wake, então não exige parent.
func TestToolResumeSyncWithoutParentAllowed(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{ConversationID: "c1", RunID: "r2", Status: subagent.StatusSucceeded}}
	tool := NewWithProvider(func() Runner { return runner })
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"continue","conversation_id":"c1"}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("resume síncrono sem pai não deveria falhar: %s", res.Content)
	}
	if runner.lastParams.ConversationID != "c1" {
		t.Fatalf("conversation_id não propagado: %#v", runner.lastParams)
	}
}

// TestToolResumeBackgroundAllowedForJobOrigin garante que origem job pode rodar
// resume+background SEM pai (parentless por contrato; auto-wake não se aplica do
// mesmo modo — AEP-0068).
func TestToolResumeBackgroundAllowedForJobOrigin(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{ConversationID: "c1", RunID: "r", Status: subagent.StatusRunning}}
	tool := NewWithProvider(func() Runner { return runner })
	ctx := eventctx.With(context.Background(), eventctx.Provenance{Source: "job", SourceJobID: "job-1"})
	res, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"continue","conversation_id":"c1","background":true}`))
	if err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if res.IsError {
		t.Fatalf("origem job resume+background não deveria falhar: %s", res.Content)
	}
	if !runner.lastParams.Background || runner.lastParams.ConversationID != "c1" {
		t.Fatalf("params não propagados no resume+background de job: %#v", runner.lastParams)
	}
}

func TestToolExplicitProfileOverridesInherited(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}}
	authorizer := &fakeProfileAuthorizer{allowed: true}
	tool := NewWithProvider(func() Runner { return runner }, authorizer)
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "parent-conv",
		TurnID:         "parent-turn",
		ProfileSlug:    "parent-profile",
		Source:         "wails",
	})

	if _, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"x","profile":"researcher"}`)); err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if runner.lastParams.ProfileSlug != "researcher" {
		t.Fatalf("profile explícito esperado researcher, veio %q", runner.lastParams.ProfileSlug)
	}
	if authorizer.calls != 1 || authorizer.request.TargetSlug != "researcher" {
		t.Fatalf("autorização cross-profile não solicitada: %#v", authorizer)
	}
}

func TestToolCrossProfileDeniedDoesNotCreateRun(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}}
	authorizer := &fakeProfileAuthorizer{allowed: false}
	tool := NewWithProvider(func() Runner { return runner }, authorizer)
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "parent-conv",
		TurnID:         "parent-turn",
		ProfileSlug:    "parent-profile",
		Source:         "wails",
	})

	result, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"x","profile":"researcher"}`))
	if err != nil || result.IsError {
		t.Fatalf("recusa é resultado normal: %#v err=%v", result, err)
	}
	if runner.lastParams.Prompt != "" {
		t.Fatalf("runner não deveria ser chamado: %#v", runner.lastParams)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "denied" || payload["authorized"] != false {
		t.Fatalf("payload de recusa inesperado: %#v", payload)
	}
}

func TestToolCrossProfileAuthorizationErrorIsStructured(t *testing.T) {
	runner := &fakeRunner{}
	authorizer := &fakeProfileAuthorizer{err: profileaccess.ErrTargetNotFound}
	tool := NewWithProvider(func() Runner { return runner }, authorizer)
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "parent-conv",
		TurnID:         "parent-turn",
		ProfileSlug:    "parent-profile",
		Source:         "wails",
	})

	result, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"x","profile":"removido"}`))
	if err != nil || !result.IsError || runner.lastParams.Prompt != "" {
		t.Fatalf("falha de autorização inesperada: %#v err=%v", result, err)
	}
	if result.Metadata["error_code"] != "profile_not_found" {
		t.Fatalf("código estruturado inesperado: %#v", result.Metadata)
	}
}

func TestToolSameExplicitProfileDoesNotAsk(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}}
	authorizer := &fakeProfileAuthorizer{allowed: false}
	tool := NewWithProvider(func() Runner { return runner }, authorizer)
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "parent-conv",
		TurnID:         "parent-turn",
		ProfileSlug:    "parent-profile",
	})

	result, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"x","profile":"parent-profile"}`))
	if err != nil || result.IsError || authorizer.calls != 0 || runner.lastParams.Prompt != "x" {
		t.Fatalf("same-profile deveria preservar fluxo atual: result=%#v auth=%#v runner=%#v err=%v", result, authorizer, runner, err)
	}
}
