package portability

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/go-pdf/fpdf"
)

func TestRenderConversationsHTMLIncludesConversationContent(t *testing.T) {
	const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jp1EAAAAASUVORK5CYII="

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Options:    ExportOptions{IncludeTimestamps: true, IncludeReasoning: true, IncludeMetadata: true},
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa de teste",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{
							Role:          "user",
							Content:       "# Titulo\n\nTexto com **negrito**.",
							Media:         `[{"type":"image/png","name":"captura.png","data":"` + tinyPNG + `"}]`,
							Audio:         tinyPNG,
							AudioMimeType: "audio/mpeg",
							CreatedAt:     time.Unix(91, 0),
						},
					},
				},
			},
		},
	}

	html, err := RenderConversationsHTML(file)
	if err != nil {
		t.Fatalf("RenderConversationsHTML() error = %v", err)
	}
	if !strings.Contains(html, "Conversa de teste") {
		t.Fatalf("html missing conversation title: %s", html)
	}
	if !strings.Contains(html, "<strong>negrito</strong>") {
		t.Fatalf("html should render markdown formatting: %s", html)
	}
	if !strings.Contains(html, "<img src=\"data:image/png;base64,") {
		t.Fatalf("html should embed attached image: %s", html)
	}
	if !strings.Contains(html, "mídia anexada") {
		t.Fatalf("html should render accented media label: %s", html)
	}
	if !strings.Contains(html, "áudio: audio/mpeg") {
		t.Fatalf("html should render accented audio label: %s", html)
	}
	if !strings.Contains(html, "var(--export-bg-base") {
		t.Fatalf("html should use export css variables: %s", html)
	}
	if strings.Contains(html, "#f5f7fb") || strings.Contains(html, "rgba(") {
		t.Fatalf("html should not include hardcoded export colors: %s", html)
	}
}

func TestRenderConversationsHTMLSanitizesMarkdownAndAttachmentMIME(t *testing.T) {
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa segura",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{
							Role:      "assistant",
							Content:   `[link](javascript:alert(1)) <script>alert(1)</script>`,
							Media:     `[{"type":"image/png\" onerror=\"alert(1)","name":"captura.png","data":"AAAA"}]`,
							CreatedAt: time.Unix(91, 0),
						},
					},
				},
			},
		},
	}

	html, err := RenderConversationsHTML(file)
	if err != nil {
		t.Fatalf("RenderConversationsHTML() error = %v", err)
	}
	if strings.Contains(strings.ToLower(html), "javascript:") {
		t.Fatalf("html should sanitize dangerous urls: %s", html)
	}
	if strings.Contains(strings.ToLower(html), "<script") {
		t.Fatalf("html should strip script tags: %s", html)
	}
	if !strings.Contains(html, `data:application/octet-stream;base64,AAAA`) {
		t.Fatalf("html should sanitize invalid attachment mime types: %s", html)
	}
}

func TestRenderConversationsHTMLSkipsInvalidBase64AndUnsafeSVG(t *testing.T) {
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa segura",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{
							Role:      "assistant",
							Content:   "ok",
							Media:     `[{"type":"image/svg+xml","name":"unsafe.svg","data":"PHN2Zz48L3N2Zz4="},{"type":"image/png","name":"broken.png","data":"not-base64"}]`,
							CreatedAt: time.Unix(91, 0),
						},
					},
				},
			},
		},
	}

	html, err := RenderConversationsHTML(file)
	if err != nil {
		t.Fatalf("RenderConversationsHTML() error = %v", err)
	}
	if strings.Contains(html, "data:image/svg+xml") {
		t.Fatalf("html should not render svg as image data uri: %s", html)
	}
	if strings.Contains(html, "not-base64") {
		t.Fatalf("html should skip invalid base64 payloads: %s", html)
	}
	if !strings.Contains(html, "application/octet-stream") {
		t.Fatalf("html should degrade unsafe svg mime to octet-stream: %s", html)
	}
}

