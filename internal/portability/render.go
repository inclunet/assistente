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
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"golang.org/x/net/html"
)

// highlightStyle é o tema Chroma usado para o syntax highlighting do HTML
// exportado. É um estilo claro coerente com o color-scheme do documento e que
// gera apenas cores hexadecimais (sem rgba), mantendo o artefato autocontido.
const highlightStyle = "github"

// markdownRenderer converte markdown puro para HTML sem syntax highlighting.
// É reutilizado pela extração de texto do PDF, onde o highlighting só geraria
// ruído.
var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(htmlrenderer.WithHardWraps()),
)

// markdownHTMLRenderer adiciona syntax highlighting via Chroma (classes CSS) aos
// blocos de código, para a exportação HTML rica.
var markdownHTMLRenderer = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithStyle(highlightStyle),
			highlighting.WithFormatOptions(
				chromahtml.WithClasses(true),
			),
		),
	),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(htmlrenderer.WithHardWraps()),
)

// chromaHighlightCSS gera a folha de estilo das classes Chroma usadas pelos
// blocos de código destacados no HTML exportado.
func chromaHighlightCSS() string {
	style := styles.Get(highlightStyle)
	if style == nil {
		style = styles.Fallback
	}
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	var buf bytes.Buffer
	if err := formatter.WriteCSS(&buf, style); err != nil {
		return ""
	}
	return buf.String()
}

type mediaAttachment struct {
	Name string
	MIME string
	Data string
}

const maxAttachmentDecodedBytes = 8 << 20

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
	case FormatMarkdown:
		md, err := RenderConversationsMarkdown(file)
		if err != nil {
			return nil, err
		}
		return []byte(md), nil
	default:
		return nil, fmt.Errorf("formato de exportação não suportado: %s", format)
	}
}

