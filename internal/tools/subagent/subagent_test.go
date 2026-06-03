package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/subagent"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/toolinvocations"
)

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

func TestToolValidationRunIDWithoutConversation(t *testing.T) {
	tool := NewWithProvider(func() Runner { return &fakeRunner{} })
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"run_id":"r1"}`))
	if !res.IsError {
		t.Fatal("esperava erro: run_id sem conversation_id")
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
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"x","background":true}`)); err != nil {
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
		res, err := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"faça X"}`))
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
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"prompt":"x"}`))
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

func TestToolExplicitProfileOverridesInherited(t *testing.T) {
	runner := &fakeRunner{result: subagent.RunResult{Status: subagent.StatusSucceeded}}
	tool := NewWithProvider(func() Runner { return runner })
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{ProfileSlug: "parent-profile"})

	if _, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"x","profile":"researcher"}`)); err != nil {
		t.Fatalf("Execute erro: %v", err)
	}
	if runner.lastParams.ProfileSlug != "researcher" {
		t.Fatalf("profile explícito esperado researcher, veio %q", runner.lastParams.ProfileSlug)
	}
}
