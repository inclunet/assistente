package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/jobprofilegrant"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

func TestCommandServiceLiteralProfileBoundary(t *testing.T) {
	for _, profile := range []any{nil, "", "{{ .event.profile }}", "profile}}", "profile\x00", 42} {
		job := &Job{Tool: jobprofilegrant.ToolSubagent, Inputs: map[string]any{"profile": profile}}
		if _, _, ok := literalJobProfile(job); ok {
			t.Errorf("profile não literal foi aceito: %q", profile)
		}
	}
	job := &Job{Tool: jobprofilegrant.ToolSubagent, Inputs: map[string]any{"profile": "specialist"}}
	if profile, fingerprint, ok := literalJobProfile(job); !ok || profile != "specialist" || fingerprint != jobprofilegrant.Fingerprint(job.Tool, profile) {
		t.Fatalf("profile literal recusado: %s %s %v", profile, fingerprint, ok)
	}
	job.Tool = "other"
	if _, _, ok := literalJobProfile(job); ok {
		t.Fatal("tool diferente recebeu identidade de delegação subagent")
	}
}

func TestCommandServicePreparationFailureDoesNotLeaveQueuedRun(t *testing.T) {
	repo, ctx, _ := setupJobsRepositoryTest(t)
	registry := tools.NewRegistry()
	job := testRepositoryJob("service-prepare-failure", "Service prepare failure")
	if err := repo.SaveJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	var lifetime context.Context
	executor := mustNewJobExecutor(t, ExecutorConfig{
		Repository: repo, ToolRegistry: registry, CircuitBreaker: NewCircuitBreaker(), EventBus: NewEventBus(),
		ToolInvocations: toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig())),
		CommandJobServiceContext: func(ctx context.Context, _ *Job, _ string, life context.Context) (context.Context, error) {
			lifetime = life
			return nil, errors.New("preparação recusada")
		},
	})
	run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
	if run.Status != RunStatusFailed || lifetime == nil || lifetime.Err() == nil {
		t.Fatalf("falha não encerrou a identidade: run=%+v lifetime=%v", run, lifetime)
	}
	runs, err := repo.GetRuns(ctx, job.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, persisted := range runs {
		if persisted.Status != RunStatusFailed {
			t.Fatalf("preparação deixou run não terminal: %+v", persisted)
		}
	}
}

func TestCommandServiceLifetimeEndsBeforeRunEndCallback(t *testing.T) {
	repo, owner, _ := setupJobsRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(&fakeTool{name: "test_tool", params: json.RawMessage(`{"type":"object"}`), response: `{}`})
	job := testRepositoryJob("service-lifetime", "Service lifetime")
	job.Tool = "test_tool"
	if err := repo.SaveJob(owner, job); err != nil {
		t.Fatal(err)
	}
	var lifetime context.Context
	ended := false
	executor := mustNewJobExecutor(t, ExecutorConfig{
		Repository: repo, ToolRegistry: registry, CircuitBreaker: NewCircuitBreaker(), EventBus: NewEventBus(),
		ToolInvocations: toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig())),
		CommandJobServiceContext: func(ctx context.Context, _ *Job, _ string, life context.Context) (context.Context, error) {
			lifetime = life
			return ctx, nil
		},
		OnRunEnd: func(_ string, _ *RunLog) {
			ended = true
			if lifetime == nil || lifetime.Err() == nil {
				t.Error("callback terminal recebeu identidade ainda viva")
			}
		},
	})
	run := executor.Execute(owner, job, &TriggerContext{Type: TriggerManual})
	if !ended || run.Status != RunStatusCompleted {
		t.Fatalf("execução não chegou ao terminal esperado: %+v", run)
	}
}

func TestCommandServiceChildDropsParentMarkerAndKeepsCancellation(t *testing.T) {
	repo, owner, _ := setupJobsRepositoryTest(t)
	registry := tools.NewRegistry()
	job := testRepositoryJob("service-child", "Service child")
	if err := repo.SaveJob(owner, job); err != nil {
		t.Fatal(err)
	}
	parent := &commandJobServiceMarker{}
	ctx, cancel := context.WithCancel(context.WithValue(owner, commandJobServiceMarkerKey{}, parent))
	defer cancel()
	called := false
	executor := mustNewJobExecutor(t, ExecutorConfig{
		Repository: repo, ToolRegistry: registry, CircuitBreaker: NewCircuitBreaker(), EventBus: NewEventBus(),
		ToolInvocations: toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig())),
		CommandJobServiceContext: func(child context.Context, _ *Job, _ string, life context.Context) (context.Context, error) {
			called = true
			if inherited, _ := child.Value(commandJobServiceMarkerKey{}).(*commandJobServiceMarker); inherited != nil {
				t.Error("execução filha herdou identidade do pai")
			}
			cancel()
			if life.Err() == nil {
				t.Error("lifetime privado perdeu cancelamento original")
			}
			return nil, context.Canceled
		},
	})
	executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
	if !called {
		t.Fatal("preparação não foi exercitada")
	}
}
