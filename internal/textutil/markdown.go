package textutil

import (
	"regexp"
	"strings"
)

var (
	reCodeFence   = regexp.MustCompile("(?s)```.*?```")
	reInlineCode  = regexp.MustCompile("`([^`]+)`")
	reLink        = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reImage       = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	reBoldItalic3 = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)
	reBoldStar    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalicStar  = regexp.MustCompile(`\*(.+?)\*`)
	reBoldItalicU = regexp.MustCompile(`___(.+?)___`)
	reBoldU       = regexp.MustCompile(`__(.+?)__`)
	reItalicU     = regexp.MustCompile(`_(.+?)_`)
	reStrike      = regexp.MustCompile(`~~(.+?)~~`)
	reHeading     = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reUL          = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	reOL          = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	reQuote       = regexp.MustCompile(`(?m)^\s*>\s?`)
	reHR          = regexp.MustCompile(`(?m)^[\-*_]{3,}\s*$`)
	reSpaces      = regexp.MustCompile(`[ \t]{2,}`)
	reBlankLines  = regexp.MustCompile(`\n{3,}`)
)

// DefaultCodeBlockSpeechLabel é o rótulo falado para blocos de código quando
// o idioma do perfil é desconhecido.
const DefaultCodeBlockSpeechLabel = "code block"

var codeBlockSpeechLabels = map[string]string{
	"pt": "bloco de código",
	"es": "bloque de código",
	"en": DefaultCodeBlockSpeechLabel,
}

// CodeBlockSpeechLabel traduz o rótulo de bloco de código para o idioma
// informado (tag BCP-47, ex.: "pt-BR"); idiomas desconhecidos caem no inglês.
func CodeBlockSpeechLabel(language string) string {
	primary := strings.ToLower(strings.TrimSpace(language))
	if idx := strings.IndexAny(primary, "-_"); idx > 0 {
		primary = primary[:idx]
	}
	if label, ok := codeBlockSpeechLabels[primary]; ok {
		return label
	}
	return DefaultCodeBlockSpeechLabel
}

// StripMarkdownForSpeech remove sintaxe Markdown para TTS, live regions e
// texto destinado a leitores de tela / síntese de fala. Não altera o
// conteúdo persistido no chat — só o payload falado/enviado como fala.
// Callers que sintetizam áudio devem fazer fallback ao texto original
// quando o resultado trimado for vazio (ex.: só HR/fence).
func StripMarkdownForSpeech(text string) string {
	return StripMarkdownForSpeechLabeled(text, DefaultCodeBlockSpeechLabel)
}

// StripMarkdownForSpeechLabeled é a variante de StripMarkdownForSpeech que
// permite localizar o marcador falado dos blocos de código.
func StripMarkdownForSpeechLabeled(text, codeBlockLabel string) string {
	if text == "" {
		return text
	}

	label := strings.TrimSpace(codeBlockLabel)
	if label == "" {
		label = DefaultCodeBlockSpeechLabel
	}

	result := text
	// Blocos de código → marcador curto no idioma da fala.
	result = reCodeFence.ReplaceAllLiteralString(result, " "+label+" ")
	result = reInlineCode.ReplaceAllString(result, "$1")
	result = reImage.ReplaceAllString(result, "$1")
	result = reLink.ReplaceAllString(result, "$1")
	result = reBoldItalic3.ReplaceAllString(result, "$1")
	result = reBoldStar.ReplaceAllString(result, "$1")
	result = reItalicStar.ReplaceAllString(result, "$1")
	result = reBoldItalicU.ReplaceAllString(result, "$1")
	result = reBoldU.ReplaceAllString(result, "$1")
	result = reItalicU.ReplaceAllString(result, "$1")
	result = reStrike.ReplaceAllString(result, "$1")
	result = reHeading.ReplaceAllString(result, "")
	result = reUL.ReplaceAllString(result, "")
	result = reOL.ReplaceAllString(result, "")
	result = reQuote.ReplaceAllString(result, "")
	result = reHR.ReplaceAllString(result, "")
	result = reSpaces.ReplaceAllString(result, " ")
	result = reBlankLines.ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}
