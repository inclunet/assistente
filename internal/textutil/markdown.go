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

// StripMarkdownForSpeech remove sintaxe Markdown para TTS, live regions e
// texto destinado a leitores de tela / síntese de fala. Não altera o
// conteúdo persistido no chat — só o payload falado/enviado como fala.
func StripMarkdownForSpeech(text string) string {
	if text == "" {
		return text
	}

	result := text
	// Blocos de código → marcador curto neutro (TTS/canais; announcer TS usa i18n).
	result = reCodeFence.ReplaceAllString(result, " code block ")
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
