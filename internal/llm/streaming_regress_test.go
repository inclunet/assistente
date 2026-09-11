package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/credentials"
)

type spyHandler struct {
	noopStreamHandler
	chunks       []string
	thinking     []string
	err          string
	done         string
	usage        Usage
	nonRetryable bool
}

func (s *spyHandler) OnChunk(c string)                         { s.chunks = append(s.chunks, c) }
func (s *spyHandler) OnThinking(c string)                      { s.thinking = append(s.thinking, c) }
func (s *spyHandler) OnThinkingDone(c string)                  { s.thinking = append(s.thinking, "done:"+c) }
func (s *spyHandler) OnDone(content string, _ Usage, _ string) { s.done = content }
func (s *spyHandler) OnError(e string)                         { s.err = e }
func (s *spyHandler) OnUsage(usage Usage)                      { s.usage = usage }
func (s *spyHandler) MarkErrorNotRetryable()                   { s.nonRetryable = true }

type cancelOnThinkingHandler struct {
	spyHandler
	cancel context.CancelFunc
}

func (h *cancelOnThinkingHandler) OnThinking(content string) {
	h.spyHandler.OnThinking(content)
	h.cancel()
}

func TestChatCompletions_StreamSemFinishReasonGeraErro(t *testing.T) {
	stream := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ola\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
		"data: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()
	p := NewOpenAIProvider(&ProviderConfig{ID: "test", BaseURL: server.URL + "/v1", AuthMode: AuthModeNone}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)
	if h.err != "streaming_interrupted" {
		t.Fatalf("esperava streaming_interrupted, veio %q done=%q", h.err, h.done)
	}
	if h.done != "" {
		t.Fatalf("não deveria chamar OnDone quando finish vazio, veio %q", h.done)
	}
	if !h.usage.OutputTokensReported || h.usage.CompletionTokens != 2 {
		t.Fatalf("usage terminal não foi propagado: %+v", h.usage)
	}
	if !h.nonRetryable {
		t.Fatal("stream interrompido após texto não pode ser repetido")
	}
}

func TestChatCompletions_ThinkingSemFechamentoFinalizaRaciocinio(t *testing.T) {
	stream := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"<thinking>segredo\"},\"finish_reason\":null}]}\n\n" + "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" + "data: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()
	p := NewOpenAIProvider(&ProviderConfig{ID: "test", BaseURL: server.URL + "/v1", AuthMode: AuthModeNone}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)
	if len(h.chunks) != 0 {
		t.Fatalf("thinking não deveria virar chunk visível: %v", h.chunks)
	}
	if got, want := h.thinking, []string{"segredo", "done:segredo"}; !slicesEqual(got, want) {
		t.Fatalf("eventos de thinking = %v, esperado %v", got, want)
	}
}

func TestChatCompletions_CancelamentoAposThinkingNaoEmiteChunk(t *testing.T) {
	stream := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"<thinking>segredo</thinking>visivel\"},\"finish_reason\":null}]}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	p := NewOpenAIProvider(&ProviderConfig{ID: "test", BaseURL: server.URL + "/v1", AuthMode: AuthModeNone}, credentials.NewManager(nil))
	h := &cancelOnThinkingHandler{cancel: cancel}
	p.StreamChat(ctx, []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)

	if len(h.chunks) != 0 {
		t.Fatalf("cancelamento não deveria deixar chunk escapar: %v", h.chunks)
	}
	if got, want := h.thinking, []string{"segredo", "done:segredo"}; !slicesEqual(got, want) {
		t.Fatalf("eventos de thinking = %v, esperado %v", got, want)
	}
}

func TestResponses_StreamSemConclusaoGeraErro(t *testing.T) {
	stream := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"ola\"}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "test", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)

	if h.err != "streaming_interrupted" {
		t.Fatalf("esperava streaming_interrupted, veio %q done=%q", h.err, h.done)
	}
	if h.done != "" {
		t.Fatalf("não deveria chamar OnDone sem conclusão, veio %q", h.done)
	}
	if !h.nonRetryable {
		t.Fatal("Responses interrompido após texto não pode ser repetido")
	}
}

