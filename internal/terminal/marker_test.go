package terminal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "sem escape",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "cor vermelha",
			input:    "\x1b[31mERROR\x1b[0m: something failed",
			expected: "ERROR: something failed",
		},
		{
			name:     "bold + reset",
			input:    "\x1b[1mbold text\x1b[0m",
			expected: "bold text",
		},
		{
			name:     "múltiplos escapes",
			input:    "\x1b[32m[OK]\x1b[0m \x1b[33m[WARN]\x1b[0m \x1b[31m[ERR]\x1b[0m",
			expected: "[OK] [WARN] [ERR]",
		},
		{
			name:     "cursor move",
			input:    "\x1b[2Jhello\x1b[1;1H",
			expected: "hello",
		},
		{
			name:     "string vazia",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommandMarker_WrapBash(t *testing.T) {
	m := newCommandMarkerForShell("bash")

	wrapped := m.wrapCommandInDirectory("ls -la", "", "bash")

	if !strings.Contains(wrapped, m.startTag) {
		t.Errorf("wrapped command should contain start tag, got: %s", wrapped)
	}
	if !strings.Contains(wrapped, m.endTag) {
		t.Errorf("wrapped command should contain end tag, got: %s", wrapped)
	}
	if !strings.Contains(wrapped, "ls -la") {
		t.Errorf("wrapped command should contain original command, got: %s", wrapped)
	}
	if !strings.Contains(wrapped, "$?") {
		t.Errorf("wrapped command should capture exit code with $?, got: %s", wrapped)
	}
}

func TestCommandMarker_WrapPowerShell(t *testing.T) {
	m := newCommandMarkerForShell("powershell")

	wrapped := m.wrapCommandInDirectory("Get-Process", "", "powershell")

	if strings.Contains(wrapped, m.startTag) || strings.Contains(wrapped, m.endTag) {
		t.Fatal("o eco do comando contém um marcador completo")
	}
	if !strings.Contains(wrapped, markerStart) || !strings.Contains(wrapped, markerEnd) || !strings.Contains(wrapped, m.id+markerSuffix) {
		t.Fatal("faltam fragmentos dos marcadores")
	}
	if !strings.Contains(wrapped, base64.StdEncoding.EncodeToString([]byte("Get-Process"))) {
		t.Errorf("wrapped command should contain original command, got: %s", wrapped)
	}
	if !strings.Contains(powerShellSessionLibrary("test"), "$LASTEXITCODE") {
		t.Error("falta captura do status nativo na biblioteca da sessão")
	}
}

func parseMarkerOutput(m *CommandMarker, raw string) parseResult {
	_, result := newMarkerStream(m).append(raw)
	return result
}

// Um encerramento sem rodapé finaliza escapes, CR e redesenhos pendentes.
func drainedMarkerText(m *CommandMarker, raw string) string {
	stream := newMarkerStream(m)
	stream.append(raw)
	stream.text.finish()
	return string(stream.text.data)
}

func TestCommandMarker_ParseOutput_Success(t *testing.T) {
	m := newCommandMarkerForShell("bash")

	// Simula output com start marker no início de linha (após prompt)
	raw := "some prompt text\n" +
		m.startTag + "\n" +
		"file1.txt\nfile2.txt\nfile3.txt\n" +
		m.endTag + "0!\n"

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "file1.txt") {
		t.Errorf("output should contain 'file1.txt', got: %q", result.Output)
	}
	if !strings.Contains(result.Output, "file3.txt") {
		t.Errorf("output should contain 'file3.txt', got: %q", result.Output)
	}
}

func TestCommandMarker_ParseOutput_NonZeroExit(t *testing.T) {
	m := newCommandMarkerForShell("bash")

	// Start marker no início de uma linha
	raw := "prompt> some-cmd\n" +
		m.startTag + "\n" +
		"command not found\n" +
		m.endTag + "127!\n"

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.ExitCode != 127 {
		t.Errorf("expected exit code 127, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "command not found") {
		t.Errorf("output should contain error message, got: %q", result.Output)
	}
}

func TestCommandMarker_ParseOutput_WithANSI(t *testing.T) {
	m := newCommandMarkerForShell("bash")

	raw := "prompt\n" +
		"\x1b[32m" + m.startTag + "\x1b[0m\n" +
		"\x1b[31mERROR\x1b[0m: test\n" +
		"\x1b[32m" + m.endTag + "1!\x1b[0m\n"

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found even with ANSI escapes")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "ERROR: test") {
		t.Errorf("output should contain cleaned text, got: %q", result.Output)
	}
}

func TestCommandMarker_ParseOutput_NotFound(t *testing.T) {
	m := newCommandMarkerForShell("bash")

	result := parseMarkerOutput(m, "some random text without markers")

	if result.Found {
		t.Error("expected markers not to be found")
	}
}

