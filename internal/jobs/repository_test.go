package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupJobsRepositoryTest(t *testing.T) (*DBRepository, context.Context, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.MCPServer{},
		&database.ToolCatalog{},
		&database.Tag{},
		&database.TagAssignment{},
		&database.JobPipeline{},
		&database.Job{},
		&database.JobTrigger{},
		&database.JobRun{},
		&database.JobEvent{},
		&database.JobRunEvent{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
	})
	if err := db.Create(&database.ToolCatalog{
		Name:               "test_tool",
		DisplayName:        "test_tool",
		Description:        "Test tool",
		Origin:             "builtin",
		AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}
	return NewDBRepository(db), database.WithUserID(context.Background(), "user-a"), database.WithUserID(context.Background(), "user-b")
}

func TestDBRepositoryJobsAreScopedByUser(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)

	jobA := testRepositoryJob("daily-report", "A")
	if err := repo.SaveJob(userA, jobA); err != nil {
		t.Fatalf("save user A: %v", err)
	}
	jobB := testRepositoryJob("daily-report", "B")
	if err := repo.SaveJob(userB, jobB); err != nil {
		t.Fatalf("save user B: %v", err)
	}

	gotA, err := repo.GetJob(userA, "daily-report")
	if err != nil {
		t.Fatalf("get user A: %v", err)
	}
	gotB, err := repo.GetJob(userB, "daily-report")
	if err != nil {
		t.Fatalf("get user B: %v", err)
	}
	if gotA.Name != "A" || gotB.Name != "B" {
		t.Fatalf("escopo por usuário falhou: A=%q B=%q", gotA.Name, gotB.Name)
	}
}

