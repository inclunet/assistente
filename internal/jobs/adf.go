package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// adfToMarkdown converte um documento ADF (Atlassian Document Format) para Markdown.
// Aceita any (map[string]any, string JSON, nil). Retorna string vazia para entradas vazias.
// Nunca faz panic.
// Uso: {{ adf_markdown .event.fields.description }}
func tplADFMarkdown(v any) (string, error) {
	doc, err := normalizeADFInput(v)
	if err != nil {
		return "", err
	}
	if doc == nil {
		return "", nil
	}

	ctx := &adfCtx{}
	adfRenderNode(ctx, doc, 0, false)
	return strings.TrimSpace(ctx.buf.String()), nil
}

// tplADFText extrai apenas o texto plano de um documento ADF (sem formatação Markdown).
// Uso: {{ adf_text .event.fields.description }}
func tplADFText(v any) (string, error) {
	doc, err := normalizeADFInput(v)
	if err != nil {
		return "", err
	}
	if doc == nil {
		return "", nil
	}

	ctx := &adfCtx{plainText: true}
	adfRenderNode(ctx, doc, 0, false)
	return strings.TrimSpace(ctx.buf.String()), nil
}

func normalizeADFInput(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	switch val := v.(type) {
	case map[string]any:
		return val, nil
	case string:
		if strings.TrimSpace(val) == "" {
			return nil, nil
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(val), &doc); err != nil {
			return nil, fmt.Errorf("adf_markdown: cannot parse JSON string: %w", err)
		}
		return doc, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("adf_markdown: unsupported input type %T", v)
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("adf_markdown: cannot re-parse value of type %T", v)
		}
		return doc, nil
	}
}

type adfCtx struct {
	buf       strings.Builder
	plainText bool
}

func (c *adfCtx) write(s string) { c.buf.WriteString(s) }

func adfRenderNode(ctx *adfCtx, node map[string]any, depth int, inList bool) {
	nodeType, _ := node["type"].(string)
	content := adfContent(node)

	switch nodeType {
	case "doc":
		adfRenderBlocks(ctx, content, depth, false)

	case "paragraph":
		adfRenderInlines(ctx, content)
		if !inList {
			ctx.write("\n\n")
		} else {
			ctx.write("\n")
		}

	case "heading":
		level := adfInt(node, "level", 1)
		if !ctx.plainText {
			ctx.write(strings.Repeat("#", level) + " ")
		}
		adfRenderInlines(ctx, content)
		ctx.write("\n\n")

	case "bulletList":
		adfRenderListItems(ctx, content, depth, false)

	case "orderedList":
		adfRenderListItems(ctx, content, depth, true)

	case "listItem":
		adfRenderBlocks(ctx, content, depth, true)

	case "codeBlock":
		lang, _ := node["attrs"].(map[string]any)
		langStr := ""
		if lang != nil {
			langStr, _ = lang["language"].(string)
		}
		if ctx.plainText {
			adfRenderInlines(ctx, content)
			ctx.write("\n\n")
		} else {
			ctx.write("```" + langStr + "\n")
			adfRenderInlines(ctx, content)
			ctx.write("\n```\n\n")
		}

	case "blockquote":
		var inner adfCtx
		inner.plainText = ctx.plainText
		adfRenderBlocks(&inner, content, depth, false)
		lines := strings.Split(strings.TrimRight(inner.buf.String(), "\n"), "\n")
		for _, line := range lines {
			if ctx.plainText {
				ctx.write(line + "\n")
			} else {
				ctx.write("> " + line + "\n")
			}
		}
		ctx.write("\n")

	case "rule":
		if ctx.plainText {
			ctx.write("---\n\n")
		} else {
			ctx.write("---\n\n")
		}

	case "panel":
		panelType := ""
		if attrs, ok := node["attrs"].(map[string]any); ok {
			panelType, _ = attrs["panelType"].(string)
		}
		if !ctx.plainText && panelType != "" {
			ctx.write("> **" + strings.ToUpper(panelType[:1]) + panelType[1:] + "**\n> \n")
			var inner adfCtx
			adfRenderBlocks(&inner, content, depth, false)
			lines := strings.Split(strings.TrimRight(inner.buf.String(), "\n"), "\n")
			for _, line := range lines {
				ctx.write("> " + line + "\n")
			}
			ctx.write("\n")
		} else {
			adfRenderBlocks(ctx, content, depth, inList)
		}

	case "table":
		adfRenderTable(ctx, content)

	case "tableRow", "tableCell", "tableHeader":
		adfRenderBlocks(ctx, content, depth, inList)

	case "mediaSingle", "mediaGroup":
		adfRenderBlocks(ctx, content, depth, inList)

	case "media":
		alt := ""
		url := ""
		if attrs, ok := node["attrs"].(map[string]any); ok {
			alt, _ = attrs["alt"].(string)
			url, _ = attrs["url"].(string)
			if url == "" {
				id, _ := attrs["id"].(string)
				if id != "" {
					url = id
				}
			}
		}
		if url != "" && !ctx.plainText {
			ctx.write(fmt.Sprintf("![%s](%s)", alt, url))
		} else if alt != "" {
			ctx.write(alt)
		}

	case "expand", "nestedExpand":
		title := ""
		if attrs, ok := node["attrs"].(map[string]any); ok {
			title, _ = attrs["title"].(string)
		}
		if title != "" {
			if ctx.plainText {
				ctx.write(title + "\n")
			} else {
				ctx.write("**" + title + "**\n\n")
			}
		}
		adfRenderBlocks(ctx, content, depth, inList)

	default:
		if len(content) > 0 {
			adfRenderBlocks(ctx, content, depth, inList)
		}
	}
}