func TestCommandMarker_ParseOutput_EmptyOutput(t *testing.T) {
	m := newCommandMarkerForShell("bash")

	// Start marker no início do texto
	raw := m.startTag + "\n" +
		m.endTag + "0!\n"

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != "" {
		t.Errorf("expected empty output, got: %q", result.Output)
	}
}

func TestCommandMarker_UniqueIDs(t *testing.T) {
	m1 := newCommandMarkerForShell("bash")
	m2 := newCommandMarkerForShell("bash")

	if m1.startTag == m2.startTag {
		t.Error("different markers should have different start tags")
	}
	if m1.endTag == m2.endTag {
		t.Error("different markers should have different end tags")
	}
}

// TestCommandMarker_ParseOutput_WithPTYEcho testa o cenário real onde o PTY ecoa
// a linha de comando inteira antes de produzir o output dos markers.
// O parser deve ignorar os markers embutidos no echo e usar apenas os que
// aparecem no início de suas próprias linhas (output do Write-Host/printf).
func TestCommandMarker_ParseOutput_WithPTYEcho(t *testing.T) {
	m := newCommandMarkerForShell("powershell")

	// Simula output real de um PTY com PowerShell:
	// 1. Prompt + echo do comando wrappado (contém markers embutidos)
	// 2. Output real do Write-Host (markers em linhas próprias)
	raw := "PS C:\\Users\\test> Write-Host '" + m.startTag + "'; echo hello; Write-Host ('" + m.endTag + "' + $LASTEXITCODE)\r\n" +
		m.startTag + "\r\n" +
		"hello\r\n" +
		m.endTag + "0!\r\n" +
		"PS C:\\Users\\test> "

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.Output != "hello" {
		t.Errorf("expected output 'hello', got: %q", result.Output)
	}
	// Um cmdlet bem-sucedido produz status explícito zero.
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

// TestCommandMarker_ParseOutput_WithPTYEcho_ExitCode testa echo do PTY
// quando há um exit code numérico após o end marker.
func TestCommandMarker_ParseOutput_WithPTYEcho_ExitCode(t *testing.T) {
	m := newCommandMarkerForShell("powershell")

	raw := "PS C:\\> Write-Host '" + m.startTag + "'; cmd /c exit 1; Write-Host ('" + m.endTag + "' + $LASTEXITCODE)\r\n" +
		m.startTag + "\r\n" +
		"\r\n" +
		m.endTag + "1!\r\n" +
		"PS C:\\> "

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
	if result.Output != "" {
		t.Errorf("expected empty output (cmd /c exit produces none), got: %q", result.Output)
	}
}

// TestCommandMarker_ParseOutput_BashEcho testa echo do PTY em bash.
func TestCommandMarker_ParseOutput_BashEcho(t *testing.T) {
	m := newCommandMarkerForShell("bash")

	raw := "user@host:~$ printf '%s\\n' '" + m.startTag + "'; ls; printf '%s%s\\n' '" + m.endTag + "' $?\n" +
		m.startTag + "\n" +
		"file1.txt\nfile2.txt\n" +
		m.endTag + "0!\n" +
		"user@host:~$ "

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != "file1.txt\nfile2.txt" {
		t.Errorf("expected clean output, got: %q", result.Output)
	}
}

// TestCommandMarker_ParseOutput_WindowsLineEndings verifica normalização de \r\n.
func TestCommandMarker_ParseOutput_WindowsLineEndings(t *testing.T) {
	m := newCommandMarkerForShell("powershell")

	raw := "prompt\r\n" +
		m.startTag + "\r\n" +
		"line1\r\nline2\r\n" +
		m.endTag + "0!\r\n"

	result := parseMarkerOutput(m, raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != "line1\nline2" {
		t.Errorf("expected normalized output, got: %q", result.Output)
	}
}

func TestWrapCommandBashOutputWithoutFinalNewline(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("shell POSIX indisponível")
	}
	marker := newCommandMarkerForShell("bash")
	out, err := exec.Command(sh, "-c", marker.wrapCommandInDirectory("printf sample; (exit 7)", "", "sh")).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	parsed := parseMarkerOutput(marker, string(out))
	if !parsed.Found || parsed.ExitCode != 7 || parsed.Output != "sample" {
		t.Fatalf("%+v", parsed)
	}
}

func TestPOSIXMarkersCancelOpenControlsAcrossReads(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("shell POSIX indisponível")
	}
	m := newCommandMarkerForShell("bash")
	command := `printf 'antes\033Pdados\n'; (exit 7)`
	out, err := exec.Command(sh, "-c", m.wrapCommandInDirectory(command, "", "sh")).Output()
	if err != nil {
		t.Fatal(err)
	}
	// O primeiro controle pertence ao prompt, o segundo ao comando. Use aqui
	// os bytes que o shell emite para as duas marcas, inclusive os CAN.
	raw := "\x1bPpendente\n" + string(out)
	for cut := 0; cut <= len(raw); cut++ {
		stream := newMarkerStream(m)
		var emitted strings.Builder
		var result parseResult
		for _, part := range []string{raw[:cut], raw[cut:]} {
			var delta []byte
			delta, result = stream.append(part)
			emitted.Write(delta)
			if result.Found {
				break
			}
		}
		if !result.Found || result.ExitCode != 7 || result.Output != "antes" || emitted.String() != "antes" {
			t.Fatalf("corte=%d resultado=%+v streaming=%q", cut, result, emitted.String())
		}
	}
}

