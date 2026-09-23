package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"

	"github.com/google/uuid"
)

func commandMaintenanceTestPolicy() commandmaintenance.Policy {
	return commandmaintenance.Policy{
		JobRetention:          time.Hour,
		RunsPerJobKeep:        1,
		ChatRetention:         time.Hour,
		InvocationRetention:   time.Hour,
		InvocationsPerUser:    1,
		InvocationsSystemKeep: 1,
		ActivationRetention:   time.Hour,
		ActivationsPerUser:    1,
		LeaseDuration:         time.Minute,
		BatchSize:             1,
	}
}

func newCommandMaintenanceTestManager(t *testing.T) (*Manager, *DBRepository, context.Context, context.Context) {
	t.Helper()
	repo, userA, userB := setupJobsRepositoryTest(t)
	if err := repo.db.Create([]database.User{
		{UUIDModel: database.UUIDModel{ID: "user-a"}, Username: "adapter-a", PasswordHash: "test", Role: database.UserRoleUser, IsActive: true},
		{UUIDModel: database.UUIDModel{ID: "user-b"}, Username: "adapter-b", PasswordHash: "test", Role: database.UserRoleUser, IsActive: true},
	}).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
	service := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), nil)
	manager := &Manager{cfg: ManagerConfig{Repository: repo, ToolInvocations: service}}
	return manager, repo, userA, userB
}

func TestCommandMaintenanceAdaptersRetainRealRowsWithExplicitUserScope(t *testing.T) {
	manager, repo, userA, userB := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}

	// A chamada direta sem userID continua fechada; somente o adapter, como
	// dono da instância, enumera usuários e injeta o escopo autenticado.
	if _, err := repo.CleanOldRuns(context.Background(), time.Hour); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("direct cleanup error = %v, want user scope error", err)
	}

	for i, ctx := range []context.Context{userA, userB} {
		userID, _ := database.RequireUserID(ctx)
		jobID := "job-" + string(rune('a'+i))
		triggerID := "trigger-" + string(rune('a'+i))
		if err := repo.db.WithContext(ctx).Create(&database.Job{
			UUIDModel:     database.UUIDModel{ID: jobID},
			UserID:        userID,
			Slug:          jobID,
			Name:          jobID,
			Enabled:       true,
			ToolCatalogID: "test-tool-catalog",
			ToolName:      "test_tool",
		}).Error; err != nil {
			t.Fatalf("create job %s: %v", jobID, err)
		}
		if err := repo.db.WithContext(ctx).Create(&database.JobTrigger{
			UUIDModel: database.UUIDModel{ID: triggerID},
			UserID:    userID,
			JobID:     jobID,
			Type:      string(TriggerManual),
			Enabled:   true,
		}).Error; err != nil {
			t.Fatalf("create trigger %s: %v", triggerID, err)
		}
		for j := 0; j < 2; j++ {
			when := time.Now().Add(-2 * time.Hour).Add(time.Duration(j) * time.Minute)
			if err := repo.db.WithContext(ctx).Create(&database.JobRun{
				UUIDModel: database.UUIDModel{ID: uuid.NewString()},
				UserID:    userID,
				JobID:     jobID,
				TriggerID: triggerID,
				Status:    RunStatusCompleted,
				QueuedAt:  when,
				StartedAt: when,
			}).Error; err != nil {
				t.Fatalf("create old run %s/%d: %v", userID, j, err)
			}
		}
	}

	policy := commandMaintenanceTestPolicy()
	policy.BatchSize = 2
	deleted, err := adapters.Jobs.Retain(context.Background(), policy)
	if err != nil {
		t.Fatalf("retain jobs: %v", err)
	}
	if deleted != 4 {
		t.Fatalf("deleted jobs rows = %d, want 4 (two users, two runs)", deleted)
	}
	var remaining int64
	if err := repo.db.Model(&database.JobRun{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining runs: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining runs = %d, want 0", remaining)
	}
}

