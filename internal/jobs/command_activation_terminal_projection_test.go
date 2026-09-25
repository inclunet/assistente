package jobs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

func TestCommandRuntimeEndsBeforeTerminalPersistenceCallback(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userID := uuid7ForTest(t)
	userCtx := database.WithUserID(context.Background(), userID)
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("automigrate command job events: %v", err)
	}
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay policy epoch: %v", err)
	}
	if err := repo.db.Create(&database.ToolCatalog{
		Name: "job_tool", DisplayName: "job_tool", Origin: tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	registry := tools.NewRegistry()
	registry.MustRegister(simpleTool{})
	invocations := toolinvocations.NewService(
		toolinvocations.NewDBRepository(repo.db),
		tools.NewExecutor(registry, tools.DefaultExecutorConfig()),
	)
	identity := commandjobactivation.RuntimeIdentity{
		UserID: userID, AuthContextType: "local_session", AuthContextID: uuid7ForTest(t),
		AuthGeneration: "auth-a", SecurityGeneration: "security-a",
	}
	manager := NewManager(ManagerConfig{
		Repository: repo, ToolRegistry: registry, ToolInvocations: invocations,
		CommandRuntimeIdentity: func(context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
			return identity, context.Background(), func() {}, nil
		},
	})
	manager.commandRuntimeAccepting = true
	manager.commandRuntime = make(map[string]commandRuntimeEntry)

	var capturedRunID string
	var capturedIdentity commandjobactivation.RuntimeIdentity
	manager.executor.onCommandRuntimeStart = func(runID string, got commandjobactivation.RuntimeIdentity, watch context.Context, token uint64) bool {
		capturedRunID, capturedIdentity = runID, got
		return manager.registerCommandRuntime(runID, got, watch, token)
	}
	var endCount atomic.Int32
	endObserved := make(chan struct{})
	var endSignalOnce sync.Once
	manager.executor.onCommandRuntimeEnd = func(runID string) {
		endCount.Add(1)
		manager.unregisterCommandRuntime(runID)
		endSignalOnce.Do(func() { close(endObserved) })
	}
	manager.executor.onRunStart = nil
	runEndEntered := make(chan struct{})
	releaseRunEnd := make(chan struct{})
	var releaseRunEndOnce sync.Once
	releaseRunEndFn := func() { releaseRunEndOnce.Do(func() { close(releaseRunEnd) }) }
	var runEndSignalOnce sync.Once
	manager.executor.onRunEnd = func(string, *RunLog) {
		runEndSignalOnce.Do(func() { close(runEndEntered) })
		<-releaseRunEnd
	}

	job := testRepositoryJob("terminal-projection", "terminal projection")
	job.Tool = "job_tool"
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	job, err := repo.GetJob(userCtx, job.ID)
	if err != nil {
		t.Fatalf("reload job: %v", err)
	}

	runResult := make(chan *RunLog, 1)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		runResult <- manager.executor.Execute(userCtx, job, &TriggerContext{Type: TriggerManual})
	}()
	t.Cleanup(func() {
		releaseRunEndFn()
		select {
		case <-workerDone:
		case <-time.After(5 * time.Second):
			t.Error("worker do executor não terminou durante cleanup")
		}
	})
	select {
	case <-runEndEntered:
	case <-time.After(5 * time.Second):
		select {
		case result := <-runResult:
			t.Fatalf("Execute terminou antes de OnRunEnd: %+v end=%d", result, endCount.Load())
		default:
		}
		t.Fatal("OnRunEnd não entrou no callback terminal")
	}
	select {
	case <-endObserved:
	default:
		t.Fatal("fim do runtime não foi publicado antes de OnRunEnd")
	}
	if endCount.Load() != 1 {
		t.Fatalf("callback de fim chamado antes do callback terminal %d vezes", endCount.Load())
	}
	if capturedRunID == "" {
		t.Fatal("runtime não foi capturado")
	}
	if err := manager.ValidateCommandRuntimeProjection(context.Background(), capturedRunID, capturedIdentity); err == nil {
		t.Fatal("projeção runtime continuou válida durante OnRunEnd bloqueado")
	}

	var terminalOutbox int64
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).
		Where("run_id = ? AND state = ?", capturedRunID, commandjobevents.StateCompleted).
		Count(&terminalOutbox).Error; err != nil {
		t.Fatalf("contar outbox terminal: %v", err)
	}
	if terminalOutbox != 1 {
		t.Fatalf("outbox terminal antes do join = %d, esperado 1", terminalOutbox)
	}

	releaseRunEndFn()
	select {
	case result := <-runResult:
		if result == nil || result.Status != RunStatusCompleted {
			t.Fatalf("resultado terminal=%+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Execute não terminou após liberar OnRunEnd")
	}
	if endCount.Load() != 1 {
		t.Fatalf("callback de fim chamado %d vezes", endCount.Load())
	}
}