func TestPOSIXFooterSurvivesCommentsAndSyntaxErrors(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("shell POSIX indisponível")
	}
	for _, tc := range []struct {
		command, output string
		code            int
	}{
		{"printf ok # nota", "ok", 0},
		{"printf ok;", "ok", 0},
		{"cat <<'FIM'\ntexto\nFIM", "texto", 0},
		{"(exit 4) # nota", "", 4},
		{"printf '", "", 2},
		{"value='a b'; printf '%s' \"$value\"", "a b", 0},
	} {
		m := newCommandMarkerForShell("bash")
		out, err := exec.Command(sh, "-c", m.wrapCommandInDirectory(tc.command, "", "sh")).Output()
		result := parseMarkerOutput(m, string(out))
		if err != nil || !result.Found || result.ExitCode != tc.code || result.Output != tc.output {
			t.Fatalf("comando=%q resultado=%+v erro=%v", tc.command, result, err)
		}
	}
}

func TestCommandMarkerRequiresStatusTerminator(t *testing.T) {
	m := newCommandMarkerForShell("bash")
	raw := m.startTag + "\noutput\n" + m.endTag
	for _, tail := range []string{"", "\n", "0", "12", "3\n", "bad\n", "\n0\n"} {
		if parseMarkerOutput(m, raw+tail).Found {
			t.Errorf("accepted incomplete status %q", tail)
		}
	}
}

func TestStatusWhitespaceRequiresTerminatedNumber(t *testing.T) {
	marker := newCommandMarkerForShell("bash")
	prefix := marker.startTag + "\nhello\n" + marker.endTag
	if got := parseMarkerOutput(marker, prefix+" 7 !\r\n"); !got.Found || got.ExitCode != 7 {
		t.Fatal(got)
	}
	if got := parseMarkerOutput(marker, prefix+" 7 "); got.Found {
		t.Fatal(got)
	}
}

func TestPowerShellMarkerAfterEchoWithoutNewline(t *testing.T) {
	shell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("Windows PowerShell indisponível")
	}
	for _, command := range []string{"cmd /c exit 7", "(cmd /c exit 7)"} {
		marker := newCommandMarkerForShell("powershell")
		script := "Write-Host -NoNewline 'eco sem quebra'; " + marker.wrapCommandInDirectory(command, "", "powershell")
		output, err := exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
		result := parseMarkerOutput(marker, string(output))
		if err != nil || !result.Found || result.ExitCode != 7 || result.Output != "" {
			t.Fatalf("comando=%q saída=%q erro=%v", command, output, err)
		}
	}
}

func TestPowerShellConPTYRedrawPreservesOutput(t *testing.T) {
	marker := newCommandMarkerForShell("powershell")
	marker.conPTY = true
	marker.viewportColumns = 3
	for _, tc := range []struct{ raw, want string }{
		{"abc\r\n\x1b[39;3Hcdef\r\n\x1b[39;3Hfgh", "abcdefgh"},
		{"abcdef\r\n\x1b[39;3Hfg", "abcdefg"},
		{"abcdefg\r\n\x1b[39;3Hgh", "abcdefg\ngh"},
		{"ab界x\r\n\x1b[39;3Hxy", "ab界xy"},
		{"abc界a\r\n\x1b[39;3Hab", "abc界ab"},
		{"界a\r\n\x1b[39;3Hab", "界ab"},
		{"abc\r\n\x1b[39;2Hcdef", "abc\ncdef"},
		{"abc\r\n\x1b[39;3Hzdef", "abc\nzdef"},
		{"abc\r\ncdef", "abc\ncdef"},
	} {
		if got := drainedMarkerText(marker, tc.raw); got != tc.want {
			t.Errorf("saída=%q, esperada %q", got, tc.want)
		}
	}
}

