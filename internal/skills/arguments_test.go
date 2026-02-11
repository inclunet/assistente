package skills

import (
	"testing"
)

func TestSubstituteArguments_FullString(t *testing.T) {
	content := "Fix GitHub issue $ARGUMENTS"
	result := SubstituteArguments(content, "42")
	expected := "Fix GitHub issue 42"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_Positional(t *testing.T) {
	content := "Review PR $1 in repo $2"
	result := SubstituteArguments(content, "123 my-repo")
	expected := "Review PR 123 in repo my-repo"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_MixedArgsAndPositional(t *testing.T) {
	content := "Task: $ARGUMENTS\nFirst arg: $1\nSecond arg: $2"
	result := SubstituteArguments(content, `hello "big world"`)
	expected := "Task: hello \"big world\"\nFirst arg: hello\nSecond arg: big world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_QuotedArgs(t *testing.T) {
	content := "Search for $1 in $2"
	result := SubstituteArguments(content, `"hello world" /tmp/dir`)
	expected := "Search for hello world in /tmp/dir"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_SingleQuotes(t *testing.T) {
	content := "Run with $1"
	result := SubstituteArguments(content, "'my arg'")
	expected := "Run with my arg"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_NoArgs(t *testing.T) {
	content := "No args skill content $ARGUMENTS"
	result := SubstituteArguments(content, "")
	expected := "No args skill content "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_EmptyContent(t *testing.T) {
	result := SubstituteArguments("", "some args")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestSubstituteArguments_NoPlaceholders(t *testing.T) {
	content := "This skill has no placeholders"
	result := SubstituteArguments(content, "some args")
	if result != content {
		t.Errorf("expected %q, got %q", content, result)
	}
}

func TestSubstituteArguments_DoubleDigitPositional(t *testing.T) {
	// Testa que $10 não é confundido com $1 + "0"
	content := "Arg10: $10, Arg1: $1"
	args := "a b c d e f g h i j"
	result := SubstituteArguments(content, args)
	expected := "Arg10: j, Arg1: a"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestParseArgs_Basic(t *testing.T) {
	args := parseArgs("hello world")
	if len(args) != 2 || args[0] != "hello" || args[1] != "world" {
		t.Errorf("unexpected: %v", args)
	}
}

func TestParseArgs_Quoted(t *testing.T) {
	args := parseArgs(`"hello world" foo`)
	if len(args) != 2 || args[0] != "hello world" || args[1] != "foo" {
		t.Errorf("unexpected: %v", args)
	}
}

func TestParseArgs_Empty(t *testing.T) {
	args := parseArgs("")
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestParseArgs_WhitespaceOnly(t *testing.T) {
	args := parseArgs("   ")
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestParseArgs_MixedQuotes(t *testing.T) {
	args := parseArgs(`"double quoted" 'single quoted' plain`)
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
	if args[0] != "double quoted" {
		t.Errorf("arg[0]: expected %q, got %q", "double quoted", args[0])
	}
	if args[1] != "single quoted" {
		t.Errorf("arg[1]: expected %q, got %q", "single quoted", args[1])
	}
	if args[2] != "plain" {
		t.Errorf("arg[2]: expected %q, got %q", "plain", args[2])
	}
}
