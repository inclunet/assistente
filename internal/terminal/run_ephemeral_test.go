package terminal

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KennethanCeyer/ptyx"
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
		{"cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Microsoft.PowerShell.Utility\\Out-String", 0},
		{"Set-Alias Out-String Get-Item; $PSDefaultParameterValues['Get-Item:ErrorAction']='Ignore'; cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Out-String", 1},
		{"Set-Alias Out-String Get-Item; cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Microsoft.PowerShell.Utility\\Out-String", 0},
		{"cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Get-Item -ErrorAction Ignore", 1},
		{"throw 'falha de teste'", 1},
		{"(cmd /c exit 7)", 7},
		{"$(cmd /c exit 7)", 7},
		{"@(cmd /c exit 7)", 7},
		{"(cmd /c exit 7); return", 7},
		{"return (cmd /c exit 7)", 7},
		{"return (cmd.exe /c exit 7)", 7},
		{"using namespace System.Text; (cmd.exe /c 'echo sample 1>&2 & exit 0' 2>&1 | Out-String -Stream)", 0},
		{"cmd /c exit 7; return (Get-Item missing-leaf -ErrorAction Ignore)", 1},
		{"@($(cmd /c exit 7)) # nota", 7},
		{"cmd /c exit 7; @(Get-Item missing-leaf -ErrorAction Ignore)", 1},
		{"cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Out-String -Stream", 0},
		{"cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Microsoft.PowerShell.Utility\\Out-String -Stream", 0},
		{"cmd /c 'echo sample 1>&2 & exit 0' 2>&1 | Out-String -InvalidParameter", 1},
		{"cmd /c exit 7; Get-Item missing-leaf -ErrorAction Ignore", 1},
		{"cmd /c exit 7; if ($true) { Get-Item missing-leaf -ErrorAction Ignore }", 1},
		{"if ($true) { cmd /c exit 9 }", 9},
		{"return (& (Get-Command cmd.exe) /c exit 7)", 7},
		{"cmd /c exit 7; & (Get-Command Get-Item) missing-leaf -ErrorAction Ignore", 1},
		{"cmd /c exit 7; $target = 'Get-Item'; & $target missing-leaf -ErrorAction Ignore", 1},
		{"$count=0; & (& { $script:count++; if ($script:count -gt 1) { throw 'reavaliação' }; Get-Command cmd.exe }) /c exit 7", 7},
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

func TestPowerShellNamespaceAndNativeReturn(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	marker := newCommandMarkerForShell("powershell")
	for _, dir := range []string{"", t.TempDir()} {
		for _, tc := range []struct {
			cmd, output string
			code        int
		}{
			{"return (cmd.exe /c exit 7)", "", 7},
			{"using namespace System.Text\n[StringBuilder]::new().Append('ok').ToString()", "ok", 0},
			{"# comentário\nusing namespace System.Text; using namespace System.Collections; (& (Get-Command Write-Output) ([StringBuilder]::new('ok').ToString()))", "ok", 0},
			{"using\u00a0`\nnamespace System.Text; [StringBuilder]::new('ok').ToString()", "ok", 0},
			{"# comentário\rusing namespace System.Text\r[StringBuilder]::new('ok').ToString()", "ok", 0},
			{"# comentário\r\nusing namespace System.Text\r\n[StringBuilder]::new('ok').ToString()", "ok", 0},
			{"using `\rnamespace System.Text; [StringBuilder]::new('ok').ToString()", "ok", 0},
			{"# comentário\r(cmd.exe /c exit 7)", "", 7},
			{"using namespace System.Text; return (cmd.exe /c exit 7)", "", 7},
			{"using namespace System.Text\n(cmd.exe /c exit 9)", "", 9},
			{"Write-Output 'inválido'; using namespace System.Text", "", 1},
			{"using namespace System.Text; Write-Output 'inválido'; if (", "", 1},
		} {
			cmd := exec.CommandContext(t.Context(), "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", marker.wrapCommandInDirectory(tc.cmd, dir, "powershell"))
			out, err := cmd.CombinedOutput()
			result := parseMarkerOutput(marker, string(out))
			if err != nil || !result.Found || result.ExitCode != tc.code || (tc.code != 1 && strings.TrimSpace(result.Output) != tc.output) {
				t.Fatalf("diretório=%q comando=%q saída=%q status=%d erro=%v", dir, tc.cmd, result.Output, result.ExitCode, err)
			}
			if tc.code == 1 && (strings.TrimSpace(result.Output) == "" || slices.Contains(strings.Split(result.Output, "\n"), "inválido")) {
				t.Fatalf("comando inválido executou ou ficou sem diagnóstico: %q", tc.cmd)
			}
		}
		if dir != "" {
			cmd := marker.wrapCommandInDirectory("using namespace System.Text; (Get-Location).ProviderPath", dir, "powershell")
			out, err := exec.CommandContext(t.Context(), "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", cmd).CombinedOutput()
			result := parseMarkerOutput(marker, string(out))
			if err != nil || !result.Found || result.ExitCode != 0 || strings.TrimSpace(result.Output) != dir {
				t.Fatalf("diretório após using: saída=%q status=%d erro=%v", result.Output, result.ExitCode, err)
			}
		}
	}
}

