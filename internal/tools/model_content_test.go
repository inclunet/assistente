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

func annotatedResult() ToolResult {
	return ToolResult{
		Content: "corpo",
		Annotations: &ResultAnnotations{
			DocumentProjection: &DocumentProjectionAnnotation{Source: "a.pdf", Format: "pdf"},
		},
	}
}

func TestSanitizeTruncatedEnvelopePreservesCompleteEnvelope(t *testing.T) {
	result := annotatedResult()
	envelope := ContentForModel(result)

	// Truncar a cauda do corpo mantém as anotações legíveis.
	truncated := envelope[:len(envelope)-2]
	if got := SanitizeTruncatedEnvelope(result, truncated); got != truncated {
		t.Fatalf("envelope íntegro foi descartado: %q", got)
	}
}

func TestSanitizeTruncatedEnvelopeDropsBrokenAnnotations(t *testing.T) {
	result := annotatedResult()
	envelope := ContentForModel(result)

	cases := map[string]string{
		"corte dentro do JSON":      envelope[:len(annotationsHeader)+5],
		"corte dentro do cabeçalho": envelope[:10],
		"envelope zerado":           "",
	}
	for name, truncated := range cases {
		t.Run(name, func(t *testing.T) {
			if got := SanitizeTruncatedEnvelope(result, truncated); got != "" {
				t.Fatalf("envelope partido sobreviveu: %q", got)
			}
		})
	}
}

func TestSanitizeTruncatedEnvelopeIgnoresResultWithoutAnnotations(t *testing.T) {
	const plain = "resultado sem anotações"
	if got := SanitizeTruncatedEnvelope(ToolResult{Content: plain}, plain); got != plain {
		t.Fatalf("got=%q, want=%q", got, plain)
	}
}
