package jobs

import (
	"context"
	"testing"
)

func TestManagerPipelineStateControlsRuntimeWithoutOverwritingJobEnabled(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SavePipeline(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: true}); err != nil {
		t.Fatalf("save pipeline: %v", err)
	}
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Pipeline = "ops"
	job.Triggers = []Trigger{{Type: TriggerInterval, Every: "1h"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 1 || got[0] != "sync-jira" {
		t.Fatalf("scheduled jobs after start: got %#v, want [sync-jira]", got)
	}

	if err := mgr.SavePipelineContext(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: false}); err != nil {
		t.Fatalf("disable pipeline: %v", err)
	}
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 0 {
		t.Fatalf("scheduled jobs after disabling pipeline: got %#v, want none", got)
	}
	gotJob, err := mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if !gotJob.Enabled {
		t.Fatal("pipeline state overwrote job-level enabled flag")
	}
	if gotJob.PipelineEnabled {
		t.Fatal("expected runtime pipeline state to be disabled")
	}

	if err := mgr.SavePipelineContext(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: true}); err != nil {
		t.Fatalf("enable pipeline: %v", err)
	}
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 1 || got[0] != "sync-jira" {
		t.Fatalf("scheduled jobs after enabling pipeline: got %#v, want [sync-jira]", got)
	}

	if err := mgr.SavePipelineContext(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: false}); err != nil {
		t.Fatalf("disable pipeline again: %v", err)
	}
	if err := mgr.DeletePipelineContext(userA, "ops"); err != nil {
		t.Fatalf("delete pipeline: %v", err)
	}
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 1 || got[0] != "sync-jira" {
		t.Fatalf("scheduled jobs after deleting disabled pipeline: got %#v, want [sync-jira]", got)
	}
	gotJob, err = mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job after delete: %v", err)
	}
	if gotJob.Pipeline != "" || !gotJob.PipelineEnabled {
		t.Fatalf("pipeline state after delete: pipeline=%q pipelineEnabled=%v", gotJob.Pipeline, gotJob.PipelineEnabled)
	}
}

func TestManagerGetJobContextReturnsCopy(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("sync-jira", "Sync Jira")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)

	got, err := mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	got.Tool = "missing_tool"

	again, err := mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job again: %v", err)
	}
	if again.Tool != "test_tool" {
		t.Fatalf("registry job was mutated through returned pointer: got %q", again.Tool)
	}
}

func TestManagerStopResetsCircuitBreakerState(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	mgr.circuitBreaker.SetMaxRunsPerHour(1)
	mgr.circuitBreaker.RecordRun("shared-job-id")
	if err := mgr.circuitBreaker.CheckRateLimit("shared-job-id", 0); err == nil {
		t.Fatal("expected recorded run to trip rate limit before reset")
	}

	mgr.Stop()
	if err := mgr.circuitBreaker.CheckRateLimit("shared-job-id", 0); err != nil {
		t.Fatalf("rate limit state leaked after stop: %v", err)
	}
}
