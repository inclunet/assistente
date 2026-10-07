package jobs

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandjobevents"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupJobsWALRepositoryTest(t *testing.T) (*DBRepository, context.Context) {
	t.Helper()
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "jobs-wal.db"))
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?_pragma=busy_timeout(1)&_pragma=journal_mode(WAL)", path)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open WAL test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQL db: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	if err := db.AutoMigrate(
		&database.User{}, &database.MCPServer{}, &database.ToolCatalog{}, &database.ToolInvocation{},
		&database.Tag{}, &database.TagAssignment{}, &database.JobPipeline{}, &database.Job{},
		&database.JobProfileGrant{}, &database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{},
		&database.JobTrigger{}, &database.JobRun{}, &database.JobEvent{}, &database.JobRunEvent{},
	); err != nil {
		t.Fatalf("migrate jobs schema: %v", err)
	}
	if err := db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatalf("migrate command event schema: %v", err)
	}
	if err := db.Create(&database.ToolCatalog{
		Name: "test_tool", DisplayName: "test_tool", Origin: "builtin", AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatalf("seed test tool: %v", err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
		_ = sqlDB.Close()
	})
	userCtx := database.WithUserID(context.Background(), uuid7ForTest(t))
	return NewDBRepository(db), userCtx
}

func TestPersistRunStateSerializesConcurrentWALWriters(t *testing.T) {
	repo, userCtx := setupJobsWALRepositoryTest(t)
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed outbox policy: %v", err)
	}
	job := testRepositoryJob("wal-contention", "WAL contention")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	const writers = 2
	start := make(chan struct{})
	errs := make(chan error, writers)
	var ready sync.WaitGroup
	ready.Add(writers)
	runsToPersist := make([]*RunLog, writers)
	eventsToPersist := make([]*RunEvent, writers)
	for i := range writers {
		runID := fmt.Sprintf("run_wal_writer_%d", i)
		runsToPersist[i] = &RunLog{RunID: runID, JobID: job.ID, Status: RunStatusQueued, QueuedAt: time.Now().UTC(), RootOriginType: "manual", RootOriginID: runID}
		eventsToPersist[i] = &RunEvent{ID: uuid7ForTest(t), RunID: runID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusQueued}
	}
	for i := range writers {
		i := i
		go func() {
			ready.Done()
			<-start
			errs <- repo.PersistRunState(userCtx, runsToPersist[i], eventsToPersist[i])
		}()
	}
	ready.Wait()
	close(start)
	var firstErr error
	for range writers {
		if err := <-errs; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		t.Fatalf("concurrent persist: %v", firstErr)
	}

	var runs, events, outbox int64
	if err := repo.db.Model(&database.JobRun{}).Where("job_id = ?", job.DatabaseID).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&database.JobRunEvent{}).Where("job_run_id IN ?", []string{"run_wal_writer_0", "run_wal_writer_1"}).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("run_id IN ?", []string{"run_wal_writer_0", "run_wal_writer_1"}).Count(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	if runs != writers || events != writers || outbox != writers {
		t.Fatalf("concurrent persistence counts = run/event/outbox %d/%d/%d, want %d/%d/%d", runs, events, outbox, writers, writers, writers)
	}
}

func TestPersistRunStateAcquiresWriterBeforeFirstRunRead(t *testing.T) {
	repo, userCtx := setupJobsWALRepositoryTest(t)
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed outbox policy: %v", err)
	}
	job := testRepositoryJob("wal-snapshot", "WAL snapshot")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	var reads, competingWrites, competingBusy atomic.Int32
	var probeActive atomic.Bool
	probeActive.Store(true)
	callbackName := "test:persist_run_state_writer_acquired_before_read"
	if err := repo.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if !probeActive.Load() || tx.Statement.Table != "job_runs" {
			return
		}
		reads.Add(1)
		writeErr := repo.db.Exec("UPDATE tool_catalog SET description = description || 'x' WHERE name = ?", "test_tool").Error
		if database.IsSQLiteBusyError(writeErr) {
			competingBusy.Add(1)
			return
		}
		if writeErr == nil {
			competingWrites.Add(1)
			return
		}
		_ = tx.AddError(fmt.Errorf("competing writer failed unexpectedly: %w", writeErr))
	}); err != nil {
		t.Fatalf("register query synchronization callback: %v", err)
	}
	t.Cleanup(func() { _ = repo.db.Callback().Query().Remove(callbackName) })

	runID := "run_wal_snapshot_regression"
	run := &RunLog{RunID: runID, JobID: job.ID, Status: RunStatusQueued, QueuedAt: time.Now().UTC(), RootOriginType: "manual", RootOriginID: runID}
	event := &RunEvent{ID: uuid7ForTest(t), RunID: runID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusQueued}
	err := repo.PersistRunState(userCtx, run, event)
	probeActive.Store(false)
	if err != nil {
		t.Fatalf("persist with competing writer probe: %v", err)
	}
	if reads.Load() == 0 || competingBusy.Load() != reads.Load() || competingWrites.Load() != 0 {
		t.Fatalf("writer acquisition probe: reads=%d busy=%d committed=%d; want one busy result per run read", reads.Load(), competingBusy.Load(), competingWrites.Load())
	}
	var runs, events, outbox int64
	if err := repo.db.Model(&database.JobRun{}).Where("id = ?", runID).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&database.JobRunEvent{}).Where("job_run_id = ?", runID).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("run_id = ?", runID).Count(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 1 || events != 1 || outbox != 1 {
		t.Fatalf("atomic result counts = run/event/outbox %d/%d/%d, want 1/1/1", runs, events, outbox)
	}
}