func TestCommandMaintenanceAdaptersCleanRealToolInvocationsPerUser(t *testing.T) {
	manager, repo, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	if err := repo.db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatalf("migrate chat tables: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for i, userID := range []string{"user-a", "user-b"} {
		if err := repo.db.Create(&database.ToolInvocation{
			UUIDModel:     database.UUIDModel{ID: "inv-" + string(rune('a'+i))},
			UserID:        userID,
			ToolCatalogID: "test-tool-catalog",
			OriginType:    toolinvocations.OriginJobRun,
			OriginID:      "run-" + string(rune('a'+i)),
			Status:        toolinvocations.StatusSucceeded,
			DryRun:        true,
			QueuedAt:      old,
		}).Error; err != nil {
			t.Fatalf("create old invocation %s: %v", userID, err)
		}
	}

	policy := commandMaintenanceTestPolicy()
	policy.BatchSize = 2
	deleted, err := adapters.Tools.CleanOldDryRuns(context.Background(), policy)
	if err != nil {
		t.Fatalf("clean dry runs: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted invocations = %d, want 2", deleted)
	}
	var remaining int64
	if err := repo.db.Model(&database.ToolInvocation{}).Where("dry_run = ?", true).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining dry runs: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining dry runs = %d, want 0", remaining)
	}

	for i, userID := range []string{"user-a", "user-b"} {
		conversationID := "conversation-old-" + string(rune('a'+i))
		if err := repo.db.Create(&database.Conversation{
			UUIDModel: database.UUIDModel{ID: conversationID},
			UserID:    userID,
			Title:     "old chat",
		}).Error; err != nil {
			t.Fatalf("create conversation %s: %v", userID, err)
		}
		if err := repo.db.Create(&database.ToolInvocation{
			UUIDModel:      database.UUIDModel{ID: "chat-old-" + string(rune('a'+i))},
			UserID:         userID,
			ToolCatalogID:  "test-tool-catalog",
			OriginType:     toolinvocations.OriginChat,
			OriginID:       conversationID,
			ConversationID: &conversationID,
			Status:         toolinvocations.StatusSucceeded,
			QueuedAt:       old,
		}).Error; err != nil {
			t.Fatalf("create old chat invocation %s: %v", userID, err)
		}
	}
	deleted, err = adapters.Tools.CleanOldChat(context.Background(), policy)
	if err != nil {
		t.Fatalf("clean old chat: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted old chat invocations = %d, want 2", deleted)
	}

	for i, userID := range []string{"user-a", "user-b"} {
		missingConversationID := "conversation-missing-" + string(rune('a'+i))
		if err := repo.db.Create(&database.ToolInvocation{
			UUIDModel:      database.UUIDModel{ID: "chat-orphan-" + string(rune('a'+i))},
			UserID:         userID,
			ToolCatalogID:  "test-tool-catalog",
			OriginType:     toolinvocations.OriginChat,
			OriginID:       missingConversationID,
			ConversationID: &missingConversationID,
			Status:         toolinvocations.StatusSucceeded,
			QueuedAt:       time.Now(),
		}).Error; err != nil {
			t.Fatalf("create orphan chat invocation %s: %v", userID, err)
		}
	}
	deleted, err = adapters.Tools.CleanOrphanChat(context.Background(), policy)
	if err != nil {
		t.Fatalf("clean orphan chat: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted orphan chat invocations = %d, want 2", deleted)
	}
}