func RenderConversationsHTML(file *ExportFile) (string, error) {
	const page = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Exportação de conversas</title>
  <style>
    :root {
      color-scheme: light;
      --export-bg-base: var(--bg-base, Canvas);
      --export-bg-surface: var(--bg-surface, Canvas);
      --export-bg-elevated: var(--bg-elevated, transparent);
      --export-bg-hover: var(--bg-hover, transparent);
      --export-text-primary: var(--text-primary, CanvasText);
      --export-text-secondary: var(--text-secondary, currentColor);
      --export-text-muted: var(--text-muted, currentColor);
      --export-text-inverse: var(--text-inverse, Canvas);
      --export-text-code: var(--text-code, currentColor);
      --export-border-subtle: var(--border-subtle, currentColor);
      --export-border-default: var(--border-default, currentColor);
      --export-border-strong: var(--border-strong, currentColor);
      --export-accent: var(--accent, currentColor);
      --export-accent-dim: var(--accent-dim, transparent);
      --export-color-success: var(--color-success, currentColor);
      --export-color-success-dim: var(--color-success-dim, transparent);
      --export-color-warning: var(--color-warning, currentColor);
      --export-color-warning-dim: var(--color-warning-dim, transparent);
      --export-color-info: var(--color-info, currentColor);
      --export-color-info-dim: var(--color-info-dim, transparent);
      --export-radius-card: var(--radius-xl, 12px);
      --export-radius-inner: var(--radius-lg, 12px);
      --export-radius-summary: var(--radius-md, 8px);
      --export-radius-flag: var(--radius-full, 9999px);
    }
    body { margin: 0; font-family: "Segoe UI", Arial, sans-serif; background: var(--export-bg-base); color: var(--export-text-primary); }
    .page { max-width: 1100px; margin: 0 auto; padding: 32px 20px 64px; }
    .hero { background: var(--export-bg-surface); border: 1px solid var(--export-border-default); border-radius: var(--export-radius-card); padding: 24px; box-shadow: none; }
    .hero h1 { margin: 0 0 8px; font-size: 28px; }
    .hero p { margin: 4px 0; color: var(--export-text-secondary); }
    .conversation { margin-top: 24px; background: var(--export-bg-surface); border: 1px solid var(--export-border-default); border-radius: var(--export-radius-card); overflow: hidden; box-shadow: none; }
    .conversation__header { padding: 20px 24px; border-bottom: 1px solid var(--export-border-subtle); background: var(--export-bg-elevated); }
    .conversation__title { margin: 0 0 8px; font-size: 22px; }
    .conversation__meta { display: flex; flex-wrap: wrap; gap: 12px; font-size: 13px; color: var(--export-text-muted); }
    .conversation__summary { margin-top: 12px; padding: 12px 14px; border-radius: var(--export-radius-summary); background: var(--export-bg-elevated); }
    .messages { padding: 20px; display: grid; gap: 12px; }
    .message { border-radius: var(--export-radius-inner); padding: 14px 16px; border: 1px solid var(--export-border-subtle); background: var(--export-bg-surface); }
    .message--user { border-color: var(--export-accent); background: var(--export-accent-dim); }
    .message--assistant { border-color: var(--export-color-success); background: var(--export-color-success-dim); }
    .message--tool { border-color: var(--export-color-warning); background: var(--export-color-warning-dim); }
    .message--system { border-color: var(--export-color-info); background: var(--export-color-info-dim); }
    .message__meta { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 8px; font-size: 12px; color: var(--export-text-muted); }
    .message__role { font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; }
    .message__content, .message__reasoning, .message__details { word-break: break-word; line-height: 1.5; }
    .message__reasoning { margin-top: 10px; padding-top: 10px; border-top: 1px dashed var(--export-border-strong); color: var(--export-text-secondary); }
    .message__flags { margin-top: 10px; display: flex; flex-wrap: wrap; gap: 8px; }
    .message__flag { padding: 4px 8px; border-radius: var(--export-radius-flag); background: var(--export-bg-hover); color: var(--export-text-secondary); font-size: 12px; border: 1px solid var(--export-border-subtle); }
    .message__details { margin-top: 10px; padding: 10px 12px; border-radius: var(--export-radius-summary); background: var(--export-bg-elevated); color: var(--export-text-code); font-family: ui-monospace, monospace; font-size: 12px; border: 1px solid var(--export-border-subtle); }
    .message__content p, .message__reasoning p, .conversation__summary p { margin: 0 0 10px; }
    .message__content p:last-child, .message__reasoning p:last-child, .conversation__summary p:last-child { margin-bottom: 0; }
    .message__content pre, .message__reasoning pre, .conversation__summary pre { margin: 10px 0 0; overflow-x: auto; padding: 12px; border-radius: var(--export-radius-summary); background: var(--export-bg-elevated); color: var(--export-text-code); border: 1px solid var(--export-border-subtle); }
    .message__content code, .message__reasoning code, .conversation__summary code { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; }
    .message__content blockquote, .message__reasoning blockquote, .conversation__summary blockquote { margin: 10px 0 0; padding-left: 12px; border-left: 4px solid var(--export-border-strong); color: var(--export-text-muted); }
    .message__content ul, .message__content ol, .message__reasoning ul, .message__reasoning ol, .conversation__summary ul, .conversation__summary ol { margin: 10px 0 0 20px; }
    .message__content table, .message__reasoning table, .conversation__summary table { margin-top: 10px; width: 100%; border-collapse: collapse; }
    .message__content th, .message__content td, .message__reasoning th, .message__reasoning td, .conversation__summary th, .conversation__summary td { border: 1px solid var(--export-border-strong); padding: 6px 8px; text-align: left; }
    .message__content img, .message__reasoning img, .conversation__summary img { max-width: 100%; border-radius: var(--export-radius-inner); display: block; margin-top: 10px; }
    .message__media { margin-top: 12px; display: grid; gap: 10px; }
    .message__media-card { padding: 12px; border-radius: var(--export-radius-inner); background: var(--export-bg-hover); border: 1px solid var(--export-border-strong); }
    .message__media-card strong { display: block; margin-bottom: 6px; }
    .message__media-card img, .message__media-card video, .message__media-card audio { width: 100%; max-width: 100%; border-radius: var(--export-radius-summary); }
    .message__media-card pre { margin: 8px 0 0; white-space: pre-wrap; max-height: 240px; overflow: auto; }
    .message__media-card a { color: var(--export-accent); }
    .message__content pre.chroma, .message__reasoning pre.chroma, .conversation__summary pre.chroma { padding: 12px; border-radius: var(--export-radius-summary); overflow-x: auto; }
    .message__content pre.chroma code, .message__reasoning pre.chroma code, .conversation__summary pre.chroma code { background: transparent; border: 0; padding: 0; }
{{ chromaCSS }}
  </style>
</head>
<body>
  <div class="page">
    <section class="hero">
      <h1>Exportação de conversas</h1>
      {{ if .Options.IncludeTimestamps }}<p>Gerado em {{ formatTime .ExportedAt }}</p>{{ end }}
      <p>{{ len .Resources.Conversations }} conversa(s) exportada(s)</p>
      <p>Formato canônico: JSON version {{ .Version }}</p>
    </section>
    {{ range .Resources.Conversations }}
      <article class="conversation">
        <header class="conversation__header">
          <h2 class="conversation__title">{{ conversationTitle .Title }}</h2>
          <div class="conversation__meta">
            {{ if $.Options.IncludeTimestamps }}<span>Criada em {{ formatTime .CreatedAt }}</span>{{ end }}
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
                {{ if $.Options.IncludeTimestamps }}<span>{{ formatTime .CreatedAt }}</span>{{ end }}
                {{ if $.Options.IncludeMetadata }}
                  {{ if .Model }}<span>Modelo: {{ .Model }}</span>{{ end }}
                  {{ if .Source }}<span>Origem: {{ .Source }}</span>{{ end }}
                  {{ if .PromptTokens }}<span>Prompt: {{ .PromptTokens }}</span>{{ end }}
                  {{ if .CompletionTokens }}<span>Resposta: {{ .CompletionTokens }}</span>{{ end }}
                {{ end }}
              </div>
              <div class="message__content">{{ renderMarkdown .Content }}</div>
              {{ if and $.Options.IncludeReasoning .Reasoning }}
                <div class="message__reasoning">{{ renderMarkdown .Reasoning }}</div>
              {{ end }}
              {{ if or .Media .AudioMimeType .ToolCalls .ToolCallID }}
                <div class="message__flags">
                  {{ if .Media }}<span class="message__flag">mídia anexada</span>{{ end }}
                  {{ if .AudioMimeType }}<span class="message__flag">áudio: {{ .AudioMimeType }}</span>{{ end }}
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
		"chromaCSS":          func() template.CSS { return template.CSS(chromaHighlightCSS()) },
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
	pdf.SetTitle("Exportação de conversas", false)
	pdf.SetAuthor("Assistente", false)

	useUTF8 := configurePDFFont(pdf)

	opts := file.Options

	pdf.AddPage()
	writePDFTitle(pdf, useUTF8, "Exportação de conversas")
	if opts.IncludeTimestamps {
		writePDFMeta(pdf, useUTF8, fmt.Sprintf("Gerado em %s", formatConversationTime(file.ExportedAt)))
	}
	writePDFMeta(pdf, useUTF8, fmt.Sprintf("%d conversa(s)", len(file.Resources.Conversations)))
	pdf.Ln(4)

	for idx, conv := range file.Resources.Conversations {
		if idx > 0 {
			pdf.AddPage()
		}

		writePDFSectionTitle(pdf, useUTF8, conversationTitle(conv.Title))
		if opts.IncludeTimestamps {
			writePDFMeta(pdf, useUTF8, fmt.Sprintf("Criada em %s", formatConversationTime(conv.CreatedAt)))
		}
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
			metaParts := make([]string, 0, 3)
			if opts.IncludeTimestamps {
				metaParts = append(metaParts, formatConversationTime(msg.CreatedAt))
			}
			if opts.IncludeMetadata {
				if msg.Model != "" {
					metaParts = append(metaParts, "modelo: "+msg.Model)
				}
				if msg.Source != "" {
					metaParts = append(metaParts, "origem: "+msg.Source)
				}
			}
			blockTitle := header
			if len(metaParts) > 0 {
				blockTitle = header + " - " + strings.Join(metaParts, " | ")
			}
			writePDFBlock(pdf, useUTF8, blockTitle, markdownToPDFText(msg.Content))
			if opts.IncludeReasoning && strings.TrimSpace(msg.Reasoning) != "" {
				writePDFIndentedBlock(pdf, useUTF8, "Reasoning", markdownToPDFText(msg.Reasoning))
			}
			if strings.TrimSpace(msg.ToolCalls) != "" {
				writePDFIndentedBlock(pdf, useUTF8, "Tool calls", msg.ToolCalls)
			}
			if err := writePDFMediaAttachments(pdf, useUTF8, msg); err != nil {
				writePDFMeta(pdf, useUTF8, "Falha ao renderizar uma ou mais mídias anexadas: "+err.Error())
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

// RenderConversationsMarkdown gera um documento Markdown (.md) a partir do
// modelo canônico de conversas, respeitando os toggles de conteúdo em
// file.Options (timestamps, reasoning e metadados).
func RenderConversationsMarkdown(file *ExportFile) (string, error) {
	opts := file.Options
	var sb strings.Builder

	sb.WriteString("# Exportação de conversas\n\n")
	if opts.IncludeTimestamps {
		sb.WriteString(fmt.Sprintf("Gerado em %s\n\n", formatConversationTime(file.ExportedAt)))
	}
	sb.WriteString(fmt.Sprintf("%d conversa(s) exportada(s)\n", len(file.Resources.Conversations)))

	for _, conv := range file.Resources.Conversations {
		sb.WriteString("\n---\n\n")
		sb.WriteString("## " + conversationTitle(conv.Title) + "\n\n")

		meta := make([]string, 0, 4)
		if opts.IncludeTimestamps {
			meta = append(meta, "Criada em "+formatConversationTime(conv.CreatedAt))
		}
		if strings.TrimSpace(conv.Channel) != "" {
			meta = append(meta, "Canal: "+conv.Channel)
		}
		if strings.TrimSpace(conv.ContactID) != "" {
			meta = append(meta, "Contato: "+conv.ContactID)
		}
		meta = append(meta, fmt.Sprintf("%d mensagem(ns)", len(conv.Messages)))
		for _, line := range meta {
			sb.WriteString("- " + line + "\n")
		}
		sb.WriteString("\n")

		if strings.TrimSpace(conv.Summary) != "" {
			sb.WriteString("> **Resumo:** " + collapseToInline(conv.Summary) + "\n\n")
		}

		for _, msg := range conv.Messages {
			sb.WriteString("### " + roleLabel(msg.Role) + "\n\n")

			info := make([]string, 0, 4)
			if opts.IncludeTimestamps {
				info = append(info, formatConversationTime(msg.CreatedAt))
			}
			if opts.IncludeMetadata {
				if strings.TrimSpace(msg.Model) != "" {
					info = append(info, "modelo: "+msg.Model)
				}
				if strings.TrimSpace(msg.Source) != "" {
					info = append(info, "origem: "+msg.Source)
				}
				if msg.PromptTokens > 0 {
					info = append(info, fmt.Sprintf("prompt: %d", msg.PromptTokens))
				}
				if msg.CompletionTokens > 0 {
					info = append(info, fmt.Sprintf("resposta: %d", msg.CompletionTokens))
				}
			}
			if len(info) > 0 {
				sb.WriteString("*" + strings.Join(info, " · ") + "*\n\n")
			}

			if content := strings.TrimRight(msg.Content, "\n"); strings.TrimSpace(content) != "" {
				sb.WriteString(content + "\n\n")
			}

			if opts.IncludeReasoning && strings.TrimSpace(msg.Reasoning) != "" {
				sb.WriteString("**Reasoning:**\n\n")
				sb.WriteString(strings.TrimRight(msg.Reasoning, "\n") + "\n\n")
			}

			if strings.TrimSpace(msg.ToolCalls) != "" {
				sb.WriteString("**Tool calls:**\n\n")
				sb.WriteString("```json\n")
				sb.WriteString(strings.TrimRight(msg.ToolCalls, "\n") + "\n")
				sb.WriteString("```\n\n")
			}

			if opts.IncludeMetadata {
				flags := make([]string, 0, 2)
				if strings.TrimSpace(msg.Media) != "" {
					flags = append(flags, "mídia anexada")
				}
				if strings.TrimSpace(msg.AudioMimeType) != "" {
					flags = append(flags, "áudio: "+msg.AudioMimeType)
				}
				if len(flags) > 0 {
					sb.WriteString("> " + strings.Join(flags, " · ") + "\n\n")
				}
			}
		}
	}

	return strings.TrimRight(sb.String(), "\n") + "\n", nil
}

// collapseToInline transforma um texto multilinha em uma única linha, útil para
// resumos exibidos como blockquote no Markdown.
func collapseToInline(text string) string {
	fields := strings.Fields(text)
	return strings.Join(fields, " ")
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

var pdfFontCandidates = defaultPDFFontCandidates

func loadSystemPDFFont() ([]byte, error) {
	candidates := pdfFontCandidates()
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, fmt.Errorf("nenhuma fonte TTF encontrada")
}

func defaultPDFFontCandidates() []string {
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
	return candidates
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
	normalized := strings.TrimSpace(role)
	switch strings.ToLower(normalized) {
	case "user":
		return "Usuário"
	case "assistant":
		return "Assistente"
	case "tool":
		return "Ferramenta"
	case "system":
		return "Sistema"
	default:
		if normalized == "" {
			return "Mensagem"
		}
		runes := []rune(normalized)
		if len(runes) == 0 {
			return "Mensagem"
		}
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		return string(runes)
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
		return "Conversa sem título"
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
	return template.HTML(sanitizeRenderedHTML(markdownToHTMLHighlighted(content)))
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
		normalizedAudio, ok := normalizeBase64Data(msg.Audio)
		if !ok {
			return template.HTML(strings.Join(parts, ""))
		}
		parts = append(parts, renderAttachmentHTML(mediaAttachment{
			Name: "Áudio da mensagem",
			MIME: sanitizeMIMEType(msg.AudioMimeType),
			Data: normalizedAudio,
		}))
	}
	return template.HTML(strings.Join(parts, ""))
}

func hasRichMedia(msg MessageExport) bool {
	return strings.TrimSpace(msg.Media) != "" || (strings.TrimSpace(msg.Audio) != "" && strings.TrimSpace(msg.AudioMimeType) != "")
}

func renderAttachmentHTML(media mediaAttachment) string {
	name := template.HTMLEscapeString(fallbackAttachmentName(media))
	dataURI := buildDataURI(media.MIME, media.Data)
	displayMIME := template.HTMLEscapeString(sanitizeMIMEType(media.MIME))
	if dataURI == "" {
		return `<div class="message__media-card"><strong>` + name + `</strong><div>` + displayMIME + `</div><div>Anexo inválido</div></div>`
	}
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
		return `<div class="message__media-card"><strong>` + name + `</strong><div>` + displayMIME + `</div>` + body + `</div>`
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
		normalizedData, ok := normalizeBase64Data(data)
		if !ok {
			continue
		}
		result = append(result, mediaAttachment{
			Name: name,
			MIME: sanitizeMIMEType(mime),
			Data: normalizedData,
		})
	}
	return result
}

func buildDataURI(mimeType, data string) string {
	normalizedData, ok := normalizeBase64Data(data)
	if !ok {
		return ""
	}
	return "data:" + sanitizeMIMEType(mimeType) + ";base64," + normalizedData
}

func normalizeBase64Data(data string) (string, bool) {
	compact := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, data)
	if strings.TrimSpace(compact) == "" {
		return "", false
	}
	if base64.StdEncoding.DecodedLen(len(compact)) > maxAttachmentDecodedBytes {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return "", false
	}
	return base64.StdEncoding.EncodeToString(decoded), true
}

func sanitizeMIMEType(mimeType string) string {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	if normalized == "" {
		return "application/octet-stream"
	}
	if strings.ContainsAny(normalized, "\"' ;,\r\n\t<>") {
		return "application/octet-stream"
	}
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '/', r == '-', r == '.', r == '+':
		default:
			return "application/octet-stream"
		}
	}

	switch {
	case isAllowedImageMIME(normalized):
		return normalized
	case strings.HasPrefix(normalized, "audio/"):
		return normalized
	case strings.HasPrefix(normalized, "video/"):
		return normalized
	case isAllowedTextMIME(normalized):
		return normalized
	case isAllowedApplicationMIME(normalized):
		return normalized
	}

	switch normalized {
	case "application/pdf", "application/octet-stream", "application/zip":
		return normalized
	default:
		return "application/octet-stream"
	}
}

