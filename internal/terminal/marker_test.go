package terminal

import (
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
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommandMarker_WrapBash(t *testing.T) {
	m := NewCommandMarker()

	wrapped := m.WrapCommand("ls -la", "bash")

	if !strings.Contains(wrapped, m.StartTag()) {
		t.Errorf("wrapped command should contain start tag, got: %s", wrapped)
	}
	if !strings.Contains(wrapped, m.EndTag()) {
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
	m := NewCommandMarker()

	wrapped := m.WrapCommand("Get-Process", "powershell")

	if !strings.Contains(wrapped, m.StartTag()) {
		t.Errorf("wrapped command should contain start tag, got: %s", wrapped)
	}
	if !strings.Contains(wrapped, m.EndTag()) {
		t.Errorf("wrapped command should contain end tag, got: %s", wrapped)
	}
	if !strings.Contains(wrapped, "Get-Process") {
		t.Errorf("wrapped command should contain original command, got: %s", wrapped)
	}
	if !strings.Contains(wrapped, "$LASTEXITCODE") {
		t.Errorf("wrapped command should capture exit code with $LASTEXITCODE, got: %s", wrapped)
	}
}

func TestCommandMarker_ParseOutput_Success(t *testing.T) {
	m := NewCommandMarker()

	// Simula output com start marker no início de linha (após prompt)
	raw := "some prompt text\n" +
		m.StartTag() + "\n" +
		"file1.txt\nfile2.txt\nfile3.txt\n" +
		m.EndTag() + "0\n"

	result := m.ParseOutput(raw)

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
	m := NewCommandMarker()

	// Start marker no início de uma linha
	raw := "prompt> some-cmd\n" +
		m.StartTag() + "\n" +
		"command not found\n" +
		m.EndTag() + "127\n"

	result := m.ParseOutput(raw)

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
	m := NewCommandMarker()

	raw := "prompt\n" +
		"\x1b[32m" + m.StartTag() + "\x1b[0m\n" +
		"\x1b[31mERROR\x1b[0m: test\n" +
		"\x1b[32m" + m.EndTag() + "1\x1b[0m\n"

	result := m.ParseOutput(raw)

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
	m := NewCommandMarker()

	result := m.ParseOutput("some random text without markers")

	if result.Found {
		t.Error("expected markers not to be found")
	}
}

func TestCommandMarker_ParseOutput_EmptyOutput(t *testing.T) {
	m := NewCommandMarker()

	// Start marker no início do texto
	raw := m.StartTag() + "\n" +
		m.EndTag() + "0\n"

	result := m.ParseOutput(raw)

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
	m1 := NewCommandMarker()
	m2 := NewCommandMarker()

	if m1.StartTag() == m2.StartTag() {
		t.Error("different markers should have different start tags")
	}
	if m1.EndTag() == m2.EndTag() {
		t.Error("different markers should have different end tags")
	}
}

// TestCommandMarker_ParseOutput_WithPTYEcho testa o cenário real onde o PTY ecoa
// a linha de comando inteira antes de produzir o output dos markers.
// O parser deve ignorar os markers embutidos no echo e usar apenas os que
// aparecem no início de suas próprias linhas (output do Write-Host/printf).
func TestCommandMarker_ParseOutput_WithPTYEcho(t *testing.T) {
	m := NewCommandMarker()

	// Simula output real de um PTY com PowerShell:
	// 1. Prompt + echo do comando wrappado (contém markers embutidos)
	// 2. Output real do Write-Host (markers em linhas próprias)
	raw := "PS C:\\Users\\test> Write-Host '" + m.StartTag() + "'; echo hello; Write-Host ('" + m.EndTag() + "' + $LASTEXITCODE)\r\n" +
		m.StartTag() + "\r\n" +
		"hello\r\n" +
		m.EndTag() + "\r\n" +
		"PS C:\\Users\\test> "

	result := m.ParseOutput(raw)

	if !result.Found {
		t.Fatal("expected markers to be found")
	}
	if result.Output != "hello" {
		t.Errorf("expected output 'hello', got: %q", result.Output)
	}
	// $LASTEXITCODE é null em PS para cmdlets, exitCode deve ser 0
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

// TestCommandMarker_ParseOutput_WithPTYEcho_ExitCode testa echo do PTY
// quando há um exit code numérico após o end marker.
func TestCommandMarker_ParseOutput_WithPTYEcho_ExitCode(t *testing.T) {
	m := NewCommandMarker()

	raw := "PS C:\\> Write-Host '" + m.StartTag() + "'; cmd /c exit 1; Write-Host ('" + m.EndTag() + "' + $LASTEXITCODE)\r\n" +
		m.StartTag() + "\r\n" +
		"\r\n" +
		m.EndTag() + "1\r\n" +
		"PS C:\\> "

	result := m.ParseOutput(raw)

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
	m := NewCommandMarker()

	raw := "user@host:~$ printf '%s\\n' '" + m.StartTag() + "'; ls; printf '%s%s\\n' '" + m.EndTag() + "' $?\n" +
		m.StartTag() + "\n" +
		"file1.txt\nfile2.txt\n" +
		m.EndTag() + "0\n" +
		"user@host:~$ "

	result := m.ParseOutput(raw)

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
	m := NewCommandMarker()

	raw := "prompt\r\n" +
		m.StartTag() + "\r\n" +
		"line1\r\nline2\r\n" +
		m.EndTag() + "0\r\n"

	result := m.ParseOutput(raw)

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