func seedCommandMaintenanceRun(t *testing.T, repo *DBRepository, userID, suffix string, startedAt time.Time) {
	t.Helper()
	jobID := "bounded-job-" + suffix
	triggerID := "bounded-trigger-" + suffix
	if err := repo.db.Create(&database.Job{
		UUIDModel:     database.UUIDModel{ID: jobID},
		UserID:        userID,
		Slug:          jobID,
		Name:          jobID,
		Enabled:       true,
		ToolCatalogID: "test-tool-catalog",
		ToolName:      "test_tool",
	}).Error; err != nil {
		t.Fatalf("create bounded job %s: %v", userID, err)
	}
	if err := repo.db.Create(&database.JobTrigger{
		UUIDModel: database.UUIDModel{ID: triggerID},
		UserID:    userID,
		JobID:     jobID,
		Type:      string(TriggerManual),
		Enabled:   true,
	}).Error; err != nil {
		t.Fatalf("create bounded trigger %s: %v", userID, err)
	}
	if err := repo.db.Create(&database.JobRun{
		UUIDModel: database.UUIDModel{ID: "bounded-run-" + suffix},
		UserID:    userID,
		JobID:     jobID,
		TriggerID: triggerID,
		Status:    RunStatusCompleted,
		QueuedAt:  startedAt,
		StartedAt: startedAt,
	}).Error; err != nil {
		t.Fatalf("create bounded run %s: %v", userID, err)
	}
}

