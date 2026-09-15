package commandexecution

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandledger"
)

func TestEnvelopePolicyAndQueueCannotMutateSignedRequest(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	f.service.config.Envelope.Authorize = func(_ context.Context, _ auth.LocalSessionPrincipal, e commandcontract.Envelope, _ commandcatalog.Definition) error {
		*e.UserID = "not-the-owner"
		*e.CommandID = "different.command"
		return nil
	}
	f.service.config.Envelope.AwaitQueue = func(_ context.Context, e commandcontract.Envelope) error {
		*e.UserID = "not-the-owner"
		*e.Arguments = json.RawMessage(`{"secret":"changed"}`)
		return nil
	}
	c := f.candidate(newTestUUID(), "pipe.read", `{}`)
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, c)
	if err != nil || record.Status != commandledger.Succeeded || f.startCalls.Load() != 1 || *record.Ownership.UserID != f.user {
		t.Fatalf("callback alterou solicitação assinada: %s %v", record.Status, err)
	}
}

func TestEnvelopeSuppressionStaleIsDurableAndNeverFallsThrough(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
		return EnvelopeResolution{Mode: commandcontract.ResolutionSuppress, BindingIDs: []string{"binding.suppress"}}, nil
	}
	var snapshots atomic.Int32
	f.service.config.Envelope.Snapshot = func(ctx context.Context, p auth.LocalSessionPrincipal, c EnvelopeCandidate) (commandcontract.Envelope, error) {
		e, err := f.snapshot(ctx, p, c)
		if snapshots.Add(1) > 1 {
			e.GlobalConfigGeneration = pipelineStringPtr("changed")
		}
		return e, err
	}
	c := f.trigger(newTestUUID())
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, c)
	if err != nil || record.Status != commandledger.RejectedStale || f.startCalls.Load() != 0 {
		t.Fatalf("stale: %s %v", record.Status, err)
	}
	f.service.config.Envelope.Snapshot = f.snapshot
	f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
		t.Fatal("replay resolveu novamente")
		return EnvelopeResolution{}, nil
	}
	replay, err := f.service.ExecuteEnvelope(context.Background(), f.token, c)
	if err != nil || replay.Status != commandledger.RejectedStale {
		t.Fatalf("replay stale: %s %v", replay.Status, err)
	}
	var count int64
	if err := f.db.Table("command_invocations").Where("invocation_id = ?", c.InvocationID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("marcador criou auditoria: %d %v", count, err)
	}
}

func TestEnvelopeLastGateDeniesBeforeStart(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	f.authorize = func(call int) error {
		if call == 3 {
			return ErrDenied
		}
		return nil
	}
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{}`))
	if err != nil || record.Status != commandledger.CancelledStale || f.startCalls.Load() != 0 || f.authorizeCalls.Load() != 3 {
		t.Fatalf("gate final: %s %v start=%d checks=%d", record.Status, err, f.startCalls.Load(), f.authorizeCalls.Load())
	}
}

func TestEnvelopeHandlerCannotMutateFinalizationOwnership(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	f.start = func(_ context.Context, inv Invocation) (ExecutionHandle, error) {
		*inv.Envelope.UserID = "changed"
		*inv.Envelope.CommandID = "changed.command"
		inv.Envelope.InvocationID = newTestUUID()
		*inv.Envelope.Arguments = json.RawMessage(`{"changed":true}`)
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	}
	c := f.candidate(newTestUUID(), "pipe.read", `{}`)
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, c)
	if err != nil || record.Status != commandledger.Succeeded || record.InvocationID != c.InvocationID || record.Ownership.UserID == nil || *record.Ownership.UserID != f.user {
		t.Fatalf("mutação escapou: %#v %v", record, err)
	}
}

func TestEnvelopeLookupDeniedDoesNotLeakOrReexecute(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	c := f.candidate(newTestUUID(), "pipe.read", `{}`)
	if _, err := f.service.ExecuteEnvelope(context.Background(), f.token, c); err != nil {
		t.Fatal(err)
	}
	f.lookup = func(int) error { return errors.New("private policy detail") }
	if _, err := f.service.GetEnvelopeInvocation(context.Background(), f.token, c.InvocationID); !errors.Is(err, ErrDenied) {
		t.Fatalf("lookup: %v", err)
	}
	if _, err := f.service.ExecuteEnvelope(context.Background(), f.token, c); !errors.Is(err, ErrDenied) {
		t.Fatalf("replay: %v", err)
	}
	if f.startCalls.Load() != 1 {
		t.Fatal("lookup reexecutou handler")
	}
}

func TestEnvelopeLostAckWhileRunningDoesNotRepeatHandler(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	c := f.candidate(newTestUUID(), "pipe.read", `{}`)
	started := make(chan struct{})
	done := make(chan Outcome, 1)
	f.start = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}, nil
	}
	type result struct {
		record commandledger.FullRecord
		err    error
	}
	completed := make(chan result, 1)
	go func() { r, e := f.service.ExecuteEnvelope(context.Background(), f.token, c); completed <- result{r, e} }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("Start não ocorreu")
	}
	replay, err := f.service.ExecuteEnvelope(context.Background(), f.token, c)
	if err != nil || replay.Status != commandledger.Running || f.startCalls.Load() != 1 {
		t.Fatalf("reentrega running: %s %v starts=%d", replay.Status, err, f.startCalls.Load())
	}
	done <- Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
	select {
	case got := <-completed:
		if got.err != nil || got.record.Status != commandledger.Succeeded {
			t.Fatalf("fim: %s %v", got.record.Status, got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("finalização bloqueada")
	}
}
