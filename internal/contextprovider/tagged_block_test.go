package contextprovider

import (
	"strings"
	"testing"
)

func TestTrimTaggedBlockToBudgetKeepsContentWhenWithinBudget(t *testing.T) {
	content := "<example>\nshort body\n</example>"

	got := TrimTaggedBlockToBudget(content, "example", "... omitted", 100)

	if got != content {
		t.Fatalf("TrimTaggedBlockToBudget() = %q, want %q", got, content)
	}
}

func TestTrimTaggedBlockToBudgetPreservesTagsAndNotice(t *testing.T) {
	got := TrimTaggedBlockToBudget("<example>\n"+strings.Repeat("body ", 50)+"\n</example>", "example", "... omitted", 60)

	if RuneLen(got) > 60 {
		t.Fatalf("length = %d, want <= 60: %q", RuneLen(got), got)
	}
	if !strings.HasPrefix(got, "<example>\n") || !strings.HasSuffix(got, "\n</example>") {
		t.Fatalf("expected preserved tags: %q", got)
	}
	if !strings.Contains(got, "... omitted") {
		t.Fatalf("expected truncation notice: %q", got)
	}
}

func TestTrimTaggedBlockToBudgetOmitsWhenEnvelopeCannotFit(t *testing.T) {
	got := TrimTaggedBlockToBudget("<example>\nbody\n</example>", "example", "... omitted", 10)

	if got != "" {
		t.Fatalf("TrimTaggedBlockToBudget() = %q, want omitted block", got)
	}
}

func TestMinimalTaggedBlockLen(t *testing.T) {
	got := MinimalTaggedBlockLen("example", "... omitted")
	want := RuneLen("<example>\n... omitted\n</example>")

	if got != want {
		t.Fatalf("MinimalTaggedBlockLen() = %d, want %d", got, want)
	}
}
