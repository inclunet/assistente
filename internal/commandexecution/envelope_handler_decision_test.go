package commandexecution

import (
	"context"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
)

func TestEnvelopeHandlerReceivesConsumedDecisionID(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	presenter := &pipelineDecisionPresenter{action: commanddecision.ApplyAction}
	decisions, err := commanddecision.New(f.db, presenter, func() (nowTime time.Time) { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.service.config.Envelope.Decisions = decisions

	observed := make(chan Invocation, 1)
	f.start = func(_ context.Context, invocation Invocation) (ExecutionHandle, error) {
		observed <- invocation
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	}
	invocationID := newTestUUID()
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(invocationID, "pipe.destroy", `{}`))
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("execução destrutiva: status=%s err=%v", record.Status, err)
	}

	var receipt struct {
		DecisionID string
		Status     string
	}
	if err := f.db.Raw("SELECT decision_id, status FROM command_decision_receipts WHERE subject_id = ?", invocationID).Scan(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.DecisionID == "" || receipt.Status != "consumed" {
		t.Fatalf("receipt não consumido: %+v", receipt)
	}
	select {
	case invocation := <-observed:
		if invocation.Envelope == nil || invocation.Envelope.AuthorizationDecisionID == nil || *invocation.Envelope.AuthorizationDecisionID != receipt.DecisionID {
			t.Fatalf("handler não recebeu o receipt consumido: envelope=%+v receipt=%q", invocation.Envelope, receipt.DecisionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler não foi iniciado")
	}
}

func TestEnvelopeHandlerDecisionRejectionDoesNotStart(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	presenter := &pipelineDecisionPresenter{action: commanddecision.DenyAction}
	decisions, err := commanddecision.New(f.db, presenter, func() (nowTime time.Time) { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.service.config.Envelope.Decisions = decisions

	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.destroy", `{}`))
	if err != nil || record.Status != commandledger.Denied || f.startCalls.Load() != 0 {
		t.Fatalf("rejeição alcançou handler: status=%s err=%v starts=%d", record.Status, err, f.startCalls.Load())
	}
}

func TestEnvelopeNoDecisionDiscardsSnapshotDecisionID(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	decisionID := newTestUUID()
	f.service.config.Envelope.Snapshot = func(_ context.Context, _ auth.LocalSessionPrincipal, c EnvelopeCandidate) (commandcontract.Envelope, error) {
		decision := decisionID
		return commandcontract.Envelope{RegistryVersion: "registry-v1", GlobalConfigGeneration: pipelineStringPtr("global-v1"), ActiveLayersGeneration: pipelineStringPtr("layers-v1"), AuthorizationDecisionID: &decision, CorrelationID: c.CorrelationID}, nil
	}
	observed := make(chan Invocation, 1)
	f.start = func(_ context.Context, invocation Invocation) (ExecutionHandle, error) {
		observed <- invocation
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	}
	if record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{}`)); err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("no-decision: status=%s err=%v", record.Status, err)
	}
	select {
	case invocation := <-observed:
		if invocation.Envelope == nil || invocation.Envelope.AuthorizationDecisionID != nil {
			t.Fatalf("ID injetado pelo snapshot alcançou handler: %+v", invocation.Envelope)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler não foi iniciado")
	}
}
