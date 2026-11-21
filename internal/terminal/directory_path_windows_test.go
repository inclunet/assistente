package terminal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveDirectoryPathLongPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), strings.Repeat("directory", 20), "child")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{path, `\\?\` + path} {
		resolved, err := ResolveDirectoryPath(input)
		if err != nil || !strings.EqualFold(resolved, path) {
			t.Fatalf("caminho longo: %q, esperado %q: %v", resolved, path, err)
		}
	}
}

func TestEphemeralAcceptsExistingJunction(t *testing.T) {
	root, err := os.MkdirTemp("", "junction-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	target, link := filepath.Join(root, "target"), filepath.Join(root, "link")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	quote := func(path string) string { return "'" + strings.ReplaceAll(path, "'", "''") + "'" }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "New-Item -ItemType Junction -Path "+quote(link)+" -Target "+quote(target)+" -ErrorAction Stop | Out-Null").CombinedOutput()
	if err != nil {
		t.Fatalf("junction: %s %v", output, err)
	}
	config := DefaultManagerConfig()
	config.DefaultShell = "powershell.exe"
	for _, suffix := range []string{"", "child"} {
		t.Run("suffix="+suffix, func(t *testing.T) {
			path, expected := filepath.Join(link, suffix), filepath.Join(target, suffix)
			resolved, err := ResolveDirectoryPath(path)
			if err != nil || !strings.EqualFold(resolved, expected) {
				t.Fatalf("destino físico: %q, esperado %q: %v", resolved, expected, err)
			}
			const rawCommand = "(Get-Location).Path"
			entry, err := NewManager(config, nil).RunEphemeral(ctx, WorkingDirectory{Path: path}, CommandText{Raw: rawCommand, Display: rawCommand}, 10*time.Second, "test")
			if err != nil || entry.ExitCode != 0 || !strings.Contains(entry.Output, expected) {
				t.Fatalf("junction legítima recusada: %+v %v", entry, err)
			}
		})
	}
}
