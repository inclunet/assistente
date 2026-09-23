package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandbridge"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestMountCommandProductRemountsSamePrincipalWhenCapturedDependenciesChange(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	previous := a.commandProduct.Load()

	newSessions, err := auth.NewSessionService(database.DB(), auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{0x61}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	newCredentials := credentials.NewManager(bytes.Repeat([]byte{0x62}, 32))
	if err := newCredentials.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x63}, 32))); err != nil {
		t.Fatal(err)
	}
	a.authMu.Lock()
	a.sessionSvc = newSessions
	a.credMgr = newCredentials
	a.authMu.Unlock()

	if err := a.mountCommandProduct(ctx); err != nil {
		t.Fatalf("remontagem com mesmo principal e dependências novas: %v", err)
	}
	current := a.commandProduct.Load()
	if current == previous {
		t.Fatal("fast path reutilizou produto apesar da troca de dependências")
	}
	if current.sessionSvc != newSessions || current.credMgr != newCredentials {
		t.Fatal("produto não capturou as dependências novas")
	}
	if _, err := previous.execute(ctx, productRemountCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("executor antigo aceitou execução após remontagem: %v", err)
	}
	record, err := current.execute(ctx, productRemountCandidate())
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("executor novo não executou: status=%s err=%v", record.Status, err)
	}
}

func TestMountCommandProductRemountsForNewSession(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	previous := a.commandProduct.Load()
	second, err := a.sessionSvc.IssueSession(ctx, &database.User{UUIDModel: database.UUIDModel{ID: previous.principal.UserID}, IsActive: true, Role: database.UserRoleUser}, "remount-second")
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: previous.principal.UserID, SessionID: second.SessionID, Role: database.UserRoleUser})

	a.bootstrapCommandLifecycleAfterAuth(ctx, &AuthUser{UserID: previous.principal.UserID, SessionID: second.SessionID, Role: database.UserRoleUser}, nil)
	current := a.commandProduct.Load()
	if current == previous || current.principal.SessionID != second.SessionID {
		t.Fatalf("produto não foi remontado para sessão nova: previous=%p current=%p principal=%+v", previous, current, current.principal)
	}
	if _, err := previous.execute(ctx, productRemountCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("executor da sessão anterior aceitou execução: %v", err)
	}
	record, err := current.execute(ctx, productRemountCandidate())
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("executor da sessão nova não executou: status=%s err=%v", record.Status, err)
	}
}

func TestMountCommandProductReturnsStoppedBeforeSamePrincipalFastPath(t *testing.T) {
	a := readyCommandProduct(t)
	previous := a.commandProduct.Load()
	a.commandLifecycleMount.Lock()
	a.commandLifecycleClosing = true
	a.commandLifecycleMount.Unlock()

	if err := a.mountCommandProduct(context.Background()); !errors.Is(err, commandruntime.ErrStopped) {
		t.Fatalf("montagem após closing retornou %v", err)
	}
	if a.commandProduct.Load() != previous {
		t.Fatal("montagem após closing alterou o produto")
	}
}

func TestMountCommandProductPreservesLockedExistingHost(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	state := a.commandHost
	if err := state.SetVaultUnlocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	newCredentials := credentials.NewManager(bytes.Repeat([]byte{0x72}, 32))
	if err := newCredentials.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x73}, 32))); err != nil {
		t.Fatal(err)
	}
	a.authMu.Lock()
	a.credMgr = newCredentials
	a.authMu.Unlock()
	if err := a.mountCommandProduct(ctx); err != nil {
		t.Fatalf("remontagem sobre host bloqueado: %v", err)
	}
	if a.commandHost != state {
		t.Fatal("remontagem substituiu o host bloqueado")
	}
	// The previous projection belongs to the replaced credential manager;
	// it must be stale, not treated as the source of the current vault state.
	ready, err := state.SourceSecurityReady(ctx)
	if err != nil || ready {
		t.Fatal("montagem inferiu vault desbloqueado a partir das dependências")
	}
	principal := a.commandProduct.Load().principal
	if _, err := state.Snapshot(ctx, principal); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("projeção anterior à troca de credenciais não foi recusada: %v", err)
	}
	if _, _, _, err := state.ResolutionSnapshot(ctx, principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("host bloqueado disponibilizou resolução: %v", err)
	}
}

func TestCommandProductOldRuntimeRejectsAfterWorkspaceSwitchAndRemounts(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := context.Background()
	previous := a.commandProduct.Load()
	second, err := a.workspaceMgr.Create("remount-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceMgr.Switch(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := previous.execute(ctx, productRemountCandidate()); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("runtime antigo aceitou workspace trocado: %v", err)
	}
	if _, err := previous.Dispatch(ctx, commandbridge.Invocation{Source: commandbridge.SourcePalette, SessionID: previous.principal.SessionID}); !errors.Is(err, commandbridge.ErrCapabilityDenied) {
		t.Fatalf("bridge antiga aceitou workspace trocado: %v", err)
	}
	if err := a.mountCommandProduct(ctx); err != nil {
		t.Fatalf("remontagem após troca de workspace: %v", err)
	}
	current := a.commandProduct.Load()
	if current == previous || current.workspaceID != second.ID {
		t.Fatalf("composição nova não capturou workspace ativo: previous=%p current=%p workspace=%q", previous, current, current.workspaceID)
	}
	record, err := current.execute(ctx, productRemountCandidate())
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("runtime novo não executou: status=%s err=%v", record.Status, err)
	}
}

func productRemountCandidate() commandexecution.EnvelopeCandidate {
	return commandexecution.EnvelopeCandidate{InvocationID: uuid.Must(uuid.NewV7()).String(), CorrelationID: uuid.Must(uuid.NewV7()).String(), CommandID: commandProductWorkspaceListID, Arguments: json.RawMessage(`{}`)}
}
