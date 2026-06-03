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
	lastParams subagent.RunParams
	result     subagent.RunResult
	err        error
}

func (f *fakeRunner) Run(_ context.Context, p subagent.RunParams) (subagent.RunResult, error) {
	f.lastParams = p
	return f.result, f.err
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
