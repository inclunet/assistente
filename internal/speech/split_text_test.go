package speech

import (
	"strings"
	"testing"
)

func TestSplitTextForTTS_Short(t *testing.T) {
	chunks := splitTextForTTS("Hello world")
	if len(chunks) != 1 {
		t.Fatalf("esperava 1 chunk, obteve %d", len(chunks))
	}
	if chunks[0] != "Hello world" {
		t.Errorf("chunk inesperado: %q", chunks[0])
	}
}

func TestSplitTextForTTS_Empty(t *testing.T) {
	chunks := splitTextForTTS("")
	if len(chunks) != 1 || chunks[0] != "" {
		t.Fatalf("esperava 1 chunk vazio, obteve %v", chunks)
	}
}

func TestSplitTextForTTS_ExactLimit(t *testing.T) {
	text := strings.Repeat("a", 4000)
	chunks := splitTextForTTS(text)
	if len(chunks) != 1 {
		t.Fatalf("esperava 1 chunk para 4000 chars, obteve %d", len(chunks))
	}
}

func TestSplitTextForTTS_LongText(t *testing.T) {
	// 8500 chars com frases separadas por ". "
	var sb strings.Builder
	for i := 0; sb.Len() < 8500; i++ {
		sb.WriteString("Esta é uma frase de teste número ")
		sb.WriteString(strings.Repeat("x", 50))
		sb.WriteString(". ")
	}
	text := sb.String()

	chunks := splitTextForTTS(text)
	if len(chunks) < 2 {
		t.Fatalf("esperava >= 2 chunks para %d chars, obteve %d", len(text), len(chunks))
	}

	// Nenhum chunk deve exceder 4000
	for i, c := range chunks {
		if len(c) > 4000 {
			t.Errorf("chunk %d tem %d chars (máx 4000)", i, len(c))
		}
	}

	// Texto reconstruído deve conter todo o conteúdo original (sem perda)
	reconstructed := strings.Join(chunks, "")
	// Pode ter espaços/newlines removidos nas juntas, mas não deve perder conteúdo significativo
	if len(reconstructed) < len(text)-len(chunks) {
		t.Errorf("texto reconstruído muito curto: %d vs original %d", len(reconstructed), len(text))
	}
}

func TestSplitTextForTTS_BreaksAtSentence(t *testing.T) {
	// Cria um texto com uma frase que termina perto do limite
	part1 := strings.Repeat("a", 3500) + ". "
	part2 := strings.Repeat("b", 3500) + "."
	text := part1 + part2

	chunks := splitTextForTTS(text)
	if len(chunks) < 2 {
		t.Fatalf("esperava >= 2 chunks, obteve %d", len(chunks))
	}
	// Primeiro chunk deve terminar no ". "
	if !strings.HasSuffix(chunks[0], ". ") {
		t.Errorf("primeiro chunk deveria terminar em '. ', terminou em: %q", chunks[0][len(chunks[0])-5:])
	}
}

func TestSplitTextForTTS_BreaksAtParagraph(t *testing.T) {
	part1 := strings.Repeat("a", 3000) + "\n\n"
	part2 := strings.Repeat("b", 3000)
	text := part1 + part2

	chunks := splitTextForTTS(text)
	if len(chunks) < 2 {
		t.Fatalf("esperava >= 2 chunks, obteve %d", len(chunks))
	}
}

func TestSplitTextForTTS_NoBreakPoints(t *testing.T) {
	// Texto sem espaços — corte bruto
	text := strings.Repeat("x", 8500)
	chunks := splitTextForTTS(text)
	if len(chunks) < 2 {
		t.Fatalf("esperava >= 2 chunks, obteve %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > 4000 {
			t.Errorf("chunk %d tem %d chars (máx 4000)", i, len(c))
		}
	}
}
