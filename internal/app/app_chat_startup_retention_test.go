package app

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandruntime"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/toolinvocations"
)

// Observa as portas sem substituir os DELETEs nem a política do repositório.
type startupChatRetentionRepository struct {
	toolinvocations.Repository
	app         *App
	orphans     atomic.Int32
	old         atomic.Int32
	beforeReady atomic.Bool
}

func (r *startupChatRetentionRepository) observe() {
	snapshot, err := CommandLifecycleSnapshot(r.app)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		r.beforeReady.Store(true)
	}
}

func (r *startupChatRetentionRepository) CleanOrphanChat(ctx context.Context) (int, error) {
	r.orphans.Add(1)
	r.observe()
	return r.Repository.CleanOrphanChat(ctx)
}

func (r *startupChatRetentionRepository) CleanOldChat(ctx context.Context, age time.Duration) (int, error) {
	r.old.Add(1)
	r.observe()
	return r.Repository.CleanOldChat(ctx, age)
}

func TestChatStartupRetentionRunsOnlyAfterCommandsReady(t *testing.T) {
	for _, capDays := range []int{1, 0} {
		t.Run(fmt.Sprintf("cap_%d", capDays), func(t *testing.T) {
			a, starts := commandJobsStartupFixture(t)
			db := database.DB()
			repo := &startupChatRetentionRepository{Repository: toolinvocations.NewDBRepository(db), app: a}
			a.jobMgr.Stop()
			a.toolInvocationSvc = toolinvocations.NewService(repo, nil)
			a.jobMgr = jobs.NewManager(jobs.ManagerConfig{
				Repository: jobs.NewDBRepository(db), ToolRegistry: a.toolRegistry,
				ToolInvocations:        a.toolInvocationSvc,
				ContextProvider:        func() context.Context { return database.WithUserID(context.Background(), a.currentUserID) },
				CommandRuntimeIdentity: a.captureCommandJobIdentity,
			})
			t.Cleanup(a.jobMgr.Stop)
			settings := config.DefaultMaintenanceSettings()
			settings.ChatToolCallsRetentionDays = capDays
			if err := config.SaveMaintenance(settings); err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), a.currentUserID)
			if err := db.Create(&database.ToolCatalog{Name: "startup-retention-tool", DisplayName: "Startup retention", Origin: "builtin", AvailabilityStatus: "available"}).Error; err != nil {
				t.Fatal(err)
			}
			toolID, err := repo.ResolveToolCatalogID(ctx, "startup-retention-tool")
			if err != nil {
				t.Fatal(err)
			}
			conversation := database.Conversation{UUIDModel: database.UUIDModel{ID: "retention-conversation"}, UserID: a.currentUserID, Title: "Retenção"}
			if err := db.Create(&conversation).Error; err != nil {
				t.Fatal(err)
			}
			message := database.ChatMessage{UUIDModel: database.UUIDModel{ID: "retention-message"}, ConversationID: conversation.ID, Role: "assistant"}
			if err := db.Create(&message).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			for _, row := range []struct {
				id, origin string
				queued     time.Time
			}{
				{"retention-orphan", "missing-message", now},
				{"retention-old", message.ID, now.Add(-72 * time.Hour)},
				{"retention-recent", message.ID, now},
			} {
				invocation := database.ToolInvocation{UUIDModel: database.UUIDModel{ID: row.id}, UserID: a.currentUserID,
					ToolCatalogID: toolID, OriginType: toolinvocations.OriginChat, OriginID: row.origin,
					ToolCallID: "call-" + row.id, Status: toolinvocations.StatusQueued, QueuedAt: row.queued}
				if err := db.Create(&invocation).Error; err != nil {
					t.Fatal(err)
				}
			}
			remaining := func() []string {
				t.Helper()
				var ids []string
				if err := db.Model(&database.ToolInvocation{}).Where("user_id = ?", a.currentUserID).Order("id").Pluck("id", &ids).Error; err != nil {
					t.Fatal(err)
				}
				return ids
			}
			if err := ResetCommandLifecycle(ctx, a, "retention_startup_test"); err != nil {
				t.Fatal(err)
			}
			a.authSessionMu.Lock()
			result := a.reloadUserScopedRuntime()
			a.authSessionMu.Unlock()
			if result.hasFailures() {
				t.Fatalf("reload: %+v", result.failures)
			}
			if repo.orphans.Load() != 0 || repo.old.Load() != 0 {
				t.Fatalf("reload executou retenção síncrona: orphans=%d old=%d", repo.orphans.Load(), repo.old.Load())
			}
			if got := remaining(); !reflect.DeepEqual(got, []string{"retention-old", "retention-orphan", "retention-recent"}) {
				t.Fatalf("reload alterou os registros: %v", got)
			}
			if starts.starts.Load() != 0 || a.commandJobsPending == nil {
				t.Fatal("reload não preservou a barreira de startup")
			}
			if err := a.bootstrapCommandsAndStartPreparedJobs(ctx, a.currentAuthUser, nil); err != nil {
				t.Fatal(err)
			}
			want := []string{"retention-recent"}
			if capDays == 0 {
				want = []string{"retention-old", "retention-recent"}
			}
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			for !reflect.DeepEqual(remaining(), want) {
				select {
				case <-deadline.C:
					t.Fatalf("manutenção não limpou após Start: got=%v want=%v orphans=%d old=%d", remaining(), want, repo.orphans.Load(), repo.old.Load())
				case <-tick.C:
				}
			}
			a.jobMgr.Stop() // drena a passagem antes das asserções finais.
			if repo.orphans.Load() == 0 || (capDays > 0 && repo.old.Load() == 0) {
				t.Fatal("portas de retenção não executadas")
			}
			if starts.starts.Load() != 1 || starts.beforeReady.Load() || repo.beforeReady.Load() {
				t.Fatal("retenção ou Start antecedeu readiness")
			}
			if a.commandJobsPending != nil {
				t.Fatal("Start não consumiu pendência")
			}
		})
	}
}
