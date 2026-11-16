package app

import (
	"os"
	"testing"
)

func TestWorkspaceStartupDirPreservesCLIAndDesktopSelection(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []*App{NewApp(), NewCommandCLIApp()} {
		if got := a.workspaceStartupDir(); got != cwd {
			t.Fatalf("CLI dir = %q, want %q", got, cwd)
		}
	}
	a := NewApp()
	SetStartupWorkspacePath(a, "")
	if got := a.workspaceStartupDir(); got != "" {
		t.Fatalf("desktop normal = %q", got)
	}
	selected := t.TempDir()
	SetStartupWorkspacePath(a, selected)
	if got := a.workspaceStartupDir(); got != selected {
		t.Fatalf("desktop escolhido = %q", got)
	}
}
