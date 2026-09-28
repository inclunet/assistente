package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func prepareCommandJobsResume(t *testing.T, a *App) {
	t.Helper()
	a.authSessionMu.Lock()
	result := a.reloadUserScopedRuntime()
	a.authSessionMu.Unlock()
	if result.hasFailures() || a.commandJobsPending == nil {
		t.Fatalf("preparação de jobs: failures=%+v pending=%+v", result.failures, a.commandJobsPending)
	}
}

func TestCommandJobsResumeAfterFirstKnownOSSession(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	ctx := context.Background()
	prepareCommandJobsResume(t, a)
	if err := a.commandHost.SetOSSessionState(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	if err := a.bootstrapCommandsAndStartPreparedJobs(ctx, a.currentAuthUser, nil); err == nil {
		t.Fatal("bootstrap com SO desconhecido liberou jobs")
	}
	if observer.starts.Load() != 0 || a.commandJobsPending == nil {
		t.Fatal("SO desconhecido iniciou jobs ou consumiu pendência")
	}
	if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	a.bootstrapCommandLifecycleAfterOSUnlock(ctx, a.commandHost)
	if observer.starts.Load() != 1 || observer.beforeReady.Load() || a.commandJobsPending != nil {
		t.Fatalf("unlock: starts=%d beforeReady=%v pending=%+v", observer.starts.Load(), observer.beforeReady.Load(), a.commandJobsPending)
	}
	a.bootstrapCommandLifecycleAfterOSUnlock(ctx, a.commandHost)
	if observer.starts.Load() != 1 {
		t.Fatal("observação repetida do SO reiniciou jobs")
	}
}

func TestCommandJobsResumeRetriesFailedStart(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	prepareCommandJobsResume(t, a)
	pending := *a.commandJobsPending
	injected := errors.New("falha única de reconciliação no Start")
	var failed atomic.Bool
	db := database.DB()
	const hook = "test:jobs_resume_start_failure"
	if err := db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "jobs" && strings.Contains(tx.Statement.SQL.String(), "TRIM(jobs.tool_name)") && failed.CompareAndSwap(false, true) {
			_ = tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.jobMgr.Stop()
		_ = db.Callback().Query().Remove(hook)
	})
	ctx := context.Background()
	if err := a.bootstrapCommandsAndStartPreparedJobs(ctx, a.currentAuthUser, nil); !errors.Is(err, injected) {
		t.Fatalf("Start não propagou falha injetada: %v", err)
	}
	if !failed.Load() || observer.starts.Load() != 1 || observer.beforeReady.Load() {
		t.Fatalf("tentativa inicial: injected=%v starts=%d beforeReady=%v", failed.Load(), observer.starts.Load(), observer.beforeReady.Load())
	}
	if a.commandJobsPending == nil || *a.commandJobsPending != pending {
		t.Fatal("Start falho perdeu ou alterou a sessão pendente")
	}
	if err := a.startPreparedJobsAfterCommands(ctx, a.currentAuthUser); err != nil {
		t.Fatal(err)
	}
	if observer.starts.Load() != 2 || observer.beforeReady.Load() || a.commandJobsPending != nil {
		t.Fatalf("retry: starts=%d beforeReady=%v pending=%+v", observer.starts.Load(), observer.beforeReady.Load(), a.commandJobsPending)
	}
	if err := a.startPreparedJobsAfterCommands(ctx, a.currentAuthUser); err != nil {
		t.Fatal(err)
	}
	if observer.starts.Load() != 2 {
		t.Fatal("retry já consumido voltou a chamar Start")
	}
}

func TestCommandJobsGenericBootstrapDoesNotReenterAuthSessionMutex(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	prepareCommandJobsResume(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Mutações de perfil chamam o bootstrap genérico segurando authSessionMu.
	// Manter o lock aqui também permite soltá-lo no caminho de falha do teste.
	a.authSessionMu.Lock()
	done := make(chan struct{})
	go func() {
		a.bootstrapCommandLifecycleAfterAuth(ctx, a.currentAuthUser, nil)
		close(done)
	}()
	select {
	case <-done:
		a.authSessionMu.Unlock()
	case <-ctx.Done():
		a.authSessionMu.Unlock()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("bootstrap não terminou mesmo após liberar authSessionMu")
		}
		t.Fatal("bootstrap genérico aguardou authSessionMu já adquirido pelo chamador")
	}
	snapshot, err := CommandLifecycleSnapshot(a)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		t.Fatalf("bootstrap genérico terminou sem publicar readiness: snapshot=%+v err=%v", snapshot, err)
	}
	if observer.starts.Load() != 0 || a.commandJobsPending == nil {
		t.Fatal("bootstrap genérico iniciou jobs ou consumiu pendência de startup")
	}
	if err := a.startPreparedJobsAfterCommands(context.Background(), a.currentAuthUser); err != nil {
		t.Fatal(err)
	}
	if observer.starts.Load() != 1 || observer.beforeReady.Load() || a.commandJobsPending != nil {
		t.Fatal("liberação explícita após bootstrap não iniciou jobs prontos uma única vez")
	}
}