func TestPersistRunStateCancellationAndOutboxFailureAreAtomic(t *testing.T) {
	repo, userCtx := setupJobsWALRepositoryTest(t)
	if _, err := repo.commandEvents.EnsureReplayPolicyEpoch(userCtx, commandjobevents.ProducerType, time.Now().UTC().Add(-time.Hour), time.Hour); err != nil {
		t.Fatalf("seed outbox policy: %v", err)
	}
	job := testRepositoryJob("wal-rollback", "WAL rollback")
	if err := repo.SaveJob(userCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	sqlDB, err := repo.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	lockConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lockConn.Close(); err != nil {
			t.Errorf("close writer connection: %v", err)
		}
	}()
	if _, err := lockConn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("hold writer lock: %v", err)
	}

	cancelCtx, cancel := context.WithTimeout(userCtx, 80*time.Millisecond)
	defer cancel()
	lockedRun := &RunLog{RunID: "run_cancelled_before_write", JobID: job.ID, Status: RunStatusQueued, QueuedAt: time.Now().UTC(), RootOriginType: "manual", RootOriginID: "run_cancelled_before_write"}
	lockedEvent := &RunEvent{ID: uuid7ForTest(t), RunID: lockedRun.RunID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusQueued}
	err = repo.PersistRunState(cancelCtx, lockedRun, lockedEvent)
	if err == nil || cancelCtx.Err() == nil {
		t.Fatalf("persist under held writer should cancel, err=%v ctx=%v", err, cancelCtx.Err())
	}
	if _, err := lockConn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatalf("release writer lock: %v", err)
	}
	var cancelledRuns, cancelledEvents, cancelledOutbox int64
	if err := repo.db.Model(&database.JobRun{}).Where("id = ?", lockedRun.RunID).Count(&cancelledRuns).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&database.JobRunEvent{}).Where("job_run_id = ?", lockedRun.RunID).Count(&cancelledEvents).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&commandjobevents.ActivationOutbox{}).Where("run_id = ?", lockedRun.RunID).Count(&cancelledOutbox).Error; err != nil {
		t.Fatal(err)
	}
	if cancelledRuns != 0 || cancelledEvents != 0 || cancelledOutbox != 0 {
		t.Fatalf("cancelled run left rows = run/event/outbox %d/%d/%d", cancelledRuns, cancelledEvents, cancelledOutbox)
	}

	if err := repo.db.Exec(`CREATE TRIGGER reject_test_outbox BEFORE INSERT ON command_job_activation_outbox BEGIN SELECT RAISE(ABORT, 'synthetic outbox failure'); END`).Error; err != nil {
		t.Fatalf("install rollback trigger: %v", err)
	}
	rollbackRunID := "run_outbox_rollback"
	run := &RunLog{RunID: rollbackRunID, JobID: job.ID, Status: RunStatusQueued, QueuedAt: time.Now().UTC(), RootOriginType: "manual", RootOriginID: rollbackRunID}
	event := &RunEvent{ID: uuid7ForTest(t), RunID: rollbackRunID, Sequence: 1, Timestamp: time.Now().UTC(), Type: RunStatusQueued}
	if err := repo.PersistRunState(userCtx, run, event); err == nil {
		t.Fatal("synthetic outbox failure should abort state persistence")
	}
	for _, model := range []any{&database.JobRun{}, &database.JobRunEvent{}, &commandjobevents.ActivationOutbox{}} {
		query := repo.db.Model(model)
		if _, ok := model.(*database.JobRun); ok {
			query = query.Where("id = ?", rollbackRunID)
		} else {
			query = query.Where("run_id = ?", rollbackRunID)
			if _, ok := model.(*database.JobRunEvent); ok {
				query = repo.db.Model(model).Where("job_run_id = ?", rollbackRunID)
			}
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback left %T rows: %d", model, count)
		}
	}
}