func TestPowerShellSplitRedrawDoesNotLeakControlBytes(t *testing.T) {
	marker := newCommandMarkerForShell("powershell")
	marker.conPTY = true
	marker.viewportColumns = 3
	for _, glyph := range []string{"é", "e\u0301"} {
		raw := "ab" + glyph + "\r\n\x1b[39;3H" + glyph + "c"
		want := "ab" + glyph + "c"
		for split := range len(raw) + 1 {
			stream := newMarkerStream(marker)
			delta, _ := stream.append(marker.startTag + "\n" + raw[:split])
			partial := string(delta)
			if !strings.HasPrefix(want, partial) {
				t.Fatalf("split=%d parcial=%q", split, partial)
			}
			delta, result := stream.append(raw[split:] + "\r\n" + marker.endTag + "0!\r\n")
			if !result.Found || result.Output != want || partial+string(delta) != want {
				t.Fatalf("split=%d emitido=%q resultado=%+v", split, partial+string(delta), result)
			}
		}
	}
}

func TestConPTYFooterWithoutStartLineBreak(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	for _, body := range []string{"", "  \t", strings.Repeat(" ", 1024)} {
		wire := m.startTag + body + m.endTag + "-7!"
		for split := 0; split < len(wire); split++ {
			stream := newMarkerStream(m)
			first, early := stream.append(wire[:split])
			last, result := stream.append(wire[split:] + "prompt")
			if early.Found || !result.Found || result.ExitCode != -7 || result.Output != body || string(first)+string(last) != body || result.afterFooter != "prompt" {
				t.Fatalf("corte %d, corpo de %d bytes: cedo=%t resultado=%+v", split, len(body), early.Found, result)
			}
		}
	}
	for _, status := range []string{"", "0", "bad!", "++1!", "1 2!", "9999999999999999999999!"} {
		stream := newMarkerStream(m)
		_, result := stream.append(m.startTag + m.endTag + status)
		if stream.started || result.Found {
			t.Fatalf("status incompleto ou inválido aceito: %q", status)
		}
	}
	for _, tc := range []struct {
		conPTY bool
		body   string
	}{
		{false, ""}, {true, "texto"}, {true, " ' + ' "},
	} {
		m.conPTY = tc.conPTY
		stream := newMarkerStream(m)
		_, result := stream.append(m.startTag + tc.body + m.endTag + "0!")
		if stream.started || result.Found {
			t.Fatalf("enquadramento indevido: ConPTY=%t corpo=%q", tc.conPTY, tc.body)
		}
	}
}

func TestConPTYRedrawAtStartMarkerPreservesBodyAcrossReads(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	m.viewportColumns = len(m.startTag)
	redraw := m.startTag + "\r\n\x1b[1;" + strconv.Itoa(m.viewportColumns) + "H_"
	for split := 0; split <= len(redraw); split++ {
		stream := newMarkerStream(m)
		stream.append(redraw[:split])
		stream.append(redraw[split:])
		if !stream.started || stream.partialOutput() != "" {
			t.Fatalf("corte %d: início=%t saída=%q", split, stream.started, stream.partialOutput())
		}
		delta, result := stream.append("corpo\r\n" + m.endTag + "3!\r\n")
		if !result.Found || result.ExitCode != 3 || result.Output != "corpo" || string(delta) != "corpo" {
			t.Fatalf("corte %d: resultado=%+v delta=%q", split, result, delta)
		}
	}
}

func TestConPTYRedrawAtEndMarkerPreservesExitAcrossReads(t *testing.T) {
	for _, status := range []string{"37", "0", "-37"} {
		m := newCommandMarkerForShell("powershell")
		m.conPTY = true
		m.viewportColumns = len(m.endTag) + 1
		raw := m.startTag + "\r\ncorpo\r\n" + m.endTag + status[:1] + "\r\n\x1b[1;" +
			strconv.Itoa(m.viewportColumns) + "H" + status + markerStatusTerminator
		want, err := strconv.Atoi(status)
		if err != nil {
			t.Fatal(err)
		}
		for split := 0; split <= len(raw); split++ {
			stream := newMarkerStream(m)
			first, result := stream.append(raw[:split])
			if split < len(raw) && result.Found {
				t.Fatalf("corte %d: aceitou status %d sem terminador", split, result.ExitCode)
			}
			output := string(first)
			if !result.Found {
				var delta []byte
				delta, result = stream.append(raw[split:])
				output += string(delta)
			}
			if !result.Found || result.ExitCode != want || result.Output != "corpo" || output != "corpo" {
				t.Fatalf("status %s corte %d: resultado=%+v saída=%q", status, split, result, output)
			}
		}
	}
}

