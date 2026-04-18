package app

import (
	"testing"
)

func TestParseSlashCommand_Basic(t *testing.T) {
	slug, args, ok := parseSlashCommand("/fix-issue 42")
	if !ok {
		t.Fatal("expected slash command to be detected")
	}
	if slug != "fix-issue" {
		t.Errorf("expected slug 'fix-issue', got %q", slug)
	}
	if args != "42" {
		t.Errorf("expected args '42', got %q", args)
	}
}

func TestParseSlashCommand_NoArgs(t *testing.T) {
	slug, args, ok := parseSlashCommand("/memory")
	if !ok {
		t.Fatal("expected slash command to be detected")
	}
	if slug != "memory" {
		t.Errorf("expected slug 'memory', got %q", slug)
	}
	if args != "" {
		t.Errorf("expected empty args, got %q", args)
	}
}

func TestParseSlashCommand_MultipleArgs(t *testing.T) {
	slug, args, ok := parseSlashCommand(`/deploy staging "my app"`)
	if !ok {
		t.Fatal("expected slash command to be detected")
	}
	if slug != "deploy" {
		t.Errorf("expected slug 'deploy', got %q", slug)
	}
	if args != `staging "my app"` {
		t.Errorf("expected args 'staging \"my app\"', got %q", args)
	}
}

func TestParseSlashCommand_NotSlash(t *testing.T) {
	_, _, ok := parseSlashCommand("hello world")
	if ok {
		t.Error("non-slash message should not be detected")
	}
}

func TestParseSlashCommand_EmptySlash(t *testing.T) {
	_, _, ok := parseSlashCommand("/")
	if ok {
		t.Error("bare '/' should not be detected")
	}
}

func TestParseSlashCommand_SpaceAfterSlash(t *testing.T) {
	_, _, ok := parseSlashCommand("/ something")
	if ok {
		t.Error("'/ something' should not be detected (space after /)")
	}
}

func TestParseSlashCommand_SpecialChars(t *testing.T) {
	_, _, ok := parseSlashCommand("/foo_bar baz")
	if ok {
		t.Error("slug with underscore should not be valid")
	}
}

func TestParseSlashCommand_UpperCase(t *testing.T) {
	slug, _, ok := parseSlashCommand("/MEMORY")
	if !ok {
		t.Fatal("expected slash command to be detected (lowercased)")
	}
	if slug != "memory" {
		t.Errorf("expected slug 'memory' (lowered), got %q", slug)
	}
}

func TestParseSlashCommand_WithWhitespace(t *testing.T) {
	slug, args, ok := parseSlashCommand("  /fix-issue 42  ")
	if !ok {
		t.Fatal("expected slash command to be detected after trimming")
	}
	if slug != "fix-issue" {
		t.Errorf("expected slug 'fix-issue', got %q", slug)
	}
	if args != "42" {
		t.Errorf("expected args '42', got %q", args)
	}
}
