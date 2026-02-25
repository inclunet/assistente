package llm

import (
	"strings"
	"testing"
)

type captureHandler struct {
	chunks   strings.Builder
	thinking strings.Builder
}

func (h *captureHandler) OnChunk(content string)              { h.chunks.WriteString(content) }
func (h *captureHandler) OnThinking(content string)           { h.thinking.WriteString(content) }
func (h *captureHandler) OnThinkingDone(fullReasoning string) {}
func (h *captureHandler) OnToolCalls(calls []ToolCall, fullResponse string, usage Usage, model string) {
}
func (h *captureHandler) OnError(err string) {}
func (h *captureHandler) OnDone(fullResponse string, usage Usage, model string) {
}

func TestSplitLocalAIChatML(t *testing.T) {
	input := "<|channel|>analysis<|message|>The user says \"oi\" which is Portuguese for \"hi\".<|end|><|start|>assistant<|channel|>final<|message|>Olá! Como posso ajudar você hoje?"
	final, reasoning, ok := SplitLocalAIChatML(input)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if strings.Contains(final, "<|channel|>") || strings.Contains(final, "<|message|>") {
		t.Fatalf("final ainda contém tokens: %q", final)
	}
	if !strings.Contains(reasoning, "Portuguese") {
		t.Fatalf("reasoning inesperado: %q", reasoning)
	}
	if !strings.Contains(final, "Olá!") {
		t.Fatalf("final inesperado: %q", final)
	}
}

func TestProcessLocalAIChatML_StreamingSplitAcrossChunks(t *testing.T) {
	var st localAIChatMLState
	var fullReasoning strings.Builder
	h := &captureHandler{}

	chunks := []string{
		"<|channel|>analysis<|message|>abc",
		"def<|end|><|start|>assistant<|channel|>final<|message|>hi",
		" there",
	}

	var out strings.Builder
	for _, c := range chunks {
		out.WriteString(processLocalAIChatML(c, &st, &fullReasoning, h))
	}

	if got := out.String(); got != "hi there" {
		t.Fatalf("final streaming inesperado: %q", got)
	}
	if got := h.thinking.String(); got != "abcdef" {
		t.Fatalf("thinking streaming inesperado: %q", got)
	}
	if got := fullReasoning.String(); got != "abcdef" {
		t.Fatalf("fullReasoning inesperado: %q", got)
	}
}

func TestSplitLocalAIChatML_NoMarkers(t *testing.T) {
	input := "resposta normal"
	final, reasoning, ok := SplitLocalAIChatML(input)
	if ok {
		t.Fatalf("expected ok=false")
	}
	if final != input {
		t.Fatalf("expected final=%q, got %q", input, final)
	}
	if reasoning != "" {
		t.Fatalf("expected reasoning empty, got %q", reasoning)
	}
}

func TestSummarizeHTTPError_Cloudflare524(t *testing.T) {
	body := []byte(`<!DOCTYPE html><html><head><title>inclunet.com.br | 524: A timeout occurred</title></head>
	<body>Cloudflare Ray ID: <strong class="font-semibold">9d37f6fd8ed68dff</strong>
	<span class="code-label">Error code 524</span></body></html>`)
	msg := summarizeHTTPError(524, body)
	if !strings.Contains(msg, "524") || !strings.Contains(strings.ToLower(msg), "cloudflare") {
		t.Fatalf("mensagem não parece resumo de 524: %q", msg)
	}
	if !strings.Contains(msg, "9d37f6fd8ed68dff") {
		t.Fatalf("Ray ID não apareceu no resumo: %q", msg)
	}
	if strings.Contains(strings.ToLower(msg), "<!doctype") {
		t.Fatalf("resumo ainda contém HTML cru: %q", msg)
	}
}
