package terminal

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunEphemeralNaoCriaSecaoPersistente(t *testing.T) {
	events := []string{}
	mgr := NewManager(DefaultManagerConfig(), func(event string, _ any) {
		events = append(events, event)
	})

	entry, err := mgr.RunEphemeral(context.Background(), "", "echo hello", 0, "test")
	if err != nil {
		t.Fatalf("RunEphemeral erro: %v", err)
	}
	if entry == nil || entry.Output == "" {
		t.Fatalf("entry vazia: %#v", entry)
	}
	if len(mgr.List()) != 0 {
		t.Fatalf("RunEphemeral não deve deixar sessão no pool, got %d", len(mgr.List()))
	}
	for _, ev := range events {
		if ev == "terminal:session_created" || ev == "terminal:session_closed" {
			t.Errorf("RunEphemeral não deve emitir %q, eventos: %v", ev, events)
		}
	}
	if mgr.Stats().TotalSessions != 0 {
		t.Errorf("Stats deve ser 0 após ephemeral, got %d", mgr.Stats().TotalSessions)
	}
}

func TestRunEphemeralTimeoutCompletesCleanupBeforeReturning(t *testing.T) {
	config := DefaultManagerConfig()
	manager := NewManager(config, nil)
	command := "printf 'antes\\n'; sleep 10"
	if runtime.GOOS == "windows" {
		command = "Write-Output 'antes'; Start-Sleep -Seconds 10"
	}

	started := time.Now()
	entry, err := manager.RunEphemeral(context.Background(), "", command, 150*time.Millisecond, "test")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("esperava timeout")
	}
	if entry == nil || entry.ExitCode != -1 {
		t.Fatalf("entry de timeout inválida: %#v", entry)
	}
	if elapsed > 6*time.Second {
		t.Fatalf("cleanup após timeout demorou %s", elapsed)
	}
	if got := manager.Stats().TotalSessions; got != 0 {
		t.Fatalf("sessão efêmera vazou para o pool: %d", got)
	}
}

func TestPowerShellNoNewlineThroughPTY(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell ConPTY")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	mgr := NewManager(cfg, nil)
	for _, tc := range []struct {
		cmd  string
		code int
	}{{"Write-Host -NoNewline 'sample'", 0}, {"Write-Host -NoNewline 'sample'; cmd /c exit 7", 7}} {
		entry, err := mgr.RunEphemeral(t.Context(), "", tc.cmd, 15*time.Second, "test")
		if err != nil || entry == nil || strings.TrimSpace(entry.Output) != "sample" || entry.ExitCode != tc.code {
			t.Fatalf("entry=%+v err=%v", entry, err)
		}
	}
}

func TestPowerShellCmdletFailureAfterNativeSuccess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	for _, command := range []string{"cmd /c exit 0; Get-Item -LiteralPath 'Z:/missing-skill-review-file' -ErrorAction SilentlyContinue", "cmd /c 'echo diagnostic 1>&2 & exit 0' 2>&1; Get-Item -LiteralPath 'Z:/missing-skill-review-file' -ErrorAction Ignore"} {
		entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), "", command, 15*time.Second, "test")
		if err != nil || entry == nil || entry.ExitCode == 0 {
			t.Fatalf("%+v %v", entry, err)
		}
	}
}

func TestPowerShellNativeStderrWithZeroExit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), "", "cmd /c 'echo sample 1>&2 & exit 0' 2>&1", 15*time.Second, "test")
	if err != nil || entry == nil || entry.ExitCode != 0 || !strings.Contains(entry.Output, "sample") {
		t.Fatalf("%+v %v", entry, err)
	}
}

func TestPowerShellPipelineAndGroupedStatus(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	for _, tc := range []struct {
		command string
		exit    int
	}{
		{"cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Out-String", 0},
		{"cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Get-Item -ErrorAction Ignore", 1},
		{"throw 'falha de teste'", 1},
		{"(cmd /c exit 7)", 7},
		{"(cmd /c exit 7) # nota)", 7},
		{"(cmd /c exit 7) # nota com espaços  ", 7},
		{"(cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Get-Item -ErrorAction Ignore)", 1},
		{"(cmd /c 'echo sample 1>&2 & exit 0' 2>&1)", 0},
		{"(cmd /c 'echo sample 1>&2 & exit 0' 2>&1) # nota", 0},
		{"(& $env:ComSpec /c exit 7)", 7},
		{"(& (Get-Command cmd.exe) /c exit 7)", 7},
		{"cmd /c exit 7; (& (Get-Command Write-Output) ok)", 0},
		{"$name = $env:ComSpec; (& $name /c exit 7)", 7},
		{"cmd /c exit 7; $app = 'Write-Output'; (& $app ok)", 0},
		{"(cmd /c exit 7);", 7},
		{"cmd /c exit 7; (Write-Output ok)", 0},
	} {
		t.Run(tc.command, func(t *testing.T) {
			entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), t.TempDir(), tc.command, 15*time.Second, "test")
			if err != nil || entry == nil || entry.ExitCode != tc.exit {
				t.Fatalf("esperado=%d entry=%+v err=%v", tc.exit, entry, err)
			}
		})
	}
}

func TestPowerShellHashPreservesOutputAndExitCode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	for _, tc := range []struct {
		name, command, output string
		code                  int
	}{
		{"literal", "Write-Output 'issue #42'", "issue #42", 0},
		{"stderr", "cmd /c 'echo diagnostic 1>&2 & exit 0' 2>&1 # nota", "diagnostic", 0},
		{"exit", "exit 3 # nota", "", 3},
		{"return", "return 'voltou' # nota", "voltou", 0},
		{"comentario", "# nada", "", 0},
		{"sintaxe", "if ( # nota", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), t.TempDir(), tc.command, 10*time.Second, "test")
			if err != nil || entry == nil || entry.ExitCode != tc.code {
				t.Fatalf("status inesperado: %v; erro=%v", entry != nil && entry.ExitCode == tc.code, err)
			}
			if tc.name == "sintaxe" {
				if strings.TrimSpace(entry.Output) == "" {
					t.Fatal("falha de sintaxe sem diagnóstico")
				}
				return
			}
			if tc.name == "stderr" && !strings.Contains(entry.Output, tc.output) || tc.name != "stderr" && strings.TrimSpace(entry.Output) != tc.output {
				t.Fatalf("output=%q", entry.Output)
			}
		})
	}
}