func TestPowerShellGroupAndTargetWhitespaceThroughPTY(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	manager := NewManager(cfg, nil)
	session, err := manager.Create("espaço entre tokens", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.CloseAll)
	for _, gap := range []string{"\v", "\u0085", "\u00a0", "\u2003", "\u2028", "\u3000", "`\r", "`\r\n", "<# nota #>"} {
		for _, command := range []string{
			gap + "(cmd.exe /c exit 7)",
			"(cmd.exe /c exit 7)" + gap,
			"&" + gap + "(Get-Command cmd.exe) /c exit 7",
		} {
			entry, err := manager.RunCommand(t.Context(), session.ID(), command, 15*time.Second, "test")
			if err != nil || entry == nil || entry.ExitCode != 7 || strings.TrimSpace(entry.Output) != "" {
				t.Fatalf("comando=%q entry=%+v err=%v", command, entry, err)
			}
		}
	}
}

func TestPowerShellExpandedCommandNameThroughPTY(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	manager := NewManager(cfg, nil)
	session, err := manager.Create("alvo expansível", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.CloseAll)
	for _, tc := range []struct {
		cmd, output string
		code        int
	}{
		{"$ext='.exe'; & cmd$ext /c exit 7", "", 7},
		{"& cm'd'$ext /c exit 7", "", 7},
		{"Set-Alias -Name 'asst$(throw)' -Value Write-Output; $end=''; & asst'$(throw)'$end ok", "ok", 0},
		{"& \"cmd$ext\" /c exit 7", "", 7},
		{"$n = 0; & cmd$($n++; $ext) /c exit 7", "", 7},
		{"Write-Output $n", "1", 0},
		{"$tail='put'; cmd /c exit 7; & Write-Out$tail $?", "False", 0},
		{"$tail='Item'; cmd /c exit 7; & Get-$tail missing-leaf -ErrorAction Ignore", "", 1},
		{"& missing_asst$ext", "CommandNotFoundException", 1},
	} {
		entry, err := manager.RunCommand(t.Context(), session.ID(), tc.cmd, 15*time.Second, "test")
		if err != nil || entry == nil || entry.ExitCode != tc.code {
			t.Fatalf("comando=%q entry=%+v err=%v", tc.cmd, entry, err)
		}
		out := strings.TrimSpace(entry.Output)
		if out != tc.output && (tc.output != "CommandNotFoundException" || !strings.Contains(out, tc.output)) {
			t.Fatalf("comando=%q saída=%q", tc.cmd, out)
		}
	}
}

