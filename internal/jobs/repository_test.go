package jobs

import (
	"context"
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
}

func TestDBRepositoryRequiresUser(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	err := repo.SaveJob(context.Background(), testRepositoryJob("x", "X"))
	if err != database.ErrUserScopeRequired {
		t.Fatalf("SaveJob sem usuário: got %v, want ErrUserScopeRequired", err)
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
