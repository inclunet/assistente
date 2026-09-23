package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandmaintenance"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

func commandMaintenanceAppFixture(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(configdir.ResetForTests)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	configdir.ResetForTests()
	a := readyCommandProduct(t)
	db := database.DB()
	if err := db.AutoMigrate(&database.MCPServer{}, &database.ToolCatalog{}, &database.ToolInvocation{}, &database.Tag{}, &database.TagAssignment{}, &database.JobPipeline{}, &database.Job{}, &database.JobProfileGrant{}, &database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{}, &database.JobTrigger{}, &database.JobRun{}, &database.JobEvent{}, &database.JobRunEvent{}, &database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(commandjobevents.Models()...); err != nil {
		t.Fatal(err)
	}
	if err := commandjobactivation.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	a.toolInvocationSvc = toolinvocations.NewService(toolinvocations.NewDBRepository(db), nil)
	a.toolRegistry = tools.NewRegistry()
	a.jobMgr = jobs.NewManager(jobs.ManagerConfig{Repository: jobs.NewDBRepository(db), ToolRegistry: a.toolRegistry, ToolInvocations: a.toolInvocationSvc, ContextProvider: func() context.Context { return database.WithUserID(context.Background(), a.currentUserID) }, CommandRuntimeIdentity: a.captureCommandJobIdentity})
	t.Cleanup(a.jobMgr.Stop)
	return a
}

func TestCommandMaintenanceAppMountsRealDomainsAndDrainsBeforeRelease(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	ctx := context.Background()
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	mounted := a.commandMaintenance.Load()
	if mounted == nil || mounted.consumer == nil {
		t.Fatal("Consumer produtivo ausente")
	}
	if err := a.configureCommandMaintenance(ctx); err != nil || a.commandMaintenance.Load() != mounted {
		t.Fatalf("montagem idempotente: %v", err)
	}
	// Exercita as mesmas portas produtivas, sem substituir domínio por no-op.
	legacy, err := jobs.NewCommandMaintenanceAdapters(a.jobMgr)
	if err != nil {
		t.Fatal(err)
	}
	ports := mounted.ports
	ports.Jobs, ports.Tools, ports.Compaction = legacy.Jobs, legacy.Tools, legacy.Compaction
	coordinator, err := commandmaintenance.New(ports)
	if err != nil {
		t.Fatal(err)
	}
	policy := commandmaintenance.Policy{JobRetention: time.Hour, RunsPerJobKeep: 10, InvocationRetention: time.Hour, InvocationsPerUser: 10, InvocationsSystemKeep: 10, ActivationRetention: time.Hour, ActivationsPerUser: 10, LeaseDuration: time.Minute, BatchSize: 2}
	report, err := coordinator.Run(ctx, policy)
	if err != nil || !report.OutboxDrained || !report.Compacted {
		t.Fatalf("ciclo real: %+v %v", report, err)
	}
	if err := a.jobMgr.Start(); err != nil {
		t.Fatal(err)
	}
	if err := a.drainCommandExecutors(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.jobMgr.Start(); !errors.Is(err, jobs.ErrCommandMaintenanceUnavailable) {
		t.Fatalf("Start após fechamento do core: %v", err)
	}
}

func TestCommandJobIdentityUsesSessionEpochAndIndependentTerminalFence(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	identity, watch, release, err := a.captureCommandJobIdentity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if identity.UserID != a.currentUserID || identity.AuthContextID != a.currentAuthUser.SessionID || identity.SecurityGeneration == "" || watch.Err() != nil {
		t.Fatalf("identidade incompleta: %+v", identity)
	}
	authority := a.commandJobAuthority.Load()
	second, secondWatch, secondRelease, err := a.captureCommandJobIdentity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer secondRelease()
	if second != identity || watch.Err() != nil || secondWatch.Err() != nil {
		t.Fatal("captura concorrente da mesma sessão invalidou outro run")
	}
	if authority.watch.Err() == nil {
		t.Fatal("fence substituída não liberada")
	}
	authority = a.commandJobAuthority.Load()
	release()
	if watch.Err() == nil || authority.watch.Err() != nil {
		t.Fatal("fim do job invalidou indevidamente a fence de eventos terminais")
	}
	if err := a.commandEpochs.InvalidateSession(ctx, identity.UserID, identity.AuthContextID); err != nil {
		t.Fatal(err)
	}
	if authority.watch.Err() == nil {
		t.Fatal("revogação não invalidou fence terminal")
	}
	if secondWatch.Err() == nil {
		t.Fatal("revogação preservou prova viva do segundo run")
	}
	if _, _, _, err := a.captureCommandJobIdentity(database.WithUserID(ctx, "foreign")); !errors.Is(err, commandjobactivation.ErrUnavailable) {
		t.Fatalf("owner estranho: %v", err)
	}
}

func TestCommandMaintenanceFailedReplacementPreservesMountedPorts(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	ctx := context.Background()
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	previous := a.commandMaintenance.Load()
	if err := a.jobMgr.ReconfigureCommandMaintenance(commandmaintenance.Ports{}); err == nil {
		t.Fatal("substituição incompleta aceita")
	}
	if a.commandMaintenance.Load() != previous {
		t.Fatal("falha alterou composição publicada")
	}
	if err := a.configureCommandMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
}
