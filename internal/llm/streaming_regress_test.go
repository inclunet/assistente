package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/credentials"
)

type spyHandler struct {
	noopStreamHandler
	chunks   []string
	thinking []string
	err      string
	done     string
}

func (s *spyHandler) OnChunk(c string) { s.chunks = append(s.chunks, c) }
func (s *spyHandler) OnThinking(c string) { s.thinking = append(s.thinking, c) }
func (s *spyHandler) OnThinkingDone(c string) { s.thinking = append(s.thinking, "done:"+c) }
func (s *spyHandler) OnDone(content string, _ Usage, _ string) { s.done = content }
func (s *spyHandler) OnError(e string) { s.err = e }

func TestStream_EmptyFinish_GeraErro(t *testing.T) {
	// SSE com conteúdo mas sem finish_reason deve virar streaming_interrupted
	stream := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ola\"},\"finish_reason\":null}]}\n\n" + "data: [DONE]\n\n"
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
}

func TestStream_ThinkingSemFechamento_NaoViraChunk(t *testing.T) {
	stream := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"<thinking>segredo\"},\"finish_reason\":null}]}\n\n" + "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" + "data: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()
	p := NewOpenAIProvider(&ProviderConfig{ID: "test", BaseURL: server.URL + "/v1", AuthMode: AuthModeNone}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)
	for _, c := range h.chunks {
		if c == "segredo" {
			t.Fatalf("thinking não deveria virar chunk visível")
		}
	}
}
