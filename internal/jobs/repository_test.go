package jobs

import (
	"context"
	"testing"

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
