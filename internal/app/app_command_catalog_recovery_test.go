package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
)

func TestRecoverCommandCatalogAfterProductClosedAndLifecycleUnmounted(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ticket := a.captureCommandCatalogRecoveryTicket()
	if ticket == nil || !ticket.current(a) {
		t.Fatal("ticket autenticado inválido antes da desmontagem")
	}
	old := a.commandProduct.Load()
	before, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	shutdownCommandCatalogProductForTest(t, ctx, a)
	if a.commandProduct.Load() != old || a.commandLifecycle.Load() != nil {
		t.Fatal("fixture não representa produto fechado com lifecycle já desmontado")
	}

	if err := a.refreshCommandProductCatalogForRecovery(ctx, ticket); err != nil {
		t.Fatalf("retomada autenticada do hot-swap parcial: %v", err)
	}
	assertCommandCatalogReadyAfterRecovery(t, a, old, before.Generation)
}

func TestRecoverCommandCatalogRetriesAfterMountDependencyFailure(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ticket := a.captureCommandCatalogRecoveryTicket()
	if ticket == nil || !ticket.current(a) {
		t.Fatal("ticket autenticado inválido antes da falha de mount")
	}
	old := a.commandProduct.Load()
	shutdownCommandCatalogProductForTest(t, ctx, a)
	a.commandLifecycleMount.Lock()
	a.authMu.Lock()
	a.commandProduct.Store(nil)
	a.commandBridge.Store(nil)
	a.commandRegistry = nil
	host := a.commandHost
	a.commandHost = nil
	a.authMu.Unlock()
	a.commandLifecycleMount.Unlock()

	if !ticket.current(a) {
		t.Fatal("a fixture invalidou o ticket ao simular falha de mount")
	}
	if err := a.refreshCommandProductCatalogForRecovery(ctx, ticket); !errors.Is(err, commandruntime.ErrMissingDependency) {
		t.Fatalf("falha de montagem injetada: got %v, want ErrMissingDependency", err)
	}
	if a.commandProduct.Load() != nil || a.commandLifecycle.Load() != nil {
		t.Fatal("falha de montagem publicou produto/lifecycle parcial")
	}

	a.authMu.Lock()
	a.commandHost = host
	a.authMu.Unlock()
	if !ticket.current(a) {
		t.Fatal("restaurar host invalidou o ticket")
	}
	if err := a.refreshCommandProductCatalogForRecovery(ctx, ticket); err != nil {
		t.Fatalf("retry após restaurar dependência de montagem: %v", err)
	}
	assertCommandCatalogReadyAfterRecovery(t, a, old, "")
}

func TestRecoverCommandCatalogJoinsStoppedLifecycleInsteadOfResettingIt(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	old := a.commandProduct.Load()
	ticket := a.captureCommandCatalogRecoveryTicket()
	if ticket == nil || !ticket.current(a) {
		t.Fatal("ticket autenticado inválido antes de parar lifecycle")
	}
	runtime := a.commandLifecycle.Load()
	if runtime == nil {
		t.Fatal("fixture sem lifecycle")
	}
	if err := runtime.Stop(ctx); err != nil && !errors.Is(err, commandruntime.ErrStopped) {
		t.Fatalf("stop fixture: %v", err)
	}
	if err := runtime.WaitStopped(ctx); err != nil {
		t.Fatalf("join fixture: %v", err)
	}
	if a.commandLifecycle.Load() != runtime || runtime.Snapshot().State != commandruntime.StateStopped {
		t.Fatal("fixture não preservou o ponteiro stopped que deve ser reconciliado")
	}

	if err := a.refreshCommandProductCatalogForRecovery(ctx, ticket); err != nil {
		t.Fatalf("retry do join/CAS do lifecycle stopped: %v", err)
	}
	assertCommandCatalogReadyAfterRecovery(t, a, old, "")
}