func TestResponses_ThinkingSemFechamentoFinalizaRaciocinio(t *testing.T) {
	stream := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"<thinking>segredo\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1,\"status\":\"completed\",\"model\":\"m\",\"output\":[]}}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "test", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)

	if len(h.chunks) != 0 {
		t.Fatalf("thinking não deveria virar chunk visível: %v", h.chunks)
	}
	if got, want := h.thinking, []string{"segredo", "done:segredo"}; !slicesEqual(got, want) {
		t.Fatalf("eventos de thinking = %v, esperado %v", got, want)
	}
}

func TestResponses_CancelamentoAposThinkingNaoEmiteChunk(t *testing.T) {
	stream := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"<thinking>segredo</thinking>visivel\"}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	p := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "test", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	h := &cancelOnThinkingHandler{cancel: cancel}
	p.StreamChat(ctx, []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)

	if len(h.chunks) != 0 {
		t.Fatalf("cancelamento não deveria deixar chunk escapar: %v", h.chunks)
	}
	if got, want := h.thinking, []string{"segredo", "done:segredo"}; !slicesEqual(got, want) {
		t.Fatalf("eventos de thinking = %v, esperado %v", got, want)
	}
}

func TestChatCompletions_TimeoutTerminalPreservaDiagnosticos(t *testing.T) {
	stream := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m-real\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"parcial\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m-real\",\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	p := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-timeout", BaseURL: server.URL + "/v1", AuthMode: AuthModeNone,
		StreamIdleTimeoutSeconds: 1,
	}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m", MaxTokens: 99}, h)

	if h.err != streamIdleErrorMessage {
		t.Fatalf("erro=%q, esperado %q", h.err, streamIdleErrorMessage)
	}
	if h.finish.Provider != "chat-timeout" || h.finish.Model != "m-real" ||
		h.finish.OutputLimit != 99 || h.finish.ResponseBytes != len("parcial") {
		t.Fatalf("finish terminal incompleto: %+v", h.finish)
	}
	if !h.usage.OutputTokensReported || h.usage.CompletionTokens != 2 {
		t.Fatalf("usage terminal incompleto: %+v", h.usage)
	}
	if !h.nonRetryable {
		t.Fatal("timeout após texto não pode ser repetido externamente")
	}
}

func TestResponses_TimeoutTerminalPreservaDiagnosticos(t *testing.T) {
	stream := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"parcial\"}\n\n" +
		"event: response.incomplete\n" +
		"data: {\"type\":\"response.incomplete\",\"sequence_number\":2,\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1,\"status\":\"incomplete\",\"model\":\"m-real\",\"output\":[],\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5,\"output_tokens_details\":{\"reasoning_tokens\":1}}}}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	p := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-timeout", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses,
		AuthMode: AuthModeNone, StreamIdleTimeoutSeconds: 1,
	}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m", MaxTokens: 99}, h)

	if h.err != streamIdleErrorMessage {
		t.Fatalf("erro=%q, esperado %q", h.err, streamIdleErrorMessage)
	}
	if h.finish.Provider != "responses-timeout" || h.finish.Model != "m-real" ||
		h.finish.OutputLimit != 99 || h.finish.ResponseBytes != len("parcial") {
		t.Fatalf("finish terminal incompleto: %+v", h.finish)
	}
	if !h.usage.OutputTokensReported || h.usage.CompletionTokens != 2 ||
		!h.usage.ReasoningTokensReported || h.usage.ReasoningTokens != 1 {
		t.Fatalf("usage terminal incompleto: %+v", h.usage)
	}
	if !h.nonRetryable {
		t.Fatal("timeout Responses após efeito não pode ser repetido externamente")
	}
}

func TestChatCompletions_ErroDeTransporteAposTextoNaoRetenta(t *testing.T) {
	stream := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"parcial\"},\"finish_reason\":null}]}\n\n" +
		"data: {json inválido}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-error", BaseURL: server.URL + "/v1", AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)

	if h.err == "" || !h.nonRetryable {
		t.Fatalf("erro após texto deve ser terminal: err=%q nonRetryable=%v", h.err, h.nonRetryable)
	}
}

func TestResponses_ErroDeTransporteAposTextoNaoRetenta(t *testing.T) {
	stream := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"parcial\"}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {json inválido}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-error", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)

	if h.err == "" || !h.nonRetryable {
		t.Fatalf("erro após efeito deve ser terminal: err=%q nonRetryable=%v", h.err, h.nonRetryable)
	}
}

func slicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
