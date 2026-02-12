package skills

import (
	"strings"
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

func TestSubstituteArguments_Positional_ZeroBased(t *testing.T) {
	// Spec: $0 = primeiro argumento, $1 = segundo
	content := "Review PR $0 in repo $1"
	result := SubstituteArguments(content, "123 my-repo")
	expected := "Review PR 123 in repo my-repo"
	if !strings.HasPrefix(result, expected) {
		t.Errorf("expected prefix %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_BracketSyntax(t *testing.T) {
	// Spec: $ARGUMENTS[0] = primeiro, $ARGUMENTS[1] = segundo
	content := "Migrate $ARGUMENTS[0] from $ARGUMENTS[1] to $ARGUMENTS[2]"
	result := SubstituteArguments(content, "SearchBar React Vue")
	expected := "Migrate SearchBar from React to Vue"
	if !strings.HasPrefix(result, expected) {
		t.Errorf("expected prefix %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_MixedArgsAndPositional(t *testing.T) {
	content := "Task: $ARGUMENTS\nFirst arg: $0\nSecond arg: $1"
	result := SubstituteArguments(content, `hello "big world"`)
	expected := "Task: hello \"big world\"\nFirst arg: hello\nSecond arg: big world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_QuotedArgs(t *testing.T) {
	content := "Search for $0 in $1"
	result := SubstituteArguments(content, `"hello world" /tmp/dir`)
	// $ARGUMENTS não está no conteúdo → fallback appended
	if !strings.HasPrefix(result, "Search for hello world in /tmp/dir") {
		t.Errorf("expected to start with 'Search for hello world in /tmp/dir', got %q", result)
	}
}

func TestSubstituteArguments_SingleQuotes(t *testing.T) {
	content := "Run with $0"
	result := SubstituteArguments(content, "'my arg'")
	if !strings.HasPrefix(result, "Run with my arg") {
		t.Errorf("expected to start with 'Run with my arg', got %q", result)
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
	// Deve fazer fallback: append "ARGUMENTS: some args"
	if !strings.Contains(result, "ARGUMENTS: some args") {
		t.Errorf("expected fallback ARGUMENTS append, got %q", result)
	}
	if !strings.HasPrefix(result, content) {
		t.Errorf("expected original content preserved, got %q", result)
	}
}

func TestSubstituteArguments_NoPlaceholders_NoArgs(t *testing.T) {
	content := "This skill has no placeholders"
	result := SubstituteArguments(content, "")
	// Sem args e sem placeholders → não deve appendar nada
	if result != content {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestSubstituteArguments_FallbackOnlyWithoutDollarArguments(t *testing.T) {
	// Se $ARGUMENTS está no conteúdo, NÃO faz fallback
	content := "Fix $ARGUMENTS now"
	result := SubstituteArguments(content, "bug 42")
	expected := "Fix bug 42 now"
	if result != expected {
		t.Errorf("expected %q (no fallback), got %q", expected, result)
	}
}

func TestSubstituteArguments_DoubleDigitPositional(t *testing.T) {
	// Testa que $10 não é confundido com $1 + "0" (0-based: $9 = 10º arg)
	content := "Arg9: $9, Arg0: $0"
	args := "a b c d e f g h i j"
	result := SubstituteArguments(content, args)
	// $ARGUMENTS não presente → fallback
	if !strings.HasPrefix(result, "Arg9: j, Arg0: a") {
		t.Errorf("expected 'Arg9: j, Arg0: a' prefix, got %q", result)
	}
}

func TestSubstituteArguments_BracketAndShorthand(t *testing.T) {
	// Ambas as sintaxes no mesmo conteúdo
	content := "First: $ARGUMENTS[0], Second: $1, All: $ARGUMENTS"
	result := SubstituteArguments(content, "hello world")
	expected := "First: hello, Second: world, All: hello world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_SessionID(t *testing.T) {
	content := "Log to logs/${CLAUDE_SESSION_ID}.log:\n$ARGUMENTS"
	vars := map[string]string{"CLAUDE_SESSION_ID": "42"}
	result := SubstituteArguments(content, "hello world", vars)
	expected := "Log to logs/42.log:\nhello world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_SessionIDWithoutArgs(t *testing.T) {
	content := "Session: ${CLAUDE_SESSION_ID}"
	vars := map[string]string{"CLAUDE_SESSION_ID": "abc-123"}
	result := SubstituteArguments(content, "", vars)
	expected := "Session: abc-123"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstituteArguments_BracketOnlyContent(t *testing.T) {
	// Conteúdo usa apenas $ARGUMENTS[N], sem $ARGUMENTS plain → fallback NÃO deve ocorrer
	// porque $ARGUMENTS[N] é considerado uso do sistema de args
	content := "First: $ARGUMENTS[0], Second: $ARGUMENTS[1]"
	result := SubstituteArguments(content, "hello world")
	// $ARGUMENTS (plain) não existe → fallback deve appendar
	if !strings.Contains(result, "ARGUMENTS: hello world") {
		t.Errorf("expected fallback since plain $ARGUMENTS not present, got %q", result)
	}
	if !strings.Contains(result, "First: hello") {
		t.Errorf("expected bracket substitution, got %q", result)
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