func TestCommandMaintenanceAdaptersBoundedCursorPolicyChangeAndCancellation(t *testing.T) {
	manager, repo, _, _ := newCommandMaintenanceTestManager(t)
	if err := repo.db.Create(&database.User{
		UUIDModel: database.UUIDModel{ID: "user-c"}, Username: "adapter-c", PasswordHash: "test", Role: database.UserRoleUser, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed third user: %v", err)
	}
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	seedCommandMaintenanceRun(t, repo, "user-a", "a", time.Now().Add(-90*time.Minute))
	seedCommandMaintenanceRun(t, repo, "user-b", "b", time.Now().Add(-3*time.Hour))
	seedCommandMaintenanceRun(t, repo, "user-c", "c", time.Now().Add(-3*time.Hour))

	policy := commandMaintenanceTestPolicy()
	policy.JobRetention = 4 * time.Hour
	policy.RunsPerJobKeep = 0
	policy.BatchSize = 1
	result, err := adapters.Jobs.RetainBatch(context.Background(), policy)
	if err != nil {
		t.Fatalf("first bounded jobs batch: %v", err)
	}
	if result.Deleted != 0 || !result.More {
		t.Fatalf("first bounded result = %+v, want no deletion and More", result)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapters.Jobs.RetainBatch(canceled, policy); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled continuation error = %v, want context canceled", err)
	}
	var remainingB int64
	if err := repo.db.Model(&database.JobRun{}).Where("user_id = ?", "user-b").Count(&remainingB).Error; err != nil {
		t.Fatalf("count user-b run after cancellation: %v", err)
	}
	if remainingB != 1 {
		t.Fatalf("user-b runs after cancellation = %d, want 1", remainingB)
	}

	// A policy change resets the cursor to user-a. Continuing at user-b would
	// leave the run that became eligible under the new policy untouched.
	policy.JobRetention = time.Hour
	result, err = adapters.Jobs.RetainBatch(context.Background(), policy)
	if err != nil {
		t.Fatalf("policy-change jobs batch: %v", err)
	}
	if result.Deleted != 1 || !result.More {
		t.Fatalf("policy-change result = %+v, want user-a deletion and More", result)
	}
	result, err = adapters.Jobs.RetainBatch(context.Background(), policy)
	if err != nil || result.Deleted != 1 || !result.More {
		t.Fatalf("second resumed result = %+v, err=%v", result, err)
	}
	result, err = adapters.Jobs.RetainBatch(context.Background(), policy)
	if err != nil || result.Deleted != 1 || result.More {
		t.Fatalf("final resumed result = %+v, err=%v", result, err)
	}
}

func TestCommandMaintenanceAdaptersBoundedToolsContinueAcrossUsers(t *testing.T) {
	manager, repo, _, _ := newCommandMaintenanceTestManager(t)
	if err := repo.db.Create(&database.User{
		UUIDModel: database.UUIDModel{ID: "user-c"}, Username: "adapter-c", PasswordHash: "test", Role: database.UserRoleUser, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("seed third user: %v", err)
	}
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	for i, userID := range []string{"user-a", "user-b", "user-c"} {
		if err := repo.db.Create(&database.ToolInvocation{
			UUIDModel:     database.UUIDModel{ID: "bounded-inv-" + string(rune('a'+i))},
			UserID:        userID,
			ToolCatalogID: "test-tool-catalog",
			OriginType:    toolinvocations.OriginJobRun,
			OriginID:      "bounded-run-" + string(rune('a'+i)),
			Status:        toolinvocations.StatusSucceeded,
			DryRun:        true,
			QueuedAt:      time.Now().Add(-3 * time.Hour),
		}).Error; err != nil {
			t.Fatalf("create bounded invocation %s: %v", userID, err)
		}
	}
	policy := commandMaintenanceTestPolicy()
	policy.BatchSize = 1
	var deleted int64
	for i := 0; i < 3; i++ {
		result, err := adapters.Tools.CleanOldDryRunsBatch(context.Background(), policy)
		if err != nil {
			t.Fatalf("bounded tool batch %d: %v", i, err)
		}
		deleted += result.Deleted
		if result.Deleted != 1 || result.More != (i < 2) {
			t.Fatalf("bounded tool result %d = %+v", i, result)
		}
	}
	if deleted != 3 {
		t.Fatalf("bounded tools deleted = %d, want 3", deleted)
	}
}

func TestCommandMaintenanceAdaptersPreservePartialCountFromFailingUser(t *testing.T) {
	manager, _, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("later cleanup failed")
	deleted, more, err := adapters.Jobs.owner.retainUsers(context.Background(), "partial", commandMaintenanceTestPolicy(), func(context.Context) (int, error) { return 3, failure })
	if deleted != 3 || !more || !errors.Is(err, failure) {
		t.Fatalf("partial=%d more=%v err=%v", deleted, more, err)
	}
}

func TestCommandMaintenanceAdaptersCancellationPreservesCountAndCursor(t *testing.T) {
	manager, _, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	policy := commandMaintenanceTestPolicy()
	policy.BatchSize = 2
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	deleted, more, err := adapters.Jobs.owner.retainUsers(ctx, "test.cancel", policy, func(context.Context) (int, error) {
		calls++
		if calls == 1 {
			cancel()
		}
		return 7, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled batch error = %v, want context canceled", err)
	}
	if deleted != 7 || !more {
		t.Fatalf("canceled batch = deleted %d, more %v; want 7,true", deleted, more)
	}
	deleted, more, err = adapters.Jobs.owner.retainUsers(context.Background(), "test.cancel", policy, func(context.Context) (int, error) {
		return 3, nil
	})
	if err != nil {
		t.Fatalf("resumed batch: %v", err)
	}
	if deleted != 3 || more {
		t.Fatalf("resumed batch = deleted %d, more %v; want 3,false", deleted, more)
	}
}

func TestCommandMaintenanceAdaptersRejectConcurrentOwnerAdmission(t *testing.T) {
	manager, _, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	if !adapters.Jobs.owner.tryAdmit() {
		t.Fatal("initial owner admission rejected")
	}
	defer adapters.Jobs.owner.release()
	if _, err := adapters.Jobs.RetainBatch(context.Background(), commandMaintenanceTestPolicy()); !errors.Is(err, ErrCommandMaintenanceBusy) {
		t.Fatalf("concurrent bounded retention error = %v, want busy", err)
	}
}

func TestCommandMaintenanceAdaptersFailClosedAndHonorCancellation(t *testing.T) {
	manager, _, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapters.Jobs.Retain(canceled, commandMaintenanceTestPolicy()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled jobs cleanup error = %v, want context canceled", err)
	}
	if _, err := adapters.Tools.CleanOldDryRuns(canceled, commandMaintenanceTestPolicy()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled tools cleanup error = %v, want context canceled", err)
	}
	if err := adapters.Compaction.Compact(canceled, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled compaction error = %v, want context canceled", err)
	}
	if _, err := NewCommandMaintenanceAdapters(nil); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("nil manager error = %v, want unavailable", err)
	}
}

func TestCommandMaintenanceAdaptersCompactUsesManagerCapability(t *testing.T) {
	manager, _, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	if err := adapters.Compaction.Compact(context.Background(), 0); err != nil {
		t.Fatalf("compact: %v", err)
	}
	manager.compactMu.Lock()
	compacted := !manager.lastCompaction.IsZero()
	manager.compactMu.Unlock()
	if !compacted {
		t.Fatal("manager capability did not record a real compaction")
	}
}

func TestCommandMaintenanceAdaptersCompactionPropagatesPhysicalError(t *testing.T) {
	manager, repo, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	sqlDB, err := repo.db.DB()
	if err != nil {
		t.Fatalf("database handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if err := adapters.Compaction.Compact(context.Background(), 0); err == nil {
		t.Fatal("closed database compaction returned nil")
	}
	manager.compactMu.Lock()
	compacted := !manager.lastCompaction.IsZero()
	manager.compactMu.Unlock()
	if compacted {
		t.Fatal("failed compaction armed throttle")
	}
}

type commandMaintenanceNoopPort struct{}

func (commandMaintenanceNoopPort) RequeueExpiredLeases(context.Context, int) (int, bool, error) {
	return 0, false, nil
}

func (commandMaintenanceNoopPort) Drain(context.Context, int) (commandmaintenance.BatchResult, error) {
	return commandmaintenance.BatchResult{}, nil
}

func (commandMaintenanceNoopPort) PurgeExpired(context.Context, int) (int, bool, error) {
	return 0, false, nil
}

func (commandMaintenanceNoopPort) Recover(context.Context, int) (commandmaintenance.BatchResult, error) {
	return commandmaintenance.BatchResult{}, nil
}

func (commandMaintenanceNoopPort) Retain(context.Context, commandmaintenance.Policy) (int64, error) {
	return 0, nil
}

func (commandMaintenanceNoopPort) CleanOldDryRuns(context.Context, commandmaintenance.Policy) (int64, error) {
	return 0, nil
}

func (commandMaintenanceNoopPort) CleanOrphanChat(context.Context, commandmaintenance.Policy) (int64, error) {
	return 0, nil
}

func (commandMaintenanceNoopPort) CleanOldChat(context.Context, commandmaintenance.Policy) (int64, error) {
	return 0, nil
}

func TestCommandMaintenanceAdaptersCoordinatorDoesNotReportCompactionOnError(t *testing.T) {
	manager, repo, _, _ := newCommandMaintenanceTestManager(t)
	adapters, err := NewCommandMaintenanceAdapters(manager)
	if err != nil {
		t.Fatalf("new adapters: %v", err)
	}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Outbox:       commandMaintenanceNoopPort{},
		Decisions:    commandMaintenanceNoopPort{},
		Invocations:  commandMaintenanceNoopPort{},
		Claims:       commandMaintenanceNoopPort{},
		Jobs:         commandMaintenanceNoopPort{},
		Tools:        commandMaintenanceNoopPort{},
		InvocationDB: commandMaintenanceNoopPort{},
		Activations:  commandMaintenanceNoopPort{},
		Compaction:   adapters.Compaction,
	})
	if err != nil {
		t.Fatalf("new coordinator: %v", err)
	}
	sqlDB, err := repo.db.DB()
	if err != nil {
		t.Fatalf("database handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	report, err := coordinator.Run(context.Background(), commandMaintenanceTestPolicy())
	if err == nil {
		t.Fatal("coordinator swallowed physical compaction error")
	}
	if report.Compacted {
		t.Fatal("coordinator reported compaction after physical error")
	}
}
