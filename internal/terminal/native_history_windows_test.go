package terminal

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/KennethanCeyer/ptyx"
)

// A configuração faz o papel do perfil do usuário antes da biblioteca de sessão.
// Os arquivos e as preferências usados pelo ensaio pertencem apenas ao processo filho.
func TestPowerShellNativeHistory(t *testing.T) {
	const command = "Write-Output 'gerenciado-revisao-1120'"
	const manual = "Write-Output 'entrada manual'"
	for _, style := range []string{"SaveIncrementally", "SaveNothing", "sem módulo"} {
		t.Run(style, func(t *testing.T) {
			root := ptyTestDir(t)
			for _, name := range []string{"APPDATA", "LOCALAPPDATA", "USERPROFILE"} {
				t.Setenv(name, root)
			}
			history := filepath.Join(root, "history.txt")
			recall := filepath.Join(root, "recall.txt")
			t.Setenv("ASST_TEST_HISTORY", history)
			t.Setenv("ASST_TEST_RECALL", recall)
			args := append([]string{"-NoProfile"}, shellStartupArgs("powershell.exe")...)
			if style != "sem módulo" {
				init := `Import-Module PSReadLine -ErrorAction Stop; Set-PSReadLineOption -HistorySavePath $env:ASST_TEST_HISTORY -HistorySaveStyle ` + style + `; Set-PSReadLineKeyHandler -Chord Ctrl+g -ScriptBlock { $line=''; $cursor=0; [Microsoft.PowerShell.PSConsoleReadLine]::GetBufferState([ref]$line,[ref]$cursor); [IO.File]::WriteAllText($env:ASST_TEST_RECALL,$line); [Microsoft.PowerShell.PSConsoleReadLine]::RevertLine() }; `
				args[len(args)-1] = init + args[len(args)-1]
			} else {
				args[len(args)-1] = "Remove-Module PSReadLine -ErrorAction SilentlyContinue; " + args[len(args)-1]
			}
			s, err := spawnSession("histórico", ptyx.SpawnOpts{Prog: "powershell.exe", Args: args, Env: os.Environ(), Dir: root, Cols: defaultCols, Rows: defaultRows}, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			s.Start()
			t.Cleanup(func() {
				if err := s.Close(); err != nil {
					t.Errorf("encerramento da sessão: %v", err)
				}
			})
			run := func(source, want string) {
				t.Helper()
				result, err := s.runCommandInDirectory(t.Context(), source, "", 20*time.Second, "test", "", nil)
				if err != nil || result == nil || result.ExitCode != 0 || result.Output != want {
					t.Fatalf("execução: %+v, erro %v", result, err)
				}
			}
			if style == "sem módulo" {
				run("[bool](Get-Module PSReadLine)", "False")
				run(command, "gerenciado-revisao-1120")
				// A entrada manual transfere o PTY para um processo que não
				// herdou as variáveis globais do primeiro PowerShell.
				if _, err := s.SendInput("powershell.exe -NoLogo -NoProfile", ""); err != nil {
					t.Fatal(err)
				}
				run("(Write-Output 'filho'); Write-Output ([bool](Get-Variable AssistenteTerminalPrepare -ErrorAction Ignore))", "filho\nFalse")
				result, err := s.runCommandInDirectory(t.Context(), "cmd /c exit 7", "", 20*time.Second, "test", "", nil)
				if err != nil || result == nil || result.ExitCode != 7 {
					t.Fatalf("status do processo filho: %+v, %v", result, err)
				}
				return
			}
			run("[string](Get-PSReadLineOption).HistorySaveStyle", style)
			if _, err := s.SendInput(manual, ""); err != nil {
				t.Fatal(err)
			}
			run(command, "gerenciado-revisao-1120")
			if _, err := s.ptyWriter.Write([]byte("\x1b[A\x07")); err != nil {
				t.Fatal(err)
			}
			var previous string
			for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
				data, _ := os.ReadFile(recall)
				previous = string(data)
				if previous != "" {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !strings.Contains(previous, markerStart) || !strings.Contains(previous, base64.StdEncoding.EncodeToString([]byte(command))) || len(previous) > 1280 {
				t.Fatalf("seta acima recuperou linha inesperada de %d bytes", len(previous))
			}
			t.Logf("linha gerenciada recuperada pela seta: %d bytes", len(previous))
			data, err := os.ReadFile(history)
			if style == "SaveNothing" {
				if !os.IsNotExist(err) {
					t.Fatalf("histórico em disco com SaveNothing: %v", err)
				}
				return
			}
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			for _, want := range []string{manual, previous} {
				if err != nil || !slices.Contains(lines, want) {
					t.Fatalf("faltou linha de %d bytes no arquivo isolado: %v", len(want), err)
				}
			}
		})
	}
}
