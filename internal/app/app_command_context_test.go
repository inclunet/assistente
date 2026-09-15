package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandforeground"
	"assistente/internal/configdir"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func newCommandWorkspaceProviderFixture(t *testing.T) (*App, auth.LocalSessionPrincipal, *workspace.Manager, string) {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Chdir(root)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	manager := workspace.NewManager(filepath.Join(root, "assistente-home"))
	if err := manager.Initialize(filepath.Join(root, "workspace")); err != nil {
		t.Fatalf("Initialize workspace fixture: %v", err)
	}
	if err := manager.SetProfile("workspace-profile"); err != nil {
		t.Fatalf("SetProfile fixture: %v", err)
	}
	if err := manager.AddTab(workspace.Tab{
		ID:   "tab-real",
		Type: workspace.TabTypeEditor,
		ProfileOverride: map[string]any{
			"slug": "tab-profile",
		},
		State: map[string]any{
			"version": float64(1),
		},
	}); err != nil {
		t.Fatalf("AddTab fixture: %v", err)
	}
	managerSnapshot, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot fixture: %v", err)
	}

	principal := auth.LocalSessionPrincipal{
		UserID:    uuid.Must(uuid.NewV7()).String(),
		SessionID: uuid.Must(uuid.NewV7()).String(),
	}
	app := &App{workspaceMgr: manager}
	app.setCurrentUserID(principal.UserID)
	app.setCurrentAuthUser(&AuthUser{UserID: principal.UserID, SessionID: principal.SessionID})
	return app, principal, manager, managerSnapshot.WorkspaceID
}

func commandWorkspaceScope(principal auth.LocalSessionPrincipal, workspaceID string) commandcontext.Scope {
	return commandcontext.Scope{
		UserID:        principal.UserID,
		AuthContextID: principal.SessionID,
		WorkspaceID:   &workspaceID,
	}
}

func commandFactPolicy(provider, fact string) commandcatalog.ContextPolicy {
	return commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{
		Provider: provider,
		Fact:     fact,
		Mode:     commandcatalog.ExactVersion,
	}}}
}

type commandForegroundReaderFixture struct {
	snapshot commandforeground.Snapshot
	err      error
}

func (r commandForegroundReaderFixture) Capture(context.Context) (commandforeground.Snapshot, error) {
	return r.snapshot, r.err
}

func TestCommandWorkspaceProviderCapturaManagerRealEProfileSemSurfaceInferida(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	provider, err := app.newCommandWorkspaceProvider(principal)
	if err != nil {
		t.Fatalf("newCommandWorkspaceProvider: %v", err)
	}
	scope := commandWorkspaceScope(principal, workspaceID)

	want, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot esperado: %v", err)
	}
	for _, fact := range []string{"workspace", "active_tab", "profile"} {
		owned, err := provider.Snapshot(context.Background(), scope, fact)
		if err != nil {
			t.Fatalf("Snapshot(%q): %v", fact, err)
		}
		if owned.Owner.UserID != principal.UserID || owned.Owner.AuthContextID != principal.SessionID || owned.Owner.WorkspaceID == nil || *owned.Owner.WorkspaceID != workspaceID {
			t.Fatalf("owner incorreto para %q: %#v", fact, owned.Owner)
		}
		if owned.Snapshot.Version != want.Version || owned.Snapshot.Version == "" || owned.Snapshot.CapturedAt.IsZero() {
			t.Fatalf("snapshot real incorreto para %q: %#v, want version %q", fact, owned.Snapshot, want.Version)
		}
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json do snapshot: %v", err)
	}
	if strings.Contains(string(encoded), "surface_id") || strings.Contains(string(encoded), "surface_snapshot_version") {
		t.Fatalf("snapshot do workspace inferiu identidade de surface: %s", encoded)
	}

	if err := manager.UpdateTab("tab-real", map[string]any{
		"state": map[string]any{"version": float64(2)},
	}); err != nil {
		t.Fatalf("UpdateTab fixture: %v", err)
	}
	updated, err := provider.Snapshot(context.Background(), scope, "active_tab")
	if err != nil {
		t.Fatalf("Snapshot após atualização da aba: %v", err)
	}
	if updated.Snapshot.Version == want.Version {
		t.Fatal("provider não observou a versão real atualizada da aba")
	}
	if err := manager.UpdateTab("tab-real", map[string]any{
		"profile_override": map[string]any{"slug": "tab-profile-updated"},
	}); err != nil {
		t.Fatalf("UpdateTab profile fixture: %v", err)
	}
	profileUpdated, err := provider.Snapshot(context.Background(), scope, "profile")
	if err != nil {
		t.Fatalf("Snapshot após atualização do profile da aba: %v", err)
	}
	if profileUpdated.Snapshot.Version == updated.Snapshot.Version {
		t.Fatal("provider não observou a versão realizada do profile da aba")
	}
}

