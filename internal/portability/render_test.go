package portability

import (
	"encoding/base64"
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
