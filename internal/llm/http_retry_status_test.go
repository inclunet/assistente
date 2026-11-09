package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
)

func TestRetryHTTPStatusOverridesErrorText(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 408, 422, 429, 500, 502, 503, 504, 524} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			want := status == 429 || status == 502 || status == 503 || status == 524
			for _, message := range []string{"request failed", "POST http://localhost:42901/502/503/524: timeout"} {
				if got := isRetryableError(message, status); got != want {
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
	for _, format := range []APIFormat{APIFormatOpenAI, APIFormatOpenAIResponses} {
		for _, status := range []int{400, 401, 403, 404} {
			t.Run(fmt.Sprintf("%s/%d", format, status), func(t *testing.T) {
				var calls atomic.Int32
				constructor := NewOpenAIProvider
				wantPath := "/429/502/503/524/v1/chat/completions"
				if format == APIFormatOpenAIResponses {
					constructor = NewOpenAIResponsesProvider
					wantPath = "/429/502/503/524/v1/responses"
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
				provider := constructor(&ProviderConfig{
					ID: "retry-status", APIFormat: format, AuthMode: AuthModeNone,
					BaseURL: server.URL + "/429/502/503/524/v1",
				}, credentials.NewManager(nil))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				h := &cancelRetryNoticeHandler{cancel: cancel}
				provider.StreamChat(ctx, []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)
				if len(h.avisos) != 0 || calls.Load() != 1 || h.err == "" || ctx.Err() != nil {
					t.Fatalf("HTTP rejection must finish once: calls=%d notices=%v error=%q context=%v", calls.Load(), h.avisos, h.err, ctx.Err())
				}
			})
		}
	}
}