func TestConPTYRedrawAfterCompleteFooter(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	m.viewportColumns = len(m.endTag) + len("3!")
	raw := m.startTag + "\r\ncorpo\r\n" + m.endTag + "3!\r\n\x1b[1;" + strconv.Itoa(m.viewportColumns) + "H!PS> "
	for split := 0; split <= len(raw); split++ {
		stream := newMarkerStream(m)
		first, result := stream.append(raw[:split])
		output := string(first)
		if !result.Found {
			last, complete := stream.append(raw[split:])
			result = complete
			output += string(last)
		}
		if !result.Found || result.ExitCode != 3 || result.Output != "corpo" || output != "corpo" {
			t.Fatalf("corte %d: resultado=%+v saída=%q", split, result, output)
		}
	}
}

func TestConPTYSessionOutputAfterFooterRedraw(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	m.viewportColumns = len(m.endTag) + len("3!")
	right := "\x1b[1;" + strconv.Itoa(m.viewportColumns) + "H"
	for _, tc := range []struct{ name, before, tail, want string }{
		{"redesenho", "", "\r\n" + right + "!PS> ", "PS> "},
		{"texto após redesenho", "", "\r\n" + right + "!!mensagem", "!mensagem"},
		{"caractere diferente", "", "\r\n" + right + "nova", "nova"},
		{"sem cursor", "", "\r\n!PS> ", "!PS> "},
		{"outra coluna", "", "\r\n\x1b[1;2H!PS> ", "!PS> "},
		{"quebra a mais", "", "\r\n\r\n" + right + "!PS> ", "\n!PS> "},
		{"rodapé fora da margem", "x", "\r\n" + right + "!PS> ", "!PS> "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for split := 0; split <= len(tc.tail); split++ {
				stream := newMarkerStream(m)
				_, result := stream.append(m.startTag + "\r\n" + tc.before + m.endTag + "3!" + tc.tail[:split])
				if !result.Found {
					t.Fatal("rodapé não encontrado")
				}
				var raw strings.Builder
				s := &Session{suppressRawOutput: true, onRawOutput: func(_, id, text string) {
					if id != "" {
						t.Errorf("dono da saída espontânea=%q", id)
					}
					raw.WriteString(text)
				}}
				s.resumeRawAfterFooter(result)
				s.recordOutput(tc.tail[split:])
				s.finishRawOutput()
				s.rawOwner.wait()
				if raw.String() != tc.want {
					t.Fatalf("corte %d: saída=%q esperada=%q", split, raw.String(), tc.want)
				}
			}
		})
	}
}

func TestConPTYFooterRedrawAcrossManualInput(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	m.viewportColumns = len(m.endTag) + len("3!")
	cursor := "\r\n\x1b[1;" + strconv.Itoa(m.viewportColumns) + "H"
	for cut := 0; cut <= len(cursor+"!"); cut++ {
		stream := newMarkerStream(m)
		_, result := stream.append(m.startTag + "\r\n" + m.endTag + "3!" + (cursor + "!")[:cut])
		s := &Session{suppressRawOutput: true, ptyWriter: io.Discard}
		var output strings.Builder
		s.onRawOutput = func(_, id, text string) {
			if id != "manual" {
				t.Errorf("corte %d: texto %q com dono %q", cut, text, id)
			}
			output.WriteString(text)
		}
		s.resumeRawAfterFooter(result)
		if _, err := s.SendInput("comando", "manual"); err != nil {
			t.Fatal(err)
		}
		s.recordOutput((cursor + "!")[cut:] + "!PS> ")
		s.finishRawOutput()
		s.rawOwner.wait()
		if output.String() != "!PS> " {
			t.Errorf("corte %d: saída=%q", cut, output.String())
		}
	}
	// Uma posição incompleta herdada serve à detecção, mas nem o EOF nem
	// um controle inválido podem entregá-la como texto à próxima entrada.
	for _, suffix := range []string{"", "\x18ok"} {
		r := rawTerminalOutput{text: parsedTerminalText{footerColumn: 80}, footerBreak: true}
		before := r.append("\r\n\x1b[1;", false) + r.changeOwner()
		after := r.append(suffix, false) + r.finish()
		if before != "" || after != strings.TrimPrefix(suffix, "\x18") {
			t.Fatalf("sufixo=%q antes=%q depois=%q", suffix, before, after)
		}
	}
}

func TestConPTYPartialStatusAtReadFailure(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	m.viewportColumns = len(m.endTag) + 1
	readErr := errors.New("falha de leitura do PTY")
	for _, status := range []string{"", "3", "3\r\n", "-", "-\r\n"} {
		stream := newMarkerStream(m)
		result := stream.finish(m.startTag + "\r\ncorpo  " + m.endTag + status)
		s := &Session{readerErr: readErr}
		output, code, err := s.exitedCommandResult(stream)
		if result.Found || !errors.Is(err, readErr) || output != "corpo  " || code != -1 {
			t.Fatalf("status %q: saída=%q código=%d erro=%v resultado=%+v", status, output, code, err, result)
		}
	}
}

