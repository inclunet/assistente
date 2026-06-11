package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"assistente/internal/configdir"
)

func TestPreprocessCommands_BasicEcho(t *testing.T) {
	var content string
	if runtime.GOOS == "windows" {
		content = "Before\n!echo hello\nAfter"
	} else {
		content = "Before\n!echo hello\nAfter"
	}

	result := PreprocessCommands(content, nil)

	if !strings.Contains(result, "hello") {
		t.Errorf("expected output to contain 'hello', got: %q", result)
	}
	if !strings.Contains(result, "Before") {
		t.Errorf("expected 'Before' to be preserved, got: %q", result)
	}
	if !strings.Contains(result, "After") {
		t.Errorf("expected 'After' to be preserved, got: %q", result)
	}
}

func TestPreprocessCommands_NoCommands(t *testing.T) {
	content := "This is normal content\nNo commands here"
	result := PreprocessCommands(content, nil)
	if result != content {
		t.Errorf("expected unchanged content, got: %q", result)
	}
}

func TestPreprocessCommands_EmptyContent(t *testing.T) {
	result := PreprocessCommands("", nil)
	if result != "" {
		t.Errorf("expected empty string, got: %q", result)
	}
}

func TestPreprocessCommands_FailedCommand(t *testing.T) {
	content := "!thiscommanddoesnotexist12345"
	result := PreprocessCommands(content, nil)
	if !strings.Contains(result, "<!-- command failed:") {
		t.Errorf("expected error comment, got: %q", result)
	}
}

func TestPreprocessCommands_BlockedCommand(t *testing.T) {
	content := "!echo blocked"
	result := PreprocessCommands(content, []string{"git", "ls"})
	if !strings.Contains(result, "<!-- command blocked:") {
		t.Errorf("expected blocked comment, got: %q", result)
	}
}

func TestPreprocessCommands_AllowedCommand(t *testing.T) {
	content := "!echo allowed"
	result := PreprocessCommands(content, []string{"echo", "git"})
	if !strings.Contains(result, "allowed") {
		t.Errorf("expected 'allowed' in output, got: %q", result)
	}
	if strings.Contains(result, "<!-- command blocked:") {
		t.Errorf("command should not be blocked, got: %q", result)
	}
}

func TestPreprocessCommands_OnlyExclamationMark(t *testing.T) {
	content := "!\nNormal line"
	result := PreprocessCommands(content, nil)
	if !strings.Contains(result, "!") {
		t.Errorf("lone '!' should be preserved, got: %q", result)
	}
	if !strings.Contains(result, "Normal line") {
		t.Errorf("'Normal line' should be preserved, got: %q", result)
	}
}

func TestPreprocessCommands_PreservesIndentation(t *testing.T) {
	content := "  Normal indented line\nAnother line"
	result := PreprocessCommands(content, nil)
	if result != content {
		t.Errorf("indented non-command lines should be preserved, got: %q", result)
	}
}

func TestPreprocessCommands_MultipleCommands(t *testing.T) {
	var content string
	if runtime.GOOS == "windows" {
		content = "!echo first\nMiddle\n!echo second"
	} else {
		content = "!echo first\nMiddle\n!echo second"
	}

	result := PreprocessCommands(content, nil)

	if !strings.Contains(result, "first") {
		t.Errorf("expected 'first' in output, got: %q", result)
	}
	if !strings.Contains(result, "Middle") {
		t.Errorf("expected 'Middle' in output, got: %q", result)
	}
	if !strings.Contains(result, "second") {
		t.Errorf("expected 'second' in output, got: %q", result)
	}
}

func TestPreprocessCommands_BacktickSyntax(t *testing.T) {
	// Formato oficial Claude Code: !`command`
	content := "PR diff:\n!`echo backtick-works`\nDone"
	result := PreprocessCommands(content, nil)

	if !strings.Contains(result, "backtick-works") {
		t.Errorf("expected backtick command output, got: %q", result)
	}
	if strings.Contains(result, "`") {
		t.Errorf("backticks should be stripped from output, got: %q", result)
	}
}

