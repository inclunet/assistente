package filesystem

import (
	"fmt"
	"strings"
)

const (
	patchPreviewContextRunes = 40
	patchPreviewSideRunes    = 300
)

// patchConfirmationPreview mostra cada alteração validada no lugar em que ela
// ocorrerá. Uma prévia do prefixo do arquivo pode ser idêntica em Antes/Depois
// quando os hunks ficam depois do limite de truncamento.
func patchConfirmationPreview(original string, spans []applyPatchSpan) (string, string) {
	var before, after strings.Builder
	lineDelta := 0
	for index, span := range spans {
		if index > 0 {
			before.WriteString("\n\n")
			after.WriteString("\n\n")
		}

		oldText := original[span.start:span.end]
		line := strings.Count(original[:span.start], "\n") + 1
		header := fmt.Sprintf("@@ -%d +%d #%d @@\n", line, line+lineDelta, span.hunk)
		before.WriteString(header)
		after.WriteString(header)

		// Inclui o restante da linha para situar contextos curtos. O recorte
		// abaixo remove o excesso sem perder o início real da alteração.
		lineStart := strings.LastIndex(original[:span.start], "\n") + 1
		lineEnd := len(original)
		if next := strings.IndexByte(original[span.end:], '\n'); next >= 0 {
			lineEnd = span.end + next
		}
		// Não incluir outro hunk da mesma linha no contexto: no painel Depois
		// isso mostraria texto antigo que não existe no arquivo final.
		if index > 0 {
			lineStart = max(lineStart, spans[index-1].end)
		}
		if index+1 < len(spans) {
			lineEnd = min(lineEnd, spans[index+1].start)
		}
		prefix := original[lineStart:span.start]
		suffix := original[span.end:lineEnd]
		oldView := prefix + oldText + suffix
		newView := prefix + span.replacement + suffix
		beforePart, afterPart := focusedPatchPair(oldView, newView)
		before.WriteString(beforePart)
		after.WriteString(afterPart)

		lineDelta += strings.Count(span.replacement, "\n") - strings.Count(oldText, "\n")
	}
	return before.String(), after.String()
}

func focusedPatchPair(before, after string) (string, string) {
	oldRunes := []rune(before)
	newRunes := []rune(after)
	commonPrefix := 0
	for commonPrefix < len(oldRunes) && commonPrefix < len(newRunes) && oldRunes[commonPrefix] == newRunes[commonPrefix] {
		commonPrefix++
	}
	commonSuffix := 0
	for commonSuffix < len(oldRunes)-commonPrefix && commonSuffix < len(newRunes)-commonPrefix &&
		oldRunes[len(oldRunes)-commonSuffix-1] == newRunes[len(newRunes)-commonSuffix-1] {
		commonSuffix++
	}
	return focusedPatchSide(oldRunes, commonPrefix, commonSuffix),
		focusedPatchSide(newRunes, commonPrefix, commonSuffix)
}

func focusedPatchSide(text []rune, commonPrefix, commonSuffix int) string {
	start := max(0, commonPrefix-patchPreviewContextRunes)
	end := min(len(text), len(text)-commonSuffix+patchPreviewContextRunes)
	view := text[start:end]

	var out strings.Builder
	if start > 0 {
		out.WriteString("…\n")
	}
	if len(view) > patchPreviewSideRunes*2 {
		out.WriteString(string(view[:patchPreviewSideRunes]))
		out.WriteString("\n…\n")
		out.WriteString(string(view[len(view)-patchPreviewSideRunes:]))
	} else {
		out.WriteString(string(view))
	}
	if end < len(text) {
		out.WriteString("\n…")
	}
	return out.String()
}