func TestProcessExitPreservesIncompleteConPTYRedraw(t *testing.T) {
	marker := newCommandMarkerForShell("powershell")
	marker.conPTY = true
	marker.viewportColumns = 3
	raw := marker.startTag + "\r\nabe\u0301\r\n\x1b[39;3He"
	session := &Session{processDone: make(chan struct{})}
	close(session.processDone)
	stream := newMarkerStream(marker)
	stream.finish(raw)
	output, code, err := session.exitedCommandResult(stream)
	if err != nil || code != 0 || output != "abe\u0301\ne" {
		t.Fatalf("output=%q code=%d err=%v", output, code, err)
	}
}

func TestConPTYInteriorColumnPreservesLineBreak(t *testing.T) {
	marker := newCommandMarkerForShell("powershell")
	marker.conPTY = true
	marker.viewportColumns = 80
	raw := "abc\r\n\x1b[2;3Hcde"
	if got := drainedMarkerText(marker, raw); got != "abc\ncde" {
		t.Errorf("saída mudou na coluna interior: %q", got)
	}
}

func TestConPTYViewportChangesWithinOutput(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	m.viewportColumns = 80
	raw := "abc\r\n\x1b[2;3Hcde\r\n\x1b[8;40;3tabc\r\n\x1b[2;3Hcde\r\n\x1b[8;40;80tabc\r\n\x1b[2;3Hcde"
	if got := drainedMarkerText(m, raw); got != "abc\ncde\nabcde\nabc\ncde" {
		t.Fatal(got)
	}
}

func TestPowerShellPreparationSelection(t *testing.T) {
	for _, tc := range []struct {
		cmd    string
		needed bool
	}{
		{"Write-Output ok", false}, {"cmd /c exit 7; return", false},
		{"cmd.exe /c --% echo sample", false}, {"throw 'falhou'", false},
		{"Write-Output 'issue #42'", false}, {"Write-Output ok # comentário", false},
		{"# nota\r(cmd /c exit 7)", true}, {"# nota\r\n(cmd /c exit 7)", true},
		{"# nota\r& (Get-Command cmd.exe) /c exit 7", true}, {"# nota\u2028(cmd /c exit 7)", false},
		{"Write-Output\u00a0(Get-Date)", false}, {"Write-Output '\u2003(issue #42)'", false},
		{"Write-Output (Get-Date) # comentário", false}, {"Write-Output '(issue #42)'", false},
		{"& (Get-Command cmd.exe) /c exit 7", true}, {"(cmd /c exit 7) # anotação", true}, {"$(cmd /c exit 7)", true}, {"@(cmd /c exit 7)", true},
		{"(cmd /c exit 7); return", true}, {"(cmd /c exit 7)\rReTuRn value", true},
		{"(cmd /c exit 7); return\u00a0value", true}, {"(cmd /c exit 7); return,42", true},
		{"(cmd /c exit 7); returned", false}, {"(cmd /c exit 7); $returnValue", false},
		{"(cmd /c exit 7); return-value", false}, {"(cmd /c exit 7); return.value", false},
		{"(cmd /c exit 7); returné", false}, {"(cmd /c exit 7); return$part", false},
		{"(cmd /c exit 7); Write-Output return", false}, {"(cmd /c exit 7); 'return'", false},
		{"Get-ChildItem . -Filter $pattern", false}, {"Get-Item . $cmd", false},
		{"Get-ChildItem . | Get-Item . $cmd", false}, {"Write-Output return . $cmd", false},
		{"Get-ChildItem <# cwd #> . -Filter $pattern", false},
		{". $cmd", true}, {"echo x | . $cmd", true}, {"echo x; . $cmd", true},
		{"$x = . $cmd", true}, {"$x += . $cmd", true}, {"$x=.$cmd", true}, {"$x=. $cmd", true}, {"return . $cmd", true},
		{"if ($false) {} . $cmd /c exit 7", true}, {"Write-Output foo#bar; if ($false) {} . $cmd", true}, {"Write-Output texto} . $cmd", true}, {`Write-Output "}" . $cmd`, false}, {`Write-Output '}' . $cmd`, false},
		{"$x[0]=.$cmd", true}, {"$x.Name=.$cmd", true}, {"$x[0] += . $cmd", true},
		{"git log --format=.$x", false}, {"Write-Output a=.$b", false}, {"Write-Output --value=.$b", false},
		{"if ($ok) { ReTuRn . $cmd }", true}, {"exit . $cmd", true}, {"throw . $cmd", true},
		{"break . $cmd", true}, {"continue . $cmd", true},
		{"foreach ($item in . $cmd) {}", true}, {"Get-Item . $cmd; . $cmd", true},
		{`Copy-Item $a .\b`, false}, {`Copy-Item $a ../b`, false}, {`Get-Item .git $file`, false},
		{`Write-Output .log ($a)`, false}, {`&cmd$ext`, true},
		{`.$cmd`, true}, {`.($cmd)`, true}, {`."$cmd"`, true}, {`.'cmd' $arg`, true},
		{".\u00a0$cmd", true}, {".`\r\n$cmd", true},
		{"using namespace System.Text; [StringBuilder]::new()", true},
	} {
		if got := selectPowerShellPreparation(tc.cmd).needed(); got != tc.needed {
			t.Errorf("preparação de %q = %t, esperava %t", tc.cmd, got, tc.needed)
		}
	}
}

