package commandexecution

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandledger"
)

func TestCommitOwnershipValidatesBoundAndClaimState(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Nanosecond, 35*time.Second + time.Nanosecond} {
		if ownership, err := NewCommitOwnership(timeout); ownership != nil || !errors.Is(err, ErrInvalidCommitOwnership) {
			t.Fatalf("timeout inválido %v: ownership=%v err=%v", timeout, ownership, err)
		}
	}
	ownership, err := NewCommitOwnership(35 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ownership.Claim(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("claim cancelada = %v", err)
	}
	if err := ownership.Claim(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ownership.Claim(context.Background()); !errors.Is(err, ErrCommitOwnershipClaimed) {
		t.Fatalf("claim repetida = %v", err)
	}
}

func capabilityPipelineFixture(t *testing.T) *envelopePipelineFixture {
	t.Helper()
	f := newEnvelopePipelineFixture(t)
	registrations := []commandcatalog.Registration{
		pipelineRegistration("pipe.read", commandcatalog.Read, commandcatalog.NoDecision, false, commandcatalog.ContextPolicy{None: true}, []commandcatalog.Source{commandcatalog.Palette}, "internal/pipe/read"),
		pipelineRegistration("pipe.write", commandcatalog.Write, commandcatalog.NoDecision, true, commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active", Mode: commandcatalog.MaxAge, MaxAgeMS: 5000}}}, []commandcatalog.Source{commandcatalog.Palette}, "internal/pipe/write"),
		pipelineRegistration("pipe.destroy", commandcatalog.Destructive, commandcatalog.Interactive, true, commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active", Mode: commandcatalog.ExactVersion}}}, []commandcatalog.Source{commandcatalog.Palette, commandcatalog.UI}, "internal/pipe/destroy"),
	}
	for i := range registrations {
		if registrations[i].Definition.ID == "pipe.write" {
			registrations[i].Definition.MutatesEffectiveCapability = true
			registrations[i].Definition.HandlerClassification = commandcatalog.HandlerBackend
			registrations[i].Handler.MutatesEffectiveCapability = true
			registrations[i].Handler.Classification = commandcatalog.HandlerBackend
		}
	}
	registry, err := commandcatalog.NewComplete(registrations)
	if err != nil {
		t.Fatal(err)
	}
	f.registry = registry
	f.service.config.Registry = registry
	handlers := make(map[string]Handler, len(f.service.config.Handlers))
	for id, handler := range f.service.config.Handlers {
		handlers[id] = handler
	}
	handlers["pipe.write"] = Handler{
		Contract: registrations[1].Handler,
		Start:    handlers["pipe.write"].Start,
	}
	f.service.config.Handlers = handlers
	return f
}

func TestEnvelopeCapabilityClaimSurvivesSelfCancellation(t *testing.T) {
	f := capabilityPipelineFixture(t)
	ownership, err := NewCommitOwnership(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan Outcome, 1)
	var cancelled atomic.Int32
	f.start = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() { cancelled.Add(1) }, CommitOwnership: ownership}, nil
	}
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	candidate := f.candidate(newTestUUID(), "pipe.write", `{}`)
	go func() {
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, candidate)
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	<-started
	snapshot, err := f.epochs.Capture(context.Background(), f.user, f.session)
	if err != nil {
		t.Fatal(err)
	}
	finish, err := f.epochs.BeginTransitionFromSnapshot(context.Background(), snapshot,
		func(context.Context) error { return nil }, ownership.Claim)
	if err != nil {
		t.Fatal(err)
	}
	done <- Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
	got := <-result
	finish()
	finish()
	if got.err != nil || got.record.Status != commandledger.Succeeded || cancelled.Load() != 0 {
		t.Fatalf("commit após auto-invalidação: status=%s err=%v cancels=%d", got.record.Status, got.err, cancelled.Load())
	}
}

func TestEnvelopeCapabilityPendingCancellationAbortsAndRejectsLateClaim(t *testing.T) {
	f := capabilityPipelineFixture(t)
	ownership, err := NewCommitOwnership(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var cancelled atomic.Int32
	f.start = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: make(chan Outcome), Cancel: func() { cancelled.Add(1) }, CommitOwnership: ownership}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := f.service.ExecuteEnvelope(ctx, f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	<-started
	cancel()
	got := <-result
	if got.err != nil || got.record.Status != commandledger.OutcomeUnknown || cancelled.Load() != 1 {
		t.Fatalf("cancelamento pending: status=%s err=%v cancels=%d", got.record.Status, got.err, cancelled.Load())
	}
	if err := ownership.Claim(context.Background()); !errors.Is(err, ErrCommitOwnershipAborted) {
		t.Fatalf("claim tardia = %v, want ErrCommitOwnershipAborted", err)
	}
}

func TestEnvelopeForeignOrdinaryCommandIgnoresCommitOwnershipMarker(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	ownership, err := NewCommitOwnership(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan Outcome)
	f.start = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}, CommitOwnership: ownership}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := f.service.ExecuteEnvelope(ctx, f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	<-started
	cancel()
	got := <-result
	if got.err != nil || got.record.Status != commandledger.OutcomeUnknown {
		t.Fatalf("comando ordinary: status=%s err=%v", got.record.Status, got.err)
	}
	if err := ownership.Claim(context.Background()); err != nil {
		t.Fatalf("marcador foi consumido pelo caminho ordinary: %v", err)
	}
}

func TestEnvelopeCapabilityClaimedCancellationIsBoundedAndUnknownOnTimeout(t *testing.T) {
	f := capabilityPipelineFixture(t)
	ownership, err := NewCommitOwnership(40 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var cancelled atomic.Int32
	f.start = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: make(chan Outcome), Cancel: func() { cancelled.Add(1) }, CommitOwnership: ownership}, nil
	}
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	<-started
	snapshot, err := f.epochs.Capture(context.Background(), f.user, f.session)
	if err != nil {
		t.Fatal(err)
	}
	finish, err := f.epochs.BeginTransitionFromSnapshot(context.Background(), snapshot,
		func(context.Context) error { return nil }, ownership.Claim)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	got := <-result
	finish()
	if got.err != nil || got.record.Status != commandledger.OutcomeUnknown || cancelled.Load() != 1 {
		t.Fatalf("timeout de commit: status=%s err=%v cancels=%d", got.record.Status, got.err, cancelled.Load())
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("espera bounded excedeu o limite: %v", elapsed)
	}
}

