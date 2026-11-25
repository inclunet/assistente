package skills

import (
	"strings"
	"testing"
)

func invokedBody(t *testing.T, content, args, session string) string {
	t.Helper()
	s := invocableSkill(content)
	mgr := &stubInvokerManager{skill: s}
	result, found, err := InvokeContext(t.Context(), "/test-skill "+args, mgr, session, invocablePolicy(s))
	if err != nil || !found || result == nil {
		t.Fatalf("invocação: found=%v err=%v", found, err)
	}
	before, after := invocationFrame(s, mgr.files)
	return strings.TrimSuffix(strings.TrimPrefix(result.Content, before), after)
}

func TestInvokedArguments_FullString(t *testing.T) {
	content := "Fix GitHub issue $ARGUMENTS"
	raw := "42"
	result := invokedBody(t, content, raw, "sessão")
	expected := "Fix GitHub issue 42"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestInvokedArguments_Positional_ZeroBased(t *testing.T) {
	// Spec: $0 = primeiro argumento, $1 = segundo
	content := "Review PR $0 in repo $1"
	raw := "123 my-repo"
	result := invokedBody(t, content, raw, "sessão")
	expected := "Review PR 123 in repo my-repo"
	if !strings.HasPrefix(result, expected) {
		t.Errorf("expected prefix %q, got %q", expected, result)
	}
}

func TestInvokedArguments_BracketSyntax(t *testing.T) {
	// Spec: $ARGUMENTS[0] = primeiro, $ARGUMENTS[1] = segundo
	content := "Migrate $ARGUMENTS[0] from $ARGUMENTS[1] to $ARGUMENTS[2]"
	raw := "SearchBar React Vue"
	result := invokedBody(t, content, raw, "sessão")
	expected := "Migrate SearchBar from React to Vue"
	if !strings.HasPrefix(result, expected) {
		t.Errorf("expected prefix %q, got %q", expected, result)
	}
}

func TestInvokedArguments_MixedArgsAndPositional(t *testing.T) {
	content := "Task: $ARGUMENTS\nFirst arg: $0\nSecond arg: $1"
	raw := `hello "big world"`
	result := invokedBody(t, content, raw, "sessão")
	expected := "Task: hello \"big world\"\nFirst arg: hello\nSecond arg: big world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestInvokedArguments_QuotedArgs(t *testing.T) {
	content := "Search for $0 in $1"
	raw := `"hello world" /tmp/dir`
	result := invokedBody(t, content, raw, "sessão")
	// $ARGUMENTS não está no conteúdo → fallback appended
	if !strings.HasPrefix(result, "Search for hello world in /tmp/dir") {
		t.Errorf("expected to start with 'Search for hello world in /tmp/dir', got %q", result)
	}
}

func TestInvokedArguments_SingleQuotes(t *testing.T) {
	content := "Run with $0"
	raw := "'my arg'"
	result := invokedBody(t, content, raw, "sessão")
	if !strings.HasPrefix(result, "Run with my arg") {
		t.Errorf("expected to start with 'Run with my arg', got %q", result)
	}
}

func TestInvokedArguments_NoArgs(t *testing.T) {
	content := "No args skill content $ARGUMENTS"
	raw := ""
	result := invokedBody(t, content, raw, "sessão")
	expected := "No args skill content "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestEmptySkillInvocationPreservesEmptyBody(t *testing.T) {
	skill := invocableSkill("")
	result, found, err := InvokeContext(t.Context(), "/test-skill texto", &stubInvokerManager{skill: skill}, "turn", invocablePolicy(skill))
	if err != nil || !found || result == nil || strings.Contains(result.Content, "texto") {
		t.Fatalf("result=%+v found=%t err=%v", result, found, err)
	}
}

func TestInvokedArguments_NoPlaceholders(t *testing.T) {
	content := "This skill has no placeholders"
	raw := "some args"
	result := invokedBody(t, content, raw, "sessão")
	// Deve fazer fallback: append "ARGUMENTS: some args"
	if !strings.Contains(result, "ARGUMENTS: some args") {
		t.Errorf("expected fallback ARGUMENTS append, got %q", result)
	}
	if !strings.HasPrefix(result, content) {
		t.Errorf("expected original content preserved, got %q", result)
	}
}

func TestInvokedArguments_NoPlaceholders_NoArgs(t *testing.T) {
	content := "This skill has no placeholders"
	raw := ""
	result := invokedBody(t, content, raw, "sessão")
	// Sem args e sem placeholders → não deve appendar nada
	if result != content {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestInvokedArguments_FallbackOnlyWithoutDollarArguments(t *testing.T) {
	// Se $ARGUMENTS está no conteúdo, NÃO faz fallback
	content := "Fix $ARGUMENTS now"
	raw := "bug 42"
	result := invokedBody(t, content, raw, "sessão")
	expected := "Fix bug 42 now"
	if result != expected {
		t.Errorf("expected %q (no fallback), got %q", expected, result)
	}
}

func TestInvokedArguments_DoubleDigitPositional(t *testing.T) {
	// Testa que $10 não é confundido com $1 + "0" (0-based: $9 = 10º arg)
	content := "Arg9: $9, Arg0: $0"
	args := "a b c d e f g h i j"
	raw := args
	result := invokedBody(t, content, raw, "sessão")
	// $ARGUMENTS não presente → fallback
	if !strings.HasPrefix(result, "Arg9: j, Arg0: a") {
		t.Errorf("expected 'Arg9: j, Arg0: a' prefix, got %q", result)
	}
}

func TestInvokedArguments_BracketAndShorthand(t *testing.T) {
	// Ambas as sintaxes no mesmo conteúdo
	content := "First: $ARGUMENTS[0], Second: $1, All: $ARGUMENTS"
	raw := "hello world"
	result := invokedBody(t, content, raw, "sessão")
	expected := "First: hello, Second: world, All: hello world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestInvokedArguments_SessionID(t *testing.T) {
	content := "Log to logs/${CLAUDE_SESSION_ID}.log:\n$ARGUMENTS"
	vars := map[string]string{"CLAUDE_SESSION_ID": "42"}
	raw := "hello world"
	result := invokedBody(t, content, raw, vars["CLAUDE_SESSION_ID"])
	expected := "Log to logs/42.log:\nhello world"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestInvokedArguments_SessionIDWithoutArgs(t *testing.T) {
	content := "Session: ${CLAUDE_SESSION_ID}"
	vars := map[string]string{"CLAUDE_SESSION_ID": "abc-123"}
	raw := ""
	result := invokedBody(t, content, raw, vars["CLAUDE_SESSION_ID"])
	expected := "Session: abc-123"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestInvokedArguments_BracketOnlyContent(t *testing.T) {
	// Conteúdo usa apenas $ARGUMENTS[N], sem $ARGUMENTS plain → fallback NÃO deve ocorrer
	// porque $ARGUMENTS[N] é considerado uso do sistema de args
	content := "First: $ARGUMENTS[0], Second: $ARGUMENTS[1]"
	raw := "hello world"
	result := invokedBody(t, content, raw, "sessão")
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

func TestArgumentExpansionSkipsPlainText(t *testing.T) {
	args := make([]string, 100)
	expand := argumentExpander("", args)
	const line = "Texto sem marcador"
	allocs := testing.AllocsPerRun(100, func() {
		if expand(line) != line {
			t.Fatal("texto alterado")
		}
	})
	if allocs != 0 {
		t.Fatalf("alocações por linha: %v", allocs)
	}
}
