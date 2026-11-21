package terminal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCommandInDirectoryRejectsDirectorySwapBeforeExecution(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		t.Run(fmt.Sprintf("ancestor=%t", ancestor), func(t *testing.T) {
			// O nome completo do subteste excede MAX_PATH em worktrees Windows;
			// use prefixo curto no diretório de build já configurado pelo runner.
			root, err := os.MkdirTemp("", "dir-swap-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(root); err != nil {
					t.Error(err)
				}
			})
			a, b := filepath.Join(root, "approved"), filepath.Join(root, "other")
			for _, path := range []string{filepath.Join(a, "child"), filepath.Join(b, "child")} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			authorized := a
			if ancestor {
				authorized = filepath.Join(a, "child")
			}
			shell, kind, args := "bash", "bash", []string{"--noprofile", "--norc", "-c"}
			quote := func(path string) string { return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'" }
			setup := "mv -- " + quote(a) + " " + quote(a+"-original") + " && ln -s -- " + quote(b) + " " + quote(a) + "; "
			if runtime.GOOS == "windows" {
				shell, kind, args = "powershell.exe", "powershell", []string{"-NoProfile", "-NonInteractive", "-Command"}
				quote = func(path string) string { return "'" + strings.ReplaceAll(path, "'", "''") + "'" }
				setup = "Move-Item -LiteralPath " + quote(a) + " -Destination " + quote(a+"-original") + " -ErrorAction Stop; New-Item -ItemType Junction -Path " + quote(a) + " -Target " + quote(b) + " -ErrorAction Stop | Out-Null; "
			}
			if _, err := exec.LookPath(shell); err != nil {
				t.Skipf("shell indisponível: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var stderr bytes.Buffer
			process := exec.CommandContext(ctx, shell, append(args, setup+newCommandMarkerForShell(kind).wrapCommandInDirectory("echo EXECUTED", authorized, kind))...)
			process.Stderr = &stderr
			output, _ := process.Output()
			if target, err := os.Readlink(a); err != nil || target != b {
				t.Fatalf("troca não exercitada: %q %v (%s, %s)", target, err, output, stderr.String())
			}
			if strings.Contains(string(output), "EXECUTED") {
				t.Fatalf("executou no destino substituído: %s", output)
			}
		})
	}
}

func TestEphemeralRejectsDirectorySwapDuringShellStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("startup Bash validado no CI Linux; junctions cobertas no wrapper PowerShell")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}
	root, home := t.TempDir(), t.TempDir()
	a, b := filepath.Join(root, "approved"), filepath.Join(root, "other")
	for _, path := range []string{a, b} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	quote := func(path string) string { return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'" }
	startup := "mv -- " + quote(a) + " " + quote(a+"-original") + " && ln -s -- " + quote(b) + " " + quote(a) + "\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(startup), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	config := DefaultManagerConfig()
	config.DefaultShell = bash
	entry, err := NewManager(config, nil).RunEphemeral(context.Background(), a, "echo EXECUTED", 5*time.Second, "test")
	if err != nil {
		t.Fatalf("execução: %+v %v", entry, err)
	}
	if resolved, err := filepath.EvalSymlinks(a); err != nil || resolved != b {
		t.Fatalf("startup não substituiu diretório: %q %v", resolved, err)
	}
	if entry.ExitCode == 0 || strings.Contains(entry.Output, "EXECUTED") {
		t.Fatalf("executou após troca no startup: %+v", entry)
	}
}

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
	directory := filepath.Join(t.TempDir(), "a'b d’água [literal]")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	shell, kind, command := "bash", "bash", "pwd -P"
	args := []string{"--noprofile", "--norc", "-c"}
	if runtime.GOOS == "windows" {
		shell, kind, command = "powershell.exe", "powershell", "[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); (Get-Location).Path"
		args = []string{"-NoProfile", "-NonInteractive", "-Command"}
	}
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("shell indisponível: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, shell, append(args, newCommandMarkerForShell(kind).wrapCommandInDirectory(command, directory, kind))...).CombinedOutput()
	if err != nil || !strings.Contains(string(output), directory) {
		t.Fatalf("caminho literal: %s %v", output, err)
	}
	output, _ = exec.CommandContext(ctx, shell, append(args, newCommandMarkerForShell(kind).wrapCommandInDirectory("echo EXECUTED", filepath.Join(directory, "missing"), kind))...).Output()
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
