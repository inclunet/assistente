package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandledger"
	"gorm.io/gorm"
)

func TestShutdownWaitsForWholeOperationAndCanResumeAfterTimeout(t *testing.T) {
	s := &Service{}
	runCtx, release, err := s.lifecycle.enter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown before release=%v", err)
	}
	if runCtx.Err() != context.Canceled {
		t.Fatalf("operation not cancelled: %v", runCtx.Err())
	}
	if _, _, err := s.lifecycle.enter(context.Background()); !errors.Is(err, ErrServiceClosed) {
		t.Fatalf("reopened=%v", err)
	}
	release()
	release()
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownCancelsRunningExecutionAndWaitsForLedgerFinalization(t *testing.T) {
	f := newExecutionFixture(t)
	started := make(chan struct{})
	f.startHook = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: make(chan Outcome), Cancel: func() { f.cancelCalls.Add(1) }}, nil
	}
	request := f.request(t, "workspace.read")
	done := make(chan error, 1)
	go func() { _, err := f.service.Execute(context.Background(), f.pair.AccessToken, request); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.service.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := f.record(t, request).Status; got != commandledger.OutcomeUnknown {
		t.Fatalf("final state=%s", got)
	}
	if f.cancelCalls.Load() != 1 {
		t.Fatalf("cancel calls=%d", f.cancelCalls.Load())
	}
	if _, err := f.service.Execute(context.Background(), f.pair.AccessToken, request); !errors.Is(err, ErrServiceClosed) {
		t.Fatalf("execution after shutdown=%v", err)
	}
}

func TestShutdownRejectsEnvelopeBeforeAnyHostCallback(t *testing.T) {
	s := &Service{complete: true, config: Config{Envelope: &EnvelopeConfig{}}}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteEnvelope(context.Background(), "", EnvelopeCandidate{}); !errors.Is(err, ErrServiceClosed) {
		t.Fatalf("envelope after shutdown=%v", err)
	}
}

func TestShutdownTracksPreparationBeforeAnInvocationExists(t *testing.T) {
	f := newExecutionFixture(t)
	entered := make(chan struct{})
	unblock := make(chan struct{})
	f.snapshotHook = func(_ int, v Versions) (Versions, error) {
		close(entered)
		<-unblock
		return v, nil
	}
	request := f.request(t, "workspace.read")
	done := make(chan error, 1)
	go func() { _, err := f.service.Execute(context.Background(), f.pair.AccessToken, request); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("preparation did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := f.service.Shutdown(ctx)
	close(unblock)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("preparation not drained: %v", err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("preparation after shutdown=%v", err)
	}
	if err := f.service.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	var rows int64
	if err := f.db.Table("command_invocations").Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 || f.startCalls.Load() != 0 {
		t.Fatalf("late reservation=%d start=%d", rows, f.startCalls.Load())
	}
}

func TestShutdownDoesNotFinishBeforeFinalCAS(t *testing.T) {
	f := newExecutionFixture(t)
	started := make(chan struct{})
	finalizing := make(chan struct{})
	allowFinalization := make(chan struct{})
	f.startHook = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: make(chan Outcome), Cancel: func() {}}, nil
	}
	if err := f.db.Callback().Update().Before("gorm:update").Register("test:block_finalization", func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if ok && tx.Statement.Table == "command_idempotency_keys" && values["status"] == commandledger.OutcomeUnknown {
			close(finalizing)
			<-allowFinalization
		}
	}); err != nil {
		t.Fatal(err)
	}
	request := f.request(t, "workspace.read")
	executed := make(chan error, 1)
	go func() {
		_, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
		executed <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- f.service.Shutdown(ctx) }()
	select {
	case <-finalizing:
	case <-time.After(5 * time.Second):
		close(allowFinalization)
		t.Fatal("no final CAS")
	}
	select {
	case err := <-stopped:
		close(allowFinalization)
		t.Fatalf("shutdown passed pending CAS: %v", err)
	default:
	}
	close(allowFinalization)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if err := <-executed; err != nil {
		t.Fatal(err)
	}
	if got := f.record(t, request).Status; got != commandledger.OutcomeUnknown {
		t.Fatalf("final=%s", got)
	}
}

// Mede somente o registro em memória adicionado ao ingresso, não latência
// ponta a ponta dos atalhos nem custo de banco/handler.
func BenchmarkExecutionLifecycle(b *testing.B) {
	var lifecycle executionLifecycle
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_, release, err := lifecycle.enter(ctx)
		if err != nil {
			b.Fatal(err)
		}
		release()
	}
}