func TestMarkerStreamConsumesOnlyNewBytes(t *testing.T) {
	marker := newCommandMarkerForShell("powershell")
	marker.conPTY = true
	marker.viewportColumns = 80
	stream := newMarkerStream(marker)
	session := Session{}
	raw := marker.startTag + "\r\x1b[0m\nprimeira\r\x1b[31m\n\x1b[8;40;3tabé\r\n\x1b[2;3Héc\r\nsegunda\r\n\x1b[31mvermelha\x1b[0m\r\nfim\r\n" + marker.endTag + "0!\r\n"
	want := "primeira\nabéc\nsegunda\nvermelha\nfim"
	var emitted strings.Builder
	for read := range len(raw) {
		session.outputBuf.WriteByte(raw[read])
		chunk := session.takeCommandOutput()
		if chunk != raw[read:read+1] || session.takeCommandOutput() != "" {
			t.Fatalf("offset %d, chunk %q", read, chunk)
		}
		delta, result := stream.append(chunk)
		emitted.Write(delta)
		if !strings.HasPrefix(want, emitted.String()) {
			t.Fatalf("offset %d, saída %q", read, emitted.String())
		}
		if result.Found {
			if raw[read] != markerStatusTerminator[0] || result.Output != want || result.ExitCode != 0 || emitted.String() != want {
				t.Fatalf("término no byte %d=%+v, saída=%q", read, result, emitted.String())
			}
			return
		}
	}
	t.Fatal("rodapé não reconhecido")
}

func TestCursorRedrawPreservesControlsAndUnixPTY(t *testing.T) {
	marker := newCommandMarkerForShell("powershell")
	marker.viewportColumns = 3
	marker.conPTY = true
	raw := "abc\r\r\n\x1b[2;3H\r\n\x1b[2;3Hcde"
	if got := drainedMarkerText(marker, raw); got != "abc\r\n\ncde" {
		t.Fatalf("controle perdido: %q", got)
	}
	marker.conPTY = false
	if got := drainedMarkerText(marker, "abc\r\n\x1b[2;3Hcde"); got != "abc\ncde" {
		t.Fatalf("saída alterada em PTY Unix: %q", got)
	}
}

func TestConPTYStartAfterRedrawnEcho(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	m.conPTY = true
	old := newCommandMarkerForShell("powershell")
	echo := old.wrapCommandInDirectory("Write-Output anterior", "", "powershell") + old.startTag + "\r\nanterior\r\n" + old.endTag + "0!\r\n" +
		m.wrapCommandInDirectory("cmd /c exit 4; Write-Output ok; return", "", "powershell")
	start := m.startTag + "\r\n"
	for split := range len(start) {
		stream := newMarkerStream(m)
		if delta, _ := stream.append(echo + start[:split]); stream.started || len(delta) != 0 {
			t.Fatalf("eco ou início parcial reconhecido no byte %d", split)
		}
	}
	partial := echo + start + "ok"
	stream := newMarkerStream(m)
	if output, _ := stream.append(partial); !stream.started || string(output) != "ok" {
		t.Fatalf("streaming iniciado=%t saída=%q", stream.started, output)
	}
	stream.finish("")
	if output := stream.partialOutput(); output != "ok" {
		t.Fatalf("cancelamento trouxe %q", output)
	}
	session := &Session{processDone: make(chan struct{})}
	close(session.processDone)
	if output, code, err := session.exitedCommandResult(stream); err != nil || output != "ok" || code != 0 {
		t.Fatalf("término do shell=%v saída=%q código=%d", err, output, code)
	}
	raw := partial + "\r\n" + m.endTag + "0!\r\n"
	if result := parseMarkerOutput(m, raw); !result.Found || result.Output != "ok" || result.ExitCode != 0 {
		t.Fatalf("resultado=%t saída=%q código=%d", result.Found, result.Output, result.ExitCode)
	}
}