func isAllowedImageMIME(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/jpg", "image/gif":
		return true
	default:
		return false
	}
}

func isAllowedTextMIME(mimeType string) bool {
	switch mimeType {
	case "text/plain", "text/markdown", "text/csv", "text/xml", "text/yaml":
		return true
	default:
		return false
	}
}

func isAllowedApplicationMIME(mimeType string) bool {
	switch mimeType {
	case "application/json",
		"application/ld+json",
		"application/xml",
		"application/yaml",
		"application/x-yaml",
		"application/csv",
		"application/pdf",
		"application/octet-stream",
		"application/zip":
		return true
	default:
		return false
	}
}

func fallbackAttachmentName(media mediaAttachment) string {
	if strings.TrimSpace(media.Name) != "" {
		return media.Name
	}
	switch mediaKind(media.MIME) {
	case "image":
		return "Imagem anexada"
	case "audio":
		return "Áudio anexado"
	case "video":
		return "Vídeo anexado"
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
		text = text[:4000] + "\n\n...[conteúdo truncado]"
	}
	return text
}

func isTextLikeMedia(mimeType string) bool {
	return isAllowedTextMIME(mimeType) || isAllowedApplicationMIME(mimeType)
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

func markdownToHTMLHighlighted(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := markdownHTMLRenderer.Convert([]byte(content), &buf); err != nil {
		return markdownToHTML(content)
	}
	return buf.String()
}

func sanitizeRenderedHTML(fragment string) string {
	if strings.TrimSpace(fragment) == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader("<div>" + fragment + "</div>"))
	if err != nil {
		return template.HTMLEscapeString(fragment)
	}

	root := findHTMLWrapper(doc)
	if root == nil {
		return template.HTMLEscapeString(fragment)
	}

	var sb strings.Builder
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		renderSanitizedHTMLNode(child, &sb)
	}
	return sb.String()
}

