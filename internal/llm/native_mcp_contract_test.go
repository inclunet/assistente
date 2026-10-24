package llm

import (
	"assistente/internal/credentials"
	"fmt"
	"github.com/openai/openai-go/responses"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeMCPLocalToolMismatch(t *testing.T) {
	local := []responses.ToolUnionParam{responses.ToolParamOfFunction("tool_catalog", map[string]any{"type": "object"}, false)}
	for _, tc := range []struct {
		name    string
		servers []MCPServerConfig
		tool    string
		want    bool
	}{
		{"local indevida", []MCPServerConfig{{AllowedTools: []string{"query"}}}, "tool_catalog", true},
		{"remota legítima", []MCPServerConfig{{AllowedTools: []string{"query"}}}, "query", false},
		{"homônima remota", []MCPServerConfig{{AllowedTools: []string{"tool_catalog"}}}, "tool_catalog", false},
		{"servidor irrestrito", []MCPServerConfig{{}}, "tool_catalog", false},
		{"segundo servidor irrestrito", []MCPServerConfig{{AllowedTools: []string{"query"}}, {}}, "tool_catalog", false},
		{"sem MCP", nil, "tool_catalog", false},
		{"sem nome", []MCPServerConfig{{AllowedTools: []string{"query"}}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeMCPLocalToolMismatch(local, tc.servers, tc.tool); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

type contractStreamHandler struct {
	mcpTrackingHandler
	nonRetryable     bool
	errText          string
	completedAtError int
}

func (h *contractStreamHandler) MarkErrorNotRetryable() { h.nonRetryable = true }
func (h *contractStreamHandler) OnError(errText string) {
	h.errText = errText
	for _, event := range h.events {
		if event.IsCompleted {
			h.completedAtError++
		}
	}
}

func TestResponsesNativeLocalMismatchPreservesResponse(t *testing.T) {
	for _, mode := range []string{"added_and_done", "done_only", "completed_without_done", "ordinary_remote_error", "interrupted", "completed_then_failed", "completed_then_transport_error"} {
		t.Run(mode, func(t *testing.T) {
			name := "tool_catalog"
			if mode == "ordinary_remote_error" {
				name = "query"
			}
			var stream strings.Builder
			emit := func(payload string) { fmt.Fprintf(&stream, "data: %s\n\n", payload) }
			if mode != "done_only" {
				emit(fmt.Sprintf(`{"type":"response.output_item.added","output_index":0,"item":{"id":"mcp_1","type":"mcp_call","name":%q,"server_label":"proxy-renamed"}}`, name))
			}
			withoutDone := strings.HasPrefix(mode, "completed_")
			if withoutDone {
				emit(`{"type":"response.mcp_call.completed","item_id":"mcp_1"}`)
			} else {
				emit(fmt.Sprintf(`{"type":"response.output_item.done","output_index":0,"item":{"id":"mcp_1","type":"mcp_call","name":%q,"server_label":"proxy-renamed","output":"Error executing tool: 'tool_catalog'","error":null}}`, name))
			}
			emit(`{"type":"response.output_text.delta","delta":"resposta preservada"}`)
			interrupted := mode == "interrupted" || mode == "completed_then_failed" || mode == "completed_then_transport_error"
			if mode == "completed_then_transport_error" {
				emit(`{"type":`) // SSE inválido força erro do leitor.
			} else if interrupted {
				emit(`{"type":"response.failed","response":{"id":"resp_1","object":"response","status":"failed","error":{"code":"server_error","message":"interrupted after execution"}}}`)
			} else {
				emit(`{"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","model":"test","output":[]}}`)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if _, err := fmt.Fprint(w, stream.String()); err != nil {
					t.Errorf("escrever stream de teste: %v", err)
				}
			}))
			defer server.Close()
			p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "test", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone}, credentials.NewManager(nil))
			native := p.WithMCPServers([]MCPServerConfig{{Name: "remote", URL: "https://example.invalid/mcp", AllowedTools: []string{"query"}}})
			fallback := &NativeMCPAdapterFallback{}
			hooks := 0
			h := &contractStreamHandler{}
			native.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "test", NativeMCPFallback: fallback, OnNativeMCPUnsupported: func() { hooks++ }}, h, ToolDefinition{Type: "function", Function: FunctionDefinition{Name: "tool_catalog", Parameters: []byte(`{"type":"object"}`)}})
			if calls.Load() != 1 {
				t.Fatalf("request repetida: %d", calls.Load())
			}
			if h.chunks.String() != "resposta preservada" {
				t.Fatalf("texto descartado: %q", h.chunks.String())
			}
			if fallback.Consume() {
				t.Fatal("incompatibilidade não autoriza replay")
			}
			wantMismatch := mode != "ordinary_remote_error"
			wantHooks := 0
			if wantMismatch {
				wantHooks = 1
			}
			if fallback.ConsumeAfterResponse() != wantMismatch || hooks != wantHooks {
				t.Fatalf("fallback/hook incorreto: hooks=%d", hooks)
			}
			if interrupted && (!h.nonRetryable || h.errText == "" || h.completedAtError != 1) {
				t.Fatalf("interrupção após efeito deve ser não repetível: %+v", h)
			}
			if fallback.ConsumeAfterResponse() {
				t.Fatal("fallback consumido duas vezes")
			}
			var done *MCPToolEvent
			for i := range h.events {
				if h.events[i].IsCompleted {
					done = &h.events[i]
				}
			}
			if done == nil || (done.Error != "") != wantMismatch {
				t.Fatalf("evento final: %+v", done)
			}
			if !withoutDone && done.Output != "Error executing tool: 'tool_catalog'" {
				t.Fatalf("output perdido: %+v", done)
			}
		})
	}
}