func TestMarkerStreamLongTailAndFragmentedControls(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	tail := strings.Repeat("a", 131072)
	stream := newMarkerStream(m)
	if delta, _ := stream.append(m.startTag + "\n" + tail); string(delta) != tail {
		t.Fatal("texto sem quebra ficou retido")
	}
	controls := "\x1b]" + tail + "\a\r\x1b[0m\n"
	want := tail + "\n" + m.endTag + "invalido\núltimo"
	rest := controls + m.endTag + "invalido\núltimo\n" + m.endTag + " 27 !\r\n"
	for _, step := range []int{1, 7, 4096} {
		stream = newMarkerStream(m)
		stream.append(m.startTag + "\n" + tail)
		var emitted strings.Builder
		emitted.WriteString(tail)
		var result parseResult
		for i := 0; i < len(rest) && !result.Found; i += step {
			var delta []byte
			delta, result = stream.append(rest[i:min(i+step, len(rest))])
			emitted.Write(delta)
		}
		if !result.Found || result.ExitCode != 27 || result.Output != want || emitted.String() != want {
			t.Fatalf("passo=%d, encontrado=%t, status=%d, bytes emitidos=%d", step, result.Found, result.ExitCode, emitted.Len())
		}
	}
}

func TestInvalidFooterImmediatelyReturnsToOutput(t *testing.T) {
	m := newCommandMarkerForShell("bash")
	stream := newMarkerStream(m)
	stream.append(m.startTag + "\n")
	payload := m.endTag + "inválido\nseguinte"
	delta, result := stream.append(payload)
	if result.Found || string(delta) != payload || stream.partialOutput() != payload {
		t.Fatalf("delta=%q resultado=%+v saída parcial=%q", delta, result, stream.partialOutput())
	}
}

func TestMarkerStreamConsumesSplitControlSequences(t *testing.T) {
	for _, seq := range []string{"\x1b[2@", "\x1b[1 q", "\x1b[>c", "\x1b[200~", "\x1b(B", "\x1b7", "\x1bP1;2|dado\x1b\\"} {
		raw := "a" + seq + "b"
		if stripANSI(raw) != "ab" {
			t.Fatalf("controle não removido: %q", seq)
		}
		for split := range len(raw) + 1 {
			m := newCommandMarkerForShell("bash")
			stream := newMarkerStream(m)
			first, _ := stream.append(m.startTag + "\n" + raw[:split])
			emitted := string(first)
			last, parsed := stream.append(raw[split:] + "\n" + m.endTag + "0!\n")
			if !parsed.Found || parsed.Output != "ab" || emitted+string(last) != "ab" {
				t.Fatalf("controle=%q corte=%d resultado=%+v saída=%q", seq, split, parsed, emitted+string(last))
			}
		}
	}
}

func TestMarkerStreamOSCWithSplitStringTerminator(t *testing.T) {
	marker := newCommandMarkerForShell("bash")
	stream := newMarkerStream(marker)
	stream.append(marker.startTag + "\n")
	if delta, _ := stream.append("antes\x1b]0;título\x1b"); string(delta) != "antes" {
		t.Fatalf("antes do terminador: %q", delta)
	}
	if delta, _ := stream.append("\\depois"); string(delta) != "depois" {
		t.Fatalf("antes da quebra: %q", delta)
	}
	stream.finish("")
	if output := stream.partialOutput(); output != "antesdepois" {
		t.Fatalf("saída drenada: %q", output)
	}
}

func BenchmarkMarkerStreamScale(b *testing.B) {
	for _, layout := range []string{"linhas", "sem-quebra"} {
		for _, kib := range []int{128, 1024, 4096} {
			b.Run(fmt.Sprintf("%s/%dKiB", layout, kib), func(b *testing.B) {
				marker := newCommandMarkerForShell("powershell")
				marker.viewportColumns = 120
				chunk := strings.Repeat("a", 4096)
				if layout == "linhas" {
					chunk = strings.Repeat(strings.Repeat("a", 63)+"\n", 64)
				}
				b.ReportAllocs()
				b.SetBytes(int64(kib * 1024))
				for b.Loop() {
					stream := newMarkerStream(marker)
					stream.append(marker.startTag + "\n")
					emitted := 0
					for range kib / 4 {
						delta, _ := stream.append(chunk)
						emitted += len(delta)
					}
					delta, result := stream.append("\n" + marker.endTag + "0!\n")
					if !result.Found || emitted+len(delta) != len(result.Output) {
						b.Fatal("sem fim")
					}
				}
			})
		}
	}
}

func TestPowerShellProbeIgnoresUnframedReply(t *testing.T) {
	m := newCommandMarkerForShell("powershell")
	for _, code := range []int{0, 1} {
		wire := m.endTag + "!\n" + m.startTag + "\n" + m.endTag + strconv.Itoa(code) + "!"
		for split := 0; split < len(wire); split++ {
			stream := newMarkerStream(m)
			stream.append(wire[:split])
			_, got := stream.append(wire[split:])
			if !got.Found || got.ExitCode != code || got.Output != "" {
				t.Fatalf("corte %d: %+v", split, got)
			}
		}
	}
}
