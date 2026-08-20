package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContentForModelKeepsPlainResultUnchanged(t *testing.T) {
	result := ToolResult{Content: "texto puro"}
	if got := ContentForModel(result); got != result.Content {
		t.Fatalf("got=%q, want=%q", got, result.Content)
	}
}

func TestContentForModelSeparatesProjectionAnnotation(t *testing.T) {
	result := ToolResult{
		Content: "Arquivo: manual.docx\n     1|# Título",
		Annotations: &ResultAnnotations{
			DocumentProjection: &DocumentProjectionAnnotation{
				Source:   "manual.docx",
				Format:   "docx",
				ReadOnly: true,
				Warnings: []string{"parcial"},
			},
		},
	}
	got := ContentForModel(result)
	const contentMarker = "\nConteúdo da tool:\n"
	parts := strings.SplitN(got, contentMarker, 2)
	if len(parts) != 2 {
		t.Fatalf("sem separador de conteúdo: %q", got)
	}
	start := strings.Index(parts[0], "{")
	var annotations ResultAnnotations
	if start < 0 {
		t.Fatalf("sem JSON de anotações: %q", parts[0])
	}
	if err := json.Unmarshal([]byte(parts[0][start:]), &annotations); err != nil {
		t.Fatal(err)
	}
	if parts[1] != result.Content {
		t.Fatalf("content=%q, want=%q", parts[1], result.Content)
	}
	projection := annotations.DocumentProjection
	if projection == nil || projection.Format != "docx" || !projection.ReadOnly {
		t.Fatalf("annotation=%+v", projection)
	}
}
