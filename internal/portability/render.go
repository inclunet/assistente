package portability

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"codeberg.org/go-pdf/fpdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
)

var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(htmlrenderer.WithHardWraps()),
)

type mediaAttachment struct {
	Name string
	MIME string
	Data string
}

func RenderConversationExport(file *ExportFile, format string) ([]byte, error) {
	switch format {
	case FormatHTML:
		html, err := RenderConversationsHTML(file)
		if err != nil {
			return nil, err
		}
		return []byte(html), nil
	case FormatPDF:
		return RenderConversationsPDF(file)
	default:
		return nil, fmt.Errorf("formato de exportacao nao suportado: %s", format)
	}
}

func RenderConversationsHTML(file *ExportFile) (string, error) {
	const page = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Exportacao de conversas</title>
  <style>
    :root { color-scheme: light; }
    body { margin: 0; font-family: "Segoe UI", Arial, sans-serif; background: #f5f7fb; color: #1f2937; }
    .page { max-width: 1100px; margin: 0 auto; padding: 32px 20px 64px; }
    .hero { background: #ffffff; border: 1px solid #d8dee9; border-radius: 16px; padding: 24px; box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08); }
    .hero h1 { margin: 0 0 8px; font-size: 28px; }
    .hero p { margin: 4px 0; color: #4b5563; }
    .conversation { margin-top: 24px; background: #ffffff; border: 1px solid #d8dee9; border-radius: 16px; overflow: hidden; box-shadow: 0 10px 30px rgba(15, 23, 42, 0.06); }
    .conversation__header { padding: 20px 24px; border-bottom: 1px solid #e5e7eb; background: #f8fafc; }
    .conversation__title { margin: 0 0 8px; font-size: 22px; }
    .conversation__meta { display: flex; flex-wrap: wrap; gap: 12px; font-size: 13px; color: #475569; }
    .conversation__summary { margin-top: 12px; padding: 12px 14px; border-radius: 10px; background: #f8fafc; }
    .messages { padding: 20px; display: grid; gap: 12px; }
    .message { border-radius: 14px; padding: 14px 16px; border: 1px solid #e5e7eb; background: #ffffff; }
    .message--user { border-color: #bfdbfe; background: #eff6ff; }
    .message--assistant { border-color: #d1fae5; background: #ecfdf5; }
    .message--tool { border-color: #fde68a; background: #fffbeb; }
    .message--system { border-color: #e9d5ff; background: #faf5ff; }
    .message__meta { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 8px; font-size: 12px; color: #475569; }
    .message__role { font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; }
    .message__content, .message__reasoning, .message__details { word-break: break-word; line-height: 1.5; }
    .message__reasoning { margin-top: 10px; padding-top: 10px; border-top: 1px dashed #cbd5e1; color: #334155; }
    .message__flags { margin-top: 10px; display: flex; flex-wrap: wrap; gap: 8px; }
    .message__flag { padding: 4px 8px; border-radius: 999px; background: #e2e8f0; color: #334155; font-size: 12px; }
    .message__details { margin-top: 10px; padding: 10px 12px; border-radius: 10px; background: #0f172a; color: #e2e8f0; font-family: ui-monospace, monospace; font-size: 12px; }
    .message__content p, .message__reasoning p, .conversation__summary p { margin: 0 0 10px; }
    .message__content p:last-child, .message__reasoning p:last-child, .conversation__summary p:last-child { margin-bottom: 0; }
    .message__content pre, .message__reasoning pre, .conversation__summary pre { margin: 10px 0 0; overflow-x: auto; padding: 12px; border-radius: 10px; background: #0f172a; color: #e2e8f0; }
    .message__content code, .message__reasoning code, .conversation__summary code { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; }
    .message__content blockquote, .message__reasoning blockquote, .conversation__summary blockquote { margin: 10px 0 0; padding-left: 12px; border-left: 4px solid #94a3b8; color: #475569; }
    .message__content ul, .message__content ol, .message__reasoning ul, .message__reasoning ol, .conversation__summary ul, .conversation__summary ol { margin: 10px 0 0 20px; }
    .message__content table, .message__reasoning table, .conversation__summary table { margin-top: 10px; width: 100%; border-collapse: collapse; }
    .message__content th, .message__content td, .message__reasoning th, .message__reasoning td, .conversation__summary th, .conversation__summary td { border: 1px solid #cbd5e1; padding: 6px 8px; text-align: left; }
    .message__content img, .message__reasoning img, .conversation__summary img { max-width: 100%; border-radius: 12px; display: block; margin-top: 10px; }
    .message__media { margin-top: 12px; display: grid; gap: 10px; }
    .message__media-card { padding: 12px; border-radius: 12px; background: rgba(255,255,255,0.72); border: 1px solid #cbd5e1; }
    .message__media-card strong { display: block; margin-bottom: 6px; }
    .message__media-card img, .message__media-card video, .message__media-card audio { width: 100%; max-width: 100%; border-radius: 10px; }
    .message__media-card pre { margin: 8px 0 0; white-space: pre-wrap; max-height: 240px; overflow: auto; }
    .message__media-card a { color: #1d4ed8; }
  </style>
</head>
<body>
  <div class="page">
    <section class="hero">
      <h1>Exportacao de conversas</h1>
      <p>Gerado em {{ formatTime .ExportedAt }}</p>
      <p>{{ len .Resources.Conversations }} conversa(s) exportada(s)</p>
      <p>Formato canônico: JSON version {{ .Version }}</p>
    </section>
    {{ range .Resources.Conversations }}
      <article class="conversation">
        <header class="conversation__header">
          <h2 class="conversation__title">{{ conversationTitle .Title }}</h2>
          <div class="conversation__meta">
            <span>Criada em {{ formatTime .CreatedAt }}</span>
            {{ if .Channel }}<span>Canal: {{ .Channel }}</span>{{ end }}
            {{ if .ContactID }}<span>Contato: {{ .ContactID }}</span>{{ end }}
            <span>{{ len .Messages }} mensagem(ns)</span>
          </div>
          {{ if .Summary }}
            <div class="conversation__summary">{{ renderMarkdown .Summary }}</div>
          {{ end }}
        </header>
        <div class="messages">
          {{ range .Messages }}
            <section class="message {{ messageClass .Role }}">
              <div class="message__meta">
                <span class="message__role">{{ roleLabel .Role }}</span>
                <span>{{ formatTime .CreatedAt }}</span>
                {{ if .Model }}<span>Modelo: {{ .Model }}</span>{{ end }}
                {{ if .Source }}<span>Origem: {{ .Source }}</span>{{ end }}
                {{ if .PromptTokens }}<span>Prompt: {{ .PromptTokens }}</span>{{ end }}
                {{ if .CompletionTokens }}<span>Resposta: {{ .CompletionTokens }}</span>{{ end }}
              </div>
              <div class="message__content">{{ renderMarkdown .Content }}</div>
              {{ if .Reasoning }}
                <div class="message__reasoning">{{ renderMarkdown .Reasoning }}</div>
              {{ end }}
              {{ if or .Media .AudioMimeType .ToolCalls .ToolCallID }}
                <div class="message__flags">
                  {{ if .Media }}<span class="message__flag">midia anexada</span>{{ end }}
                  {{ if .AudioMimeType }}<span class="message__flag">audio: {{ .AudioMimeType }}</span>{{ end }}
                  {{ if .ToolCallID }}<span class="message__flag">toolCallId: {{ .ToolCallID }}</span>{{ end }}
                </div>
              {{ end }}
              {{ if hasRichMedia . }}
                <div class="message__media">{{ renderMessageMedia . }}</div>
              {{ end }}
              {{ if .ToolCalls }}
                <div class="message__details">{{ renderPreformatted .ToolCalls }}</div>
              {{ end }}
            </section>
          {{ end }}
        </div>
      </article>
    {{ end }}
  </div>
</body>
</html>`

	tmpl, err := template.New("conversation-export").Funcs(template.FuncMap{
		"formatTime":         formatConversationTime,
		"roleLabel":          roleLabel,
		"messageClass":       messageClass,
		"conversationTitle":  conversationTitle,
		"renderMarkdown":     renderMarkdownTemplate,
		"renderPreformatted": renderPreformattedTemplate,
		"renderMessageMedia": renderMessageMediaTemplate,
		"hasRichMedia":       hasRichMedia,
	}).Parse(page)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func RenderConversationsPDF(file *ExportFile) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 12)
	pdf.SetTitle("Exportacao de conversas", false)
	pdf.SetAuthor("Assistente", false)

	useUTF8 := configurePDFFont(pdf)

	pdf.AddPage()
	writePDFTitle(pdf, useUTF8, "Exportacao de conversas")
	writePDFMeta(pdf, useUTF8, fmt.Sprintf("Gerado em %s", formatConversationTime(file.ExportedAt)))
	writePDFMeta(pdf, useUTF8, fmt.Sprintf("%d conversa(s)", len(file.Resources.Conversations)))
	pdf.Ln(4)

	for idx, conv := range file.Resources.Conversations {
		if idx > 0 {
			pdf.AddPage()
		}

		writePDFSectionTitle(pdf, useUTF8, conversationTitle(conv.Title))
		writePDFMeta(pdf, useUTF8, fmt.Sprintf("Criada em %s", formatConversationTime(conv.CreatedAt)))
		if conv.Channel != "" {
			writePDFMeta(pdf, useUTF8, fmt.Sprintf("Canal: %s", conv.Channel))
		}
		if conv.ContactID != "" {
			writePDFMeta(pdf, useUTF8, fmt.Sprintf("Contato: %s", conv.ContactID))
		}
		writePDFMeta(pdf, useUTF8, fmt.Sprintf("Mensagens: %d", len(conv.Messages)))
		pdf.Ln(2)

		if strings.TrimSpace(conv.Summary) != "" {
			writePDFBlock(pdf, useUTF8, "Resumo", markdownToPDFText(conv.Summary))
		}

		for _, msg := range conv.Messages {
			header := roleLabel(msg.Role)
			metaParts := []string{formatConversationTime(msg.CreatedAt)}
			if msg.Model != "" {
				metaParts = append(metaParts, "modelo: "+msg.Model)
			}
			if msg.Source != "" {
				metaParts = append(metaParts, "origem: "+msg.Source)
			}
			writePDFBlock(pdf, useUTF8, header+" - "+strings.Join(metaParts, " | "), markdownToPDFText(msg.Content))
			if strings.TrimSpace(msg.Reasoning) != "" {
				writePDFIndentedBlock(pdf, useUTF8, "Reasoning", markdownToPDFText(msg.Reasoning))
			}
			if strings.TrimSpace(msg.ToolCalls) != "" {
				writePDFIndentedBlock(pdf, useUTF8, "Tool calls", msg.ToolCalls)
			}
			if err := writePDFMediaAttachments(pdf, useUTF8, msg); err != nil {
				writePDFMeta(pdf, useUTF8, "Falha ao renderizar uma ou mais midias anexadas: "+err.Error())
			}
			if msg.AudioMimeType != "" || msg.ToolCallID != "" {
				flags := make([]string, 0, 3)
				if msg.AudioMimeType != "" {
					flags = append(flags, "audio: "+msg.AudioMimeType)
				}
				if msg.ToolCallID != "" {
					flags = append(flags, "toolCallId: "+msg.ToolCallID)
				}
				writePDFMeta(pdf, useUTF8, strings.Join(flags, " | "))
			}
			pdf.Ln(2)
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func configurePDFFont(pdf *fpdf.Fpdf) bool {
	if fontBytes, err := loadSystemPDFFont(); err == nil && len(fontBytes) > 0 {
		copied := append([]byte(nil), fontBytes...)
		pdf.AddUTF8FontFromBytes("assistente", "", copied)
		pdf.SetFont("assistente", "", 11)
		return true
	}
	pdf.SetFont("Helvetica", "", 11)
	return false
}

func loadSystemPDFFont() ([]byte, error) {
	candidates := []string{}
	switch runtime.GOOS {
	case "windows":
		winDir := os.Getenv("WINDIR")
		if winDir == "" {
			winDir = `C:\Windows`
		}
		candidates = append(candidates,
			filepath.Join(winDir, "Fonts", "segoeui.ttf"),
			filepath.Join(winDir, "Fonts", "arial.ttf"),
			filepath.Join(winDir, "Fonts", "calibri.ttf"),
		)
	case "darwin":
		candidates = append(candidates,
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/Library/Fonts/Arial Unicode.ttf",
		)
	default:
		candidates = append(candidates,
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
		)
	}

	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, fmt.Errorf("nenhuma fonte TTF encontrada")
}

func writePDFTitle(pdf *fpdf.Fpdf, useUTF8 bool, text string) {
	pdf.SetFontSize(18)
	pdf.SetTextColor(17, 24, 39)
	pdf.CellFormat(0, 10, pdfText(text, useUTF8), "", 1, "L", false, 0, "")
	pdf.SetFontSize(11)
}

func writePDFSectionTitle(pdf *fpdf.Fpdf, useUTF8 bool, text string) {
	pdf.SetFontSize(15)
	pdf.SetTextColor(17, 24, 39)
	pdf.CellFormat(0, 8, pdfText(text, useUTF8), "", 1, "L", false, 0, "")
	pdf.SetFontSize(11)
}

func writePDFMeta(pdf *fpdf.Fpdf, useUTF8 bool, text string) {
	pdf.SetTextColor(71, 85, 105)
	pdf.MultiCell(0, 5, pdfText(text, useUTF8), "", "L", false)
	pdf.SetTextColor(17, 24, 39)
}

func writePDFBlock(pdf *fpdf.Fpdf, useUTF8 bool, title string, content string) {
	pdf.SetFillColor(248, 250, 252)
	pdf.SetDrawColor(203, 213, 225)
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFontSize(11)
	pdf.MultiCell(0, 7, pdfText(title, useUTF8), "1", "L", true)
	pdf.MultiCell(0, 6, pdfText(content, useUTF8), "1", "L", false)
}

func writePDFIndentedBlock(pdf *fpdf.Fpdf, useUTF8 bool, title string, content string) {
	left, _, right, _ := pdf.GetMargins()
	pdf.SetLeftMargin(left + 6)
	pdf.SetRightMargin(right + 2)
	writePDFBlock(pdf, useUTF8, title, content)
	pdf.SetLeftMargin(left)
	pdf.SetRightMargin(right)
}

func pdfText(text string, useUTF8 bool) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if useUTF8 {
		return text
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || (r >= 32 && r <= 255) {
			return r
		}
		return '?'
	}, text)
}

func roleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "Usuario"
	case "assistant":
		return "Assistente"
	case "tool":
		return "Ferramenta"
	case "system":
		return "Sistema"
	default:
		if role == "" {
			return "Mensagem"
		}
		return strings.ToUpper(role[:1]) + role[1:]
	}
}

func messageClass(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "message--user"
	case "assistant":
		return "message--assistant"
	case "tool":
		return "message--tool"
	case "system":
		return "message--system"
	default:
		return ""
	}
}

func conversationTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Conversa sem titulo"
	}
	return title
}

func formatConversationTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func renderMarkdownTemplate(content string) template.HTML {
	return template.HTML(markdownToHTML(content))
}

func renderPreformattedTemplate(content string) template.HTML {
	if strings.TrimSpace(content) == "" {
		return template.HTML("")
	}
	return template.HTML("<pre>" + template.HTMLEscapeString(content) + "</pre>")
}

func renderMessageMediaTemplate(msg MessageExport) template.HTML {
	var parts []string
	for _, media := range parseMediaAttachments(msg.Media) {
		parts = append(parts, renderAttachmentHTML(media))
	}
	if strings.TrimSpace(msg.Audio) != "" && strings.TrimSpace(msg.AudioMimeType) != "" {
		parts = append(parts, renderAttachmentHTML(mediaAttachment{
			Name: "Audio da mensagem",
			MIME: msg.AudioMimeType,
			Data: msg.Audio,
		}))
	}
	return template.HTML(strings.Join(parts, ""))
}

func hasRichMedia(msg MessageExport) bool {
	return len(parseMediaAttachments(msg.Media)) > 0 || (strings.TrimSpace(msg.Audio) != "" && strings.TrimSpace(msg.AudioMimeType) != "")
}

func renderAttachmentHTML(media mediaAttachment) string {
	name := template.HTMLEscapeString(fallbackAttachmentName(media))
	dataURI := buildDataURI(media.MIME, media.Data)
	switch mediaKind(media.MIME) {
	case "image":
		return `<div class="message__media-card"><strong>` + name + `</strong><img src="` + dataURI + `" alt="` + name + `"></div>`
	case "audio":
		return `<div class="message__media-card"><strong>` + name + `</strong><audio controls preload="metadata" src="` + dataURI + `"></audio></div>`
	case "video":
		return `<div class="message__media-card"><strong>` + name + `</strong><video controls preload="metadata" src="` + dataURI + `"></video></div>`
	default:
		preview := attachmentTextPreview(media)
		body := `<a download="` + name + `" href="` + dataURI + `">Baixar anexo</a>`
		if preview != "" {
			if strings.Contains(media.MIME, "markdown") {
				body += string(renderMarkdownTemplate(preview))
			} else {
				body += "<pre>" + template.HTMLEscapeString(preview) + "</pre>"
			}
		}
		return `<div class="message__media-card"><strong>` + name + `</strong><div>` + template.HTMLEscapeString(media.MIME) + `</div>` + body + `</div>`
	}
}

func parseMediaAttachments(mediaJSON string) []mediaAttachment {
	if strings.TrimSpace(mediaJSON) == "" {
		return nil
	}

	var raw []map[string]any
	if err := json.Unmarshal([]byte(mediaJSON), &raw); err != nil {
		return nil
	}

	result := make([]mediaAttachment, 0, len(raw))
	for _, item := range raw {
		mime, _ := item["type"].(string)
		data, _ := item["data"].(string)
		name, _ := item["name"].(string)
		if strings.TrimSpace(mime) == "" || strings.TrimSpace(data) == "" {
			continue
		}
		result = append(result, mediaAttachment{
			Name: name,
			MIME: mime,
			Data: data,
		})
	}
	return result
}

func buildDataURI(mimeType, data string) string {
	return "data:" + mimeType + ";base64," + data
}

func fallbackAttachmentName(media mediaAttachment) string {
	if strings.TrimSpace(media.Name) != "" {
		return media.Name
	}
	switch mediaKind(media.MIME) {
	case "image":
		return "Imagem anexada"
	case "audio":
		return "Audio anexado"
	case "video":
		return "Video anexado"
	default:
		return "Arquivo anexado"
	}
}

func mediaKind(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	default:
		return "file"
	}
}

func attachmentTextPreview(media mediaAttachment) string {
	if !isTextLikeMedia(media.MIME) {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(media.Data)
	if err != nil {
		return ""
	}
	text := string(decoded)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if len(text) > 4000 {
		text = text[:4000] + "\n\n...[conteudo truncado]"
	}
	return text
}

func isTextLikeMedia(mimeType string) bool {
	return strings.HasPrefix(mimeType, "text/") ||
		strings.Contains(mimeType, "json") ||
		strings.Contains(mimeType, "xml") ||
		strings.Contains(mimeType, "yaml") ||
		strings.Contains(mimeType, "csv")
}

func markdownToHTML(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := markdownRenderer.Convert([]byte(content), &buf); err != nil {
		return "<p>" + template.HTMLEscapeString(content) + "</p>"
	}
	return buf.String()
}

func markdownToPDFText(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return htmlFragmentToText(markdownToHTML(content))
}

func htmlFragmentToText(fragment string) string {
	doc, err := html.Parse(strings.NewReader("<div>" + fragment + "</div>"))
	if err != nil {
		return fragment
	}
	var root *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if root != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "div" {
			root = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if root == nil {
		return fragment
	}

	var sb strings.Builder
	renderHTMLNodeToText(root, &sb)
	text := strings.ReplaceAll(sb.String(), "\u00a0", " ")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

func renderHTMLNodeToText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
		return
	}
	if n.Type != html.ElementNode {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderHTMLNodeToText(c, sb)
		}
		return
	}

	switch n.Data {
	case "br":
		sb.WriteString("\n")
		return
	case "p", "div", "section", "article", "blockquote", "pre":
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderHTMLNodeToText(c, sb)
		}
		sb.WriteString("\n\n")
		return
	case "li":
		sb.WriteString("- ")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderHTMLNodeToText(c, sb)
		}
		sb.WriteString("\n")
		return
	case "tr":
		first := true
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
				if !first {
					sb.WriteString(" | ")
				}
				renderHTMLNodeToText(c, sb)
				first = false
			}
		}
		sb.WriteString("\n")
		return
	case "img":
		alt := htmlAttr(n, "alt")
		if alt == "" {
			alt = "imagem"
		}
		sb.WriteString("[")
		sb.WriteString(alt)
		sb.WriteString("]")
		return
	}

	if strings.HasPrefix(n.Data, "h") && len(n.Data) == 2 && n.Data[1] >= '1' && n.Data[1] <= '6' {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderHTMLNodeToText(c, sb)
		}
		sb.WriteString("\n\n")
		return
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderHTMLNodeToText(c, sb)
	}
}

func htmlAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func writePDFMediaAttachments(pdf *fpdf.Fpdf, useUTF8 bool, msg MessageExport) error {
	var errs []string
	for idx, media := range parseMediaAttachments(msg.Media) {
		if err := writePDFAttachment(pdf, useUTF8, media, fmt.Sprintf("media-%d", idx)); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if strings.TrimSpace(msg.Audio) != "" && strings.TrimSpace(msg.AudioMimeType) != "" {
		if err := writePDFAttachment(pdf, useUTF8, mediaAttachment{
			Name: "Audio da mensagem",
			MIME: msg.AudioMimeType,
			Data: msg.Audio,
		}, "audio-message"); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func writePDFAttachment(pdf *fpdf.Fpdf, useUTF8 bool, media mediaAttachment, key string) error {
	switch mediaKind(media.MIME) {
	case "image":
		if err := writePDFImageAttachment(pdf, media, key); err != nil {
			writePDFMeta(pdf, useUTF8, fallbackAttachmentName(media)+": "+err.Error())
			return err
		}
	default:
		lines := []string{fallbackAttachmentName(media), media.MIME}
		if preview := attachmentTextPreview(media); preview != "" {
			if strings.Contains(media.MIME, "markdown") {
				preview = markdownToPDFText(preview)
			}
			lines = append(lines, preview)
		}
		writePDFIndentedBlock(pdf, useUTF8, "Anexo", strings.Join(lines, "\n"))
	}
	return nil
}

func writePDFImageAttachment(pdf *fpdf.Fpdf, media mediaAttachment, key string) error {
	decoded, err := base64.StdEncoding.DecodeString(media.Data)
	if err != nil {
		return err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		return err
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return fmt.Errorf("formato de imagem nao suportado no PDF: %s", format)
	}

	info := pdf.RegisterImageOptionsReader(key+"."+format, fpdf.ImageOptions{ImageType: format}, bytes.NewReader(decoded))
	if info == nil {
		return fmt.Errorf("nao foi possivel registrar imagem no PDF")
	}

	writePDFMeta(pdf, true, fallbackAttachmentName(media))
	pageW, pageH := pdf.GetPageSize()
	left, _, right, bottom := pdf.GetMargins()
	maxW := pageW - left - right
	if maxW > 120 {
		maxW = 120
	}
	displayW := maxW
	displayH := displayW * float64(cfg.Height) / float64(cfg.Width)
	if pdf.GetY()+displayH+bottom > pageH {
		pdf.AddPage()
	}
	x := pdf.GetX()
	y := pdf.GetY()
	pdf.ImageOptions(key+"."+format, x, y, displayW, displayH, false, fpdf.ImageOptions{ImageType: format}, 0, "")
	pdf.SetY(y + displayH + 2)
	return nil
}
