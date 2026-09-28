package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"gorm.io/gorm"
)

type startupOrderJobsRepository struct {
	app         *App
	starts      atomic.Int32
	beforeReady atomic.Bool
}

func (r *startupOrderJobsRepository) observeStart(tx *gorm.DB) {
	if tx.Statement.Table != "jobs" {
		return
	}
	// ReconcileUnauthorizedJobs é a primeira operação de Start. ListJobs
	// também serve ao catálogo e não comprova início do scheduler.
	if !strings.Contains(tx.Statement.SQL.String(), "TRIM(jobs.tool_name)") {
		return
	}
	r.starts.Add(1)
	snapshot, err := CommandLifecycleSnapshot(r.app)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		r.beforeReady.Store(true)
	}
}

func commandJobsStartupFixture(t *testing.T) (*App, *startupOrderJobsRepository) {
	t.Helper()
	a := commandMaintenanceAppFixture(t)
	repository := &startupOrderJobsRepository{app: a}
	const hook = "test:jobs_startup_order"
	if err := database.DB().Callback().Query().After("gorm:query").Register(hook, repository.observeStart); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.jobMgr.Stop()
		_ = database.DB().Callback().Query().Remove(hook)
	})
	return a, repository
}

func TestCommandJobsWaitForBootstrapAfterRuntimeReload(t *testing.T) {
	a, repository := commandJobsStartupFixture(t)
	ctx := context.Background()
	if err := ResetCommandLifecycle(ctx, a, "startup_test"); err != nil {
		t.Fatal(err)
	}
	a.authSessionMu.Lock()
	result := a.reloadUserScopedRuntime()
	a.authSessionMu.Unlock()
	if result.hasFailures() {
		t.Fatalf("reload: %+v", result.failures)
	}
	if a.commandMaintenance.Load() == nil || a.commandJobsPending == nil {
		t.Fatal("jobs não preparados")
	}
	if repository.starts.Load() != 0 {
		t.Fatal("reload iniciou jobs antes dos comandos")
	}
	if err := a.startPreparedJobsAfterCommands(ctx, a.currentAuthUser); err == nil {
		t.Fatal("runtime frio liberou jobs")
	}
	if err := a.bootstrapCommandsAndStartPreparedJobs(ctx, a.currentAuthUser, nil); err != nil {
		t.Fatal(err)
	}
	if repository.starts.Load() != 1 || repository.beforeReady.Load() {
		t.Fatalf("Start não sucedeu à publicação: starts=%d beforeReady=%v", repository.starts.Load(), repository.beforeReady.Load())
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("mapa após Start: %v", err)
	}
	if a.commandJobsPending != nil {
		t.Fatal("pendência não consumida")
	}
	if err := a.startPreparedJobsAfterCommands(ctx, a.currentAuthUser); err != nil {
		t.Fatal(err)
	}
	if repository.starts.Load() != 1 {
		t.Fatal("jobs iniciados duas vezes")
	}
}

func TestCommandJobsCancelledBootstrapKeepsPendingUntilRetry(t *testing.T) {
	a, repository := commandJobsStartupFixture(t)
	a.authSessionMu.Lock()
	a.reloadUserScopedRuntime()
	a.authSessionMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.bootstrapCommandsAndStartPreparedJobs(ctx, a.currentAuthUser, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento: %v", err)
	}
	if repository.starts.Load() != 0 || a.commandJobsPending == nil {
		t.Fatal("bootstrap cancelado perdeu pendência ou iniciou jobs")
	}
	if err := a.bootstrapCommandsAndStartPreparedJobs(context.Background(), a.currentAuthUser, nil); err != nil {
		t.Fatal(err)
	}
	if repository.starts.Load() != 1 || repository.beforeReady.Load() {
		t.Fatal("retry não respeitou readiness")
	}
}

