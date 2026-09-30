package llm

import (
	"assistente/internal/oauthflow"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatGPTRequestAndTerminalEvents(t *testing.T) {
	for _, terminal := range []string{"completed", "incomplete", "failed", "interrupted"} {
		t.Run(terminal, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/responses" || r.Method != "POST" {
					t.Errorf("route: %s", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["stream"] != true || body["store"] != false {
					t.Errorf("stream/store: %+v", body)
				}
				for _, key := range []string{"temperature", "top_p", "max_output_tokens", "previous_response_id"} {
					if _, ok := body[key]; ok {
						t.Errorf("unsupported %s", key)
					}
				}
				if body["instructions"] != "system instruction" {
					t.Error("missing instructions")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n")
				if terminal != "interrupted" {
					_, _ = fmt.Fprintf(w, "event: response.%s\ndata: {\"type\":\"response.%s\",\"response\":{\"model\":\"account-model\",\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\",\"message\":\"Plan limit\"}}}\n\n", terminal, terminal)
				}
			}))
			defer server.Close()
			config := &ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, APIFormat: APIFormatOpenAIResponses, BaseURL: server.URL + "/v1", AuthMode: AuthModeNone}
			p := NewOpenAIResponsesProvider(config, nil)
			if p.NativeMCPCapable() || config.SupportsTTS() || config.SupportsSTT() {
				t.Fatal("unsupported capability advertised")
			}
			text, err := p.SendChat(context.Background(), []Message{{Role: "system", Content: "system instruction"}, {Role: "user", Content: "Hi"}}, ChatParams{Model: "account-model", Temperature: 0.8, TopP: 0.5, MaxTokens: 100})
			if terminal == "completed" {
				if err != nil || text != "Hello" {
					t.Fatalf("%q %v", text, err)
				}
			} else if err == nil {
				t.Fatal("incomplete stream succeeded")
			} else {
				expected := map[string]string{"failed": "chatgpt_plan_limit", "incomplete": "chatgpt_response_incomplete", "interrupted": "chatgpt_stream_interrupted"}[terminal]
				if err.Error() != expected {
					t.Fatalf("untranslated remote failure: %v", err)
				}
			}
		})
	}
}
func TestChatGPTCatalogPreservesAccountOrderAndLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"models":[{"slug":"z","display_name":"Preferred","visibility":"list"},{"slug":"hidden","visibility":"hidden"},{"slug":"a","display_name":"Second","visibility":"list"}]}`)
	}))
	defer server.Close()
	p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, BaseURL: server.URL, AuthMode: AuthModeNone}, nil)
	models, err := p.ModelOptions(context.Background())
	if err != nil || len(models) != 2 || models[0].Value != "z" || models[0].Label != "Preferred" {
		t.Fatalf("models=%+v err=%v", models, err)
	}
}
func TestChatGPTFunctionNamespace(t *testing.T) {
	p := NewOpenAIResponsesProvider(&ProviderConfig{Type: ProviderChatGPT, BaseURL: "https://api.openai.com/v1"}, nil)
	params := p.buildResponsesParams(context.Background(), "model", []Message{{Role: "user", Content: "Hi"}}, ChatParams{}, nil, ToolDefinition{Type: "function", Function: FunctionDefinition{Name: "local_tool", Parameters: json.RawMessage(`{"type":"object"}`)}})
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"type":"namespace"`) || !strings.Contains(string(data), `"name":"local_tool"`) {
		t.Fatalf("tools: %s", data)
	}
}

func TestChatGPTUnknownRemoteFailureDoesNotExposeDescription(t *testing.T) {
	if got := chatGPTFailure(context.Background(), "secret echoed by remote"); got != "chatgpt_request_failed" {
		t.Fatal(got)
	}
}

type chatGPTTerminalHandler struct {
	spyHandler
	sequence []string
}

func (h *chatGPTTerminalHandler) OnThinkingDone(value string) {
	h.spyHandler.OnThinkingDone(value)
	h.sequence = append(h.sequence, "thinking_done")
}
func (h *chatGPTTerminalHandler) OnError(value string) {
	h.spyHandler.OnError(value)
	h.sequence = append(h.sequence, "error")
}
func TestChatGPTFailuresFinishThinkingBeforeError(t *testing.T) {
	for _, terminal := range []string{"failed", "incomplete", "interrupted", "transport"} {
		t.Run(terminal, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "event: response.reasoning_summary_text.delta\ndata: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"thought\"}\n\n")
				switch terminal {
				case "transport":
					_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: invalid-json\n\n")
				case "interrupted":
				default:
					_, _ = fmt.Fprintf(w, "event: response.%s\ndata: {\"type\":\"response.%s\",\"response\":{}}\n\n", terminal, terminal)
				}
			}))
			defer server.Close()
			p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone}, nil)
			handler := &chatGPTTerminalHandler{}
			p.streamChatResponses(context.Background(), "model", []Message{{Role: "user", Content: "Hi"}}, ChatParams{}, handler)
			if strings.Join(handler.sequence, ",") != "thinking_done,error" || handler.done != "" || !handler.nonRetryable {
				t.Fatalf("terminal lifecycle: %+v", handler)
			}
		})
	}
}