func TestPowerShellFinalPipelineAndConditionalReturnThroughPTY(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	manager := NewManager(cfg, nil)
	dir := t.TempDir()
	session, err := manager.Create("status da última etapa", dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.CloseAll)
	run := func(command string, expected int) {
		t.Helper()
		entry, err := manager.RunCommand(t.Context(), session.ID(), command, 15*time.Second, "test")
		if err != nil || entry == nil || entry.ExitCode != expected {
			t.Fatalf("comando=%q esperado=%d entry=%+v err=%v", command, expected, entry, err)
		}
	}

	// Só muda a presença de leaf. Ignore deixa o erro fora de $Error.
	const failedNative = "cmd /c 'echo leaf& exit 7' | Get-Item -ErrorAction Ignore"
	const successfulNative = "cmd /c 'echo leaf& exit 0' | Get-Item -ErrorAction Ignore"
	run(failedNative, 1)
	run(failedNative+" | Microsoft.PowerShell.Utility\\Out-String", 1)
	run(successfulNative, 1)
	if err := os.WriteFile(filepath.Join(dir, "leaf"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	run(failedNative, 1)
	run(successfulNative, 0)
	run("cmd /c 'echo leaf& exit 7' | Out-String -Stream", 7)
	run("Write-Output leaf | cmd /c exit 9", 9)
	run("cmd /c exit 7; if ($true) { Get-Item missing-leaf -ErrorAction Ignore; return }; cmd /c exit 9", 1)
	run("cmd /c exit 7; if ($false) { Get-Item missing-leaf -ErrorAction Ignore; return }; cmd /c exit 9", 1)
	run("cmd /c exit 7; if ($false) { Get-Item missing-leaf -ErrorAction Ignore; return }; (cmd /c exit 9)", 9)
	run("if ($true) { cmd /c exit 7; if ($true) { Get-Item missing-leaf -ErrorAction Ignore; return }; cmd /c exit 9 }", 1)
	run("cmd /c exit 7; cmd /c exit $(Get-Item missing-leaf -ErrorAction Ignore; return)", 1)
	run("cmd /c exit 7; function f { return }; cmd /c exit 9", 9)
}

func TestPowerShellStrictModeThroughPTY(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	manager := NewManager(cfg, nil)
	session, err := manager.Create("StrictMode", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.CloseAll)
	for _, tc := range []struct {
		command, output string
		code            int
	}{
		{"cmd /c exit 0; Set-StrictMode -Version Latest; & (Get-Command Write-Output) ok", "ok", 0},
		{"& (Get-Command Get-Item) missing-leaf -ErrorAction Ignore", "", 1},
		{"$(cmd /c exit 7)", "", 7},
		{"cmd /c exit 7; return @()", "", 7},
		{"Remove-Variable LASTEXITCODE -Scope Global -ErrorAction Ignore; Get-Item missing-leaf -ErrorAction Ignore", "", 1},
	} {
		entry, err := manager.RunCommand(t.Context(), session.ID(), tc.command, 15*time.Second, "test")
		if err != nil || entry == nil || entry.ExitCode != tc.code || strings.TrimSpace(entry.Output) != tc.output {
			t.Fatalf("comando=%q entry=%+v err=%v", tc.command, entry, err)
		}
	}
}

func TestPowerShellGroupingPreservesOutputOnThrow(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "powershell.exe"
	const body = "Write-Output antes; Write-Error falha -ErrorAction Stop"
	for _, group := range []string{"@(" + body + ")", "$(" + body + ")", "( & { " + body + " } )"} {
		// O shell de referência emite a saída que saiu do grupo e a mensagem
		// do erro, com o mesmo contrato público de falha do terminal.
		want, err := exec.Command(cfg.DefaultShell, "-NoProfile", "-NonInteractive", "-Command", "try { "+group+" } catch { Write-Output $_.Exception.Message; exit 1 }").Output()
		var controlExit *exec.ExitError
		controlLines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(want), "\r\n", "\n")), "\n")
		if !errors.As(err, &controlExit) || controlExit.ExitCode() != 1 || controlLines[len(controlLines)-1] != "falha" {
			t.Fatalf("controle %q: saída=%q erro=%v", group, want, err)
		}
		entry, err := NewManager(cfg, nil).RunEphemeral(t.Context(), t.TempDir(), group, 15*time.Second, "test")
		if err != nil || entry == nil || entry.ExitCode != controlExit.ExitCode() || !strings.Contains(entry.Output, "falha") {
			t.Fatalf("grupo=%q entry=%+v err=%v", group, entry, err)
		}
		lines := strings.Split(strings.TrimSpace(entry.Output), "\n")
		if slices.Contains(lines, "antes") != slices.Contains(controlLines, "antes") {
			t.Fatalf("grupo=%q controle=%q terminal=%q", group, controlLines, lines)
		}
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
		{"stop_parsing", "cmd.exe /c --% echo sample", "sample", 0, "exact"},
		{"stop_parsing_falha", "cmd.exe /c --% exit 7", "", 7, "exact"},
		{"variaveis_da_guarda", "if (Get-Variable __asstPath,__asstDirectory -ErrorAction Ignore) { throw 'guarda visível' }; Write-Output ok", "ok", 0, "exact"},
		{"literal", "Write-Output 'issue #42'", "issue #42", 0, "exact"},
		{"stderr", "cmd /c 'echo diagnostic 1>&2 & exit 0' 2>&1 # nota", "diagnostic", 0, "contains"},
		{"stderr_multilinha", "cmd /c 'echo diagnostic 1>&2 & exit 0' 2>&1\n# nota", "diagnostic", 0, "contains"},
		{"exit", "exit 3 # nota", "", 3, "exact"},
		{"return", "return 'voltou' # nota", "voltou", 0, "exact"},
		{"return_nativo", "cmd /c exit 7; return", "", 7, "exact"},
		{"return_array_vazio", "cmd /c exit 7; return @()", "", 7, "exact"},
		{"return_subexpressao_vazia", "cmd /c exit 7; return $()", "", 7, "exact"},
		{"array_com_saida", "@(Write-Output primeiro; cmd /c exit 7)", "primeiro", 7, "exact"},
		{"subexpressao_com_saida", "$(Write-Output primeiro; cmd /c exit 7)", "primeiro", 7, "exact"},
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
	if _, err := session.SendInput("exit 3", "manual"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, code, err := session.waitForMarker(ctx, newCommandMarkerForShell(shellType(session.shell)), "test")
	var exit *ptyx.ExitError
	if !errors.As(err, &exit) || exit.ExitCode != 3 || ctx.Err() != nil || code != -1 {
		t.Fatalf("code=%d err=%v ctx=%v", code, err, ctx.Err())
	}
}

func TestExplicitCloseAfterStartDoesNotCompleteCommand(t *testing.T) {
	started := make(chan struct{}, 1)
	var emitted strings.Builder
	manager := NewManager(DefaultManagerConfig(), func(event string, data any) {
		payload, ok := data.(map[string]string)
		if event != "terminal:command_output" || !ok {
			return
		}
		emitted.WriteString(payload["output"])
		if strings.Contains(emitted.String(), "pronto") {
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
	command := "printf pronto; read -r aguarde"
	if runtime.GOOS == "windows" {
		command = "Write-Host -NoNewline pronto; Start-Sleep -Seconds 30"
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
		if !errors.Is(got.err, ErrSessionClosed) || got.entry == nil || got.entry.ExitCode != -1 || got.entry.Output != "pronto" {
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
	var streamed strings.Builder
	manager := NewManager(cfg, func(event string, data any) {
		if payload, ok := data.(map[string]string); event == "terminal:command_output" && ok {
			streamed.WriteString(payload["output"])
		}
	})
	session, err := manager.Create("persistente", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.CloseAll)
	for _, tc := range []struct {
		command, output string
		code            int
	}{
		{"$Host.UI.RawUI.WindowSize.Width", strconv.Itoa(defaultCols), 0},
		{"cmd.exe /c --% echo sample", "sample", 0},
		{"cmd /c exit 7; return", "", 7},
		{"Write-Output $LASTEXITCODE", "7", 0},
		{"Get-Item missing-leaf -ErrorAction Ignore", "", 1},
		{"cmd /c exit 7", "", 7},
		{"Write-Output pronto", "pronto", 0},
		{"Write-Output '" + strings.Repeat("abcdefghij", 90) + "'", strings.Repeat("abcdefghij", 90), 0},
		{"cmd /c exit 4; Write-Output ok; return", "ok", 0},
		{"mode.com con cols=80; Write-Output ('x' * 161)", strings.Repeat("x", 161), 0},
		{"$Host.UI.RawUI.WindowSize.Width", "80", 0},
		{"Write-Output ('y' * 161)", strings.Repeat("y", 161), 0},
	} {
		streamed.Reset()
		entry, err := manager.RunCommand(t.Context(), session.ID(), tc.command, 10*time.Second, "test")
		if err != nil || entry == nil || entry.ExitCode != tc.code || strings.TrimSpace(entry.Output) != tc.output {
			t.Fatalf("comando=%q entry=%+v err=%v", tc.command, entry, err)
		}
		if strings.TrimSpace(streamed.String()) != tc.output {
			t.Fatalf("comando=%q streaming=%q", tc.command, streamed.String())
		}
	}
}

func TestPersistentShellFooterAfterUnterminatedDCS(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.DefaultShell = "bash"
	first := "printf '\\033Pdados\\n'"
	failed := "printf 'antes\\033Pdados\\n'; (exit 7)"
	valid := "printf 'a\\033P0|dados\\ncontinua\\033\\\\b'"
	if runtime.GOOS == "windows" {
		cfg.DefaultShell = "powershell.exe"
		first = "[Console]::Write(([string][char]27 + 'Pdados' + [Environment]::NewLine))"
		failed = "[Console]::Write(('antes' + [char]27 + 'Pdados' + [Environment]::NewLine)); cmd /c exit 7"
		valid = "[Console]::Write(('a' + [char]27 + 'P0|dados' + [Environment]::NewLine + 'continua' + [char]27 + '\\b'))"
	}
	var streamed strings.Builder
	m := NewManager(cfg, func(event string, data any) {
		if payload, ok := data.(map[string]string); event == "terminal:command_output" && ok {
			streamed.WriteString(payload["output"])
		}
	})
	s, err := m.Create("dcs", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.CloseAll)
	for _, tc := range []struct {
		command, want string
		status        int
	}{
		{first, "", 0},
		{failed, "antes", 7},
		{valid, "ab", 0},
		{"echo ok", "ok", 0},
	} {
		streamed.Reset()
		entry, err := m.RunCommand(t.Context(), s.ID(), tc.command, 8*time.Second, "test")
		if err != nil || entry == nil || entry.Output != tc.want || entry.ExitCode != tc.status || streamed.String() != tc.want {
			t.Fatalf("comando=%q entrada=%+v streaming=%q erro=%v", tc.command, entry, streamed.String(), err)
		}
	}
}
