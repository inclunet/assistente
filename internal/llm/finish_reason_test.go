package llm

import "testing"

func TestFinishReasonNormalizers(t *testing.T) {
	tests := []struct {
		name string
		got  FinishInfo
		want FinishReason
		raw  string
	}{
		{"openai chat length", normalizeOpenAIChatFinishReason("length"), FinishReasonMaxTokens, "length"},
		{"openai chat tool calls", normalizeOpenAIChatFinishReason("tool_calls"), FinishReasonToolCalls, "tool_calls"},
		{"responses max output", normalizeOpenAIResponsesFinishReason("max_output_tokens"), FinishReasonMaxTokens, "max_output_tokens"},
		{"responses filtro", normalizeOpenAIResponsesFinishReason("content_filter"), FinishReasonContentFilter, "content_filter"},
		{"anthropic max tokens", normalizeAnthropicFinishReason("max_tokens"), FinishReasonMaxTokens, "max_tokens"},
		{"anthropic tool use", normalizeAnthropicFinishReason("tool_use"), FinishReasonToolCalls, "tool_use"},
		{"google max tokens", normalizeGoogleFinishReason("MAX_TOKENS"), FinishReasonMaxTokens, "MAX_TOKENS"},
		{"google malformed não finge truncamento", normalizeGoogleFinishReason("MALFORMED_FUNCTION_CALL"), FinishReasonOther, "MALFORMED_FUNCTION_CALL"},
		{"acp max tokens", normalizeACPFinishReason("max_tokens"), FinishReasonMaxTokens, "max_tokens"},
		{"acp cancelado", normalizeACPFinishReason("cancelled"), FinishReasonCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Reason != tt.want || tt.got.RawReason != tt.raw {
				t.Fatalf("got=%#v, want reason=%q raw=%q", tt.got, tt.want, tt.raw)
			}
		})
	}
}

func TestFinishInfoWithToolCallsAlinhaCallbackSemOcultarLimite(t *testing.T) {
	tests := []struct {
		name string
		info FinishInfo
		want FinishReason
	}{
		{"stop vira tool calls", FinishInfo{Reason: FinishReasonStop, RawReason: "STOP"}, FinishReasonToolCalls},
		{"ausente vira tool calls", FinishInfo{}, FinishReasonToolCalls},
		{"max tokens permanece", FinishInfo{Reason: FinishReasonMaxTokens, RawReason: "MAX_TOKENS"}, FinishReasonMaxTokens},
		{"filtro permanece", FinishInfo{Reason: FinishReasonContentFilter, RawReason: "SAFETY"}, FinishReasonContentFilter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := finishInfoWithToolCalls(tt.info, 1)
			if got.Reason != tt.want || got.RawReason != tt.info.RawReason {
				t.Fatalf("got=%#v, want reason=%q raw=%q", got, tt.want, tt.info.RawReason)
			}
		})
	}
}

type finishCapturingHandler struct {
	FinishInfo
}

func (h *finishCapturingHandler) OnChunk(string)                                {}
func (h *finishCapturingHandler) OnThinking(string)                             {}
func (h *finishCapturingHandler) OnThinkingDone(string)                         {}
func (h *finishCapturingHandler) OnToolCalls([]ToolCall, string, Usage, string) {}
func (h *finishCapturingHandler) OnError(string)                                {}
func (h *finishCapturingHandler) OnDone(string, Usage, string)                  {}
func (h *finishCapturingHandler) OnMCPToolEvent(MCPToolEvent)                   {}
func (h *finishCapturingHandler) OnFinishReason(info FinishInfo)                { h.FinishInfo = info }

func TestReportFinishReasonMantemStreamHandlerCompativel(t *testing.T) {
	handler := &finishCapturingHandler{}
	want := FinishInfo{Reason: FinishReasonMaxTokens, RawReason: "length"}

	ReportFinishReason(handler, want)

	if handler.FinishInfo != want {
		t.Fatalf("finish=%#v, want %#v", handler.FinishInfo, want)
	}
}
