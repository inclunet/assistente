package jobs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

func TestCommandDomainEpochDriftInvalidatesRunBeforeQueued(t *testing.T) {
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
	sessionID := uuid7ForTest(t)
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epochA, err := epochs.Capture(userCtx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	identityA := commandjobactivation.RuntimeIdentity{
		UserID: userID, AuthContextType: "local_session", AuthContextID: sessionID,
		AuthGeneration: epochA.AuthGeneration, SecurityGeneration: epochA.SecurityGeneration,
	}
	identityB := identityA
	identityB.SecurityGeneration = identityA.SecurityGeneration + "-next"
	var runtimeCalls atomic.Int32

	tool := &commandOriginTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	invocations := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	mgr := mustNewManager(t, ManagerConfig{
		Repository: repo, ToolRegistry: registry, ToolInvocations: invocations,
		ContextProvider: func() context.Context { return userCtx },
		CommandRuntimeIdentity: func(ctx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
			call := runtimeCalls.Add(1)
			identity := identityA
			if call >= 3 {
				identity = identityB
			}
			watch, release, err := epochs.WatchSecurityEpoch(ctx, epochA)
			if err != nil {
				return commandjobactivation.RuntimeIdentity{}, nil, nil, err
			}
			return identity, watch, release, nil
		},
	})
	t.Cleanup(mgr.eventBus.Close)

	job := testRepositoryJob("epoch-drift-job", "Epoch drift")
	job.Triggers = []Trigger{{Type: TriggerEvent, Listen: "tasklist.task.updated"}}
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("persist job: %v", err)
	}
	if _, err := repo.GetJob(userCtx, job.ID); err != nil {
		t.Fatalf("verify persisted job: %v", err)
	}
	mgr.registry.Set(job)
	parentSeen := make(chan *TriggerContext, 1)
	childSeen := make(chan *TriggerContext, 1)
	parentDone := make(chan struct{}, 1)
	var once sync.Once

	mgr.eventBus.Subscribe("tasklist.task.updated", "epoch-parent", func(ctx context.Context, _ string, _ map[string]any) {
		trigger := &TriggerContext{Type: TriggerEvent}
		inheritCommandEventOrigin(ctx, trigger)
		origin, ok := ctx.Value(commandEventOriginKey{}).(commandEventOrigin)
		if !ok || origin.runtimeIdentity == nil || *origin.runtimeIdentity != identityA {
			t.Errorf("listener perdeu runtime identity A: ok=%v origin=%+v", ok, origin)
		}
		if trigger.RootOriginType != "internal_event" || trigger.RootOriginID == "" {
			t.Errorf("listener não herdou raiz privada A: %+v", trigger)
		}
		parentSeen <- trigger

		// O sink preserva o marker privado existente. A publicação filha não
		// recebe root/provenance de payload e deve observar a troca para B.
		if err := mgr.TasklistDomainEventSink().PublishDomainEvent(ctx, "tasklist.task.created", map[string]any{"task_id": "child"}); err != nil {
			t.Errorf("publicação filha: %v", err)
		}
		// O executor real é o boundary sob teste; usar diretamente aqui evita
		// que a pré-condição de registro/enable do Manager mascare a janela de
		// runtime que este teste precisa observar.
		origin, ok = ctx.Value(commandEventOriginKey{}).(commandEventOrigin)
		if !ok {
			t.Errorf("listener perdeu marker para o executor")
			return
		}
		runCtx := context.WithValue(userCtx, commandEventOriginKey{}, origin)
		run := mgr.executor.Execute(runCtx, job, trigger)
		t.Logf("epoch run status=%s error=%q run_id=%s", run.Status, run.Error, run.RunID)
		once.Do(func() { parentDone <- struct{}{} })
	})
	mgr.eventBus.Subscribe("tasklist.task.created", "epoch-child", func(ctx context.Context, _ string, _ map[string]any) {
		trigger := &TriggerContext{Type: TriggerEvent}
		inheritCommandEventOrigin(ctx, trigger)
		childSeen <- trigger
	})

	if err := mgr.TasklistDomainEventSink().PublishDomainEvent(userCtx, "tasklist.task.updated", map[string]any{"task_id": "parent"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-parentDone:
	case <-time.After(2 * time.Second):
		t.Fatal("executor não concluiu o run do listener")
	}
	select {
	case child := <-childSeen:
		if child.RootOriginType != "unknown" || child.RootOriginID != "" {
			t.Fatalf("child event promoveu epoch stale: %+v", child)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener filho não recebeu evento")
	}
	select {
	case parent := <-parentSeen:
		if parent.RootOriginType != "internal_event" || parent.RootOriginID == "" {
			t.Fatalf("parent listener não passou epoch A: %+v", parent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener pai não recebeu evento")
	}

	runs, err := repo.GetRuns(userCtx, job.ID, 1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("run após drift = len %d err %v", len(runs), err)
	}
	run := runs[0]
	if run.RootOriginType != "unknown" || run.RootOriginID != "" {
		t.Fatalf("run stale não ficou unknown: (%q,%q)", run.RootOriginType, run.RootOriginID)
	}
	var outboxCount int64
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("run_id = ?", run.RunID).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if outboxCount != 0 {
		t.Fatalf("run stale produziu outbox ativável: %d", outboxCount)
	}
	if runtimeCalls.Load() < 4 {
		t.Fatalf("runtime identity não foi revalidado no encadeamento: %d chamadas", runtimeCalls.Load())
	}
}

func TestInheritCommandEventOriginFailsClosedWithoutRuntimeGuard(t *testing.T) {
	identity := commandjobactivation.RuntimeIdentity{
		UserID: "user", AuthContextType: "local_session", AuthContextID: "session",
		AuthGeneration: "auth", SecurityGeneration: "security",
	}
	origin := commandEventOrigin{
		userID: "user", rootType: "internal_event", rootID: "root",
		runtimeIdentity: &identity,
	}
	ctx := context.WithValue(database.WithUserID(context.Background(), identity.UserID), commandEventOriginKey{}, origin)
	trigger := &TriggerContext{Type: TriggerEvent}
	inheritCommandEventOrigin(ctx, trigger)
	if trigger.RootOriginType != "unknown" || trigger.RootOriginID != "" {
		t.Fatalf("marker sem guard não foi rejeitado: %+v", trigger)
	}
}

func TestWithCommandEventOriginPreservesRuntimeGuardAcrossIntermediateRun(t *testing.T) {
	identity := commandjobactivation.RuntimeIdentity{
		UserID: "user", AuthContextType: "local_session", AuthContextID: "session",
		AuthGeneration: "auth", SecurityGeneration: "security",
	}
	var guardCalls atomic.Int32
	guard := func(context.Context, commandjobactivation.RuntimeIdentity) bool {
		guardCalls.Add(1)
		return true
	}
	parent := commandEventOrigin{
		userID: "user", rootType: "internal_event", rootID: "root",
		runtimeIdentity: &identity, runtimeGuard: guard,
	}
	ctx := context.WithValue(database.WithUserID(context.Background(), identity.UserID), commandEventOriginKey{}, parent)
	e := &JobExecutor{}
	intermediate := e.withCommandEventOrigin(ctx, &Job{ID: "intermediate"}, &TriggerContext{Type: TriggerEvent}, &RunLog{
		RunID: "run-intermediate", RootOriginType: "internal_event", RootOriginID: "root",
	})
	child := &TriggerContext{Type: TriggerEvent}
	inheritCommandEventOrigin(intermediate, child)
	if child.RootOriginType != "internal_event" || child.RootOriginID != "root" {
		t.Fatalf("guard não foi preservado no salto intermediário: %+v", child)
	}
	if guardCalls.Load() != 1 {
		t.Fatalf("guard não foi revalidado no filho: %d", guardCalls.Load())
	}
}
