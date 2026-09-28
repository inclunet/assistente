package app

import (
	"context"
	"errors"
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
	orphanErr   error
	oldErr      error
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
	if r.orphanErr != nil {
		return 0, r.orphanErr
	}
	return r.Repository.CleanOrphanChat(ctx)
}

func (r *startupChatRetentionRepository) CleanOldChat(ctx context.Context, age time.Duration) (int, error) {
	r.old.Add(1)
	r.observe()
	if r.oldErr != nil {
		return 0, r.oldErr
	}
	return r.Repository.CleanOldChat(ctx, age)
}

func TestChatStartupRetentionCoordinatedAndLegacyPaths(t *testing.T) {
	for _, tc := range []struct {
		capDays int
		legacy  bool
	}{{1, false}, {0, false}, {1, true}, {0, true}} {
		capDays := tc.capDays
		t.Run(fmt.Sprintf("legacy_%t_cap_%d", tc.legacy, capDays), func(t *testing.T) {
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
			if tc.legacy {
				if a.commandMaintenance.Load() != nil {
					t.Fatal("fixture legada não pode conter coordenador já montado")
				}
				a.commandStorageVersion = ""
			}
			a.authSessionMu.Lock()
			result := a.reloadUserScopedRuntime()
			a.authSessionMu.Unlock()
			if result.hasFailures() {
				t.Fatalf("reload: %+v", result.failures)
			}
			want := []string{"retention-recent"}
			if capDays == 0 {
				want = []string{"retention-old", "retention-recent"}
			}
			if tc.legacy {
				// Sem armazenamento de comandos, o contrato legado preserva a
				// passagem síncrona de Start: o reload já retorna com a limpeza.
				if got := remaining(); !reflect.DeepEqual(got, want) {
					t.Fatalf("retenção legada não ocorreu durante reload: got=%v want=%v", got, want)
				}
				a.jobMgr.Stop()
				if repo.orphans.Load() == 0 || (capDays > 0 && repo.old.Load() == 0) || starts.starts.Load() != 1 || a.commandJobsPending != nil || !repo.beforeReady.Load() {
					t.Fatal("caminho legado deixou de executar retenção síncrona independente dos comandos")
				}
				return
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

func TestChatStartupRetentionLegacyReportsFailuresWithoutFailingJobs(t *testing.T) {
	for _, failure := range []string{"orphan", "cap", "both"} {
		t.Run(failure, func(t *testing.T) {
			a, starts := commandJobsStartupFixture(t)
			a.jobMgr.Stop()
			repo := &startupChatRetentionRepository{Repository: toolinvocations.NewDBRepository(database.DB()), app: a}
			orphanErr, capErr := errors.New("orphan cleanup failed"), errors.New("age cap failed")
			if failure != "cap" {
				repo.orphanErr = orphanErr
			}
			if failure != "orphan" {
				repo.oldErr = capErr
			}
			a.toolInvocationSvc = toolinvocations.NewService(repo, nil)
			a.jobMgr = jobs.NewManager(jobs.ManagerConfig{
				Repository: jobs.NewDBRepository(database.DB()), ToolRegistry: a.toolRegistry,
				ToolInvocations:        a.toolInvocationSvc,
				ContextProvider:        func() context.Context { return database.WithUserID(context.Background(), a.currentUserID) },
				CommandRuntimeIdentity: a.captureCommandJobIdentity,
			})
			t.Cleanup(a.jobMgr.Stop)
			a.commandStorageVersion = ""
			settings := config.DefaultMaintenanceSettings()
			settings.ChatToolCallsRetentionDays = 1
			if err := config.SaveMaintenance(settings); err != nil {
				t.Fatal(err)
			}
			emitter := &testEmitter{}
			a.emitter = emitter
			a.authSessionMu.Lock()
			result := a.reloadUserScopedRuntime()
			a.authSessionMu.Unlock()
			if len(result.failures) != 1 || result.failures[0].Subsystem != runtimeSubsystemToolInvocations {
				t.Fatalf("diagnóstico deve conter somente tool_invocations: %+v", result.failures)
			}
			if starts.starts.Load() != 1 || a.commandJobsPending != nil {
				t.Fatal("retenção impediu Start legado")
			}
			got := a.jobMgr.InitialChatRetentionError()
			if errors.Is(got, orphanErr) != (failure != "cap") || errors.Is(got, capErr) != (failure != "orphan") {
				t.Fatalf("diagnóstico perdeu erros: %v", got)
			}
			a.emitRuntimePartialInit(result)
			events := emitter.find(RuntimePartialInitEventName)
			if len(events) != 1 {
				t.Fatalf("avisos=%d", len(events))
			}
			payload, ok := events[0].data.(RuntimePartialInitPayload)
			if !ok || !reflect.DeepEqual(payload.Subsystems, result.failures) {
				t.Fatalf("aviso inválido: %+v", events[0])
			}
			if err := a.jobMgr.Start(); err != nil {
				t.Fatalf("Start já ativo: %v", err)
			}
			if repo.orphans.Load() != 1 || repo.old.Load() != 1 || a.jobMgr.InitialChatRetentionError() == nil {
				t.Fatal("Start idempotente repetiu cleanup ou apagou diagnóstico")
			}
			a.jobMgr.Stop()
			repo.orphanErr, repo.oldErr = nil, nil // só após join do loop
			if err := a.jobMgr.Start(); err != nil {
				t.Fatalf("nova partida: %v", err)
			}
			if got := a.jobMgr.InitialChatRetentionError(); got != nil {
				t.Fatalf("erro antigo sobreviveu à nova partida: %v", got)
			}
			if starts.starts.Load() != 2 || repo.orphans.Load() != 2 || repo.old.Load() != 2 {
				t.Fatal("nova partida não executou cleanup exatamente uma vez")
			}
		})
	}
}
