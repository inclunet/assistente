package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestProcessTemplate_IncludeRealMemory(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	memoryPath := filepath.Join(homeDir, ".assistente", "memory", "memory.md")
	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		t.Skipf("memory.md not found at %s (skip on CI)", memoryPath)
	}

	content := `<user_memory>
Current date/time: {{ now }}

{{ include "memory/memory.md" }}
</user_memory>`
	result := ProcessTemplate(content, nil)

	if strings.Contains(result, "{{") {
		t.Errorf("templates should be resolved, got: %q", result)
	}
	if !strings.Contains(result, "<user_memory>") {
		t.Errorf("expected <user_memory> wrapper, got: %q", result)
	}
	year := time.Now().Format("2006")
	if !strings.Contains(result, year) {
		t.Errorf("expected current year in now output, got: %q", result)
	}
	if len(result) < 100 {
		t.Errorf("expected substantial content from memory.md, got only %d chars: %q", len(result), result)
	}
	t.Logf("Include result (%d bytes):\n%s", len(result), result[:min(500, len(result))])
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
