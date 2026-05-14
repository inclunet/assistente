package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"assistente/internal/database"
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
	infos, err := mgr.GetJobsContext(userA)
	if err != nil {
		t.Fatalf("get job infos: %v", err)
	}
	if len(infos) != 1 || infos[0].EffectiveEnabled || infos[0].PipelineEnabled {
		t.Fatalf("job info did not expose effective disabled pipeline state: %#v", infos)
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
	job.Inputs = map[string]any{"query": "original"}
	job.Tags = []string{"ops"}
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
	got.Inputs["query"] = "mutated"
	got.Tags[0] = "mutated"
	got.Triggers[0].Type = TriggerCron

	again, err := mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job again: %v", err)
	}
	if again.Tool != "test_tool" {
		t.Fatalf("registry job was mutated through returned pointer: got %q", again.Tool)
	}
	if again.Inputs["query"] == "mutated" {
		t.Fatalf("registry inputs map was mutated through returned pointer: %#v", again.Inputs)
	}
	if again.Tags[0] == "mutated" {
		t.Fatalf("registry tags slice was mutated through returned pointer: %#v", again.Tags)
	}
	if again.Triggers[0].Type == TriggerCron {
		t.Fatalf("registry triggers slice was mutated through returned pointer: %#v", again.Triggers)
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

func TestManagerContextFromPreservesParentUserScope(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})

	scoped := mgr.contextFrom(userB)
	got, ok := database.UserIDFromContext(scoped)
	if !ok {
		t.Fatal("expected scoped context to keep a user")
	}
	want, _ := database.UserIDFromContext(userB)
	if got != want {
		t.Fatalf("contextFrom overwrote parent user: got %q, want %q", got, want)
	}
}

func TestManagerGetJobEventsReturnsFullDay(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	day := time.Date(2026, 5, 13, 12, 0, 0, 0, time.Local)
	for i := 0; i < 505; i++ {
		entry := &EventEntry{
			ID:        fmt.Sprintf("event-%03d", i),
			Timestamp: day.Add(time.Duration(i) * time.Second),
			Type:      "info",
			Message:   "event",
		}
		if err := repo.LogEvent(userA, entry); err != nil {
			t.Fatalf("log event %d: %v", i, err)
		}
	}

	events, err := mgr.GetJobEvents("2026-05-13")
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 505 {
		t.Fatalf("GetJobEvents truncated full day: got %d, want 505", len(events))
	}
}

func TestManagerGetJobEventsEmptyDateDefaultsToToday(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	today := time.Now().In(time.Local)
	entries := []*EventEntry{
		{ID: "today-event", Timestamp: today, Type: "info", Message: "today"},
		{ID: "old-event", Timestamp: today.AddDate(0, 0, -1), Type: "info", Message: "old"},
	}
	for _, entry := range entries {
		if err := repo.LogEvent(userA, entry); err != nil {
			t.Fatalf("log event %s: %v", entry.ID, err)
		}
	}

	events, err := mgr.GetJobEvents("")
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].ID != "today-event" {
		t.Fatalf("empty date should return only today's events, got %#v", events)
	}
}
