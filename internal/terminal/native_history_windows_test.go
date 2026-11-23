package terminal

import (
	"encoding/base64"
	"fmt"
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
			const key = "0b360089-5254-4139-9921-bdf535551618"
			args := []string{"-NoProfile", "-NoExit", "-Command", powerShellSessionLibrary(key)}
			if style != "sem módulo" {
				init := `Import-Module PSReadLine -ErrorAction Stop; Set-PSReadLineOption -HistorySavePath $env:ASST_TEST_HISTORY -HistorySaveStyle ` + style + `; Set-PSReadLineKeyHandler -Chord Ctrl+g -ScriptBlock { $line=''; $cursor=0; [Microsoft.PowerShell.PSConsoleReadLine]::GetBufferState([ref]$line,[ref]$cursor); [IO.File]::WriteAllText($env:ASST_TEST_RECALL,$line); [Microsoft.PowerShell.PSConsoleReadLine]::RevertLine() }; `
				args[len(args)-1] = init + args[len(args)-1]
			} else {
				args[len(args)-1] = "Remove-Module PSReadLine -ErrorAction SilentlyContinue; " + args[len(args)-1]
			}
			s, err := spawnSession("histórico", ptyx.SpawnOpts{Prog: "powershell.exe", Args: args, Env: os.Environ(), Dir: root, Cols: defaultCols, Rows: defaultRows}, key, nil, nil, nil, nil)
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
				if _, err := s.SendInput(`powershell.exe -NoLogo -NoProfile -NoExit -Command "Import-Module PSReadLine -ErrorAction SilentlyContinue; if (Get-Module PSReadLine) { Set-PSReadLineOption -HistorySavePath ([Environment]::GetEnvironmentVariable('ASST_TEST_HISTORY') + '.child') -HistorySaveStyle SaveNothing }"`, ""); err != nil {
					t.Fatal(err)
				}
				run("(Write-Output 'filho'); Write-Output ([bool](Get-Variable AssistenteTerminalLibrary -ErrorAction Ignore))", "filho\nFalse")
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
				data, readErr := os.ReadFile(recall)
				if readErr != nil && !os.IsNotExist(readErr) {
					t.Fatalf("leitura da linha recuperada: %v", readErr)
				}
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
				if err == nil {
					t.Fatal("arquivo de histórico criado com SaveNothing")
				}
				if !os.IsNotExist(err) {
					t.Fatalf("consulta do arquivo de histórico: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("leitura do arquivo de histórico: %v", err)
			}
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			index := slices.Index(lines, previous)
			if index < 1 || !strings.Contains(lines[index-1], libraryVariable) || strings.Contains(lines[index-1], base64.StdEncoding.EncodeToString([]byte(command))) {
				t.Fatal("a linha anterior ao comando deveria consultar apenas a biblioteca")
			}
			t.Logf("consulta: %d bytes; consulta + comando: %d bytes", len(lines[index-1]), len(lines[index-1])+len(previous))
			for _, want := range []string{manual, previous} {
				if !slices.Contains(lines, want) {
					t.Fatalf("faltou linha de %d bytes no arquivo isolado", len(want))
				}
			}
		})
	}
}

