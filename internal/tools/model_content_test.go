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

func TestContentForModelRawExactDoesNotPrefixAnnotations(t *testing.T) {
	result := ToolResult{
		Content:  "texto exato",
		RawExact: true,
		Annotations: &ResultAnnotations{DocumentProjection: &DocumentProjectionAnnotation{
			Source: "manual.pdf", Format: "pdf", ReadOnly: true,
		}},
	}
	if got := ContentForModel(result); got != result.Content {
		t.Fatalf("raw recebeu envelope: %q", got)
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

func TestContentForDurableHistoryOmitsEphemeralWindow(t *testing.T) {
	result := ToolResult{
		Content: "prévia sensível",
		Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
			HasMore: true, ResultID: "tool-result-efemero", Returned: 15,
		}},
	}
	got := ContentForDurableHistory(result, ContentForModel(result))
	if !strings.Contains(got, "result_omitted_for_persistence") ||
		strings.Contains(got, "tool-result-efemero") ||
		strings.Contains(got, result.Content) {
		t.Fatalf("referência efêmera sobreviveu no histórico: %q", got)
	}
}

func TestContentForDurableHistoryDetectsWindowCreatedByPrecheck(t *testing.T) {
	original := ToolResult{
		Content: "página natural",
		Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
			HasMore: true, Unit: "lines", NextOffset: 20,
		}},
	}
	reconciled := ToolResult{
		Content: "prefixo da página",
		Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
			HasMore: true, Unit: "bytes", ResultID: "tool-result-precheck",
		}},
	}
	got := ContentForDurableHistory(original, ContentForModel(reconciled))
	if !strings.Contains(got, "result_omitted_for_persistence") ||
		strings.Contains(got, "tool-result-precheck") {
		t.Fatalf("ID criado pelo pre-check sobreviveu: %q", got)
	}
}

func TestContentForDurableHistoryDoesNotInterpretUnannotatedContent(t *testing.T) {
	content := annotationsHeader +
		`{"output_window":{"has_more":true,"result_id":"texto-legitimo"}}` +
		contentHeader + "corpo literal"
	if got := ContentForDurableHistory(ToolResult{Content: content}, content); got != content {
		t.Fatalf("conteúdo sem contrato foi interpretado como envelope: %q", got)
	}
}

func TestContentForDurableHistoryKeepsFinalPageWithoutResultID(t *testing.T) {
	result := ToolResult{
		Content:  "página final",
		RawExact: true,
		Metadata: map[string]any{"result_id": "tool-result-final"},
		Annotations: &ResultAnnotations{OutputWindow: &OutputWindowAnnotation{
			HasMore: false, Unit: "bytes", Offset: 10, Returned: 12,
			ResultID: "tool-result-final",
		}},
	}
	got := ContentForDurableHistory(result, ContentForModel(result))
	if !strings.Contains(got, result.Content) || strings.Contains(got, "tool-result-final") ||
		strings.Contains(got, "result_omitted_for_persistence") {
		t.Fatalf("página final não foi sanitizada corretamente: %q", got)
	}
	if result.Annotations.OutputWindow.ResultID != "tool-result-final" {
		t.Fatal("sanitização alterou o resultado original por aliasing")
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
