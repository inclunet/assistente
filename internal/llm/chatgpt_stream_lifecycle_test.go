package llm

import (
	"context"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type lifecycleBody struct {
	io.ReadCloser
	closed atomic.Bool
}

func (b *lifecycleBody) Close() error { b.closed.Store(true); return b.ReadCloser.Close() }

type lifecycleTransport struct{ body *lifecycleBody }

func (t lifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: t.body, Request: r}, nil
}
func lifecycleProvider(events string) (*OpenAIProvider, *lifecycleBody) {
	body := &lifecycleBody{ReadCloser: io.NopCloser(strings.NewReader(events))}
	client := openai.NewClient(option.WithBaseURL("https://fixture.invalid/v1"), option.WithAPIKey("fixture"), option.WithHTTPClient(&http.Client{Transport: lifecycleTransport{body}}), option.WithMaxRetries(0))
	provider := NewOpenAIResponsesProvider(&ProviderConfig{ID: "chatgpt", Type: ProviderChatGPT, BaseURL: "https://fixture.invalid/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone}, nil)
	provider.streamClient = &client
	return provider, body
}
func TestChatGPTCompletionClosesBodyWithoutCallerCancellation(t *testing.T) {
	provider, body := lifecycleProvider("data: {\"type\":\"response.completed\",\"response\":{\"model\":\"model\"}}\n\n")
	ctx := context.Background()
	if _, err := provider.SendChat(ctx, []Message{{Role: "user", Content: "Hi"}}, ChatParams{Model: "model"}); err != nil {
		t.Fatal(err)
	}
	if !body.closed.Load() || ctx.Err() != nil {
		t.Fatal("completion relied on caller cancellation to close HTTP body")
	}
}

type cancelAfterDeltaHandler struct {
	chatGPTTerminalHandler
	cancel   context.CancelFunc
	thinking bool
}

func (h *cancelAfterDeltaHandler) OnChunk(value string) {
	h.spyHandler.OnChunk(value)
	if !h.thinking {
		h.cancel()
	}
}
func (h *cancelAfterDeltaHandler) OnThinking(value string) {
	h.spyHandler.OnThinking(value)
	if h.thinking {
		h.cancel()
	}
}
func TestChatGPTCancellationAfterDeltaHasOneTerminal(t *testing.T) {
	for _, thinking := range []bool{false, true} {
		kind := "response.output_text.delta"
		if thinking {
			kind = "response.reasoning_summary_text.delta"
		}
		t.Run(kind, func(t *testing.T) {
			event := "data: {\"type\":\"" + kind + "\",\"delta\":\"First\"}\n\n"
			provider, body := lifecycleProvider(event + event)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handler := &cancelAfterDeltaHandler{cancel: cancel, thinking: thinking}
			provider.streamChatResponses(ctx, "model", []Message{{Role: "user", Content: "Hi"}}, ChatParams{}, handler)
			count := 0
			for _, value := range handler.sequence {
				if value == "error" {
					count++
				}
			}
			if count != 1 || handler.err != "chatgpt_request_cancelled" || !handler.nonRetryable || !body.closed.Load() {
				t.Fatalf("inconsistent terminal: count=%d err=%s closed=%v", count, handler.err, body.closed.Load())
			}
		})
	}
}

type cancelAtFinalizationHandler struct {
	chatGPTTerminalHandler
	cancel      context.CancelFunc
	diagnostics bool
	doneCount   int
}

func (h *cancelAtFinalizationHandler) OnThinkingDone(value string) {
	h.chatGPTTerminalHandler.OnThinkingDone(value)
	if !h.diagnostics {
		h.cancel()
	}
}
func (h *cancelAtFinalizationHandler) OnFinishReason(FinishInfo) {
	if h.diagnostics {
		h.cancel()
	}
}
func (h *cancelAtFinalizationHandler) OnDone(string, Usage, string) { h.doneCount++ }
func TestChatGPTCancellationDuringFinalizationHasOneTerminal(t *testing.T) {
	for _, diagnostics := range []bool{false, true} {
		events := "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"Reason\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Text\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"model\"}}\n\n"
		provider, body := lifecycleProvider(events)
		ctx, cancel := context.WithCancel(context.Background())
		handler := &cancelAtFinalizationHandler{cancel: cancel, diagnostics: diagnostics}
		provider.streamChatResponses(ctx, "model", nil, ChatParams{}, handler)
		cancel()
		errors := 0
		for _, event := range handler.sequence {
			if event == "error" {
				errors++
			}
		}
		if errors != 1 || handler.doneCount != 0 || handler.err != "chatgpt_request_cancelled" || !handler.nonRetryable || !body.closed.Load() {
			t.Fatalf("finalization cancellation: diagnostics=%v errors=%d done=%d err=%s", diagnostics, errors, handler.doneCount, handler.err)
		}
	}
}