// O perfil sintético executa antes da instalação, no mesmo escopo que uma
// configuração do usuário. A biblioteca ocupada deve permanecer intacta.
func TestPowerShellProfileLibraryCollision(t *testing.T) {
	const key = "sessao"
	const otherLibrary = "[Tuple[string,scriptblock,scriptblock,scriptblock]]::new('outra',{throw 'status do perfil'},{throw 'preparo do perfil'},{throw 'normalizador do perfil'})"
	for _, tc := range []struct{ name, setup, verify string }{
		{"nome legado", "$global:AssistenteTerminalStatus='perfil'", "$global:AssistenteTerminalStatus"},
		{"três nomes legados", "$global:AssistenteTerminalStatus=$global:AssistenteTerminalPrepare=$global:AssistenteTerminalTarget='perfil'", "$global:AssistenteTerminalTarget"},
		{"nome ocupado", "$global:AssistenteTerminalLibrary='perfil'", "$global:AssistenteTerminalLibrary"},
		{"outra instalação", "$global:AssistenteTerminalLibrary=" + otherLibrary, "$global:AssistenteTerminalLibrary.Item1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := ptyTestDir(t)
			args := []string{"-NoProfile", "-NoExit", "-Command", isolatedPowerShellEditor(root) + "; " + tc.setup + "; " + powerShellSessionLibrary(key)}
			s, err := spawnSession("perfil", ptyx.SpawnOpts{Prog: "powershell.exe", Args: args, Dir: root, Cols: defaultCols, Rows: defaultRows}, key, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			s.Start()
			t.Cleanup(func() {
				if err := s.Close(); err != nil {
					t.Error(err)
				}
			})
			e, err := s.runCommandInDirectory(t.Context(), "Write-Output "+tc.verify+"; (cmd /c exit 7)", "", 15*time.Second, "test", "", nil)
			want := "perfil"
			if tc.name == "outra instalação" {
				want = "outra"
			}
			if err != nil || e == nil || e.Output != want || e.ExitCode != 7 {
				t.Fatalf("resultado=%+v erro=%v", e, err)
			}
		})
	}
}

func isolatedPowerShellEditor(root string) string {
	path := strings.ReplaceAll(filepath.Join(root, "history.txt"), "'", "''")
	return "if (Get-Module PSReadLine) { Set-PSReadLineOption -HistorySavePath '" + path + "' -HistorySaveStyle SaveNothing }; Remove-Module PSReadLine -ErrorAction Ignore"
}

// Mede da criação ao primeiro prompt e ao primeiro resultado, sob o mesmo
// perfil sintético. A moldura autônoma representa o caminho anterior à biblioteca.
func BenchmarkPowerShellSessionStartup(b *testing.B) {
	for _, compact := range []bool{false, true} {
		name := "autonoma"
		if compact {
			name = "biblioteca"
		}
		b.Run(name, func(b *testing.B) {
			var prompts, results time.Duration
			root := b.TempDir()
			for i := 0; i < b.N; i++ {
				ready := filepath.Join(root, fmt.Sprintf("ready-%d", i))
				startup := isolatedPowerShellEditor(root) + "; function global:prompt { [IO.File]::WriteAllText('" + strings.ReplaceAll(ready, "'", "''") + "','1'); 'PS> ' }"
				tick := time.Now()
				key := ""
				if compact {
					key = "a4aa6b3f-4ca3-42d9-b93d-8679793c8389"
					startup += "; " + powerShellSessionLibrary(key)
				}
				s, err := spawnSession(name, ptyx.SpawnOpts{Prog: "powershell.exe", Args: []string{"-NoProfile", "-NoExit", "-Command", startup}, Dir: root, Cols: defaultCols, Rows: defaultRows}, key, nil, nil, nil, nil)
				if err != nil {
					b.Fatal(err)
				}
				s.Start()
				func() {
					defer func() {
						if err := s.Close(); err != nil {
							b.Error(err)
						}
					}()
					for {
						if _, err := os.Stat(ready); err == nil {
							break
						}
						if time.Since(tick) > 20*time.Second {
							b.Fatal("primeiro prompt indisponível")
						}
						time.Sleep(5 * time.Millisecond)
					}
					prompts += time.Since(tick)
					e, err := s.runCommandInDirectory(b.Context(), "Write-Output pronto", "", 20*time.Second, "bench", "", nil)
					if err != nil || e == nil || e.Output != "pronto" || e.ExitCode != 0 {
						b.Fatalf("resultado=%+v erro=%v", e, err)
					}
					results += time.Since(tick)
				}()
			}
			b.ReportMetric(float64(prompts.Microseconds())/float64(b.N)/1000, "ms/prompt")
			b.ReportMetric(float64(results.Microseconds())/float64(b.N)/1000, "ms/primeiro-resultado")
		})
	}
}
