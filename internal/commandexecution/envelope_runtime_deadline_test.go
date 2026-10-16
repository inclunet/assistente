package commandexecution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandledger"
)

func newRuntimeOwnedJobService(t *testing.T, f *envelopePipelineFixture, start func(context.Context, Invocation) (ExecutionHandle, error)) *Service {
	t.Helper()
	definition, ok := f.registry.Lookup("pipe.write")
	if !ok {
		t.Fatal("definição pipe.write ausente")
	}
	definition.HandlerClassification = commandcatalog.HandlerJob
	contract := pipelineContract(definition.Effect, definition.HasMutableTarget, definition.HandlerRoute)
	contract.Classification = commandcatalog.HandlerJob
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: contract}})
	if err != nil {
		t.Fatal(err)
	}
	config := f.service.config
	config.Registry = registry
	config.Handlers = map[string]Handler{
		definition.ID: {
			Contract:            contract,
			ExecutionTimeout:    5 * time.Minute,
			RuntimeOwnsDeadline: true,
			Start:               start,
		},
	}
	config.ExecutionTimeout = 5 * time.Minute
	service, err := NewComplete(config)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestEnvelopeJobRuntimeOwnsDeadlinePreservesCallerDeadline(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	started := make(chan context.Context, 1)
	service := newRuntimeOwnedJobService(t, f, func(ctx context.Context, _ Invocation) (ExecutionHandle, error) {
		started <- ctx
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	})

	caller, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	record, err := service.ExecuteEnvelope(caller, f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	runtimeCtx := <-started
	deadline, ok := runtimeCtx.Deadline()
	if !ok {
		t.Fatal("runtime de job perdeu o deadline original do caller")
	}
	remaining := time.Until(deadline)
	if remaining <= 5*time.Minute || remaining > 10*time.Minute {
		t.Fatalf("deadline do runtime=%v, esperado caller >5m e <=10m", remaining)
	}
}

func TestEnvelopeJobRuntimePreservesShorterCallerDeadline(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	started := make(chan context.Context, 1)
	service := newRuntimeOwnedJobService(t, f, func(ctx context.Context, _ Invocation) (ExecutionHandle, error) {
		started <- ctx
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	})

	caller, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	record, err := service.ExecuteEnvelope(caller, f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	deadline, ok := (<-started).Deadline()
	if !ok {
		t.Fatal("runtime não recebeu deadline do caller")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 30*time.Second {
		t.Fatalf("deadline curto do runtime=%v, esperado <=30s", remaining)
	}
}

func TestEnvelopeJobRuntimeFollowsLifecycleCancellation(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	started := make(chan context.Context, 1)
	done := make(chan Outcome)
	var cancels atomic.Int32
	service := newRuntimeOwnedJobService(t, f, func(ctx context.Context, _ Invocation) (ExecutionHandle, error) {
		started <- ctx
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() { cancels.Add(1) }}, nil
	})
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record: record, err: err}
	}()
	var runtimeCtx context.Context
	select {
	case runtimeCtx = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("job não alcançou Start")
	}
	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runtimeCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle não cancelou o runtime do job")
	}
	select {
	case outcome := <-result:
		if outcome.err != nil || outcome.record.Status != commandledger.OutcomeUnknown {
			t.Fatalf("resultado após shutdown=%+v", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execução não terminou após shutdown")
	}
	if got := cancels.Load(); got != 1 {
		t.Fatalf("Cancel chamado %d vezes, esperado 1", got)
	}
}

func TestEnvelopeJobRuntimeFollowsCallerCancellationAfterStart(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	started := make(chan context.Context, 1)
	done := make(chan Outcome)
	service := newRuntimeOwnedJobService(t, f, func(ctx context.Context, _ Invocation) (ExecutionHandle, error) {
		started <- ctx
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}, nil
	})
	caller, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := service.ExecuteEnvelope(caller, f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record: record, err: err}
	}()
	var runtimeCtx context.Context
	select {
	case runtimeCtx = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("job não alcançou Start")
	}
	cancel()
	select {
	case <-runtimeCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("cancelamento do caller não chegou ao runtime")
	}
	select {
	case outcome := <-result:
		if outcome.err != nil || outcome.record.Status != commandledger.OutcomeUnknown {
			t.Fatalf("resultado após cancelamento=%+v", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execução não terminou após cancelamento")
	}
}

func TestRuntimeOwnsDeadlineRejectsLegacyAndNonJobHandlers(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	legacy := f.service.config
	handler := legacy.Handlers["pipe.write"]
	handler.RuntimeOwnsDeadline = true
	legacy.Handlers["pipe.write"] = handler
	if _, err := New(legacy); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("New aceitou RuntimeOwnsDeadline: %v", err)
	}

	complete := f.service.config
	handler = complete.Handlers["pipe.write"]
	handler.RuntimeOwnsDeadline = true
	complete.Handlers["pipe.write"] = handler
	if _, err := NewComplete(complete); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewComplete aceitou opt-in não-job: %v", err)
	}
}

func TestEnvelopeJobExpiredQueueNeverStarts(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	var starts atomic.Int32
	service := newRuntimeOwnedJobService(t, f, func(context.Context, Invocation) (ExecutionHandle, error) {
		starts.Add(1)
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	})
	jobHandler := service.config.Handlers["pipe.write"]
	jobHandler.ExecutionTimeout = time.Second
	service.config.Handlers["pipe.write"] = jobHandler
	caller := context.Background()
	queueEntered := make(chan struct{})
	service.config.Envelope.AwaitQueue = func(ctx context.Context, _ commandcontract.Envelope) error {
		close(queueEntered)
		<-ctx.Done()
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ctx.Err()
		}
		return nil
	}

	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := service.ExecuteEnvelope(caller, f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record: record, err: err}
	}()
	select {
	case <-queueEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("fila não foi alcançada")
	}
	select {
	case outcome := <-result:
		if outcome.err != nil || outcome.record.Status != commandledger.CancelledStale {
			t.Fatalf("resultado após expiração na fila=%+v", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execução não terminou após expiração na fila")
	}
	if got := starts.Load(); got != 0 {
		t.Fatalf("Start ocorreu após expiração na fila: %d", got)
	}
}
