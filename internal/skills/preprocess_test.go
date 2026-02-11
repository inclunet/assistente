package skills

import (
	"runtime"
	"strings"
	"testing"
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
