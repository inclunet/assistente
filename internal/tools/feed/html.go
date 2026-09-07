package feed

import (
	"strings"

	"golang.org/x/net/html"
)

// htmlToText extrai texto legível de um fragmento HTML (descrições/conteúdo de
// feeds costumam vir com HTML embutido). Remove script/style e colapsa espaços.
// Em caso de erro de parse, cai para uma remoção simples de tags.
func htmlToText(raw string) string {
	if raw == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return collapseWhitespace(stripTagsSimple(raw))
	}
	var sb strings.Builder
	extractText(doc, &sb)
	return collapseWhitespace(sb.String())
}

func extractText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "script", "style", "noscript", "svg", "head":
			return
		}
		if isBlockElement(n.Data) {
			sb.WriteString("\n")
		}
	}
	if n.Type == html.TextNode {
		if text := strings.TrimSpace(n.Data); text != "" {
			sb.WriteString(text)
			sb.WriteString(" ")
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, sb)
	}
	if n.Type == html.ElementNode && isBlockElement(n.Data) {
		sb.WriteString("\n")
	}
}

func isBlockElement(tag string) bool {
	switch strings.ToLower(tag) {
	case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li",
		"table", "tr", "td", "th", "blockquote", "pre", "hr", "br", "section",
		"article", "aside", "nav", "header", "footer", "main", "figure",
		"figcaption", "details", "summary":
		return true
	}
	return false
}

func stripTagsSimple(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			sb.WriteRune(' ')
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// collapseWhitespace reduz múltiplas quebras de linha e espaços redundantes.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if words := strings.Fields(line); len(words) > 0 {
			cleaned = append(cleaned, strings.Join(words, " "))
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
