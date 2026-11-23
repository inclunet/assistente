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

func TestNativeZshMarkerAndDirectory(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh indisponível")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	quotedDir := filepath.Join(dir, "a'b [teste]")
	if err := os.Mkdir(quotedDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		directory, command, output string
		code                       int
	}{
		{"", "printf ok", "ok", 0},
		{"", "(exit 7)", "", 7},
		{"", "printf '", "", 1},
		{quotedDir, "pwd -P", quotedDir, 0},
		{quotedDir, "(exit 7)", "", 7},
		{quotedDir, "printf '", "", 1},
		{filepath.Join(dir, "ausente"), "printf EXECUTED", "", 1},
	} {
		kind := shellType(zsh)
		marker := newCommandMarkerForShell(kind)
		frame := marker.wrapCommandInDirectory(tc.command, tc.directory, kind)
		out, err := exec.CommandContext(t.Context(), zsh, "-fc", "unsetopt POSIX_BUILTINS\n"+frame+"\n[[ ! -o POSIX_BUILTINS ]]").Output()
		result := parseMarkerOutput(marker, string(out))
		if err != nil || !result.Found || result.ExitCode != tc.code || result.Output != tc.output {
			t.Fatalf("diretório=%q comando=%q resultado=%+v erro=%v", tc.directory, tc.command, result, err)
		}
	}

	// O shell interativo muda o diretório no startup; o executor efêmero
	// deve restabelecê-lo e conservar as opções nativas que o perfil escolheu.
	profile := t.TempDir()
	t.Setenv("ZDOTDIR", profile)
	t.Setenv("skip_global_compinit", "")
	// Reproduz a leitura de uma tecla pelo compinit global do Ubuntu mesmo
	// nas distribuições que não incluem esse completador no perfil.
	zshenv := "if [[ $OSTYPE == linux* && -z $skip_global_compinit && -o interactive ]]; then read -q; fi\n"
	if err := os.WriteFile(filepath.Join(profile, ".zshenv"), []byte(zshenv), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, ".zshrc"), []byte("unsetopt POSIX_BUILTINS\ncd /\nasst_started=yes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = zsh
	entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), quotedDir, "print -r -- executado >> execucoes; pwd -P; ([[ $asst_started == yes && ! -o POSIX_BUILTINS ]] && exit 7)", 10*time.Second, "test")
	if err != nil || entry == nil || entry.ExitCode != 7 || strings.TrimSpace(entry.Output) != quotedDir {
		t.Fatalf("zsh no PTY: entry=%+v erro=%v", entry, err)
	}
	executions, err := os.ReadFile(filepath.Join(quotedDir, "execucoes"))
	if err != nil || string(executions) != "executado\n" {
		t.Fatalf("execuções=%q erro=%v", executions, err)
	}
}

func TestNativeZshPersistentStartupByOrigin(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("compinit global do zsh no Linux")
	}
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh indisponível")
	}
	for _, origin := range []string{"run_command", "terminal_session", "manual"} {
		t.Run(origin, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("ZDOTDIR", dir)
			t.Setenv("skip_global_compinit", "")
			// O perfil registra o valor herdado. No caso manual, ele define a
			// variável depois disso para evitar o diálogo do completador no teste;
			// não verifica o comportamento interativo desse diálogo.
			profile := "asst_origin=${skip_global_compinit:-manual}\n"
			if origin == "manual" {
				profile += "skip_global_compinit=1\n"
			} else {
				profile += "[[ -z $skip_global_compinit ]] && read -q\n"
			}
			if err := os.WriteFile(filepath.Join(dir, ".zshenv"), []byte(profile), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := DefaultManagerConfig()
			cfg.DefaultShell = zsh
			manager := NewManager(cfg, nil)
			t.Cleanup(manager.CloseAll)
			var id string
			switch origin {
			case "run_command":
				session, err := manager.Acquire(t.Context(), dir)
				if err != nil {
					t.Fatal(err)
				}
				id = session.ID()
			case "terminal_session":
				info, err := manager.CreateToolSession("modelo", dir)
				if err != nil {
					t.Fatal(err)
				}
				id = info.ID
			case "manual":
				info, err := manager.CreateInfo("usuário", dir)
				if err != nil {
					t.Fatal(err)
				}
				id = info.ID
			}
			entry, err := manager.RunCommand(t.Context(), id, "print -r -- $asst_origin >> visitas; pwd -P; (exit 7)", 10*time.Second, "test")
			if err != nil || entry == nil || entry.ExitCode != 7 || strings.TrimSpace(entry.Output) != dir {
				t.Fatalf("entry=%+v erro=%v", entry, err)
			}
			visits, err := os.ReadFile(filepath.Join(dir, "visitas"))
			want := "1\n"
			if origin == "manual" {
				want = "manual\n"
			}
			if err != nil || string(visits) != want {
				t.Fatalf("execuções=%q esperado=%q erro=%v", visits, want, err)
			}
		})
	}
}
