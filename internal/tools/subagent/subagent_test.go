package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/eventctx"
	"assistente/internal/subagent"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/toolinvocations"
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
	runner := &fakeRunner{statusResult: subagent.StatusResult{ConversationID: "c1", RunID: "r1", Status: subagent.StatusRunning}}
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

func TestToolValidationNothingToDo(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !res.IsError {
		t.Fatal("esperava erro: nada a fazer")
	}
}

func TestToolResumeNotSupportedYet(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"x","conversation_id":"c1"}`))
	if !res.IsError {
		t.Fatal("esperava erro: resume (conversation_id + prompt) é Fase 3")
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

func TestToolExplicitProfileOverridesInherited(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}}
	tool := NewWithProvider(func() Runner { return runner })
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "parent-conv",
		TurnID:         "parent-turn",
		ProfileSlug:    "parent-profile",
	})

	if _, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"x","profile":"researcher"}`)); err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if runner.lastParams.ProfileSlug != "researcher" {
		t.Fatalf("profile explícito esperado researcher, veio %q", runner.lastParams.ProfileSlug)
	}
}
