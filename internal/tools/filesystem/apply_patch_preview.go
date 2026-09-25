package filesystem

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	patchPreviewContextRunes = 40
	patchPreviewSideRunes    = 300
)

// patchConfirmationPreview mostra cada hunk validado no ponto em que a
// alteração ocorrerá. O contexto e o trecho modificado são recortados antes
// de montar os blocos, sem copiar uma linha inteira potencialmente enorme.
func patchConfirmationPreview(original string, spans []applyPatchSpan) (string, string) {
	var before, after strings.Builder
	line, lineDelta, previousEnd := 1, 0, 0
	for index, span := range spans {
		if index > 0 {
			before.WriteString("\n\n")
			after.WriteString("\n\n")
		}

		oldText := original[span.start:span.end]
		line += strings.Count(original[previousEnd:span.start], "\n")
		header := fmt.Sprintf("@@ -%d +%d #%d @@\n", line, line+lineDelta, span.hunk)
		before.WriteString(header)
		after.WriteString(header)

		// Contexto da linha, limitado pelos hunks vizinhos: o painel Depois não
		// pode exibir texto antigo de outra alteração na mesma linha.
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
		beforePart, afterPart := focusedPatchPair(
			original[lineStart:span.start], oldText, span.replacement, original[span.end:lineEnd],
		)
		before.WriteString(beforePart)
		after.WriteString(afterPart)

		line += strings.Count(oldText, "\n")
		lineDelta += strings.Count(span.replacement, "\n") - strings.Count(oldText, "\n")
		previousEnd = span.end
	}
	return before.String(), after.String()
}

// focusedPatchPair compara o texto substituído sem concatenar o restante da
// linha. Os offsets são calculados por runa; só as fatias exibidas são copiadas.
func focusedPatchPair(prefix, oldText, newText, suffix string) (string, string) {
	commonPrefix := 0
	for commonPrefix < len(oldText) && commonPrefix < len(newText) {
		oldRune, oldSize := utf8.DecodeRuneInString(oldText[commonPrefix:])
		newRune, newSize := utf8.DecodeRuneInString(newText[commonPrefix:])
		if oldRune != newRune || oldSize != newSize {
			break
		}
		commonPrefix += oldSize
	}
	commonOldSuffix, commonNewSuffix := 0, 0
	for commonOldSuffix < len(oldText)-commonPrefix && commonNewSuffix < len(newText)-commonPrefix {
		oldRune, oldSize := utf8.DecodeLastRuneInString(oldText[:len(oldText)-commonOldSuffix])
		newRune, newSize := utf8.DecodeLastRuneInString(newText[:len(newText)-commonNewSuffix])
		if oldRune != newRune || oldSize != newSize {
			break
		}
		commonOldSuffix += oldSize
		commonNewSuffix += newSize
	}

	left, leftOmitted := patchLeftContext(prefix, oldText[:commonPrefix])
	right, rightOmitted := patchRightContext(oldText[len(oldText)-commonOldSuffix:], suffix)
	before := patchPreviewSide(left, leftOmitted, oldText[commonPrefix:len(oldText)-commonOldSuffix], right, rightOmitted)
	after := patchPreviewSide(left, leftOmitted, newText[commonPrefix:len(newText)-commonNewSuffix], right, rightOmitted)
	return before, after
}

func patchLastRunes(text string, limit int) (string, int) {
	start, count := len(text), 0
	for start > 0 && count < limit {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		start -= size
		count++
	}
	return text[start:], count
}

func patchFirstRunes(text string, limit int) (string, int) {
	end, count := 0, 0
	for end < len(text) && count < limit {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
		count++
	}
	return text[:end], count
}

func patchLeftContext(prefix, common string) (string, bool) {
	fromCommon, count := patchLastRunes(common, patchPreviewContextRunes)
	fromPrefix, _ := patchLastRunes(prefix, patchPreviewContextRunes-count)
	return fromPrefix + fromCommon, len(fromPrefix) < len(prefix) || len(fromCommon) < len(common)
}

func patchRightContext(common, suffix string) (string, bool) {
	fromCommon, count := patchFirstRunes(common, patchPreviewContextRunes)
	fromSuffix, _ := patchFirstRunes(suffix, patchPreviewContextRunes-count)
	return fromCommon + fromSuffix, len(fromCommon) < len(common) || len(fromSuffix) < len(suffix)
}

func patchPreviewSide(left string, leftOmitted bool, changed string, right string, rightOmitted bool) string {
	var out strings.Builder
	if leftOmitted {
		out.WriteString("…\n")
	}
	out.WriteString(left)
	first, _ := patchFirstRunes(changed, patchPreviewSideRunes)
	last, _ := patchLastRunes(changed, patchPreviewSideRunes)
	if len(first)+len(last) < len(changed) {
		out.WriteString(first)
		out.WriteString("\n…\n")
		out.WriteString(last)
	} else {
		out.WriteString(changed)
	}
	out.WriteString(right)
	if rightOmitted {
		out.WriteString("\n…")
	}
	return out.String()
}