func TestChatGPTCatalogFailuresAreSafeCodes(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{
		{401, "chatgpt_reauthorization_required"}, {403, "chatgpt_permission_required"}, {429, "chatgpt_rate_limit"}, {503, "chatgpt_temporarily_unavailable"}, {400, "chatgpt_request_failed"}, {200, "chatgpt_request_failed"},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, "private remote detail")
			}))
			defer server.Close()
			p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, BaseURL: server.URL, AuthMode: AuthModeNone}, nil)
			_, err := p.ModelOptions(context.Background())
			if err == nil || err.Error() != tc.code {
				t.Fatalf("error=%v", err)
			}
		})
	}
	if got := chatGPTTransportFailure(context.Background(), fmt.Errorf("wrapped: %w", oauthflow.ErrTransient)); got != "chatgpt_temporarily_unavailable" {
		t.Fatal(got)
	}
}

func TestChatGPTMissingAuthorizationRequiresReconnect(t *testing.T) {
	if got := chatGPTTransportFailure(context.Background(), fmt.Errorf("wrapped: %w", oauthflow.ErrNotFound)); got != "chatgpt_reauthorization_required" {
		t.Fatal(got)
	}
}
func TestChatGPTWatchdogDistinguishesUserCancellation(t *testing.T) {
	for _, userCancel := range []bool{false, true} {
		t.Run(fmt.Sprint(userCancel), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "event: response.reasoning_summary_text.delta\ndata: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"thought\"}\n\n")
				w.(http.Flusher).Flush()
				if userCancel {
					cancel()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, StreamIdleTimeoutSeconds: 1}, nil)
			handler := &chatGPTTerminalHandler{}
			p.streamChatResponses(ctx, "model", []Message{{Role: "user", Content: "Hi"}}, ChatParams{}, handler)
			expected := streamIdleErrorMessage
			if userCancel {
				expected = "chatgpt_request_cancelled"
			}
			if handler.err != expected || handler.done != "" || !handler.nonRetryable {
				t.Fatalf("classification: %+v", handler)
			}
			if !userCancel && strings.Join(handler.sequence, ",") != "thinking_done,error" {
				t.Fatalf("thinking: %v", handler.sequence)
			}
		})
	}
}

func TestChatGPTCollectorRequiresTerminalCallback(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		h := &chatGPTCollector{}
		text, err := h.result(ctx)
		expected := "chatgpt_stream_interrupted"
		if cancelled {
			expected = "chatgpt_request_cancelled"
		}
		if text != "" || err == nil || err.Error() != expected {
			t.Fatalf("silent termination: %q %v", text, err)
		}
		h.OnDone("", Usage{}, "model")
		if text, err = h.result(ctx); err != nil || text != "" {
			t.Fatal("legitimate empty completion rejected", err)
		}
		cancel()
	}
}
func TestChatGPTCompletedDoesNotWaitForEOF(t *testing.T) {
	for _, lateError := range []bool{false, true} {
		t.Run(fmt.Sprint(lateError), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Done\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"account-model\"}}\n\n")
				if lateError {
					_, _ = fmt.Fprint(w, "data: invalid-json\n\n")
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone}, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			text, err := p.SendChat(ctx, []Message{{Role: "user", Content: "Hi"}}, ChatParams{Model: "account-model"})
			if err != nil || text != "Done" || ctx.Err() != nil {
				t.Fatalf("terminal event ignored: %q %v", text, err)
			}
		})
	}
}

func TestChatGPTAlreadyCanceledUsesStableError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, BaseURL: "https://api.openai.com/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone}, nil)
	h := &chatGPTTerminalHandler{}
	p.streamChatResponses(ctx, "model", nil, ChatParams{}, h)
	if h.err != "chatgpt_request_cancelled" || !h.nonRetryable {
		t.Fatalf("unstable cancellation: %+v", h)
	}
	if _, err := p.SendChat(ctx, nil, ChatParams{Model: "model"}); err == nil || err.Error() != "chatgpt_request_cancelled" {
		t.Fatalf("synchronous cancellation: %v", err)
	}
}
