package app

import (
	"context"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
	"github.com/google/uuid"
)

func newCommandHostAppFixture(t *testing.T) (*App, *commandexecution.HostState, auth.LocalSessionPrincipal, *commandbindings.Configuration) {
	t.Helper()
	app := &App{authKeyringDelete: func() error { return nil }}
	epochs, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	state, err := commandexecution.NewHostState(epochs, "fixture-v1")
	if err != nil {
		t.Fatal(err)
	}
	principal := auth.LocalSessionPrincipal{UserID: uuid.Must(uuid.NewV7()).String(), SessionID: uuid.Must(uuid.NewV7()).String()}
	bindings, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(context.Background(), principal.UserID, bindings); err != nil {
		t.Fatal(err)
	}
	app.commandHost = state
	app.setCurrentUserID(principal.UserID)
	app.setCurrentAuthUser(&AuthUser{UserID: principal.UserID, SessionID: principal.SessionID})
	return app, state, principal, bindings
}

func TestCommandHostVaultHookDoesNotInventOSState(t *testing.T) {
	app, state, principal, bindings := newCommandHostAppFixture(t)
	ctx := context.Background()
	app.markCommandVaultUnlocked()
	initial, err := state.Snapshot(ctx, principal)
	if err != nil || initial.Unlocked {
		t.Fatal("cofre inventou estado do SO", initial, err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	opened, err := state.Snapshot(ctx, principal)
	if err != nil || !opened.Unlocked {
		t.Fatal(opened, err)
	}
	app.resetCommandHostSession(true)
	if _, _, err := state.UserConfiguration(ctx, principal.UserID); err == nil {
		t.Fatal("mapa antigo retido")
	}
	if err := state.PublishUserConfiguration(ctx, principal.UserID, bindings); err != nil {
		t.Fatal(err)
	}
	closed, err := state.Snapshot(ctx, principal)
	if err != nil || closed.Unlocked {
		t.Fatal("republicação reabriu cofre", closed, err)
	}
	if closed.GlobalConfig == opened.GlobalConfig || closed.ActiveLayers == opened.ActiveLayers {
		t.Fatal("republicação reutilizou gerações")
	}
}

func TestCommandHostAuthFailuresAndRollbackClearMapWithoutExternalIO(t *testing.T) {
	for name, action := range map[string]func(*App){
		"login": func(a *App) {
			if _, err := a.Login(LoginRequest{}); err == nil {
				t.Error("fixture deveria falhar antes de IO")
			}
		},
		"logout": func(a *App) {
			if err := a.Logout(LogoutRequest{}); err == nil {
				t.Error("fixture deveria falhar antes de IO")
			}
		},
		"rollback": func(a *App) { a.rollbackLoginState("") },
	} {
		t.Run(name, func(t *testing.T) {
			app, state, principal, _ := newCommandHostAppFixture(t)
			action(app)
			if _, _, err := state.UserConfiguration(context.Background(), principal.UserID); err == nil {
				t.Fatal("operação reteve mapa antigo")
			}
		})
	}
}

func TestCommandHostFailedVaultOperationsDoNotPublishUnlocked(t *testing.T) {
	app, state, principal, _ := newCommandHostAppFixture(t)
	if err := state.SetOSSessionState(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetupVault(""); err == nil {
		t.Fatal("fixture sem core deveria recusar setup")
	}
	if err := app.UnlockVault("", ""); err == nil {
		t.Fatal("fixture sem core deveria recusar unlock")
	}
	versions, err := state.Snapshot(context.Background(), principal)
	if err != nil || versions.Unlocked {
		t.Fatal("falha de cofre liberou comandos", versions, err)
	}
}