func findHTMLWrapper(root *html.Node) *html.Node {
	var wrapper *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if wrapper != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "div" {
			wrapper = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return wrapper
}

func renderSanitizedHTMLNode(n *html.Node, sb *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		sb.WriteString(template.HTMLEscapeString(n.Data))
		return
	case html.ElementNode:
		if isBlockedHTMLTag(n.Data) {
			return
		}
		if !isAllowedHTMLTag(n.Data) {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				renderSanitizedHTMLNode(child, sb)
			}
			return
		}

		sb.WriteString("<")
		sb.WriteString(n.Data)
		for _, attr := range sanitizeHTMLAttrs(n) {
			sb.WriteString(" ")
			sb.WriteString(attr.Key)
			sb.WriteString(`="`)
			sb.WriteString(template.HTMLEscapeString(attr.Val))
			sb.WriteString(`"`)
		}
		if isVoidHTMLTag(n.Data) {
			sb.WriteString(">")
			return
		}
		sb.WriteString(">")
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			renderSanitizedHTMLNode(child, sb)
		}
		sb.WriteString("</")
		sb.WriteString(n.Data)
		sb.WriteString(">")
	default:
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			renderSanitizedHTMLNode(child, sb)
		}
	}
}

func sanitizeHTMLAttrs(n *html.Node) []html.Attribute {
	attrs := make([]html.Attribute, 0, len(n.Attr))
	for _, attr := range n.Attr {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		if strings.HasPrefix(key, "on") {
			continue
		}

		// class é seguro (não executa) e necessário para as classes de
		// syntax highlighting do Chroma nos blocos de código.
		if key == "class" {
			attrs = append(attrs, html.Attribute{Key: key, Val: attr.Val})
			continue
		}

		switch n.Data {
		case "a":
			if key == "href" && isSafeRenderedURL(attr.Val, false) {
				attrs = append(attrs, html.Attribute{Key: key, Val: strings.TrimSpace(attr.Val)})
			}
			if key == "title" {
				attrs = append(attrs, html.Attribute{Key: key, Val: attr.Val})
			}
		case "img":
			if key == "src" && isSafeRenderedURL(attr.Val, false) {
				attrs = append(attrs, html.Attribute{Key: key, Val: strings.TrimSpace(attr.Val)})
			}
			if key == "alt" || key == "title" {
				attrs = append(attrs, html.Attribute{Key: key, Val: attr.Val})
			}
		}
	}
	return attrs
}

