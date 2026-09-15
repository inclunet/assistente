package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

func TestCoreDrainWaitsForRegisteredExecutorFinalizationIncludingServiceCopy(t *testing.T) {
	f := newExecutionFixture(t)
	copyOfService := *f.service // cópia conserva o mesmo lifecycle registrado no core
	started, finalizing, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	f.startHook = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: make(chan Outcome), Cancel: func() {}}, nil
	}
	if err := f.db.Callback().Update().Before("gorm:update").Register("test:hold_core_drain", func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if ok && tx.Statement.Table == "command_idempotency_keys" && values["status"] == commandledger.OutcomeUnknown {
			close(finalizing)
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	epoch, err := f.epochs.Capture(context.Background(), f.user.ID, f.pair.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	request := f.request(t, "workspace.read")
	executed := make(chan error, 1)
	go func() {
		_, err := copyOfService.Execute(context.Background(), f.pair.AccessToken, request)
		executed <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler não iniciou")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	proof, err := f.epochs.CloseAndDrain(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || proof.Includes(epoch.SecurityGeneration) {
		close(release)
		t.Fatalf("drain prematuro=%v", err)
	}
	select {
	case <-finalizing:
	default:
		close(release)
		t.Fatal("finalização não iniciou")
	}
	if _, err := New(f.service.config); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		close(release)
		t.Fatalf("novo executor após fechamento=%v", err)
	}
	close(release)
	if err := <-executed; err != nil {
		t.Fatal(err)
	}
	proof, err = f.epochs.CloseAndDrain(context.Background())
	if err != nil || !proof.Includes(epoch.SecurityGeneration) {
		t.Fatalf("sem prova após drain=%v", err)
	}
	user := f.user.ID
	sealed, err := f.store.SealDrainedGeneration(context.Background(), proof, commandledger.GenerationScope{UserID: &user, AuthContextType: "local_session", AuthContextID: f.pair.SessionID, SecurityGeneration: epoch.SecurityGeneration})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := f.store.RecoverClosedGenerationWithProof(context.Background(), sealed, 128)
	if err != nil || batch.Processed != 0 {
		t.Fatalf("recuperação repetiu finalização: %+v %v", batch, err)
	}
	if f.record(t, request).Status != commandledger.OutcomeUnknown {
		t.Fatal("status final incorreto")
	}
}

func TestCoreDrainAllowsCommonRecoveryAfterFinalizationWriteFailure(t *testing.T) {
	f := newExecutionFixture(t)
	epoch, err := f.epochs.Capture(context.Background(), f.user.ID, f.pair.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("CREATE TRIGGER fail_final_write BEFORE UPDATE ON command_invocations WHEN NEW.status = 'succeeded' BEGIN SELECT RAISE(ABORT, 'final write failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	request := f.request(t, "workspace.read")
	if _, err := f.service.Execute(context.Background(), f.pair.AccessToken, request); err == nil {
		t.Fatal("falha de finalização ocultada")
	}
	proof, err := f.epochs.CloseAndDrain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	user := f.user.ID
	sealed, err := f.store.SealDrainedGeneration(context.Background(), proof, commandledger.GenerationScope{UserID: &user, AuthContextType: "local_session", AuthContextID: f.pair.SessionID, SecurityGeneration: epoch.SecurityGeneration})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := f.store.RecoverClosedGenerationWithProof(context.Background(), sealed, 1)
	if err != nil || batch.Processed != 1 || batch.More {
		t.Fatalf("recovery=%+v %v", batch, err)
	}
	if got := f.record(t, request).Status; got != commandledger.OutcomeUnknown {
		t.Fatalf("status=%s", got)
	}
	if f.startCalls.Load() != 1 {
		t.Fatalf("efeito reexecutado: %d", f.startCalls.Load())
	}
}
