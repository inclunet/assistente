package terminal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEphemeralRestoresDirectoryAfterShellStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("regressão de inicialização bash executada no CI Linux")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}
	home, authorized, other := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	startup := "cd '" + strings.ReplaceAll(other, "'", "'\"'\"'") + "'\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(startup), 0600); err != nil {
		t.Fatal(err)
	}
	config := DefaultManagerConfig()
	config.DefaultShell = bash
	m := NewManager(config, nil)
	entry, err := m.RunEphemeral(context.Background(), authorized, "pwd -P", 5*time.Second, "test")
	if err != nil || entry.ExitCode != 0 {
		t.Fatalf("execução: %+v %v", entry, err)
	}
	expected, err := filepath.EvalSymlinks(authorized)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entry.Output, expected) || strings.Contains(entry.Output, other) {
		t.Fatalf("executou fora do autorizado: %s", entry.Output)
	}
}

func TestCommandInDirectoryQuotesPathsAndStopsOnFailure(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "a'b [literal]")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	shell, kind, command := "bash", "bash", "pwd -P"
	args := []string{"--noprofile", "--norc", "-c"}
	if runtime.GOOS == "windows" {
		shell, kind, command = "powershell.exe", "powershell", "(Get-Location).Path"
		args = []string{"-NoProfile", "-NonInteractive", "-Command"}
	}
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("shell indisponível: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, shell, append(args, commandInDirectory(command, directory, kind))...).CombinedOutput()
	if err != nil || !strings.Contains(string(output), directory) {
		t.Fatalf("caminho literal: %s %v", output, err)
	}
	output, _ = exec.CommandContext(ctx, shell, append(args, commandInDirectory("echo EXECUTED", filepath.Join(directory, "missing"), kind))...).Output()
	if strings.Contains(string(output), "EXECUTED") {
		t.Fatalf("executou após falhar cd: %s", output)
	}
}

func TestEphemeralPreservesMultilineAliasesAndLiteralArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("regressão Bash executada no CI Linux")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}
	config := DefaultManagerConfig()
	config.DefaultShell = bash
	m := NewManager(config, nil)
	command := "alias permission_alias='echo alias-ok'\npermission_alias\nprintf '%s\\n' 'literal $(echo unexpected)'\nfalse"
	entry, err := m.RunEphemeral(context.Background(), t.TempDir(), command, 5*time.Second, "test")
	if err != nil || entry.ExitCode != 1 {
		t.Fatalf("status perdido: %+v %v", entry, err)
	}
	if !strings.Contains(entry.Output, "alias-ok") || !strings.Contains(entry.Output, "literal $(echo unexpected)") {
		t.Fatalf("alias ou argumento literal alterado: %s", entry.Output)
	}
}

func TestEphemeralSupportsPOSIXShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("regressão /bin/sh executada no CI Linux")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh indisponível")
	}
	config := DefaultManagerConfig()
	config.DefaultShell = sh
	entry, err := NewManager(config, nil).RunEphemeral(context.Background(), t.TempDir(), "printf '%s\\n' 'posix literal $(echo unexpected)'", 5*time.Second, "test")
	if err != nil || entry.ExitCode != 0 || !strings.Contains(entry.Output, "posix literal $(echo unexpected)") {
		t.Fatalf("execução POSIX: %+v %v", entry, err)
	}
}