func adfRenderBlocks(ctx *adfCtx, content []map[string]any, depth int, inList bool) {
	for _, child := range content {
		adfRenderNode(ctx, child, depth, inList)
	}
}

func adfRenderListItems(ctx *adfCtx, items []map[string]any, depth int, ordered bool) {
	for i, item := range items {
		indent := strings.Repeat("  ", depth)
		bullet := "- "
		if ordered {
			bullet = fmt.Sprintf("%d. ", i+1)
		}
		ctx.write(indent + bullet)

		var inner adfCtx
		inner.plainText = ctx.plainText
		adfRenderNode(&inner, item, depth+1, true)
		text := strings.TrimRight(inner.buf.String(), "\n")

		lines := strings.Split(text, "\n")
		for j, line := range lines {
			if j == 0 {
				ctx.write(line + "\n")
			} else {
				ctx.write(indent + strings.Repeat(" ", len(bullet)) + line + "\n")
			}
		}
	}
	if depth == 0 {
		ctx.write("\n")
	}
}

func adfRenderTable(ctx *adfCtx, rows []map[string]any) {
	if len(rows) == 0 {
		return
	}

	var tableData [][]string
	hasHeader := false

	for _, row := range rows {
		cells := adfContent(row)
		var rowData []string
		for _, cell := range cells {
			cellType, _ := cell["type"].(string)
			if cellType == "tableHeader" {
				hasHeader = true
			}
			var inner adfCtx
			inner.plainText = ctx.plainText
			adfRenderBlocks(&inner, adfContent(cell), 0, false)
			cellText := strings.TrimSpace(inner.buf.String())
			cellText = strings.ReplaceAll(cellText, "\n", " ")
			rowData = append(rowData, cellText)
		}
		tableData = append(tableData, rowData)
	}

	if ctx.plainText || len(tableData) == 0 {
		for _, row := range tableData {
			ctx.write(strings.Join(row, " | ") + "\n")
		}
		ctx.write("\n")
		return
	}

	// Markdown table
	for i, row := range tableData {
		ctx.write("| " + strings.Join(row, " | ") + " |\n")
		if i == 0 && hasHeader {
			seps := make([]string, len(row))
			for j := range seps {
				seps[j] = "---"
			}
			ctx.write("| " + strings.Join(seps, " | ") + " |\n")
		}
	}
	ctx.write("\n")
}

