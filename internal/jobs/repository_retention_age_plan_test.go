package jobs

import (
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestOldRunRetentionDoesNotSortEntireHistory(t *testing.T) {
	repo, ctx, _ := setupJobsRepositoryTest(t)
	old := time.Now().Add(-48 * time.Hour)
	for _, run := range []database.JobRun{
		{UUIDModel: database.UUIDModel{ID: "old-a"}, UserID: "user-a", JobID: "job", Status: RunStatusCompleted, QueuedAt: old, StartedAt: old},
		{UUIDModel: database.UUIDModel{ID: "old-b"}, UserID: "user-a", JobID: "job", Status: RunStatusCompleted, QueuedAt: old, StartedAt: old},
		{UUIDModel: database.UUIDModel{ID: "other-user"}, UserID: "user-b", JobID: "job", Status: RunStatusCompleted, QueuedAt: old, StartedAt: old},
		{UUIDModel: database.UUIDModel{ID: "recent"}, UserID: "user-a", JobID: "job", Status: RunStatusCompleted, QueuedAt: time.Now(), StartedAt: time.Now()},
	} {
		if err := repo.db.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
	}
	var sql string
	var args []any
	if err := repo.db.Callback().Query().After("gorm:query").Register("capture_age_selection", func(tx *gorm.DB) {
		if tx.Statement.Table == "job_runs" {
			sql = tx.Statement.SQL.String()
			args = append([]any(nil), tx.Statement.Vars...)
		}
	}); err != nil {
		t.Fatal(err)
	}
	deleted, more, err := repo.CleanOldRunsBatch(ctx, 24*time.Hour, 1)
	if err != nil || deleted != 1 || !more {
		t.Fatalf("first batch=(%d,%v,%v)", deleted, more, err)
	}
	if sql == "" {
		t.Fatal("seleção produtiva não observada")
	}
	for _, step := range explainRetentionPlan(t, repo.db, sql, args) {
		if strings.Contains(step, "TEMP B-TREE") {
			t.Fatalf("seleção ordena histórico antes do LIMIT: %s", step)
		}
	}
	deleted, more, err = repo.CleanOldRunsBatch(ctx, 24*time.Hour, 1)
	if err != nil || deleted != 1 || more {
		t.Fatalf("last batch=(%d,%v,%v)", deleted, more, err)
	}
	var remaining []string
	if err := repo.db.Model(&database.JobRun{}).Order("id").Pluck("id", &remaining).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Join(remaining, ",") != "other-user,recent" {
		t.Fatalf("retidos=%v", remaining)
	}
}