func TestEnvelopeCapabilityHandlerPanicIsOutcomeUnknown(t *testing.T) {
	f := capabilityPipelineFixture(t)
	f.start = func(context.Context, Invocation) (ExecutionHandle, error) { panic("handler panic") }
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
	if !errors.Is(err, ErrExecution) || record.Status != commandledger.OutcomeUnknown {
		t.Fatalf("panic do handler: status=%s err=%v", record.Status, err)
	}
}

func TestAwaitClaimedOutcomeUsesAlreadyConsumedDoneValue(t *testing.T) {
	f := capabilityPipelineFixture(t)
	definition, ok := f.registry.Lookup("pipe.write")
	if !ok {
		t.Fatal("definição de capability ausente")
	}
	ownership, err := NewCommitOwnership(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Claim(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan Outcome, 1)
	done <- Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
	status, _ := awaitEnvelopeOutcomeWithResult(ctx, ExecutionHandle{
		ID: newTestUUID(), Done: done, Cancel: func() {}, CommitOwnership: ownership,
	}, definition)
	if status != commandledger.Succeeded {
		t.Fatalf("outcome já consumido: status=%s", status)
	}
}

func TestAwaitOwnedCancellationDoesNotWaitForInvalidHandle(t *testing.T) {
	f := capabilityPipelineFixture(t)
	definition, ok := f.registry.Lookup("pipe.write")
	if !ok {
		t.Fatal("definição de capability ausente")
	}
	ownership, err := NewCommitOwnership(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Claim(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	status, _ := awaitEnvelopeOutcomeWithResult(ctx, ExecutionHandle{CommitOwnership: ownership}, definition)
	if status != commandledger.OutcomeUnknown || time.Since(started) > 100*time.Millisecond {
		t.Fatalf("handle inválido aguardou ownership: status=%s elapsed=%v", status, time.Since(started))
	}
}

func TestAwaitClaimedOutcomeRejectsDelayedOutcomeAfterDeadline(t *testing.T) {
	f := capabilityPipelineFixture(t)
	definition, ok := f.registry.Lookup("pipe.write")
	if !ok {
		t.Fatal("definição de capability ausente")
	}
	ownership, err := NewCommitOwnership(20 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Claim(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan Outcome, 1)
	done <- Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
	status, _ := awaitEnvelopeOutcomeWithResult(ctx, ExecutionHandle{
		ID: newTestUUID(), Done: done, Cancel: func() {}, CommitOwnership: ownership,
	}, definition)
	if status != commandledger.OutcomeUnknown {
		t.Fatalf("outcome atrasado após deadline = %s, want outcome_unknown", status)
	}
}

func TestAwaitClaimedOutcomeRejectsDoneWhenTimerIsAlsoReady(t *testing.T) {
	f := capabilityPipelineFixture(t)
	definition, ok := f.registry.Lookup("pipe.write")
	if !ok {
		t.Fatal("definição de capability ausente")
	}
	ownership, err := NewCommitOwnership(20 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Claim(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Entramos com Done pronto e com o prazo vencido: o timer de duração zero
	// também está pronto, e a escolha aleatória do select não pode confirmar o
	// payload fora da janela.
	time.Sleep(30 * time.Millisecond)
	done := make(chan Outcome, 1)
	done <- Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
	var cancelled atomic.Int32
	status, _ := awaitClaimedEnvelopeOutcome(ExecutionHandle{
		ID: newTestUUID(), Done: done, Cancel: func() { cancelled.Add(1) }, CommitOwnership: ownership,
	}, definition)
	if status != commandledger.OutcomeUnknown || cancelled.Load() != 1 {
		t.Fatalf("empate timer/Done: status=%s cancels=%d", status, cancelled.Load())
	}
}
