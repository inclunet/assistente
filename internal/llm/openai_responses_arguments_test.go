package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/credentials"
)

type responsesFinalArgumentsHandler struct {
	providerRetryHandler
	calls  []ToolCall
	finish FinishInfo
}

func (h *responsesFinalArgumentsHandler) OnToolCalls(calls []ToolCall, _ string, _ Usage, _ string) {
	h.calls = calls
}
func (h *responsesFinalArgumentsHandler) OnFinishReason(info FinishInfo) { h.finish = info }

func TestResponsesUsesFinalFunctionArguments(t *testing.T) {
	const valid = `{"pattern":"**/*.go"}`
	tests := []struct{ name, delta, final string }{
		{"complete_deltas", valid, valid},
		{"without_deltas", "", valid},
		{"partial_deltas", `{"pattern":`, valid},
		{"different_valid_deltas", `{"pattern":"*.txt"}`, valid},
		{"invalid_final_not_repaired", valid, `{"pattern":`},
		{"empty_final_not_replaced", valid, ""},
	}
	for _, chatGPT := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("chatgpt_%t/%s", chatGPT, tc.name), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					sequence := 0
					emit := func(event map[string]any) {
						sequence++
						event["sequence_number"] = sequence
						data, err := json.Marshal(event)
						if err != nil {
							t.Errorf("encode event: %v", err)
							return
						}
						_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
					}
					item := map[string]any{"id": "fc_1", "type": "function_call", "status": "in_progress", "call_id": "call_1", "name": "search_files", "arguments": ""}
					emit(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
					if tc.delta != "" {
						emit(map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_1", "output_index": 0, "delta": tc.delta})
					}
					emit(map[string]any{"type": "response.function_call_arguments.done", "item_id": "fc_1", "output_index": 0, "arguments": tc.final})
					item["arguments"], item["status"] = tc.final, "completed"
					emit(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
					emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_1", "object": "response", "status": "completed", "model": "test-model", "output": []any{item}}})
				}))
				defer server.Close()
				config := &ProviderConfig{ID: "test", Type: ProviderOpenAI, BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone}
				if chatGPT {
					config.Type = ProviderChatGPT
				}
				provider := NewOpenAIResponsesProvider(config, credentials.NewManager(nil))
				h := &responsesFinalArgumentsHandler{}
				provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "find go files"}}, ChatParams{Model: "test-model"}, h, ToolDefinition{Type: "function", Function: FunctionDefinition{Name: "search_files", Parameters: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"]}`)}})
				if len(h.errors) > 0 || len(h.calls) != 1 || h.done != 0 {
					t.Fatalf("errors=%v calls=%d done=%d", h.errors, len(h.calls), h.done)
				}
				got := h.calls[0]
				if got.ID != "call_1" || got.Function.Name != "search_files" || got.Function.Arguments != tc.final {
					t.Fatalf("call=%+v, want call_1/search_files with final arguments %q", got, tc.final)
				}
				if h.finish.Reason != FinishReasonToolCalls {
					t.Fatalf("finish=%+v", h.finish)
				}
			})
		}
	}
}