func TestPreprocessCommands_BacktickAndPlain(t *testing.T) {
	// Ambas as sintaxes no mesmo conteúdo
	var content string
	if runtime.GOOS == "windows" {
		content = "!echo plain\n!`echo backtick`"
	} else {
		content = "!echo plain\n!`echo backtick`"
	}
	result := PreprocessCommands(content, nil)

	if !strings.Contains(result, "plain") {
		t.Errorf("expected plain command output, got: %q", result)
	}
	if !strings.Contains(result, "backtick") {
		t.Errorf("expected backtick command output, got: %q", result)
	}
}

func TestStripBackticks(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"`echo hello`", "echo hello"},
		{"echo hello", "echo hello"},
		{"`git log --oneline`", "git log --oneline"},
		{"``", ""},
		{"`", "`"},
		{"", ""},
	}

	for _, tt := range tests {
		result := stripBackticks(tt.input)
		if result != tt.expected {
			t.Errorf("stripBackticks(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestIsCommandAllowed_NilAllowsAll(t *testing.T) {
	if !isCommandAllowed("anything --flag", nil) {
		t.Error("nil allowedCommands should allow everything")
	}
}

func TestIsCommandAllowed_EmptyBlocksAll(t *testing.T) {
	if isCommandAllowed("echo hello", []string{}) {
		t.Error("empty allowedCommands should block everything")
	}
}

func TestIsCommandAllowed_CaseInsensitive(t *testing.T) {
	if !isCommandAllowed("ECHO hello", []string{"echo"}) {
		t.Error("should match case-insensitively")
	}
}

func TestIsCommandAllowed_MatchesExecutable(t *testing.T) {
	if !isCommandAllowed("git log --oneline -5", []string{"git", "echo"}) {
		t.Error("should match by executable name")
	}
	if isCommandAllowed("rm -rf /", []string{"git", "echo"}) {
		t.Error("should not match non-allowed executable")
	}
}

// --- ProcessTemplate tests ---

func TestProcessTemplate_NoTemplates(t *testing.T) {
	content := "This is plain content\nNo templates here"
	result := ProcessTemplate(content, nil)
	if result != content {
		t.Errorf("expected unchanged content, got: %q", result)
	}
}

func TestProcessTemplate_EmptyContent(t *testing.T) {
	result := ProcessTemplate("", nil)
	if result != "" {
		t.Errorf("expected empty string, got: %q", result)
	}
}

func TestProcessTemplate_MalformedTemplate(t *testing.T) {
	content := "Before {{ malformed After"
	result := ProcessTemplate(content, nil)
	if result != content {
		t.Errorf("malformed template should return original content, got: %q", result)
	}
}

func TestProcessTemplate_IncludeNonexistent(t *testing.T) {
	content := `Before
{{ include "nonexistent/file-that-does-not-exist-xyz.md" }}
After`
	result := ProcessTemplate(content, nil)
	if !strings.Contains(result, "Before") {
		t.Errorf("expected 'Before' preserved, got: %q", result)
	}
	if !strings.Contains(result, "After") {
		t.Errorf("expected 'After' preserved, got: %q", result)
	}
}

func TestProcessTemplate_IncludeInvalidPath(t *testing.T) {
	content := `{{ include "no-slash" }}`
	result := ProcessTemplate(content, nil)
	if strings.Contains(result, "no-slash") {
		t.Errorf("invalid path should return empty, got: %q", result)
	}
}

func TestProcessTemplate_IncludeEmptyParts(t *testing.T) {
	content := `{{ include "/file.md" }}`
	result := ProcessTemplate(content, nil)
	if result != "" && strings.TrimSpace(result) != "" {
		t.Logf("include with empty namespace returned: %q", result)
	}
}

func TestProcessTemplate_PreservesNonTemplateBraces(t *testing.T) {
	content := "JSON: {\"key\": \"value\"}\nNormal text"
	result := ProcessTemplate(content, nil)
	if result != content {
		t.Errorf("non-template braces should be preserved, got: %q", result)
	}
}

func TestProcessTemplate_Now(t *testing.T) {
	content := "Timestamp: {{ now }}"
	result := ProcessTemplate(content, nil)

	year := time.Now().Format("2006")
	if !strings.Contains(result, year) {
		t.Errorf("expected current year %s in output, got: %q", year, result)
	}
	if strings.Contains(result, "{{") {
		t.Errorf("template should be resolved, got: %q", result)
	}
}

// TestProcessTemplate_IncludeFromFixture valida o fluxo {{ include }} + {{ now }}
// de ponta a ponta usando uma fixture controlada (HOME temporário), em vez de
// depender do memory.md real da máquina. Assim o teste é determinístico e roda
// no CI.
func TestProcessTemplate_IncludeFromFixture(t *testing.T) {
	// Reset registrado ANTES do Setenv para rodar por último (LIFO): garante que
	// o cache de paths volte ao ambiente real depois que o env for restaurado.
	t.Cleanup(configdir.ResetForTests)

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)        // Unix
	t.Setenv("USERPROFILE", tmp) // Windows
	configdir.ResetForTests()

	memoryDir := filepath.Join(tmp, ".assistente", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	// O conteúdo incluído contém um trecho parecido com template ({{ ... }}) de
	// propósito: ele deve ser inserido VERBATIM, não reprocessado.
	const marker = "MEMORIA_FIXTURE_MARKER"
	const literalTemplate = "{{ if .task.link }}true{{ end }}"
	fixture := marker + "\nGo template doc: `" + literalTemplate + "`\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "memory.md"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	content := `<user_memory>
Current date/time: {{ now }}

{{ include "memory/memory.md" }}
</user_memory>`
	result := ProcessTemplate(content, nil)

	// As diretivas do wrapper devem ter sido resolvidas.
	if strings.Contains(result, "{{ now }}") || strings.Contains(result, "{{ include") {
		t.Errorf("wrapper directives should be resolved, got: %q", result)
	}
	if !strings.Contains(result, "<user_memory>") {
		t.Errorf("expected <user_memory> wrapper, got: %q", result)
	}
	year := time.Now().Format("2006")
	if !strings.Contains(result, year) {
		t.Errorf("expected current year from {{ now }}, got: %q", result)
	}
	// Conteúdo da fixture foi incluído...
	if !strings.Contains(result, marker) {
		t.Errorf("expected included fixture content (marker %q), got: %q", marker, result)
	}
	// ...e o trecho parecido com template sobreviveu VERBATIM (include não reprocessa).
	if !strings.Contains(result, literalTemplate) {
		t.Errorf("included content should be verbatim (expected %q), got: %q", literalTemplate, result)
	}
}

func TestProcessTemplate_ExecWithData(t *testing.T) {
	content := `{{ if .ToolCallingEnabled }}tools on{{ else }}tools off{{ end }}`
	data := struct {
		ToolCallingEnabled bool
	}{ToolCallingEnabled: true}

	result := ProcessTemplate(content, data)
	if strings.TrimSpace(result) != "tools on" {
		t.Errorf("expected template to render using data, got: %q", result)
	}
}

func TestProcessTemplate_SurfaceStateAndContext(t *testing.T) {
	content := `tipo={{ .Surface.Type }}; arquivo={{ index .Surface.State "filePath" }}; seleção={{ index .Surface.Context "selectedText" }}`
	data := struct {
		Surface struct {
			Type    string
			State   map[string]any
			Context map[string]any
		}
	}{}
	data.Surface.Type = "editor"
	data.Surface.State = map[string]any{"filePath": "/tmp/readme.md"}
	data.Surface.Context = map[string]any{"selectedText": "hello"}

	result := ProcessTemplate(content, data)
	if strings.TrimSpace(result) != "tipo=editor; arquivo=/tmp/readme.md; seleção=hello" {
		t.Errorf("expected surface data rendered, got: %q", result)
	}
}