func TestRenderConversationsHTMLRejectsUnsafeTextLikeMime(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("<html><body>unsafe</body></html>"))
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa segura",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{
							Role:      "assistant",
							Content:   "ok",
							Media:     `[{"type":"text/html","name":"unsafe.html","data":"` + payload + `"}]`,
							CreatedAt: time.Unix(91, 0),
						},
					},
				},
			},
		},
	}

	html, err := RenderConversationsHTML(file)
	if err != nil {
		t.Fatalf("RenderConversationsHTML() error = %v", err)
	}
	if strings.Contains(html, "data:text/html") {
		t.Fatalf("html should not keep text/html attachments executable: %s", html)
	}
	if !strings.Contains(html, "application/octet-stream") {
		t.Fatalf("html should degrade unsafe text/html mime to octet-stream: %s", html)
	}
}

func TestNormalizeBase64DataRejectsOversizedPayload(t *testing.T) {
	oversized := strings.Repeat("A", base64.StdEncoding.EncodedLen(maxAttachmentDecodedBytes+1))
	if _, ok := normalizeBase64Data(oversized); ok {
		t.Fatal("normalizeBase64Data() should reject oversized payloads")
	}
}

func TestRoleLabelNormalizesUnicodeFallbackSafely(t *testing.T) {
	if got := roleLabel("  ánalise "); got != "Ánalise" {
		t.Fatalf("roleLabel() = %q, want %q", got, "Ánalise")
	}
	if got := roleLabel(" "); got != "Mensagem" {
		t.Fatalf("roleLabel() = %q, want %q", got, "Mensagem")
	}
}

func sampleConversationFile(opts ExportOptions) *ExportFile {
	return &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Options:    opts,
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa de teste",
					CreatedAt: time.Unix(90, 0),
					Summary:   "Resumo da conversa.",
					Messages: []MessageExport{
						{
							Role:             "user",
							Content:          "Olá, **mundo**.",
							CreatedAt:        time.Unix(91, 0),
							Model:            "gpt-4o",
							Source:           "chat",
							PromptTokens:     12,
							CompletionTokens: 34,
							Reasoning:        "Pensando no problema.",
							ToolCalls:        `{"name":"search"}`,
						},
					},
				},
			},
		},
	}
}

func TestRenderConversationsMarkdownIncludesContent(t *testing.T) {
	file := sampleConversationFile(ExportOptions{
		IncludeTimestamps: true,
		IncludeReasoning:  true,
		IncludeMetadata:   true,
	})

	md, err := RenderConversationsMarkdown(file)
	if err != nil {
		t.Fatalf("RenderConversationsMarkdown() error = %v", err)
	}
	if !strings.Contains(md, "# Exportação de conversas") {
		t.Fatalf("markdown missing document title: %s", md)
	}
	if !strings.Contains(md, "## Conversa de teste") {
		t.Fatalf("markdown missing conversation heading: %s", md)
	}
	if !strings.Contains(md, "### Usuário") {
		t.Fatalf("markdown missing role heading: %s", md)
	}
	if !strings.Contains(md, "Olá, **mundo**.") {
		t.Fatalf("markdown should preserve message markdown: %s", md)
	}
	if !strings.Contains(md, "**Reasoning:**") || !strings.Contains(md, "Pensando no problema.") {
		t.Fatalf("markdown should include reasoning when enabled: %s", md)
	}
	if !strings.Contains(md, "```json") || !strings.Contains(md, `{"name":"search"}`) {
		t.Fatalf("markdown should include tool calls as fenced code: %s", md)
	}
	if !strings.Contains(md, "modelo: gpt-4o") {
		t.Fatalf("markdown should include metadata when enabled: %s", md)
	}
}

func TestRenderConversationsMarkdownRespectsToggles(t *testing.T) {
	file := sampleConversationFile(ExportOptions{
		IncludeTimestamps: false,
		IncludeReasoning:  false,
		IncludeMetadata:   false,
	})

	md, err := RenderConversationsMarkdown(file)
	if err != nil {
		t.Fatalf("RenderConversationsMarkdown() error = %v", err)
	}
	if strings.Contains(md, "**Reasoning:**") {
		t.Fatalf("markdown should omit reasoning when disabled: %s", md)
	}
	if strings.Contains(md, "modelo: gpt-4o") || strings.Contains(md, "origem: chat") {
		t.Fatalf("markdown should omit metadata when disabled: %s", md)
	}
	if strings.Contains(md, "Gerado em") || strings.Contains(md, "Criada em") {
		t.Fatalf("markdown should omit timestamps when disabled: %s", md)
	}
	if !strings.Contains(md, "Olá, **mundo**.") {
		t.Fatalf("markdown should still include message content: %s", md)
	}
}

