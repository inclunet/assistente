package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/terminal"
	"assistente/internal/workspace"
)

func isolateWorkspaceStartup(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Chdir(root)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	return root
}

func TestWorkspaceStartupRestoresLastWithoutUsingLauncherDirectory(t *testing.T) {
	root := isolateWorkspaceStartup(t)
	last := filepath.Join(root, "project")
	manager := workspace.NewManager(configdir.GetHomeDir())
	if err := manager.Initialize(last); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "installation")
	if err := os.Mkdir(launcher, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(launcher)
	a := NewApp()
	SetStartupWorkspaceDirectory(a, "")
	if err := a.initWorkspace(); err != nil {
		t.Fatal(err)
	}
	if got := a.workspaceMgr.ActivePath(); got != last {
		t.Fatalf("workspace = %q, want last workspace %q", got, last)
	}
	if got, err := a.initialToolWorkDir(); err != nil || got != last {
		t.Fatalf("tools directory = %q, error %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(launcher, ".assistente")); !os.IsNotExist(err) {
		t.Fatalf("startup wrote to launcher directory: %v", err)
	}
}

func TestWorkspaceStartupEmptyDirectoryHasChatAndCommandContext(t *testing.T) {
	root := isolateWorkspaceStartup(t)
	a := NewApp()
	SetStartupWorkspaceDirectory(a, root)
	if err := a.initWorkspace(); err != nil {
		t.Fatal(err)
	}
	ws := a.workspaceMgr.Active()
	if ws == nil || len(ws.Tabs.Items) != 1 || ws.Tabs.Items[0].Type != workspace.TabTypeChat || ws.Tabs.Active != ws.Tabs.Items[0].ID {
		t.Fatalf("new workspace has no active chat: %+v", ws)
	}
	if _, err := a.workspaceMgr.CommandSnapshot(); err != nil {
		t.Fatalf("new workspace lacks command context: %v", err)
	}
	principal := auth.LocalSessionPrincipal{UserID: "user", SessionID: "session"}
	a.setCurrentUserID(principal.UserID)
	a.setCurrentAuthUser(&AuthUser{UserID: principal.UserID, SessionID: principal.SessionID})
	if scope, err := a.commandMutationCurrentScope(principal); err != nil || scope.WorkspaceID == nil || *scope.WorkspaceID != ws.ID {
		t.Fatalf("commands cannot resolve new workspace: %+v, %v", scope, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".assistente", "workspace.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceStartupFirstLaunchUsesHomeDefault(t *testing.T) {
	root := isolateWorkspaceStartup(t)
	a := NewApp()
	SetStartupWorkspaceDirectory(a, "")
	if err := a.initWorkspace(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".assistente", "workspaces", "default")
	if got := a.workspaceMgr.ActivePath(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	ws := a.workspaceMgr.Active()
	if ws == nil || len(ws.Tabs.Items) != 1 || ws.Tabs.Items[0].Type != workspace.TabTypeChat {
		t.Fatalf("no default chat: %+v", ws)
	}
	if _, err := a.workspaceMgr.CommandSnapshot(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceStartupFailureDoesNotExposeController(t *testing.T) {
	root := isolateWorkspaceStartup(t)
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, ".assistente"), []byte("obstacle"), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	SetStartupWorkspaceDirectory(a, blocked)
	if err := a.initWorkspace(); err == nil {
		t.Fatal("workspace initialization failure was swallowed")
	}
	if a.workspaceCtrl != nil {
		t.Fatal("exposed controller without a workspace")
	}
}

func TestWorkspaceStartupFailureStopsStartupBeforeServices(t *testing.T) {
	root := isolateWorkspaceStartup(t)
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, ".assistente"), []byte("obstacle"), 0600); err != nil {
		t.Fatal(err)
	}
	restore := database.SetDB(nil)
	t.Cleanup(func() { _ = database.Close(); restore() })
	a := NewApp()
	SetDesktopDatabasePath(a, filepath.Join(root, "startup-test.db"))
	SetStartupWorkspaceDirectory(a, blocked)
	err := a.StartupWithAdapters(context.Background(), events.NoopEmitter{}, nil, nil)
	if a.cancel != nil {
		a.cancel()
	}
	if err == nil {
		t.Fatal("startup succeeded without workspace")
	}
	if a.workspaceCtrl != nil || a.toolRegistry != nil || a.credMgr != nil {
		t.Fatal("services initialized after workspace failed")
	}
	select {
	case <-a.startupResult.done:
		if !errors.Is(a.startupResult.wait(), err) {
			t.Fatal("startup failure not published")
		}
	default:
		t.Fatal("startup waiters were not released")
	}
}

func TestWorkspaceStartupCLIPreservesTolerantInitialization(t *testing.T) {
	root := isolateWorkspaceStartup(t)
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, ".assistente"), []byte("obstacle"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(blocked)
	a := NewApp() // asst não define a pasta de startup desktop.
	if err := a.initWorkspace(); err != nil {
		t.Fatalf("CLI startup became fatal: %v", err)
	}
	if a.workspaceCtrl == nil {
		t.Fatal("legacy CLI controller not wired")
	}
}

func TestWorkspaceStartupDeepLinkTerminalUsesWorkspaceDirectory(t *testing.T) {
	root := isolateWorkspaceStartup(t)
	project := filepath.Join(root, "project")
	a := NewApp()
	SetStartupWorkspaceDirectory(a, project)
	if err := a.initWorkspace(); err != nil {
		t.Fatal(err)
	}
	a.terminalMgr = terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	t.Cleanup(a.terminalMgr.CloseAll)
	a.wireTerminal()
	// Controller chamado pelo bind Terminal.CreateTerminalSession no deep link.
	info, err := a.terminalCtrl.CreateTerminalSession("deep-link")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(info.CWD) != project {
		t.Fatalf("terminal cwd = %q, want %q", info.CWD, project)
	}
}
