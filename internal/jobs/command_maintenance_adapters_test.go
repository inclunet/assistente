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

	deleted, err := adapters.Jobs.Retain(context.Background(), commandMaintenanceTestPolicy())
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
			UUIDModel:      database.UUIDModel{ID: "inv-" + string(rune('a'+i))},
			UserID:         userID,
			ToolCatalogID:  "test-tool-catalog",
			OriginType:     toolinvocations.OriginJobRun,
			OriginID:       "run-" + string(rune('a'+i)),
			Status:         toolinvocations.StatusSucceeded,
			DryRun:         true,
			QueuedAt:       old,
		}).Error; err != nil {
			t.Fatalf("create old invocation %s: %v", userID, err)
		}
	}

	deleted, err := adapters.Tools.CleanOldDryRuns(context.Background(), commandMaintenanceTestPolicy())
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
	deleted, err = adapters.Tools.CleanOldChat(context.Background(), commandMaintenanceTestPolicy())
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
	deleted, err = adapters.Tools.CleanOrphanChat(context.Background(), commandMaintenanceTestPolicy())
	if err != nil {
		t.Fatalf("clean orphan chat: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted orphan chat invocations = %d, want 2", deleted)
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
