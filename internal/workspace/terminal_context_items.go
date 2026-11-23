package workspace

import (
	"sort"
	"strings"
	"unicode/utf8"
)

const terminalItemSeparator = "\n---\n"
const terminalContextItemsPerKind = 40

type terminalContextItem struct {
	kind, header, output string
}

func (e terminalContextItem) text() string {
	if e.output == "" {
		return e.header
	}
	return e.header + "\n" + e.output
}

// Os itens descrevem exatamente o texto mostrado na prévia. Separadores,
// comandos aparentes e quebras de linha dentro de uma saída são dados literais.
func terminalContextItems(content map[string]any, limit int) []terminalContextItem {
	raw, ok := content["recentOutputItems"].([]any)
	if !ok || len(raw) == 0 || len(raw) > 2*terminalContextItemsPerKind {
		return nil
	}
	items := make([]terminalContextItem, 0, len(raw))
	var body strings.Builder
	counts := map[string]int{}
	for i, value := range raw {
		fields, _ := value.(map[string]any)
		kind, _ := fields["kind"].(string)
		header, hasHeader := fields["header"].(string)
		output, hasOutput := fields["output"].(string)
		counts[kind]++
		if (kind != "command" && kind != "session") || counts[kind] > terminalContextItemsPerKind ||
			!hasHeader || header == "" || !hasOutput ||
			len(header)+len(output)+body.Len() > 4*limit {
			return nil
		}
		item := terminalContextItem{kind, header, output}
		items = append(items, item)
		if i > 0 {
			body.WriteString(terminalItemSeparator)
		}
		body.WriteString(item.text())
	}
	preview, _ := content["recentOutput"].(string)
	truncated, _ := content["truncated"].(bool)
	if len(preview) > 4*limit || runeLen(preview) > limit ||
		(preview != body.String() && (!truncated || !strings.HasSuffix(preview, "\n"+body.String()))) {
		return nil
	}
	return items
}

func fitTerminalContextItems(items []terminalContextItem, remaining int) string {
	parts := make([]string, len(items))
	var count int
	for _, kind := range []string{"command", "session"} {
		for i := len(items) - 1; i >= 0 && remaining > 0; i-- {
			item := items[i]
			if item.kind != kind {
				continue
			}
			available := remaining
			if count > 0 {
				available -= len(terminalItemSeparator)
			}
			excerpt := fitTerminalContextItem(item, available, count == 0)
			if excerpt == "" {
				// O orçamento não cobre a identidade e o fim deste item. Não
				// preencha seu lugar com comandos antigos ou saída da sessão.
				remaining = 0
				break
			}
			parts[i] = excerpt
			remaining = available - runeLen(escapeXMLText(excerpt))
			count++
		}
	}
	ordered := make([]string, 0, count)
	for _, part := range parts {
		if part != "" {
			ordered = append(ordered, part)
		}
	}
	return strings.Join(ordered, terminalItemSeparator)
}

func fitTerminalContextItem(item terminalContextItem, budget int, first bool) string {
	header := item.header
	headerSize := runeLen(escapeXMLText(header))
	outputSize, minOutput := 0, 0
	if item.output != "" {
		outputSize = 1 + runeLen(escapeXMLText(item.output)) // quebra de linha e saída inteira
		last, size := utf8.DecodeLastRuneInString(item.output)
		minOutput = 1 + runeLen(escapeXMLText(string(last)))
		if size < len(item.output) {
			minOutput++ // reticências antes da cauda
		}
	}
	if headerSize+outputSize <= budget {
		return item.text()
	}
	if headerSize+minOutput > budget {
		if !first || item.kind != "command" {
			return ""
		}
		headerBudget := budget - min(outputSize, max(minOutput, budget/2))
		header = fitXMLRunes(item.header, headerBudget-1, false)
		if len(strings.TrimSpace(header)) <= 1 && headerBudget < budget-minOutput {
			// Um nome iniciado por caractere escapado pode custar mais que a
			// sua metade. Use o restante preservando o mínimo da saída.
			header = fitXMLRunes(item.header, budget-minOutput-1, false)
		}
		if len(strings.TrimSpace(header)) <= 1 {
			return ""
		}
		header += "…"
		headerSize = runeLen(escapeXMLText(header))
	}
	item.header = header
	if headerSize+outputSize <= budget {
		return item.text()
	}
	suffix := fitXMLRunes(item.output, budget-headerSize-2, true)
	if suffix == "" {
		return ""
	}
	return header + "\n…" + suffix
}

// Cada ponto de código adicional custa pelo menos um caractere no XML.
func fitXMLRunes(text string, budget int, tail bool) string {
	runes := []rune(text)
	portion := func(count int) string {
		if tail {
			return string(runes[len(runes)-count:])
		}
		return string(runes[:count])
	}
	fit := sort.Search(len(runes)+1, func(n int) bool {
		return runeLen(escapeXMLText(portion(n))) > budget
	})
	return portion(max(0, fit-1))
}