func TestDBRepositorySaveJobNormalizesPipelineTriggersAndTags(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Pipeline = "Ops"
	job.Tags = []string{"Ops", "jira", "ops"}
	job.Triggers = []Trigger{{Type: TriggerInterval, Every: "1h"}}

	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	if job.Pipeline != "ops" {
		t.Fatalf("pipeline normalizada no objeto salvo: got %q, want ops", job.Pipeline)
	}
	if len(job.Tags) != 2 || job.Tags[0] != "ops" || job.Tags[1] != "jira" {
		t.Fatalf("tags normalizadas no objeto salvo: %#v", job.Tags)
	}
	got, err := repo.GetJob(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Pipeline != "ops" {
		t.Fatalf("pipeline: got %q, want ops", got.Pipeline)
	}
	if len(got.Triggers) != 2 {
		t.Fatalf("triggers: got %d, want interval + manual", len(got.Triggers))
	}
	if len(got.Tags) != 2 || got.Tags[0] != "jira" || got.Tags[1] != "ops" {
		t.Fatalf("tags normalizadas: %#v", got.Tags)
	}
	listed, err := repo.ListJobs(userA, JobFilter{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(listed) != 1 || len(listed[0].Tags) != 2 || listed[0].Tags[0] != "jira" || listed[0].Tags[1] != "ops" {
		t.Fatalf("tags normalizadas na listagem: %#v", listed)
	}
}

func TestDBRepositoryKeepsJobEnabledSeparateFromPipelineState(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SavePipeline(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: false}); err != nil {
		t.Fatalf("save pipeline: %v", err)
	}
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Pipeline = "ops"
	job.Enabled = true
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	got, err := repo.GetJob(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if !got.Enabled {
		t.Fatal("job-level enabled flag was folded into disabled pipeline state")
	}
	if got.PipelineEnabled {
		t.Fatal("expected disabled pipeline runtime state to be preserved separately")
	}
}

func TestDBRepositoryRequiresUser(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	err := repo.SaveJob(context.Background(), testRepositoryJob("x", "X"))
	if err != database.ErrUserScopeRequired {
		t.Fatalf("SaveJob sem usuário: got %v, want ErrUserScopeRequired", err)
	}
}

func TestDBRepositorySaveJobCreatesUnresolvedMCPToolForImportedServer(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	userID := "user-a"
	if err := repo.db.Create(&database.MCPServer{
		UserID: userID,
		Slug:   "jira",
		Name:   "Jira",
	}).Error; err != nil {
		t.Fatalf("seed mcp server: %v", err)
	}
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Tool = "mcp_jira__create_issue"
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job with unresolved mcp tool: %v", err)
	}
	var row database.ToolCatalog
	if err := repo.db.Where("name = ?", "mcp_jira__create_issue").First(&row).Error; err != nil {
		t.Fatalf("expected placeholder tool catalog row: %v", err)
	}
	if row.AvailabilityStatus != "unavailable" || row.MCPServerID == nil {
		t.Fatalf("unexpected placeholder row: %#v", row)
	}
}

func TestDBRepositorySaveJobPreservesCreatedByOnUpdate(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Metadata.CreatedBy = "seed-import"
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save initial: %v", err)
	}

	update := testRepositoryJob("sync-jira", "Sync Jira Updated")
	update.Metadata.CreatedBy = "malicious-update"
	if err := repo.SaveJob(userA, update); err != nil {
		t.Fatalf("save update: %v", err)
	}
	got, err := repo.GetJob(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Metadata.CreatedBy != "seed-import" {
		t.Fatalf("created_by: got %q, want seed-import", got.Metadata.CreatedBy)
	}
}

func TestDBRepositorySaveJobRejectsNilJob(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, nil); err == nil {
		t.Fatal("expected nil job to return an error")
	}
}

func TestDBRepositoryLogRunMatchesEventTriggerByExpression(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("event-job", "Event Job")
	job.Triggers = []Trigger{
		{Type: TriggerEvent, Listen: "deploy"},
		{Type: TriggerEvent, Listen: "deploy-extra"},
	}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-event",
		JobID:     "event-job",
		Status:    "completed",
		Trigger:   TriggerInfo{Type: TriggerEvent, Event: "deploy"},
		StartedAt: time.Now(),
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
	var row database.JobRun
	if err := repo.db.Preload("Trigger").First(&row, "id = ?", "run-event").Error; err != nil {
		t.Fatalf("load run row: %v", err)
	}
	if row.Trigger == nil || row.Trigger.Expression != "deploy" {
		t.Fatalf("trigger expression: %#v", row.Trigger)
	}
}

func TestDBRepositorySaveJobKeepsDistinctTriggersWithSameExpression(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("deploy-listeners", "Deploy Listeners")
	job.Triggers = []Trigger{
		{Type: TriggerEvent, Listen: "deploy", When: `{{ eq .event.env "prod" }}`},
		{Type: TriggerEvent, Listen: "deploy", When: `{{ eq .event.env "staging" }}`},
	}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	triggers, err := repo.ListTriggers(userA, "deploy-listeners")
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	eventTriggers := 0
	seen := map[string]bool{}
	for _, trigger := range triggers {
		if trigger.Type != TriggerEvent {
			continue
		}
		eventTriggers++
		seen[trigger.When] = true
	}
	if eventTriggers != 2 || !seen[`{{ eq .event.env "prod" }}`] || !seen[`{{ eq .event.env "staging" }}`] {
		t.Fatalf("distinct event triggers were not preserved: %#v", triggers)
	}
}

func TestDBRepositoryLogRunRejectsDuplicateRunID(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("duplicate-run", "Duplicate Run")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{RunID: "run-duplicate", JobID: "duplicate-run", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: time.Now()}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log initial run: %v", err)
	}
	duplicate := *rl
	duplicate.Status = "failed"
	if err := repo.LogRun(userA, &duplicate); err == nil {
		t.Fatal("expected duplicate run id to fail instead of overwriting")
	}
	got, err := repo.GetRun(userA, "duplicate-run", "run-duplicate")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("duplicate run overwrote original status: got %q", got.Status)
	}
}

func TestDBRepositoryLogRunPreservesEmptyRuntimeJSONValues(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("empty-runtime-json", "Empty Runtime JSON")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:          "run-empty-json",
		JobID:          "empty-runtime-json",
		Status:         "completed",
		Trigger:        TriggerInfo{Type: TriggerManual},
		StartedAt:      time.Now(),
		ResolvedInputs: map[string]any{},
		Output:         map[string]any{},
		EventsEmitted:  []string{},
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}

	got, err := repo.GetRun(userA, "empty-runtime-json", "run-empty-json")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.ResolvedInputs == nil || len(got.ResolvedInputs) != 0 {
		t.Fatalf("resolved inputs not preserved as empty object: %#v", got.ResolvedInputs)
	}
	if got.Output == nil || len(got.Output) != 0 {
		t.Fatalf("output not preserved as empty object: %#v", got.Output)
	}
	if got.EventsEmitted == nil || len(got.EventsEmitted) != 0 {
		t.Fatalf("events emitted not preserved as empty array: %#v", got.EventsEmitted)
	}
}

