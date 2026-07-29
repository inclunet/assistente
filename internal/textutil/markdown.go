package textutil

import (
	"regexp"
	"strings"
)

// StripMarkdownForSpeech remove sintaxe Markdown para TTS, live regions e
// texto destinado a leitores de tela / síntese de fala. Não altera o
// conteúdo persistido no chat — só o payload falado/enviado como fala.
func StripMarkdownForSpeech(text string) string {
	if text == "" {
		return text
	}

	result := text
	// Blocos de código → marcador curto (evita ler fences e código cru)
	result = regexp.MustCompile("(?s)```.*?```").ReplaceAllString(result, " bloco de código ")
	result = regexp.MustCompile("`([^`]+)`").ReplaceAllString(result, "$1")
	// Links [texto](url) → texto
	result = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(result, "$1")
	// Imagens ![alt](url) → alt
	result = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`).ReplaceAllString(result, "$1")
	// Negrito / itálico / tachado
	result = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`).ReplaceAllString(result, "$1")
	result = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(result, "$1")
	result = regexp.MustCompile(`\*(.+?)\*`).ReplaceAllString(result, "$1")
	result = regexp.MustCompile(`___(.+?)___`).ReplaceAllString(result, "$1")
	result = regexp.MustCompile(`__(.+?)__`).ReplaceAllString(result, "$1")
	result = regexp.MustCompile(`_(.+?)_`).ReplaceAllString(result, "$1")
	result = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(result, "$1")
	// Cabeçalhos, listas, citações (início de linha)
	result = regexp.MustCompile(`(?m)^#{1,6}\s+`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?m)^\s*[-*+]\s+`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?m)^\s*\d+\.\s+`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?m)^\s*>\s?`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?m)^[\-*_]{3,}\s*$`).ReplaceAllString(result, "")
	// Colapsa espaços / linhas em branco
	result = regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(result, " ")
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}
