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
		comparison            string
	}{
		{"literal", "Write-Output 'issue #42'", "issue #42", 0, "exact"},
		{"stderr", "cmd /c 'echo diagnostic 1>&2 & exit 0' 2>&1 # nota", "diagnostic", 0, "contains"},
		{"stderr_multilinha", "cmd /c 'echo diagnostic 1>&2 & exit 0' 2>&1\n# nota", "diagnostic", 0, "contains"},
		{"exit", "exit 3 # nota", "", 3, "exact"},
		{"return", "return 'voltou' # nota", "voltou", 0, "exact"},
		{"return_nativo", "cmd /c exit 7; return", "", 7, "exact"},
		{"return_nativo_com_nota", "cmd /c exit 7; return # nota", "", 7, "exact"},
		{"return_apos_sucesso", "cmd /c exit 7; Write-Output ok; return", "ok", 0, "exact"},
		{"sintaxe_sem_nota", "if (", "", 1, "diagnostic"},
		{"comentario", "# nada", "", 0, "exact"},
		{"sintaxe", "if ( # nota", "", 1, "diagnostic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), t.TempDir(), tc.command, 10*time.Second, "test")
			if err != nil || entry == nil || entry.ExitCode != tc.code {
				t.Fatalf("esperado %d, entry=%+v err=%v", tc.code, entry, err)
			}
			switch tc.comparison {
			case "diagnostic":
				if strings.TrimSpace(entry.Output) == "" {
					t.Fatal("falha de sintaxe sem diagnóstico")
				}
			case "contains":
				if !strings.Contains(entry.Output, tc.output) {
					t.Fatalf("output=%q, esperado trecho %q", entry.Output, tc.output)
				}
			case "exact":
				if strings.TrimSpace(entry.Output) != tc.output {
					t.Fatalf("output=%q, esperado %q", entry.Output, tc.output)
				}
			default:
				t.Fatalf("comparação desconhecida: %q", tc.comparison)
			}
		})
	}
}

func TestBashExitBeforeEndMarker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash em PTY POSIX")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "bash"
	entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), t.TempDir(), "printf 'antes'; exit 3", 10*time.Second, "test")
	if err != nil || entry == nil || entry.ExitCode != 3 || !strings.HasPrefix(entry.Output, "antes") {
		t.Fatalf("entry=%+v err=%v", entry, err)
	}
}

func TestProcessExitBeforeStartMarker(t *testing.T) {
	manager := NewManager(DefaultManagerConfig(), nil)
	session, err := manager.Create("sem marcador", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	// A entrada manual encerra o mesmo PTY sem imprimir um marcador de início.
	if _, err := session.SendInput("exit 0", "manual"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, code, err := session.waitForMarker(ctx, newCommandMarkerForShell(shellType(session.shell)), "test")
	if err == nil || ctx.Err() != nil || code != -1 {
		t.Fatalf("code=%d err=%v ctx=%v", code, err, ctx.Err())
	}
}

func TestExplicitCloseAfterStartDoesNotCompleteCommand(t *testing.T) {
	started := make(chan struct{}, 1)
	manager := NewManager(DefaultManagerConfig(), func(event string, data any) {
		if event == "terminal:command_output" && strings.Contains(data.(map[string]string)["output"], "pronto") {
			select {
			case started <- struct{}{}:
			default:
			}
		}
	})
	session, err := manager.Create("fechamento", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.CloseAll)
	command := "printf 'pronto\\n'; read -r aguarde"
	if runtime.GOOS == "windows" {
		command = "Write-Output pronto; Start-Sleep -Seconds 30"
	}
	type outcome struct {
		entry *HistoryEntry
		err   error
	}
	done := make(chan outcome, 1)
	ctx := t.Context()
	go func() {
		entry, err := manager.RunCommand(ctx, session.ID(), command, 20*time.Second, "test")
		done <- outcome{entry, err}
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("comando não iniciou")
	}
	if err := manager.Close(session.ID()); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err == nil || got.entry == nil || got.entry.ExitCode != -1 {
			t.Fatalf("entry=%+v err=%v", got.entry, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fechamento não concluiu a espera")
	}
}

func TestPowerShellReturnInPersistentSession(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	manager := NewManager(cfg, nil)
	session, err := manager.Create("persistente", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.CloseAll)
	for _, tc := range []struct {
		command, output string
		code            int
	}{
		{"cmd /c exit 7; return", "", 7},
		{"Write-Output pronto", "pronto", 0},
		{"Write-Output '" + strings.Repeat("abcdefghij", 90) + "'", strings.Repeat("abcdefghij", 90), 0},
		{"cmd /c exit 4; Write-Output ok; return", "ok", 0},
	} {
		entry, err := manager.RunCommand(t.Context(), session.ID(), tc.command, 10*time.Second, "test")
		if err != nil || entry == nil || entry.ExitCode != tc.code || strings.TrimSpace(entry.Output) != tc.output {
			t.Fatalf("comando=%q entry=%+v err=%v", tc.command, entry, err)
		}
	}
}