func isSafeRenderedURL(raw string, allowData bool) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.ContainsAny(lower, "\"'<> \r\n\t") {
		return false
	}
	if strings.HasPrefix(lower, "#") || strings.HasPrefix(lower, "/") || strings.HasPrefix(lower, "./") || strings.HasPrefix(lower, "../") {
		return true
	}
	if !strings.Contains(lower, ":") {
		return true
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") {
		return true
	}
	return allowData && strings.HasPrefix(lower, "data:")
}

func isAllowedHTMLTag(tag string) bool {
	switch tag {
	case "p", "br", "hr", "strong", "em", "code", "pre", "blockquote",
		"ul", "ol", "li", "table", "thead", "tbody", "tr", "th", "td",
		"a", "img", "h1", "h2", "h3", "h4", "h5", "h6", "span", "div", "del":
		return true
	default:
		return false
	}
}

func isBlockedHTMLTag(tag string) bool {
	switch tag {
	case "script", "style", "iframe", "object", "embed", "link", "meta":
		return true
	default:
		return false
	}
}

func isVoidHTMLTag(tag string) bool {
	switch tag {
	case "br", "hr", "img":
		return true
	default:
		return false
	}
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
		normalizedAudio, ok := normalizeBase64Data(msg.Audio)
		if !ok {
			errs = append(errs, "áudio da mensagem inválido ou acima do limite de exportação")
		} else if err := writePDFAttachment(pdf, useUTF8, mediaAttachment{
			Name: "Áudio da mensagem",
			MIME: sanitizeMIMEType(msg.AudioMimeType),
			Data: normalizedAudio,
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
		if err := writePDFImageAttachment(pdf, useUTF8, media, key); err != nil {
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

func writePDFImageAttachment(pdf *fpdf.Fpdf, useUTF8 bool, media mediaAttachment, key string) error {
	decoded, err := base64.StdEncoding.DecodeString(media.Data)
	if err != nil {
		return err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		return err
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return fmt.Errorf("formato de imagem não suportado no PDF: %s", format)
	}

	info := pdf.RegisterImageOptionsReader(key+"."+format, fpdf.ImageOptions{ImageType: format}, bytes.NewReader(decoded))
	if info == nil {
		return fmt.Errorf("não foi possível registrar imagem no PDF")
	}

	writePDFMeta(pdf, useUTF8, fallbackAttachmentName(media))
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
