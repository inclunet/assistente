package commandexecution

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/commandledger"
)

func TestExecuteEnvelopeWithResultReturnsDetachedRawAndKeepsLedgerRedacted(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	raw := json.RawMessage(`{ }`)
	f.start = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		done := make(chan Outcome, 1)
		done <- Outcome{Status: commandledger.Succeeded, Result: raw}
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}, nil
	}
	candidate := f.candidate(newTestUUID(), "pipe.read", `{}`)
	record, output, err := f.service.ExecuteEnvelopeWithResult(context.Background(), f.token, candidate)
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("execução: status=%s err=%v", record.Status, err)
	}
	if string(output) != `{}` {
		t.Fatalf("output validado = %s, want {}", output)
	}
	raw[0] = '['
	if string(output) != `{}` {
		t.Fatalf("mutação do buffer do handler alterou output: %s", output)
	}
	output[0] = 'X'
	if raw[0] != '[' {
		t.Fatal("mutação do output alterou buffer do handler")
	}
	if record.ResultSummary == nil || *record.ResultSummary == `{}` {
		t.Fatalf("ledger não permaneceu redigido: %#v", record.ResultSummary)
	}
	if record.Envelope.Arguments == nil || string(*record.Envelope.Arguments) != `{"version":1,"redacted":true}` {
		t.Fatalf("argumentos não permaneceram redigidos: %#v", record.Envelope.Arguments)
	}
}

func TestExecuteEnvelopeWithResultReplayDoesNotReturnOrReexecute(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	raw := json.RawMessage(`{ }`)
	f.start = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		done := make(chan Outcome, 1)
		done <- Outcome{Status: commandledger.Succeeded, Result: raw}
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}, nil
	}
	candidate := f.candidate(newTestUUID(), "pipe.read", `{}`)
	first, output, err := f.service.ExecuteEnvelopeWithResult(context.Background(), f.token, candidate)
	if err != nil || first.Status != commandledger.Succeeded || len(output) == 0 {
		t.Fatalf("primeira execução: status=%s output=%s err=%v", first.Status, output, err)
	}
	replay, replayOutput, err := f.service.ExecuteEnvelopeWithResult(context.Background(), f.token, candidate)
	if err != nil || replay.Status != commandledger.Succeeded || replayOutput != nil {
		t.Fatalf("replay: status=%s output=%s err=%v", replay.Status, replayOutput, err)
	}
	if f.startCalls.Load() != 1 {
		t.Fatalf("handler executado %d vezes", f.startCalls.Load())
	}
}

func TestExecuteEnvelopeWithResultNeverReturnsOutputForInvalidFailedOrCancelled(t *testing.T) {
	tests := []struct {
		name   string
		result Outcome
	}{
		{name: "resultado inválido", result: Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`[]`)}},
		{name: "failed", result: Outcome{Status: commandledger.Failed, Result: json.RawMessage(`{"value":"hidden"}`)}},
		{name: "cancelled", result: Outcome{Status: commandledger.Cancelled, Result: json.RawMessage(`{"value":"hidden"}`)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newEnvelopePipelineFixture(t)
			f.start = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
				done := make(chan Outcome, 1)
				done <- tc.result
				return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}, nil
			}
			record, output, err := f.service.ExecuteEnvelopeWithResult(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{}`))
			if tc.result.Status == commandledger.Succeeded {
				if err != nil || record.Status != commandledger.OutcomeUnknown {
					t.Fatalf("resultado inválido: status=%s err=%v", record.Status, err)
				}
			} else if err != nil || record.Status != tc.result.Status {
				t.Fatalf("resultado terminal: status=%s err=%v", record.Status, err)
			}
			if output != nil {
				t.Fatalf("output deveria ser nil: %s", output)
			}
		})
	}
}

func TestExecuteEnvelopeWithResultFinalCommitFailureReturnsNilOutput(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	f.service.config.FinalizationTimeout = 0
	_, output, err := f.service.ExecuteEnvelopeWithResult(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{}`))
	if err == nil {
		t.Fatal("commit final com prazo zero deveria falhar")
	}
	if f.startCalls.Load() != 1 {
		t.Fatalf("falha ocorreu antes do handler: StartCalls=%d", f.startCalls.Load())
	}
	if output != nil {
		t.Fatalf("falha de commit final devolveu output: %s", output)
	}
}