func TestRenderConversationsHTMLRespectsToggles(t *testing.T) {
	on := sampleConversationFile(ExportOptions{IncludeTimestamps: true, IncludeReasoning: true, IncludeMetadata: true})
	htmlOn, err := RenderConversationsHTML(on)
	if err != nil {
		t.Fatalf("RenderConversationsHTML() error = %v", err)
	}
	if !strings.Contains(htmlOn, "Modelo: gpt-4o") {
		t.Fatalf("html should include metadata when enabled: %s", htmlOn)
	}
	if !strings.Contains(htmlOn, `<div class="message__reasoning">`) {
		t.Fatalf("html should include reasoning when enabled: %s", htmlOn)
	}

	off := sampleConversationFile(ExportOptions{IncludeTimestamps: false, IncludeReasoning: false, IncludeMetadata: false})
	htmlOff, err := RenderConversationsHTML(off)
	if err != nil {
		t.Fatalf("RenderConversationsHTML() error = %v", err)
	}
	if strings.Contains(htmlOff, "Modelo: gpt-4o") {
		t.Fatalf("html should omit metadata when disabled: %s", htmlOff)
	}
	if strings.Contains(htmlOff, `<div class="message__reasoning">`) {
		t.Fatalf("html should omit reasoning when disabled: %s", htmlOff)
	}
	if strings.Contains(htmlOff, "Criada em") || strings.Contains(htmlOff, "Gerado em") {
		t.Fatalf("html should omit timestamps when disabled: %s", htmlOff)
	}
}

func TestRenderConversationsHTMLHighlightsCode(t *testing.T) {
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Options:    ExportOptions{IncludeTimestamps: true, IncludeReasoning: true, IncludeMetadata: true},
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa com código",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{
							Role:      "assistant",
							Content:   "```go\nfunc main() {}\n```",
							CreatedAt: time.Unix(91, 0),
						},
					},
				},
			},
		},
	}

	html, err := RenderConversationsHTML(file)
	if err != nil {
		t.Fatalf("RenderConversationsHTML() error = %v", err)
	}
	if !strings.Contains(html, `class="chroma"`) {
		t.Fatalf("html should wrap highlighted code with chroma class: %s", html)
	}
	if !strings.Contains(html, `<span class=`) {
		t.Fatalf("html should emit chroma token spans for highlighted code: %s", html)
	}
	if !strings.Contains(html, ".chroma") {
		t.Fatalf("html should embed chroma stylesheet: %s", html)
	}
	if strings.Contains(html, "rgba(") {
		t.Fatalf("chroma stylesheet should not introduce rgba colors: %s", html)
	}
}

func TestRenderConversationsHTMLGatesMetadataFlags(t *testing.T) {
	const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jp1EAAAAASUVORK5CYII="

	build := func(opts ExportOptions) string {
		t.Helper()
		file := &ExportFile{
			Version:    ExportVersion,
			ExportedAt: time.Unix(100, 0),
			Options:    opts,
			Resources: ExportResources{
				Conversations: []ConversationExport{
					{
						Title:     "Conversa com flags",
						CreatedAt: time.Unix(90, 0),
						Messages: []MessageExport{
							{
								Role:          "tool",
								Content:       "ok",
								Media:         `[{"type":"image/png","name":"captura.png","data":"` + tinyPNG + `"}]`,
								AudioMimeType: "audio/mpeg",
								ToolCallID:    "call_123",
								CreatedAt:     time.Unix(91, 0),
							},
						},
					},
				},
			},
		}
		html, err := RenderConversationsHTML(file)
		if err != nil {
			t.Fatalf("RenderConversationsHTML() error = %v", err)
		}
		return html
	}

	on := build(ExportOptions{IncludeMetadata: true})
	if !strings.Contains(on, "message__flags") {
		t.Fatalf("html should render metadata flags when enabled: %s", on)
	}
	if !strings.Contains(on, "toolCallId: call_123") {
		t.Fatalf("html should render toolCallId flag when metadata enabled: %s", on)
	}

	off := build(ExportOptions{IncludeMetadata: false})
	if strings.Contains(off, `class="message__flags"`) {
		t.Fatalf("html should omit metadata flags container when disabled: %s", off)
	}
	if strings.Contains(off, "toolCallId: call_123") {
		t.Fatalf("html should omit toolCallId flag when metadata disabled: %s", off)
	}
}

