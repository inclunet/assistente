package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandjobevents"
	"assistente/internal/database"

	"github.com/google/uuid"
)

func uuid7ForTest(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestDBRepositoryPersistRunStateIsIncrementalAndOutboxAtomic(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("automigrate command job events: %v", err)
	}
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay policy epoch: %v", err)
	}
	job := testRepositoryJob("incremental-job", "Incremental")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	runID := "run_opaque_incremental"
	run := &RunLog{
		RunID: runID, JobID: job.ID, Status: RunStatusQueued, QueuedAt: now,
		RootOriginType: "manual", RootOriginID: runID,
	}
	queued := &RunEvent{ID: uuid7ForTest(t), RunID: runID, Sequence: 1, Timestamp: now, Type: RunStatusQueued}
	if err := repo.PersistRunState(userCtx, run, queued); err != nil {
		t.Fatalf("persist queued: %v", err)
	}
	var row database.JobRun
	if err := repo.db.Where("id = ?", runID).First(&row).Error; err != nil {
		t.Fatalf("load queued run: %v", err)
	}
	if row.Status != RunStatusQueued || row.QueuedAt.IsZero() || !row.StartedAt.IsZero() {
		t.Fatalf("queued row = status=%q queued_at=%v started_at=%v", row.Status, row.QueuedAt, row.StartedAt)
	}
	if row.RootOriginType != "manual" || row.RootOriginID != runID {
		t.Fatalf("queued root origin = (%q, %q), want authenticated manual root", row.RootOriginType, row.RootOriginID)
	}
	var queuedTimeline database.JobRunEvent
	if err := repo.db.Where("id = ?", queued.ID).First(&queuedTimeline).Error; err != nil {
		t.Fatalf("load queued timeline: %v", err)
	}
	if queuedTimeline.RootOriginType != "manual" || queuedTimeline.RootOriginID != runID {
		t.Fatalf("timeline root origin = (%q, %q), want authenticated manual root", queuedTimeline.RootOriginType, queuedTimeline.RootOriginID)
	}
	var outboxCount int64
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", queued.ID).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("queued fact/outbox count = %d, want 1", outboxCount)
	}

	startedAt := now.Add(time.Second)
	run.Status, run.StartedAt = RunStatusRunning, startedAt
	started := &RunEvent{ID: uuid7ForTest(t), RunID: runID, Timestamp: startedAt, Type: RunStatusRunning}
	if err := repo.PersistRunState(userCtx, run, started); err != nil {
		t.Fatalf("persist started: %v", err)
	}
	row = database.JobRun{}
	if err := repo.db.Where("id = ?", runID).First(&row).Error; err != nil {
		t.Fatalf("load started run: %v", err)
	}
	if row.Status != RunStatusRunning || row.StartedAt.IsZero() {
		t.Fatalf("started row = status=%q started_at=%v", row.Status, row.StartedAt)
	}

	run.Status, run.CompletedAt, run.Error = RunStatusFailed, startedAt.Add(time.Second), "synthetic failure"
	terminal := &RunEvent{ID: uuid7ForTest(t), RunID: runID, Timestamp: run.CompletedAt, Type: RunStatusFailed, Message: run.Error}
	if err := repo.PersistRunState(userCtx, run, terminal); err != nil {
		t.Fatalf("persist terminal: %v", err)
	}
	if err := repo.PersistRunState(userCtx, run, terminal); err != nil {
		t.Fatalf("idempotent terminal upsert: %v", err)
	}
	var eventCount int64
	if err := repo.db.Model(&database.JobRunEvent{}).Where("job_run_id = ?", runID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count run events: %v", err)
	}
	if eventCount != 3 {
		t.Fatalf("run event count after terminal replay = %d, want 3", eventCount)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("run_id = ?", runID).Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox facts: %v", err)
	}
	if outboxCount != 3 {
		t.Fatalf("outbox count after terminal replay = %d, want 3", outboxCount)
	}
}

func TestDBRepositoryUnknownRootPersistsTimelineButNeverOutbox(t *testing.T) {
	for _, triggerType := range []TriggerType{TriggerEvent, TriggerWebhook} {
		if rootType, rootID := runRoot(&TriggerContext{Type: triggerType}, "run_opaque"); rootType != "unknown" || rootID != "" {
			t.Fatalf("%s without authenticated origin = (%q, %q), want (unknown, empty)", triggerType, rootType, rootID)
		}
	}
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("automigrate command job events: %v", err)
	}
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay policy epoch: %v", err)
	}
	job := testRepositoryJob("unknown-root", "Unknown root")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	runID := "run_opaque_unknown_root"
	run := &RunLog{RunID: runID, JobID: job.ID, Status: RunStatusQueued, QueuedAt: time.Now().UTC(), RootOriginType: "unknown"}
	event := &RunEvent{ID: uuid7ForTest(t), RunID: runID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusQueued}
	if err := repo.PersistRunState(userCtx, run, event); err != nil {
		t.Fatalf("persist unknown-root run: %v", err)
	}
	if run.RootOriginType != "unknown" || run.RootOriginID != "" {
		t.Fatalf("unknown root was changed: (%q, %q)", run.RootOriginType, run.RootOriginID)
	}
	var timelineCount, outboxCount int64
	if err := repo.db.Model(&database.JobRunEvent{}).Where("job_run_id = ?", runID).Count(&timelineCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("run_id = ?", runID).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if timelineCount != 1 || outboxCount != 0 {
		t.Fatalf("unknown root persistence = timeline=%d outbox=%d, want 1/0", timelineCount, outboxCount)
	}
}

func TestDBRepositoryRejectsPartialAuthenticatedRoot(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	job := testRepositoryJob("partial-root", "Partial root")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	runID := "run_opaque_partial_root"
	run := &RunLog{RunID: runID, JobID: job.ID, Status: RunStatusQueued, QueuedAt: time.Now().UTC(), RootOriginType: "manual"}
	event := &RunEvent{ID: uuid7ForTest(t), RunID: runID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusQueued}
	if err := repo.PersistRunState(userCtx, run, event); err == nil {
		t.Fatal("partial authenticated root should be rejected")
	}
}

func TestDBRepositoryLegacyRunWithoutAuthenticatedRootDoesNotEnterOutbox(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	if err := repo.db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("automigrate command job events: %v", err)
	}
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay policy epoch: %v", err)
	}
	job := testRepositoryJob("legacy-fact", "Legacy")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatal(err)
	}
	runID := uuid7ForTest(t)
	run := &RunLog{RunID: runID, JobID: job.ID, Status: RunStatusQueued, QueuedAt: time.Now()}
	event := &RunEvent{ID: uuid7ForTest(t), RunID: runID, Sequence: 1, Timestamp: time.Now(), Type: RunStatusQueued}
	if err := repo.PersistRunState(userCtx, run, event); err != nil {
		t.Fatalf("persist legacy-shaped run: %v", err)
	}
	var count int64
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("legacy-shaped run created %d outbox entries", count)
	}
}

func TestCommandJobEventAdapterFailsClosed(t *testing.T) {
	adapter := commandjobevents.NewAdapter(nil)
	if adapter.Enabled() {
		t.Fatal("adapter sem store não deveria estar habilitado")
	}
	if err := adapter.Dispatch(context.Background(), commandjobevents.ActivationOutbox{}); !errors.Is(err, commandjobevents.ErrAdapterDisabled) {
		t.Fatalf("dispatch sem claims/manutenção = %v", err)
	}
}
