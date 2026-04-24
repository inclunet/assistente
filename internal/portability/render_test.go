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