func TestCommandJobsRejectStaleSession(t *testing.T) {
	a, repository := commandJobsStartupFixture(t)
	a.authSessionMu.Lock()
	a.reloadUserScopedRuntime()
	a.authSessionMu.Unlock()
	stale := *a.currentAuthUser
	stale.SessionID = "sessao-antiga"
	if err := a.startPreparedJobsAfterCommands(context.Background(), &stale); err == nil {
		t.Fatal("sessão antiga iniciou jobs")
	}
	if repository.starts.Load() != 0 {
		t.Fatal("Start chamado por sessão antiga")
	}
}

func TestCommandJobsRuntimeRetryBootstrapsBeforeStart(t *testing.T) {
	a, repository := commandJobsStartupFixture(t)
	result, err := a.RetryUserRuntimeInit()
	if err != nil || len(result.Subsystems) != 0 {
		t.Fatalf("retry: %+v %v", result, err)
	}
	if repository.starts.Load() != 1 || repository.beforeReady.Load() {
		t.Fatal("retry iniciou jobs sem comandos")
	}
}

func TestCommandJobsLegacyStartIsNotReportedAsBootstrapFailure(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	// Sem armazenamento habilitado, preservamos o fluxo legado de jobs.
	a.commandStorageVersion = ""
	a.authSessionMu.Lock()
	result := a.reloadUserScopedRuntime()
	a.authSessionMu.Unlock()
	if result.hasFailures() || observer.starts.Load() != 1 || a.commandJobsPending != nil {
		t.Fatalf("legacy: failures=%+v starts=%d pending=%v", result.failures, observer.starts.Load(), a.commandJobsPending)
	}
	a.addPendingJobsStartupFailure(&result, a.currentAuthUser, errors.New("bootstrap de comandos indisponível"))
	if result.hasFailures() {
		t.Fatal("jobs legados em execução reportados como falha")
	}
}

func TestCommandJobsPendingStartupFailureIsReportedOnce(t *testing.T) {
	a, _ := commandJobsStartupFixture(t)
	a.authSessionMu.Lock()
	result := a.reloadUserScopedRuntime()
	a.authSessionMu.Unlock()
	if result.hasFailures() || a.commandJobsPending == nil {
		t.Fatal("jobs não preparados")
	}
	for i := 0; i < 2; i++ {
		a.addPendingJobsStartupFailure(&result, a.currentAuthUser, errors.New("bootstrap indisponível"))
	}
	if len(result.failures) != 1 || result.failures[0].Subsystem != runtimeSubsystemJobs {
		t.Fatalf("failures=%+v", result.failures)
	}
}

func TestCommandJobsMountedMaintenanceStorageFailureDoesNotDowngrade(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	prepareCommandJobsResume(t, a)
	if a.commandMaintenance.Load() == nil {
		t.Fatal("maintenance was not mounted")
	}
	version := a.commandStorageVersion
	a.authMu.Lock()
	a.commandStorageVersion = ""
	a.commandStorageErr = errors.New("storage temporarily unavailable")
	a.authMu.Unlock()
	a.authSessionMu.Lock()
	result := a.reloadUserScopedRuntime()
	a.authSessionMu.Unlock()
	if !result.hasFailures() || observer.starts.Load() != 0 || a.commandJobsPending != nil {
		t.Fatalf("storage failure downgraded maintenance: failures=%+v starts=%d pending=%v", result.failures, observer.starts.Load(), a.commandJobsPending)
	}
	found := false
	for _, failure := range result.failures {
		found = found || failure.Subsystem == runtimeSubsystemJobs
	}
	if !found {
		t.Fatal("storage failure did not report unavailable jobs")
	}
	a.authMu.Lock()
	a.commandStorageVersion = version
	a.commandStorageErr = nil
	a.authMu.Unlock()
	prepareCommandJobsResume(t, a)
	if err := a.bootstrapCommandsAndStartPreparedJobs(context.Background(), a.currentAuthUser, nil); err != nil {
		t.Fatal(err)
	}
	if observer.starts.Load() != 1 || observer.beforeReady.Load() || a.commandJobsPending != nil {
		t.Fatal("recovered storage did not start jobs after readiness")
	}
}