func TestCommandWorkspaceProviderRejeitaWorkspaceDivergenteEFatoDesconhecido(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	provider, err := app.newCommandWorkspaceProvider(principal)
	if err != nil {
		t.Fatal(err)
	}

	otherWorkspaceID := "ws-outro-opaco"
	scope := commandWorkspaceScope(principal, otherWorkspaceID)
	if _, err := provider.Snapshot(context.Background(), scope, "workspace"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("workspace divergente aceito: %v", err)
	}

	scope = commandWorkspaceScope(principal, workspaceID)
	if _, err := provider.Snapshot(context.Background(), scope, "surface"); !errors.Is(err, commandcontext.ErrProviderUnavailable) {
		t.Fatalf("fato desconhecido aceito: %v", err)
	}
	if _, err := provider.Snapshot(context.Background(), scope, "workspace"); err != nil {
		t.Fatalf("workspace correto rejeitado após mismatch: %v", err)
	}

	other, err := manager.Create("workspace alternativo")
	if err != nil {
		t.Fatalf("Create workspace alternativo: %v", err)
	}
	if _, err := manager.Switch(other.ID); err != nil {
		t.Fatalf("Switch workspace alternativo: %v", err)
	}
	if _, err := provider.Snapshot(context.Background(), scope, "workspace"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("workspace ativo divergente aceito: %v", err)
	}
	otherScope := commandWorkspaceScope(principal, other.ID)
	if _, err := provider.Snapshot(context.Background(), otherScope, "workspace"); err != nil {
		t.Fatalf("workspace ativo correto após Switch rejeitado: %v", err)
	}
}

func TestCommandWorkspaceProviderRejeitaLogoutETrocaDeSessao(t *testing.T) {
	app, principal, _, workspaceID := newCommandWorkspaceProviderFixture(t)
	provider, err := app.newCommandWorkspaceProvider(principal)
	if err != nil {
		t.Fatal(err)
	}
	scope := commandWorkspaceScope(principal, workspaceID)

	app.setCurrentAuthUser(nil)
	if _, err := provider.Snapshot(context.Background(), scope, "workspace"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("provider continuou ativo após logout: %v", err)
	}

	app.setCurrentAuthUser(&AuthUser{UserID: principal.UserID, SessionID: uuid.Must(uuid.NewV7()).String()})
	if _, err := provider.Snapshot(context.Background(), scope, "workspace"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("provider aceitou sessão trocada: %v", err)
	}

	app.setCurrentAuthUser(&AuthUser{UserID: principal.UserID, SessionID: principal.SessionID})
	foreignUser := uuid.Must(uuid.NewV7()).String()
	app.setCurrentUserID(foreignUser)
	if _, err := provider.Snapshot(context.Background(), scope, "workspace"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("provider aceitou currentUserID divergente: %v", err)
	}
}

func TestCommandWorkspaceProviderValidaPrincipalEscopoEContexto(t *testing.T) {
	app, principal, _, workspaceID := newCommandWorkspaceProviderFixture(t)
	if _, err := app.newCommandWorkspaceProvider(auth.LocalSessionPrincipal{UserID: "user-opaco", SessionID: "session"}); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("principal inválido aceito: %v", err)
	}

	provider, err := app.newCommandWorkspaceProvider(principal)
	if err != nil {
		t.Fatal(err)
	}
	foreignUser := uuid.Must(uuid.NewV7()).String()
	foreignScope := commandcontext.Scope{UserID: foreignUser, AuthContextID: principal.SessionID, WorkspaceID: &workspaceID}
	if _, err := provider.Snapshot(context.Background(), foreignScope, "workspace"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("usuário divergente aceito: %v", err)
	}
	if _, err := provider.Snapshot(context.Background(), commandWorkspaceScope(principal, workspaceID), "workspace"); err != nil {
		t.Fatalf("escopo correto rejeitado: %v", err)
	}
	if _, err := provider.Snapshot(nil, commandWorkspaceScope(principal, workspaceID), "workspace"); !errors.Is(err, commandcontext.ErrProviderUnavailable) {
		t.Fatalf("contexto nil não rejeitado: %v", err)
	}
}