func TestSanitizeRenderedHTMLRestrictsClassToHighlightTags(t *testing.T) {
	in := `<p class="hero">texto</p>` +
		`<pre class="chroma"><code class="language-go"><span class="k">func</span></code></pre>`
	out := sanitizeRenderedHTML(in)

	if strings.Contains(out, `<p class=`) {
		t.Fatalf("sanitizer should drop class on non-highlight tags: %s", out)
	}
	if !strings.Contains(out, `<pre class="chroma">`) {
		t.Fatalf("sanitizer should keep class on pre: %s", out)
	}
	if !strings.Contains(out, `<span class="k">`) {
		t.Fatalf("sanitizer should keep class on span: %s", out)
	}
	if !strings.Contains(out, `<code class="language-go">`) {
		t.Fatalf("sanitizer should keep class on code: %s", out)
	}
}

func TestRenderConversationsPDFRespectsTogglesStillRenders(t *testing.T) {
	file := sampleConversationFile(ExportOptions{IncludeTimestamps: false, IncludeReasoning: false, IncludeMetadata: false})
	pdfBytes, err := RenderConversationsPDF(file)
	if err != nil {
		t.Fatalf("RenderConversationsPDF() error = %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes), "%PDF") {
		t.Fatalf("pdf header missing, got %q", string(pdfBytes[:4]))
	}
}

func TestRenderConversationExportSupportsMarkdownFormat(t *testing.T) {
	file := sampleConversationFile(ExportOptions{IncludeTimestamps: true, IncludeReasoning: true, IncludeMetadata: true})
	out, err := RenderConversationExport(file, FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderConversationExport(markdown) error = %v", err)
	}
	if !strings.Contains(string(out), "# Exportação de conversas") {
		t.Fatalf("markdown export missing title: %s", string(out))
	}
}

func TestRenderConversationsPDFReturnsBytes(t *testing.T) {
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa PDF",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{Role: "assistant", Content: "Conteudo exportado", CreatedAt: time.Unix(91, 0)},
					},
				},
			},
		},
	}

	pdfBytes, err := RenderConversationsPDF(file)
	if err != nil {
		t.Fatalf("RenderConversationsPDF() error = %v", err)
	}
	if len(pdfBytes) == 0 {
		t.Fatal("RenderConversationsPDF() returned empty content")
	}
	if !strings.HasPrefix(string(pdfBytes), "%PDF") {
		t.Fatalf("pdf header missing, got %q", string(pdfBytes[:4]))
	}
}

func TestRenderConversationsPDFFallsBackWhenSystemFontMissing(t *testing.T) {
	originalCandidates := pdfFontCandidates
	pdfFontCandidates = func() []string {
		return []string{filepath.Join(t.TempDir(), "missing.ttf")}
	}
	t.Cleanup(func() {
		pdfFontCandidates = originalCandidates
	})

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Unix(100, 0),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa PDF sem fonte",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{Role: "assistant", Content: "Conteúdo com unicode 你好", CreatedAt: time.Unix(91, 0)},
					},
				},
			},
		},
	}

	pdfBytes, err := RenderConversationsPDF(file)
	if err != nil {
		t.Fatalf("RenderConversationsPDF() error = %v", err)
	}
	if len(pdfBytes) == 0 {
		t.Fatal("RenderConversationsPDF() returned empty content")
	}
	if !strings.HasPrefix(string(pdfBytes), "%PDF") {
		t.Fatalf("pdf header missing, got %q", string(pdfBytes[:4]))
	}
}

func TestWritePDFMediaAttachmentsRejectsInvalidAudioBase64(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	err := writePDFMediaAttachments(pdf, false, MessageExport{
		Audio:         "not-base64",
		AudioMimeType: "audio/mpeg",
	})
	if err == nil {
		t.Fatal("writePDFMediaAttachments() error = nil, want invalid audio error")
	}
	if !strings.Contains(err.Error(), "áudio da mensagem inválido") {
		t.Fatalf("unexpected error: %v", err)
	}
}