func TestCommandCatalogRecoveryTicketStopsOnLogoutAndWorkspaceChange(t *testing.T) {
	a := commandJobPublicationApp(t)
	ticket := a.captureCommandCatalogRecoveryTicket()
	if ticket == nil || !ticket.current(a) {
		t.Fatal("ticket autenticado inválido no início")
	}
	if _, err := a.workspaceMgr.Create("recovery-ticket-workspace"); err != nil {
		t.Fatal(err)
	}
	if !ticket.current(a) {
		t.Fatal("criar workspace inativo cancelou ticket inesperadamente")
	}
	workspaces, err := a.workspaceMgr.List()
	if err != nil {
		t.Fatal(err)
	}
	var otherID string
	for _, item := range workspaces {
		if item.ID != ticket.workspaceID {
			otherID = item.ID
			break
		}
	}
	if otherID == "" {
		t.Fatal("fixture não criou workspace alternativo")
	}
	if _, err := a.workspaceMgr.Switch(otherID); err != nil {
		t.Fatal(err)
	}
	if ticket.current(a) {
		t.Fatal("ticket continuou válido após troca de workspace")
	}

	second := a.captureCommandCatalogRecoveryTicket()
	if second == nil {
		t.Fatal("ticket não recapturado para sessão corrente")
	}
	a.setCurrentAuthUser(nil)
	if second.current(a) {
		t.Fatal("ticket continuou válido após logout")
	}
}

func TestCommandCatalogRecoverySingleflightCancelsAndJoins(t *testing.T) {
	a := commandJobPublicationApp(t)
	root, cancel := context.WithCancel(context.Background())
	previousRoot := a.ctx
	a.ctx = root
	if err := a.lockCommandBootstrap(context.Background()); err != nil {
		cancel()
		a.ctx = previousRoot
		t.Fatal(err)
	}
	heldBootstrap := true
	t.Cleanup(func() {
		cancel()
		if heldBootstrap {
			a.unlockCommandBootstrap()
		}
		joinCtx, joinCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer joinCancel()
		a.stopCommandCatalogRecovery(joinCtx)
		a.ctx = previousRoot
	})

	a.requestCommandCatalogRecovery()
	a.commandCatalogRecoveryMu.Lock()
	firstDone := a.commandCatalogRecoveryDone
	a.commandCatalogRecoveryMu.Unlock()
	if firstDone == nil {
		t.Fatal("request não iniciou worker")
	}
	for range 4 {
		a.requestCommandCatalogRecovery()
	}
	a.commandCatalogRecoveryMu.Lock()
	if a.commandCatalogRecoveryDone != firstDone || !a.commandCatalogRecoveryRunning {
		a.commandCatalogRecoveryMu.Unlock()
		t.Fatal("eventos concorrentes iniciaram workers adicionais")
	}
	a.commandCatalogRecoveryMu.Unlock()

	cancel()
	a.unlockCommandBootstrap()
	heldBootstrap = false
	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("worker não respeitou cancelamento/join")
	}
}

func shutdownCommandCatalogProductForTest(t *testing.T, ctx context.Context, a *App) {
	t.Helper()
	p := a.commandProduct.Load()
	lifecycle := a.commandLifecycle.Load()
	if p == nil || lifecycle == nil {
		t.Fatal("fixture sem produto/lifecycle")
	}
	if err := ResetCommandLifecycle(ctx, a, "test_catalog_recovery"); err != nil {
		t.Fatalf("reset fixture: %v", err)
	}
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown product fixture: %v", err)
	}
	if err := a.shutdownMountedCommandLifecycle(ctx, lifecycle); err != nil {
		t.Fatalf("shutdown lifecycle fixture: %v", err)
	}
	if err := p.bridge.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown bridge fixture: %v", err)
	}
}

func assertCommandCatalogReadyAfterRecovery(t *testing.T, a *App, old *commandProductRuntime, previousGeneration string) {
	t.Helper()
	current := a.commandProduct.Load()
	if current == nil || current == old {
		t.Fatal("recuperação não criou um produto novo")
	}
	if current.service == nil || current.bridge == nil {
		t.Fatal("produto novo não montou executor/bridge")
	}
	snapshot, err := CommandLifecycleSnapshot(a)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		t.Fatalf("lifecycle não está pronto/publicado: snapshot=%+v err=%v", snapshot, err)
	}
	keyboardMap, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(keyboardMap.Bindings) == 0 {
		t.Fatalf("mapa não recuperado: bindings=%d err=%v", len(keyboardMap.Bindings), err)
	}
	if previousGeneration != "" && keyboardMap.Generation == previousGeneration {
		t.Fatal("recuperação reutilizou geração de mapa anterior")
	}
	if _, err := old.execute(context.Background(), productRemountCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("produto antigo foi reabilitado: %v", err)
	}
}
