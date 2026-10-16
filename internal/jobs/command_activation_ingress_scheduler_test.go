package jobs

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

// TestManagerIntervalIngressUsesTheStartedManagerRoute prova o caminho
// Manager.Start -> Scheduler -> Manager.executeJob -> JobExecutor. Não chama
// executor.Execute diretamente, para que o teste cubra o ingresso real.
func TestManagerIntervalIngressUsesTheStartedManagerRoute(t *testing.T) {
	testManagerSchedulerIngress(t, TriggerInterval, "100ms", "interval")
}

// TestManagerCronIngressUsesStartedManagerRoute prova o callback registrado
// pelo cron sem esperar o relógio. Entries()[0].Job.Run é o Job real criado
// pelo AddFunc do Scheduler; a execução segue até Manager e o outbox.
func TestManagerCronIngressUsesStartedManagerRoute(t *testing.T) {
	testManagerSchedulerIngress(t, TriggerCron, "* * * * *", "cron")
}

func testManagerSchedulerIngress(t *testing.T, triggerType TriggerType, spec, wantOrigin string) {
	t.Helper()
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("migrate command events: %v", err)
	}
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay epoch: %v", err)
	}
	userID, err := database.RequireUserID(userCtx)
	if err != nil {
		t.Fatal(err)
	}
	authContextID := uuid7ForTest(t)
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := epochs.Capture(userCtx, userID, authContextID)
	if err != nil {
		t.Fatal(err)
	}

	tool := &commandOriginTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	invocations := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	mgr := mustNewManager(t, ManagerConfig{
		Repository:      repo,
		ToolRegistry:    registry,
		ToolInvocations: invocations,
		ContextProvider: func() context.Context { return userCtx },
		CommandRuntimeIdentity: func(ctx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
			watch, release, err := epochs.WatchSecurityEpoch(ctx, epoch)
			if err != nil {
				return commandjobactivation.RuntimeIdentity{}, nil, nil, err
			}
			return commandjobactivation.RuntimeIdentity{
				UserID: userID, AuthContextType: "local_session", AuthContextID: authContextID,
				AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
			}, watch, release, nil
		},
	})
	runDone := make(chan *RunLog, 1)
	var runDoneOnce sync.Once
	mgr.executor.onRunEnd = func(_ string, run *RunLog) {
		runDoneOnce.Do(func() { runDone <- run })
	}

	job := testRepositoryJob(fmt.Sprintf("ingress-%s", wantOrigin), "Ingress "+wantOrigin)
	job.Triggers = []Trigger{{Type: triggerType}}
	if triggerType == TriggerCron {
		job.Triggers[0].Expression = spec
	} else {
		job.Triggers[0].Every = spec
	}
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save %s job: %v", wantOrigin, err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager for %s: %v", wantOrigin, err)
	}
	t.Cleanup(mgr.Stop)

	if triggerType == TriggerCron {
		entries := mgr.scheduler.cron.Entries()
		if len(entries) != 1 {
			t.Fatalf("cron ingress registered %d entries, want 1", len(entries))
		}
		entries[0].Job.Run()
	}

	var run *RunLog
	select {
	case run = <-runDone:
		mgr.scheduler.Unschedule(job.ID)
	case <-time.After(5 * time.Second):
		t.Fatalf("manager %s ingress did not produce a terminal run", wantOrigin)
	}
	if run.Trigger.Type != triggerType || string(run.Trigger.Type) != wantOrigin {
		t.Fatalf("%s ingress recorded trigger=%+v, want %s", wantOrigin, run.Trigger, wantOrigin)
	}
	if run.Status != RunStatusCompleted {
		t.Fatalf("%s ingress run did not complete: %+v", wantOrigin, run)
	}

	var outbox []commandjobevents.ActivationOutbox
	if err := repo.db.Where("run_id = ?", run.RunID).Order("sequence ASC").Find(&outbox).Error; err != nil {
		t.Fatalf("load %s activation outbox: %v", wantOrigin, err)
	}
	if len(outbox) == 0 {
		t.Fatalf("%s ingress completed without activation outbox", wantOrigin)
	}
	for _, row := range outbox {
		if row.RootOriginType != wantOrigin || row.RootOriginID != run.RunID {
			t.Fatalf("%s ingress outbox origin=%q/%q, want %q/%q", wantOrigin, row.RootOriginType, row.RootOriginID, wantOrigin, run.RunID)
		}
	}
	states := map[string]bool{}
	for _, row := range outbox {
		states[row.State] = true
	}
	for _, state := range []string{commandjobevents.StateQueued, commandjobevents.StateStarted, commandjobevents.StateCompleted} {
		if !states[state] {
			t.Fatalf("%s ingress outbox missing state %q: %#v", wantOrigin, state, outbox)
		}
	}
}