func TestNewCommandFactBusRegistraBackendsEDeixaPortasUIPendentes(t *testing.T) {
	app, principal, _, workspaceID := newCommandWorkspaceProviderFixture(t)
	bus, err := app.newCommandFactBus(principal)
	if err != nil {
		t.Fatalf("newCommandFactBus: %v", err)
	}

	workspaceScope := commandWorkspaceScope(principal, workspaceID)
	for _, fact := range []string{"workspace", "active_tab", "profile"} {
		proof, err := bus.Capture(context.Background(), workspaceScope, commandFactPolicy("workspace", fact))
		if err != nil {
			t.Fatalf("fact backend %q indisponível: %v", fact, err)
		}
		if proof.ContextVersion() == "" {
			t.Fatalf("fact backend %q não devolveu versão", fact)
		}
	}
	combined := commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{
		{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion},
		{Provider: "foreground", Fact: "foreground", Mode: commandcatalog.ExactVersion},
	}}
	if _, err := bus.Capture(context.Background(), workspaceScope, combined); err != nil && !errors.Is(err, commandcontext.ErrProviderUnavailable) {
		t.Fatalf("facts workspace + foreground não compuseram: %v", err)
	}

	foregroundScope := commandcontext.Scope{UserID: principal.UserID, AuthContextID: principal.SessionID}
	foregroundProof, err := bus.Capture(context.Background(), foregroundScope, commandFactPolicy("foreground", "foreground"))
	if err != nil && !errors.Is(err, commandcontext.ErrProviderUnavailable) {
		t.Fatalf("foreground retornou erro não relacionado à disponibilidade: %v", err)
	}
	if err == nil && foregroundProof.ContextVersion() == "" {
		t.Fatal("foreground real não devolveu versão")
	}

	for _, provider := range []string{"surface", "dialog", "focus", "window", "window_ui"} {
		_, err := bus.Capture(context.Background(), workspaceScope, commandFactPolicy(provider, provider))
		if !errors.Is(err, commandcontext.ErrMissingSnapshot) {
			t.Fatalf("porta UI %q foi inventada ou erro incorreto: %v", provider, err)
		}
	}
}

func TestCommandForegroundProviderUsaCapturedAtNativoEGuardaSessao(t *testing.T) {
	app, principal, _, _ := newCommandWorkspaceProviderFixture(t)
	capturedAt := time.Date(2026, time.September, 14, 12, 34, 56, 789, time.UTC)
	provider := &commandForegroundProvider{
		app: app,
		reader: commandForegroundReaderFixture{snapshot: commandforeground.Snapshot{
			Version:    "foreground-version-real",
			CapturedAt: capturedAt,
		}},
		scopeBind: commandForegroundScopeBind(principal),
	}
	globalScope := commandcontext.Scope{UserID: principal.UserID, AuthContextID: principal.SessionID}

	owned, err := provider.Snapshot(context.Background(), globalScope, "foreground")
	if err != nil {
		t.Fatalf("foreground snapshot: %v", err)
	}
	if owned.Snapshot.Version != "foreground-version-real" || !owned.Snapshot.CapturedAt.Equal(capturedAt) {
		t.Fatalf("wrapper alterou observação nativa: %#v", owned.Snapshot)
	}
	if _, err := provider.Snapshot(context.Background(), globalScope, "surface"); !errors.Is(err, commandcontext.ErrProviderUnavailable) {
		t.Fatalf("fact foreground aceitou nome indevido: %v", err)
	}
	workspaceID := "ws-foreground-opaco"
	workspaceOwned, err := provider.Snapshot(context.Background(), commandWorkspaceScope(principal, workspaceID), "foreground")
	if err != nil {
		t.Fatalf("foreground não compôs com scope de workspace: %v", err)
	}
	if workspaceOwned.Owner.WorkspaceID == nil || *workspaceOwned.Owner.WorkspaceID != workspaceID {
		t.Fatalf("foreground não preservou ownership do escopo recebido: %#v", workspaceOwned.Owner)
	}

	app.setCurrentAuthUser(nil)
	if _, err := provider.Snapshot(context.Background(), globalScope, "foreground"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("foreground continuou ativo após logout: %v", err)
	}
	app.setCurrentAuthUser(&AuthUser{UserID: principal.UserID, SessionID: uuid.Must(uuid.NewV7()).String()})
	if _, err := provider.Snapshot(context.Background(), globalScope, "foreground"); !errors.Is(err, commandcontext.ErrOwnerMismatch) {
		t.Fatalf("foreground aceitou sessão trocada: %v", err)
	}
}
