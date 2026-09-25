package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandinstance"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"github.com/google/uuid"
)

// Segunda composição App no mesmo banco e sessão local, sem copiar mutexes,
// epochs ou runtime. O cofre desbloqueado é montado como nas demais fixtures.
func restartCommandApp(t *testing.T, old *App) *App {
	t.Helper()
	ctx := context.Background()
	a := &App{ctx: ctx, sessionSvc: old.sessionSvc, credMgr: old.credMgr,
		workspaceMgr: old.workspaceMgr, questionnaireMgr: old.questionnaireMgr,
		commandStorageVersion: old.commandStorageVersion}
	a.setCurrentUserID(old.currentUserID)
	user := *old.currentAuthUser
	a.setCurrentAuthUser(&user)
	core, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	a.commandHost, err = commandexecution.NewHostState(core, commandProductRegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.commandHost.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ShutdownCommandLifecycle(ctx, a)
		_ = a.drainCommandExecutors(ctx)
		_ = a.shutdownCommandBridgeIfConfigured(ctx)
	})
	return a
}

func reserveRestartCommand(t *testing.T, a *App) (commandledger.LocalReadRequest, *commandledger.Store) {
	t.Helper()
	ctx := context.Background()
	epoch, err := a.commandEpochs.Capture(ctx, a.currentUserID, a.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandledger.New(database.DB(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	req := commandledger.LocalReadRequest{
		InvocationID:   uuid.Must(uuid.NewV7()).String(),
		Owner:          commandledger.Owner{UserID: epoch.UserID, AuthContextID: epoch.SessionID},
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		RegistryVersion: commandProductRegistryVersion, GlobalConfigGeneration: "global",
		ActiveLayersGeneration: "layers", CommandID: commandProductWorkspaceListID,
		SourceType: "palette", ArgumentsFingerprint: "args", RequestFingerprintVersion: "v1",
		RequestFingerprint: "request", CorrelationID: uuid.Must(uuid.NewV7()).String(),
		ReceivedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if _, err := store.Reserve(ctx, req); err != nil {
		t.Fatal(err)
	}
	return req, store
}

func TestCommandRestartExcludesLiveInstanceAndRecoversRegisteredPastGeneration(t *testing.T) {
	ctx := context.Background()
	old := readyCommandProduct(t)
	req, store := reserveRestartCommand(t, old)
	fresh := restartCommandApp(t, old)
	if err := fresh.mountCommandProduct(ctx); !errors.Is(err, commandinstance.ErrBusy) {
		t.Fatalf("segunda instância não recusou disputa: %v", err)
	}
	if fresh.commandProduct.Load() != nil {
		t.Fatal("publicou produto durante disputa")
	}
	before, err := store.Get(ctx, req.Owner, req.InvocationID)
	if err != nil || before.Status != commandledger.Evaluating {
		t.Fatalf("pendência viva alterada: %+v %v", before, err)
	}

	// Simula persistência deixada antes da reconciliação, sem encerrar processo
	// nem soltar a autoridade de um executor vivo. Não chama recovery local.
	if _, err := old.commandEpochs.CloseAndDrain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := old.commandEpochs.ReleaseInstance(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.mountCommandProduct(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	after, err := store.Get(ctx, req.Owner, req.InvocationID)
	if err != nil || after.Status != commandledger.OutcomeUnknown {
		t.Fatalf("recovery não fechou ledger/audit: %+v %v", after, err)
	}
	var status string
	if err := database.DB().Table("command_idempotency_keys").Select("status").Where("invocation_id = ?", req.InvocationID).Scan(&status).Error; err != nil || status != "outcome_unknown" {
		t.Fatalf("ledger não acompanhou audit: %s %v", status, err)
	}
	if replay, err := store.Reserve(ctx, req); err != nil || replay.Created || replay.Record.Status != commandledger.OutcomeUnknown {
		t.Fatalf("replay ressuscitou a reserva encerrada: %+v %v", replay, err)
	}
	if result, err := fresh.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err != nil || result.Status != "succeeded" {
		t.Fatalf("nova composição não executou comando real: %+v %v", result, err)
	}
}

func TestCommandRestartKeepsCurrentGenerationPendingAndFailsClosed(t *testing.T) {
	a := readyCommandProduct(t)
	req, store := reserveRestartCommand(t, a)
	runtime := &appCommandLifecycleRuntime{epochs: a.commandEpochs}
	if err := runtime.Recover(context.Background(), commandruntime.Generation{Value: "current"}); !errors.Is(err, commandruntime.ErrNotReady) {
		t.Fatalf("recovery confundiu geração atual com abandonada: %v", err)
	}
	record, err := store.Get(context.Background(), req.Owner, req.InvocationID)
	if err != nil || record.Status != commandledger.Evaluating {
		t.Fatalf("geração atual alterada: %+v %v", record, err)
	}
}
