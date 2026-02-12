package telegram

import (
	"strings"
	"testing"
)

func TestSplitMessage_Short(t *testing.T) {
	parts := SplitMessage("Hello, world!")
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0] != "Hello, world!" {
		t.Fatalf("expected 'Hello, world!', got %q", parts[0])
	}
}

func TestSplitMessage_Empty(t *testing.T) {
	parts := SplitMessage("")
	if len(parts) != 1 || parts[0] != "" {
		t.Fatalf("expected 1 empty part, got %v", parts)
	}
}

func TestSplitMessage_ExactLimit(t *testing.T) {
	text := strings.Repeat("a", maxMessageLength)
	parts := SplitMessage(text)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part for exact limit, got %d", len(parts))
	}
}

func TestSplitMessage_Long(t *testing.T) {
	// Cria texto com linhas de 100 chars, totalizando ~8000 chars
	var sb strings.Builder
	for i := 0; i < 80; i++ {
		sb.WriteString(strings.Repeat("x", 99))
		sb.WriteString("\n")
	}
	text := sb.String()

	parts := SplitMessage(text)
	if len(parts) < 2 {
		t.Fatalf("expected at least 2 parts, got %d", len(parts))
	}

	// Nenhuma parte deve exceder o limite
	for i, part := range parts {
		if len(part) > maxMessageLength {
			t.Fatalf("part %d exceeds limit: %d chars", i, len(part))
		}
	}

	// Todas as partes juntas devem recompor o texto original
	joined := strings.Join(parts, "")
	if joined != text {
		t.Fatalf("joined parts differ from original (len %d vs %d)", len(joined), len(text))
	}
}