func adfRenderInlines(ctx *adfCtx, content []map[string]any) {
	for _, node := range content {
		nodeType, _ := node["type"].(string)

		switch nodeType {
		case "text":
			text, _ := node["text"].(string)
			if ctx.plainText {
				ctx.write(text)
			} else {
				ctx.write(adfApplyMarks(text, node))
			}

		case "hardBreak":
			ctx.write("\n")

		case "inlineCard":
			url := ""
			if attrs, ok := node["attrs"].(map[string]any); ok {
				url, _ = attrs["url"].(string)
			}
			if url != "" {
				if ctx.plainText {
					ctx.write(url)
				} else {
					ctx.write(url)
				}
			}

		case "emoji":
			if attrs, ok := node["attrs"].(map[string]any); ok {
				text, _ := attrs["text"].(string)
				if text == "" {
					shortName, _ := attrs["shortName"].(string)
					text = shortName
				}
				ctx.write(text)
			}

		case "mention":
			if attrs, ok := node["attrs"].(map[string]any); ok {
				text, _ := attrs["text"].(string)
				if text == "" {
					text, _ = attrs["displayName"].(string)
				}
				if text != "" {
					ctx.write(text)
				} else {
					id, _ := attrs["id"].(string)
					ctx.write("@" + id)
				}
			}

		case "date":
			if attrs, ok := node["attrs"].(map[string]any); ok {
				ts, _ := attrs["timestamp"].(string)
				if ts != "" {
					ctx.write(ts)
				}
			}

		case "status":
			if attrs, ok := node["attrs"].(map[string]any); ok {
				text, _ := attrs["text"].(string)
				if text != "" {
					ctx.write("[" + text + "]")
				}
			}

		default:
			// Fallback: try to extract text from content
			inner := adfContent(node)
			if len(inner) > 0 {
				adfRenderInlines(ctx, inner)
			}
		}
	}
}

func adfApplyMarks(text string, node map[string]any) string {
	marks, ok := node["marks"]
	if !ok {
		return text
	}

	marksSlice, ok := marks.([]any)
	if !ok || len(marksSlice) == 0 {
		return text
	}

	result := text
	for _, m := range marksSlice {
		mark, ok := m.(map[string]any)
		if !ok {
			continue
		}
		markType, _ := mark["type"].(string)

		switch markType {
		case "strong":
			result = "**" + result + "**"
		case "em":
			result = "*" + result + "*"
		case "code":
			result = "`" + result + "`"
		case "strike":
			result = "~~" + result + "~~"
		case "link":
			href := ""
			if attrs, ok := mark["attrs"].(map[string]any); ok {
				href, _ = attrs["href"].(string)
			}
			if href != "" {
				result = "[" + result + "](" + href + ")"
			}
		case "underline":
			result = "<u>" + result + "</u>"
		case "subsup":
			if attrs, ok := mark["attrs"].(map[string]any); ok {
				subType, _ := attrs["type"].(string)
				if subType == "sub" {
					result = "<sub>" + result + "</sub>"
				} else {
					result = "<sup>" + result + "</sup>"
				}
			}
		}
	}

	return result
}

// --- Helpers ---

func adfContent(node map[string]any) []map[string]any {
	raw, ok := node["content"]
	if !ok {
		return nil
	}
	slice, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(slice))
	for _, item := range slice {
		m, ok := item.(map[string]any)
		if ok {
			result = append(result, m)
		}
	}
	return result
}

func adfInt(node map[string]any, path string, fallback int) int {
	raw, ok := node["attrs"]
	if !ok {
		return fallback
	}
	attrs, ok := raw.(map[string]any)
	if !ok {
		return fallback
	}
	val, ok := attrs[path]
	if !ok {
		return fallback
	}
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return fallback
	}
}
