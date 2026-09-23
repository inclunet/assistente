package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandledger"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/workspace"

	"github.com/google/uuid"
)

// Reinicializa App/core/Manager sobre o mesmo arquivo temporário. A liberação
// nativa é real. Não simula queda de processo por executável auxiliar e não
// altera registros de startup/epochs para fabricar uma prova de restart.
func restartCommandMaintenanceApp(t *testing.T, previous *App) *App {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	previous.jobMgr.Stop()
	if err := ShutdownCommandLifecycle(ctx, previous); err != nil {
		t.Fatal(err)
	}
	core, err := previous.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	// Fecha os executores, mas deixa a recuperação para o novo bootstrap.
	// Isso reproduz registros duráveis pendentes após a perda da instância.
	if _, err := core.CloseAndDrain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := core.ReleaseInstance(ctx); err != nil {
		t.Fatal(err)
	}
	if err := previous.shutdownCommandBridgeIfConfigured(ctx); err != nil {
		t.Fatal(err)
	}

	manager := workspace.NewManager(filepath.Join(t.TempDir(), "restarted-workspace-home"))
	if err := manager.Initialize(previous.workspaceMgr.ActivePath()); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), sessionSvc: previous.sessionSvc, credMgr: previous.credMgr,
		questionnaireMgr: questionnaire.NewManager(func(string, any) {}), workspaceMgr: manager,
		commandStorageVersion: previous.commandStorageVersion, authKeyringDelete: func() error { return nil }}
	a.setCurrentUserID(previous.currentUserID)
	a.setCurrentAuthUser(previous.currentAuthUser)
	newCore, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	host, err := commandexecution.NewHostState(newCore, commandProductRegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := host.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	a.commandHost = host
	a.toolRegistry = tools.NewRegistry()
	a.toolInvocationSvc = toolinvocations.NewService(toolinvocations.NewDBRepository(database.DB()), tools.NewExecutor(a.toolRegistry, tools.DefaultExecutorConfig()))
	a.jobMgr = jobs.NewManager(jobs.ManagerConfig{Repository: jobs.NewDBRepository(database.DB()),
		ToolRegistry: a.toolRegistry, ToolInvocations: a.toolInvocationSvc,
		ContextProvider:        func() context.Context { return database.WithUserID(a.ctx, a.currentUserID) },
		CommandRuntimeIdentity: a.captureCommandJobIdentity})
	t.Cleanup(func() {
		a.jobMgr.Stop()
		_ = ShutdownCommandLifecycle(context.Background(), a)
		_ = a.drainCommandExecutors(context.Background())
		_ = a.shutdownCommandBridgeIfConfigured(context.Background())
	})
	if err := a.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCommandMaintenanceRestartRemountsRealApp(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	settings := config.DefaultMaintenanceSettings()
	settings.CommandJobActivationLeaseSeconds = 3
	settings.JobRetentionHours = 1
	settings.ChatToolCallsRetentionDays = 1
	settings.VacuumMinFreeBytes = 0
	if err := config.SaveMaintenance(settings); err != nil {
		t.Fatal(err)
	}
	if err := a.configureCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	legacy := seedCommandMaintenanceLegacy(t, a.currentUserID)
	next := restartCommandMaintenanceApp(t, a)
	if next.commandEpochs == a.commandEpochs || next.jobMgr == a.jobMgr || next.commandMaintenance.Load() == a.commandMaintenance.Load() {
		t.Fatal("restart reutilizou core, Manager ou consumidor da instância anterior")
	}
	if err := next.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	legacy.awaitCleanedAndCompacted(t)
}

func TestCommandMaintenanceRestartRecoversMultipleOwnersAndProtectsLiveJob(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	settings := config.DefaultMaintenanceSettings()
	settings.CommandJobActivationLeaseSeconds = 3
	settings.JobRetentionHours = 1
	settings.ChatToolCallsRetentionDays = 1
	settings.CommandInvocationRetentionDays = 1
	settings.CommandActivationTerminalRetentionDays = 1
	settings.VacuumMinFreeBytes = 0
	if err := config.SaveMaintenance(settings); err != nil {
		t.Fatal(err)
	}
	if err := a.configureCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	pending := seedCommandMaintenancePending(t, a)
	legacy := seedCommandMaintenanceLegacy(t, pending.CurrentUserID, pending.OtherUserID)
	next := restartCommandMaintenanceApp(t, a)
	pending.assertAfterRestart(t)
	liveStore, err := commandledger.New(database.DB(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	liveEpoch, err := next.commandEpochs.Capture(context.Background(), next.currentUserID, next.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	currentPending := seedCommandMaintenanceInvocation(t, liveStore, liveEpoch, "current-generation")
	currentSystem := seedCommandMaintenanceSystem(t, liveStore, liveEpoch.SecurityGeneration)
	// Órfãos sem fonte são inseridos após o bootstrap: não representam uma
	// configuração válida para publicação do catálogo no boot.
	seedClaimsAndLeases(t, 4, pending.CurrentUserID, pending.OtherUserID)
	var staleIDs []string
	if err := database.DB().Model(&commandactivation.Claim{}).Pluck("activation_id", &staleIDs).Error; err != nil {
		t.Fatal(err)
	}
	var oldClaim commandactivation.Claim
	if err := database.DB().Where("activation_id = ?", staleIDs[0]).Take(&oldClaim).Error; err != nil {
		t.Fatal(err)
	}
	oldClaim.ActivationID = uuid.Must(uuid.NewV7()).String()
	oldClaim.State = commandactivation.StateInactive
	oldClaim.UpdatedAt = time.Now().UTC().Add(-48 * time.Hour)
	oldClaim.ActivatedAt = oldClaim.UpdatedAt
	oldClaim.ExpiresAt = timePtr(oldClaim.UpdatedAt)
	if err := database.DB().Create(&oldClaim).Error; err != nil {
		t.Fatal(err)
	}
	// Sentinela somente no SQLite temporário: uma inversão da ordem que
	// apague runs antes de reconciliar claims antigas deve falhar, não apenas
	// produzir o mesmo estado final por caminhos diferentes.
	if err := database.DB().Exec(`CREATE TRIGGER maintenance_recovery_before_jobs
		BEFORE DELETE ON job_runs WHEN EXISTS (
		SELECT 1 FROM command_layer_activation_state
		WHERE security_generation = 'security-seed' AND state = 'active')
		BEGIN SELECT RAISE(ABORT, 'claims recovery must precede jobs retention'); END`).Error; err != nil {
		t.Fatal(err)
	}
	control := startCommandMaintenanceLiveJob(t, next)
	runID := <-control.RunID
	claim, first := waitLiveClaimAndLease(t, runID)
	legacy.awaitCleanedAndCompacted(t)
	deadline := time.Now().Add(10 * time.Second)
	for {
		var stale, oldAudit, oldActivations int64
		var lease commandjobactivation.Lease
		db := database.DB()
		if err := db.Model(&commandactivation.Claim{}).Where("activation_id IN ? AND state = ?", staleIDs, commandactivation.StateActive).Count(&stale).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Table("command_invocations").Where("invocation_id = ?", pending.OldTerminalID).Count(&oldAudit).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&commandactivation.Claim{}).Where("activation_id = ?", oldClaim.ActivationID).Count(&oldActivations).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Where("activation_id = ?", claim.ActivationID).Take(&lease).Error; err != nil {
			t.Fatalf("manutenção perdeu lease viva: %v", err)
		}
		if stale == 0 && oldAudit == 0 && oldActivations == 0 && lease.ExpiresAt.After(first.ExpiresAt) && time.Now().After(first.ExpiresAt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ciclo não convergiu: claims antigas=%d auditoria antiga=%d ativações antigas=%d lease=%+v", stale, oldAudit, oldActivations, lease)
		}
		time.Sleep(25 * time.Millisecond)
	}
	var active commandactivation.Claim
	if err := database.DB().Where("activation_id = ?", claim.ActivationID).Take(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active.State != commandactivation.StateActive {
		t.Fatalf("job vivo foi encerrado pela manutenção: %+v", active)
	}
	var running int64
	if err := database.DB().Model(&database.ToolInvocation{}).Where("origin_id = ? AND status = ?", runID, toolinvocations.StatusRunning).Count(&running).Error; err != nil {
		t.Fatal(err)
	}
	if running != 1 {
		t.Fatalf("ledger da tool viva: running=%d, want 1", running)
	}
	for _, table := range []string{"command_invocations", "command_idempotency_keys"} {
		var live int64
		if err := database.DB().Table(table).Where("invocation_id IN ? AND status = ?", []string{currentPending, currentSystem}, commandledger.Evaluating).Count(&live).Error; err != nil {
			t.Fatal(err)
		}
		if live != 2 {
			t.Fatalf("geração atual recuperada indevidamente em %s: %d", table, live)
		}
		var expired int64
		if err := database.DB().Table(table).Where("invocation_id = ?", pending.OldTerminalID).Count(&expired).Error; err != nil {
			t.Fatal(err)
		}
		if expired != 0 {
			t.Fatalf("terminal expirado preservado em %s", table)
		}
	}
	pending.assertAfterRestart(t)
	control.Release()
	result, err := control.Join()
	if err != nil || result == nil || result.Status != jobs.RunStatusCompleted {
		t.Fatalf("conclusão do job: %+v %v", result, err)
	}
	if err := waitForNoLiveLease(claim.ActivationID); err != nil {
		t.Fatal(err)
	}
}

type commandMaintenanceLegacySeed struct {
	runIDs, invocationIDs []string
}

func seedCommandMaintenanceLegacy(t *testing.T, userIDs ...string) commandMaintenanceLegacySeed {
	t.Helper()
	db := database.DB()
	var initialVacuumMode int
	if err := db.Raw("PRAGMA auto_vacuum").Scan(&initialVacuumMode).Error; err != nil {
		t.Fatal(err)
	}
	if initialVacuumMode != 0 {
		t.Fatalf("fixture deve iniciar sem auto_vacuum para observar compactação real: %d", initialVacuumMode)
	}
	seed := commandMaintenanceLegacySeed{}
	old := time.Now().Add(-72 * time.Hour)
	for _, userID := range userIDs {
		jobID, triggerID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		conversationID := uuid.NewString()
		for _, row := range []any{
			&database.Job{UUIDModel: database.UUIDModel{ID: jobID}, UserID: userID, Slug: jobID, Name: "legacy retention", ToolCatalogID: "legacy-catalog", ToolName: "legacy-tool"},
			&database.JobTrigger{UUIDModel: database.UUIDModel{ID: triggerID}, UserID: userID, JobID: jobID, Type: string(jobs.TriggerManual)},
			&database.JobRun{UUIDModel: database.UUIDModel{ID: runID}, UserID: userID, JobID: jobID, TriggerID: triggerID, Status: jobs.RunStatusCompleted, QueuedAt: old, StartedAt: old},
			&database.Conversation{UUIDModel: database.UUIDModel{ID: conversationID}, UserID: userID, Title: "retained conversation"},
		} {
			if err := db.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		seed.runIDs = append(seed.runIDs, runID)
		missingConversation := uuid.NewString()
		for _, row := range []database.ToolInvocation{
			{UserID: userID, ToolCatalogID: "legacy-catalog", OriginType: toolinvocations.OriginJobRun, OriginID: runID, Status: toolinvocations.StatusSucceeded, DryRun: true, QueuedAt: old},
			{UserID: userID, ToolCatalogID: "legacy-catalog", OriginType: toolinvocations.OriginChat, OriginID: conversationID, ConversationID: &conversationID, Status: toolinvocations.StatusSucceeded, QueuedAt: old},
			{UserID: userID, ToolCatalogID: "legacy-catalog", OriginType: toolinvocations.OriginChat, OriginID: missingConversation, ConversationID: &missingConversation, Status: toolinvocations.StatusSucceeded, QueuedAt: time.Now()},
		} {
			row.ID = uuid.NewString()
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			seed.invocationIDs = append(seed.invocationIDs, row.ID)
		}
	}
	return seed
}

func (s commandMaintenanceLegacySeed) awaitCleanedAndCompacted(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var runs, invocations int64
	var mode int
	for {
		db := database.DB()
		if err := db.Model(&database.JobRun{}).Where("id IN ?", s.runIDs).Count(&runs).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&database.ToolInvocation{}).Where("id IN ?", s.invocationIDs).Count(&invocations).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Raw("PRAGMA auto_vacuum").Scan(&mode).Error; err != nil {
			t.Fatal(err)
		}
		if runs == 0 && invocations == 0 && mode == 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("manutenção não convergiu: runs=%d tools=%d auto_vacuum=%d", runs, invocations, mode)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