func TestDBRepositoryLogRunPersistsRunEvents(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("run-events", "Run Events")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-with-events",
		JobID:     "run-events",
		Status:    "failed",
		Trigger:   TriggerInfo{Type: TriggerManual},
		StartedAt: time.Now(),
		RunEvents: []RunEvent{
			{Type: "triggered", Message: "started"},
			{Type: "failed", Message: "failed"},
		},
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
	events, err := repo.GetRunEvents(userA, "run-with-events")
	if err != nil {
		t.Fatalf("get run events: %v", err)
	}
	if len(events) != 2 || events[0].Type != "triggered" || events[1].Type != "failed" {
		t.Fatalf("unexpected run events: %#v", events)
	}
}

func TestDBRepositorySaveJobNamesUnknownTool(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("unknown-tool", "Unknown Tool")
	job.Tool = "not_a_real_tool"
	err := repo.SaveJob(userA, job)
	if err == nil || !strings.Contains(err.Error(), "not_a_real_tool") {
		t.Fatalf("expected missing tool error to include tool name, got %v", err)
	}
}

func TestDBRepositoryListEventsReturnsJobSlugAndFiltersByJob(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	jobA := testRepositoryJob("job-a", "Job A")
	jobB := testRepositoryJob("job-b", "Job B")
	if err := repo.SaveJob(userA, jobA); err != nil {
		t.Fatalf("save job A: %v", err)
	}
	if err := repo.SaveJob(userA, jobB); err != nil {
		t.Fatalf("save job B: %v", err)
	}
	now := time.Now()
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: now, Type: "emitted", Event: "done", Message: "A"}); err != nil {
		t.Fatalf("log event A: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-b", Timestamp: now.Add(time.Second), Type: "emitted", Event: "done", Message: "B"}); err != nil {
		t.Fatalf("log event B: %v", err)
	}

	got, err := repo.ListEvents(userA, EventFilter{JobID: "job-a"})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("events count: got %d, want 1", len(got))
	}
	if got[0].JobID != "job-a" || got[0].Message != "A" {
		t.Fatalf("unexpected event: %#v", got[0])
	}
}

func TestDBRepositoryListEventsFiltersByDateRange(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("job-a", "Job A")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	day := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: day.Add(-time.Hour), Type: "emitted", Message: "old"}); err != nil {
		t.Fatalf("log old event: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: day.Add(time.Hour), Type: "emitted", Message: "inside"}); err != nil {
		t.Fatalf("log inside event: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: day.Add(25 * time.Hour), Type: "emitted", Message: "new"}); err != nil {
		t.Fatalf("log new event: %v", err)
	}

	got, err := repo.ListEvents(userA, EventFilter{StartAt: day, EndAt: day.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(got) != 1 || got[0].Message != "inside" {
		t.Fatalf("unexpected events: %#v", got)
	}
}

func TestDBRepositoryGetLastRunsBatch(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("job-a", "Job A")); err != nil {
		t.Fatalf("save job A: %v", err)
	}
	if err := repo.SaveJob(userA, testRepositoryJob("job-b", "Job B")); err != nil {
		t.Fatalf("save job B: %v", err)
	}
	start := time.Now()
	runs := []RunLog{
		{RunID: "run-a-old", JobID: "job-a", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: start},
		{RunID: "run-a-new", JobID: "job-a", Status: "failed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: start.Add(time.Hour)},
		{RunID: "run-b", JobID: "job-b", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: start.Add(2 * time.Hour)},
	}
	for i := range runs {
		if err := repo.LogRun(userA, &runs[i]); err != nil {
			t.Fatalf("log run %s: %v", runs[i].RunID, err)
		}
	}

	got, err := repo.GetLastRuns(userA, []string{"job-a", "job-b"})
	if err != nil {
		t.Fatalf("get last runs: %v", err)
	}
	if got["job-a"] == nil || got["job-a"].RunID != "run-a-new" {
		t.Fatalf("job-a last run: %#v", got["job-a"])
	}
	if got["job-b"] == nil || got["job-b"].RunID != "run-b" {
		t.Fatalf("job-b last run: %#v", got["job-b"])
	}
}

func testRepositoryJob(id, name string) *Job {
	return &Job{
		ID:      id,
		Name:    name,
		Enabled: true,
		Tool:    "test_tool",
		Triggers: []Trigger{
			{Type: TriggerManual},
		},
	}
}
