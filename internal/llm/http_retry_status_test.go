package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
)

func TestResponsesFailureUsesTerminalCategoryBeforeText(t *testing.T) {
	for _, tc := range []struct {
		name, code, errorType string
		wantRetry             bool
	}{
		{"invalid-request", "", "invalid_request_error", false},
		{"authentication", "invalid_api_key", "", false},
		{"permission", "", "permission_error", false},
		{"not-found", "model_not_found", "", false},
		{"unknown-fallback", "unknown", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/responses" {
					t.Errorf("unexpected route: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				event := map[string]any{"type": "response.failed", "sequence_number": 1, "response": map[string]any{
					"id": "resp_test", "object": "response", "status": "failed", "model": "m", "output": []any{},
					"error": map[string]any{"code": tc.code, "type": tc.errorType, "message": "rejected /429/503/"},
				}}
				payload, err := json.Marshal(event)
				if err != nil {
					t.Error(err)
					return
				}
				_, _ = fmt.Fprintf(w, "event: response.failed\ndata: %s\n\n", payload)
			}))
			defer server.Close()
			provider := NewOpenAIResponsesProvider(&ProviderConfig{ID: "sse-status", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, BaseURL: server.URL + "/v1"}, credentials.NewManager(nil))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			h := &cancelRetryNoticeHandler{cancel: cancel}
			provider.StreamChat(ctx, []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m", MaxTokens: testParameterPointer(32)}, h)
			if calls.Load() != 1 || h.err == "" || (len(h.avisos) > 0) != tc.wantRetry || (ctx.Err() != nil) != tc.wantRetry {
				t.Fatalf("calls=%d notices=%v error=%q context=%v wantRetry=%v", calls.Load(), h.avisos, h.err, ctx.Err(), tc.wantRetry)
			}
		})
	}
}

func TestRetryHTTPStatusOverridesErrorText(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   bool
	}{
		{400, false}, {401, false}, {403, false}, {404, false}, {408, true}, {422, false},
		{429, true}, {500, false}, {502, true}, {503, true}, {504, true}, {524, true},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			status, want := tc.status, tc.want
			for _, message := range []string{"request failed", "POST http://localhost:42901/502/503/524: timeout"} {
				if got := isRetryableError(message, ProviderError{StatusCode: status}); got != want {
					t.Fatalf("status=%d message=%q: retry=%v, want %v", status, message, got, want)
				}
			}
		})
	}
}

// Interrompe uma regressão no primeiro aviso, sem aguardar todo o backoff.
type cancelRetryNoticeHandler struct {
	espiaoAvisos
	cancel context.CancelFunc
}

func (h *cancelRetryNoticeHandler) OnTurnNotice(n TurnNotice) {
	h.espiaoAvisos.OnTurnNotice(n)
	if n.Kind == TurnNoticeStreamRetry {
		h.cancel()
	}
}

func TestHTTPRejectionDoesNotRetryNumbersInURL(t *testing.T) {
	for _, format := range []APIFormat{APIFormatOpenAI, APIFormatOpenAIResponses, APIFormatAnthropic} {
		for _, status := range []int{400, 401, 403, 404} {
			t.Run(fmt.Sprintf("%s/%d", format, status), func(t *testing.T) {
				var calls atomic.Int32
				wantPath := "/429/502/503/524/v1/chat/completions"
				switch format {
				case APIFormatOpenAIResponses:
					wantPath = "/429/502/503/524/v1/responses"
				case APIFormatAnthropic:
					wantPath = "/429/502/503/524/v1/v1/messages"
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.URL.Path != wantPath {
						t.Errorf("request path=%q, want %q", r.URL.Path, wantPath)
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("x-should-retry", "false")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"error":{"message":"request rejected","type":"invalid_request_error"}}`))
				}))
				defer server.Close()
				provider := NewChatProvider(&ProviderConfig{
					ID: "retry-status", APIFormat: format, AuthMode: AuthModeNone,
					BaseURL: server.URL + "/429/502/503/524/v1",
				}, credentials.NewManager(nil), nil)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				h := &cancelRetryNoticeHandler{cancel: cancel}
				provider.StreamChat(ctx, []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m", MaxTokens: testParameterPointer(32)}, h)
				if len(h.avisos) != 0 || calls.Load() != 1 || h.err == "" || ctx.Err() != nil {
					t.Fatalf("HTTP rejection must finish once: calls=%d notices=%v error=%q context=%v", calls.Load(), h.avisos, h.err, ctx.Err())
				}
			})
		}
	}
}
