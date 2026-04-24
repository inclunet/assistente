package portability

import (
	"strings"
	"testing"
	"time"
)

func TestRenderConversationsHTMLIncludesConversationContent(t *testing.T) {
	const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jp1EAAAAASUVORK5CYII="

	file := &ExportFile{
		Version:    1,
		ExportedAt: time.Unix(100, 0),
		Resources: ExportResources{
			Conversations: []ConversationExport{
				{
					Title:     "Conversa de teste",
					CreatedAt: time.Unix(90, 0),
					Messages: []MessageExport{
						{
							Role:      "user",
							Content:   "# Titulo\n\nTexto com **negrito**.",
							Media:     `[{"type":"image/png","name":"captura.png","data":"` + tinyPNG + `"}]`,
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
	if !strings.Contains(html, "Conversa de teste") {
		t.Fatalf("html missing conversation title: %s", html)
	}
	if !strings.Contains(html, "<strong>negrito</strong>") {
		t.Fatalf("html should render markdown formatting: %s", html)
	}
	if !strings.Contains(html, "<img src=\"data:image/png;base64,") {
		t.Fatalf("html should embed attached image: %s", html)
	}
}

func TestRenderConversationsHTMLSanitizesMarkdownAndAttachmentMIME(t *testing.T) {
	file := &ExportFile{
		Version:    1,
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
		Version:    1,
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

func TestRenderConversationsPDFReturnsBytes(t *testing.T) {
	file := &ExportFile{
		Version:    1,
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
