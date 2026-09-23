package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func readyCommandProduct(t *testing.T) *App {
	t.Helper()
	a, _ := appLifecycleProductMountFixture(t)
	ctx := context.Background()
	if err := a.credMgr.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32))); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ShutdownCommandLifecycle(ctx, a)
		_ = a.drainCommandExecutors(ctx)
		_ = a.shutdownCommandBridgeIfConfigured(ctx)
	})
	if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, a); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CommandLifecycleSnapshot(a)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		t.Fatalf("not ready: %+v %v", snapshot, err)
	}
	return a
}

func TestCommandProductExecutesRealWorkspaceOperationAndPersistsExactlyOnce(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	c := commandexecution.EnvelopeCandidate{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: commandProductWorkspaceListID, Arguments: json.RawMessage(`{}`)}
	first, err := p.execute(context.Background(), c)
	if err != nil || first.Status != commandledger.Succeeded {
		t.Fatalf("execute: %+v %v", first, err)
	}
	second, err := p.execute(context.Background(), c)
	if err != nil || second.ID != first.ID || second.Status != commandledger.Succeeded {
		t.Fatalf("replay: %+v %v", second, err)
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", c.InvocationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit count %d err %v", count, err)
	}
	public, err := a.GetPaletteInvocation(c.InvocationID)
	if err != nil || public.Status != "succeeded" {
		t.Fatalf("lookup %+v %v", public, err)
	}
	result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil || result.Status != "succeeded" || result.InvocationID == c.InvocationID {
		t.Fatalf("public ingress %+v %v", result, err)
	}
}

func TestCommandProductRefusesUnknownInvalidAndRevokedRequests(t *testing.T) {
	a := readyCommandProduct(t)
	for _, request := range []struct {
		id, args string
		wantErr error
	}{
		{"unknown.command", `{}`, nil},
		{commandProductWorkspaceListID, `{"user_id":"forged"}`, commandexecution.ErrInvalidRequest},
	} {
		result, err := a.ExecutePaletteCommand(request.id, json.RawMessage(request.args))
		if err == nil && result.Status == "succeeded" {
			t.Fatalf("invalid request accepted: %+v", result)
		}
		if request.wantErr != nil && !errors.Is(err, request.wantErr) {
			t.Fatalf("request %q error = %v, want %v", request.id, err, request.wantErr)
		}
	}
	p := a.commandProduct.Load()
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", p.principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err == nil && result.Status == "succeeded" {
		t.Fatal("revoked session executed")
	}
}

func TestCommandProductResetAndShutdownCloseAdmission(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	if err := ResetCommandLifecycle(ctx, a, "test_reset"); err != nil {
		t.Fatal(err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err == nil && result.Status == "succeeded" {
		t.Fatal("reset kept admission enabled")
	}
	if err := a.drainCommandExecutors(ctx); err != nil {
		t.Fatal(err)
	}
	if result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, nil); err == nil && result.Status == "succeeded" {
		t.Fatal("drained executor accepted work")
	}
}

func TestCommandProductColdMountDoesNotInferVaultFromCredentials(t *testing.T) {
	a, _ := appLifecycleProductMountFixture(t)
	// Remova somente o host pré-desbloqueado da fixture para exercitar boot frio.
	a.authMu.Lock()
	a.commandHost = nil
	a.authMu.Unlock()
	if a.credMgr == nil || a.vaultSvc != nil || a.commandHost != nil {
		t.Fatal("fixture deve ter credenciais em memória, sem prova de unlock")
	}
	if err := a.mountCommandProduct(context.Background()); !errors.Is(err, commandruntime.ErrMissingDependency) {
		t.Fatalf("mount sem cofre autoritativo: %v", err)
	}
	if a.commandHost != nil || a.commandProduct.Load() != nil || a.commandBridge.Load() != nil || a.commandLifecycle.Load() != nil {
		t.Fatal("mount recusado publicou dependências parciais")
	}
}

func TestCommandProductCancelledRemountDoesNotReuseReadyFastPath(t *testing.T) {
	a := readyCommandProduct(t)
	before := a.commandProduct.Load()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.mountCommandProduct(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("mount cancelado retornou %v", err)
	}
	if a.commandProduct.Load() != before {
		t.Fatal("mount cancelado substituiu produto")
	}
}
