package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/desktopinstance"
	"assistente/internal/workspace"
)

func TestParseWorkspaceArgs(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{dir, "--other"}} {
		path, remaining, err := parseWorkspaceArgs(args)
		if err != nil || path != filepath.Clean(dir) || !reflect.DeepEqual(remaining, []string{"--other"}) {
			t.Fatalf("parseWorkspaceArgs(%v) = %q, %v, %v", args, path, remaining, err)
		}
	}
	current, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	if path, _, err := parseWorkspaceArgs([]string{"."}); err != nil || path != current {
		t.Fatalf("pasta atual = %q, %v", path, err)
	}
	path, remaining, err := parseWorkspaceArgs([]string{"--other"})
	if err != nil || path != "" || !reflect.DeepEqual(remaining, []string{"--other"}) {
		t.Fatalf("abertura normal = %q, %v, %v", path, remaining, err)
	}
	flags := []string{"-assetdir", dir, "-loglevel", "Debug", "-tsprefix", "prefix"}
	if path, remaining, err := parseWorkspaceArgs(flags); err != nil || path != "" || !reflect.DeepEqual(remaining, flags) {
		t.Fatalf("argumentos Wails = %q, %v, %v", path, remaining, err)
	}
	for _, args := range [][]string{{""}, {filepath.Join(dir, "inexistente")}} {
		if _, _, err := parseWorkspaceArgs(args); err == nil {
			t.Errorf("parseWorkspaceArgs(%v) deveria falhar", args)
		}
	}
}

func TestRunReservaBancoDoUltimoWorkspaceComFlagsWails(t *testing.T) {
	home, selected := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	mgr := workspace.NewManager(filepath.Join(home, ".assistente"))
	if err := mgr.Initialize(selected); err != nil {
		t.Fatal(err)
	}
	localDB := filepath.Join(selected, ".assistente", "conversations.db")
	if err := os.WriteFile(localDB, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	previousAcquire := acquireDesktopInstance
	t.Cleanup(func() { acquireDesktopInstance = previousAcquire })
	var reserved string
	acquireDesktopInstance = func(path string, _ func()) (desktopReservation, error) {
		reserved = path
		return nil, desktopinstance.ErrAlreadyRunning
	}
	if code := run([]string{"assistente", "-assetdir", "frontend"}); code != 0 {
		t.Fatalf("run = %d", code)
	}
	if reserved != localDB {
		t.Fatalf("banco reservado = %q, esperado %q", reserved, localDB)
	}
}

func TestRunPreservaFlagsWailsEConfiguraPastaDoWorkspace(t *testing.T) {
	home, selected := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	previousResolve, previousAcquire := resolveDesktopDatabase, acquireDesktopInstance
	t.Cleanup(func() { resolveDesktopDatabase, acquireDesktopInstance = previousResolve, previousAcquire })
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	resolveDesktopDatabase = func() (string, error) {
		if got := configdir.GetWorkDir(); got != filepath.Join(selected, ".assistente") {
			t.Fatalf("configuração = %q", got)
		}
		if got, err := os.Getwd(); err != nil || got != cwd {
			t.Fatalf("cwd do Wails = %q, %v", got, err)
		}
		return filepath.Join(home, "conversations.db"), nil
	}
	acquireDesktopInstance = func(string, func()) (desktopReservation, error) { return nil, desktopinstance.ErrAlreadyRunning }
	if code := run([]string{"assistente", selected, "-assetdir", "frontend"}); code != 0 {
		t.Fatalf("run = %d", code)
	}
}

func TestRunResolveBancoNoDiretorioEscolhido(t *testing.T) {
	home := t.TempDir()
	workspaceDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	previousResolve, previousAcquire := resolveDesktopDatabase, acquireDesktopInstance
	t.Cleanup(func() { resolveDesktopDatabase, acquireDesktopInstance = previousResolve, previousAcquire })
	var seen string
	resolveDesktopDatabase = func() (string, error) {
		var err error
		seen, err = os.Getwd()
		return filepath.Join(home, "conversations.db"), err
	}
	acquireDesktopInstance = func(string, func()) (desktopReservation, error) {
		return nil, desktopinstance.ErrAlreadyRunning
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"assistente"}, home},
		{[]string{"assistente", workspaceDir}, workspaceDir},
	} {
		if result := run(test.args); result != 0 {
			t.Fatalf("run(%v) = %d", test.args, result)
		}
		if seen != test.want {
			t.Fatalf("run(%v) resolveu banco em %q, esperado %q", test.args, seen, test.want)
		}
	}
}
